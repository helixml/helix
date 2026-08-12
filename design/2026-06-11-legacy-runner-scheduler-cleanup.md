# Legacy runner/scheduler cleanup audit (2026-06-11)

## Context

Helix pivoted from a **scheduler + standalone-runner** architecture to **sandbox-manages-runners
via Docker Compose profiles**. The scheduler package was already deleted (`5d9abe3fc`), the
standalone runner binary/Dockerfile/Helm chart were already deleted (`ecbc6dd27`, `6d660a6d1`,
`a5b011ddf`). This audit finds what's left behind, and - critically - what is NOT dead.

Trigger: CodeQL alerts #383-386 (`runner/*.py`) after re-enabling code scanning.

## CRITICAL: what is ALIVE (do NOT delete)

The word "runner" is overloaded. The **new** architecture keeps a lot of "runner" code:

- `api/pkg/runner/` (Go) - **fully alive**. Three subpackages, all referenced:
  - `composeparse` -> used by `composemgr`, `inferenceproxy`
  - `gpuarch` -> used by `gpudetect`
  - `profile` -> used by `server` (runner-profile CRUD + assignment handlers)
- `composemgr`, `inferenceproxy`, `inferencerouter` - apply/route to compose profiles.
- `runner_profile_handlers.go`, `runner_assignment_handlers.go`, `types/runner_profile.go`.
- `RunnerToken` config + `RUNNER_TOKEN` env - sandbox auth secret, used everywhere.
- Admin runner logs (`/api/v1/admin/runners/{id}/logs`).

"Docker Compose profiles for the runners" = the `RunnerProfile` system above (operator-declared
compose YAML the sandbox applies). It is NOT the dev-compose `runner_gpu` services (see below).

## DEAD - safe to delete (old standalone inference/agent runner + scheduler orphans)

| # | Item | Evidence it's dead |
|---|------|--------------------|
| A | Top-level `runner/` dir (Python: helix-diffusers, axolotl, sdxl) | No imports/build refs anywhere. Old inference-runner image payload. Resolves CodeQL #383-386. |
| B | `docker-compose.dev.yaml` `runner_gpu` + `runner_gpu_amd` services + `x-runner-config` anchor | Build from deleted `Dockerfile.runner`, run deleted `./runner-cmd/helix-runner`, set `HELIX_COMMAND=runner` (the `runner` subcommand is gone). Cannot build. |
| C | `stack`: `build-runner`, `build-runner-image`, `mock-runner`, `setup_runner_profile` | Reference deleted `./runner-cmd/helix-runner` + `Dockerfile.runner`. |
| D | `build-and-push.sh` | References deleted `Dockerfile.runner`; not in CI. |
| E | `config.go:349-352` (`SlotTTL`, `RunnerTTL`, `SchedulingStrategy`, `QueueSize`) | Defined, never read. Scheduler-era. |
| F | Stale swagger schema in `swagger/docs.go` (deleted endpoints/types) | Regenerate via `./stack update_openapi`. |

## NEEDS CARE - not a blind delete (frontend phase 2)

`frontend/src/types/dashboard.ts`, `services/dashboardService.ts` (`useGetDashboardData` is a stub
returning `{runners: []}`), and the commented-out `FloatingRunnerState` in `Layout.tsx`. BUT live
consumers exist: `EditHelixModel.tsx`, `ModelInstanceLogs.tsx` read these types/hook. Removing
requires tracing those consumers so they degrade gracefully - separate, careful pass.

## Ambiguous (verify, don't assume)

- `api/pkg/system/log_buffer.go` `ModelInstanceLogBuffer` keyed by `slotID` - is it still populated?
- `install.sh` `--runner` provisioning (`ghcr.io/helixml/runner:$RUNNER_TAG`) - keep only if legacy
  remote-runner deployments are still supported; ask before touching.

## CodeQL status

- #195-197 (`api/pkg/runner/files.go`, deleted) -> auto-closed to `fixed` by the post-merge main scan.
- #383-386 (`runner/*.py`) -> resolved by deleting the top-level `runner/` dir (item A).

## Plan (commit-by-commit, build/test each)

1. Delete top-level `runner/` dir (A). Resolves #383-386.
2. Remove dead dev-compose runner services + anchor (B) and `stack` functions (C), `build-and-push.sh` (D).
3. Remove orphaned config fields (E); `go build ./...`.
4. Regenerate swagger (F).
5. (Separate) Frontend dashboard-stub cleanup with consumer tracing.

Do NOT touch anything in the ALIVE list.

## Executed (2026-06-11)

28 files, ~-4980 lines. All Go builds (`go build ./...`), `bash -n stack`, compose YAML
parse, and frontend `tsc --noEmit` pass.

- **Deleted**: top-level `runner/` (Python inference runner; resolves CodeQL #383-386),
  `build-and-push.sh`.
- **docker-compose.dev.yaml**: removed `runner_gpu`/`runner_gpu_amd` services + `x-runner-config`
  anchor.
- **stack**: removed `setup_runner_profile`, `build-runner`, `build-runner-image`, `mock-runner`,
  the `WITH_RUNNER` mode threaded through `build`/`start`/`start-tmux`/`stop`, and the scheduler-era
  slot helpers (`list-slots`/`slots`/`active-slots`/`slot-stats`/`wipe-slots`, `WIPE_SLOTS`) +
  `ollama-sync` (targets deleted `api/pkg/ollamav11`). Help text updated.
- **config.go**: removed `ModelTTL`/`SlotTTL`/`RunnerTTL`/`SchedulingStrategy`/`QueueSize` (zero readers).
- **Frontend**: two dead subsystems, both killed by the scheduler-pivot commit:
  - runner-dashboard: `types/dashboard.ts` runner types, `useGetDashboardData` stub,
    `ModelInstanceLogs`, `LogViewerModal` + the `'logs'`/`runner` floating-modal path,
    `EditHelixModel` download-status panel, `Dashboard` `isLoadingDashboardData` stub.
  - memory-estimation (backend handler + generated client already gone; services were stubs
    returning "no longer available"): `MemoryEstimationWidget`, `useMemoryEstimation`,
    `memoryEstimationService`, `MemoryEstimate*` types.

### Not done / caveats
- **`stack` start/stop/start-tmux verified working** on the dev VM (2026-06-12). All `./stack`
  commands exercised post-cleanup; no regressions from removing the runner/slot/ollama plumbing.
- Stale **swagger** schema (deleted scheduler endpoints/types in `swagger/docs.go`) left for a
  `./stack update_openapi` regen (needs the toolchain/VM).
- FloatingModal `'rdp'`/ScreenshotViewer path left intact (not runner/scheduler; appears unused but
  out of scope).
