# Implementation Tasks: Right-Size Spec-Task Sandbox Defaults and Replay Stream Init on Reconnect

> **Settled by Luke — fixed, not suggestions.** Ladder 1/2048, 4/8192, 8/16384,
> **12/24576**, **16/32768**. Default **12 vCPU / 24576 MB**, still
> operator-configurable with 12/24576 as the no-env fallback. Migration touches
> **only** rows holding exactly `{"vcpus": 4, "memory_mb": 8192}` — never the 178
> rows holding `{"vcpus": 8, "memory_mb": 16384}`. `CreateSandboxDialog.tsx`
> ladder moves too. All Part B work stands unchanged.

## 0. Before starting

- [x] Read the prior-art spec at `helix-specs/design/tasks/002903_default-spec-task/` and lift its frontend de-duplication plan and OpenAPI fan-out warning
- [x] Re-run `grep -rn "8192\|16384\|DefaultSpecTaskSandbox\|ValidPreset" api frontend/src` and diff against the call-site table in `design.md` — a new caller may have landed — **no new callers; table is accurate**
- [x] Confirm `spec_tasks.sandbox_resource_overrides` is genuinely `jsonb` (`\d+ spec_tasks`) before writing the migration comparison — **confirmed `jsonb`**
- [x] Baseline the local inner-Helix DB: `spec_tasks` is empty (0 rows), so the migration is exercised by seeding rows by hand during verification

## 1. Part A — types, ladder, config

- [x] Set `DefaultSpecTaskSandboxVCPUs = 12` / `DefaultSpecTaskSandboxMemoryMB = 24576` in `api/pkg/types/simple_spec_task.go:86-87`
- [x] Add rungs `12/24576` and `16/32768` to `SpecTaskSandboxPresetForVCPUs` and `ValidPreset` (`api/pkg/types/simple_spec_task.go:117-144`), keeping 1/2048, 4/8192 and 8/16384 valid — **collapsed both into one `SpecTaskSandboxPresets` table so they cannot drift**
- [x] Add `configuredSpecTaskSandboxDefault atomic.Pointer[SandboxResourceOverrides]` plus `SetDefaultSpecTaskSandboxResources` in `types`, backing `DefaultSpecTaskSandboxResources()` without changing its signature; document the "set once at startup, before serving" contract
- [x] Add `DefaultSpecTaskVCPUs` (`HELIX_SPEC_TASK_SANDBOX_DEFAULT_VCPUS`, default `12`) and `DefaultSpecTaskMemoryMB` (`HELIX_SPEC_TASK_SANDBOX_DEFAULT_MEMORY_MB`, default `24576`) to the `Sandboxes` struct in `api/pkg/config/config.go:166`
- [x] Call `types.SetDefaultSpecTaskSandboxResources` once at API startup; fail startup loudly when the configured pair fails `ValidPreset()` rather than falling back silently — **wired into `LoadServerConfig` so every entry point gets it; `config` already imports `types`, no cycle**
- [x] Clamp vCPUs to `runtime.NumCPU()` in `sandboxResourceLimits` (`api/pkg/hydra/devcontainer.go:1104`), logging once when a clamp occurs — Docker rejects `--cpus` above the host CPU count
- [x] Add `SpecTaskSandboxVCPUList()` so validation errors and tool descriptions render the ladder from the table rather than hand-copying it

## 2. Part A — stop materializing and backfill

- [x] Delete the `sandboxResources = types.DefaultSpecTaskSandboxResources()` fallback in `api/pkg/services/spec_driven_task_service.go:163` so an unspecified size stores `nil`
- [x] Delete the same fallback in `api/pkg/org/infrastructure/runtime/helix/spectasks.go:215`
- [x] Confirm `HydraExecutor.resolveSpecTaskLaunchConfig` (`api/pkg/external-agent/hydra_executor.go:697`) resolves `nil` to the live default at container-create for every start path, including forks and reconciler resumes
- [ ] Check the API response / frontend render path for a now-nil `task.sandbox_resource_overrides` — it must fall back to the shared default, not render blank
- [x] Add migration `api/pkg/store/migrations/0009_unmaterialize_spec_task_sandbox_default.up.sql` NULLing `spec_tasks.sandbox_resource_overrides` where it equals exactly `{"vcpus": 4, "memory_mb": 8192}` — see Open Question 1 on `NULL` vs the explicit new pair
- [x] **Never match `{"vcpus": 8, "memory_mb": 16384}`** — 178 rows on meta hold that as a deliberate user choice. Do not generalise the predicate to "equals a default"
- [ ] Record the `SELECT sandbox_resource_overrides, count(*) FROM spec_tasks GROUP BY 1` counts before and after, and put both in the PR body — that diff is the safety story for this migration
- [ ] Confirm the migration is a no-op on meta, whose 31 stale rows were hand-backfilled to `12/24576` on 2026-08-24
- [x] Add the `.down.sql` as a documented no-op (the reversal information does not exist, and re-materializing all NULLs would break the 4102 rows that never had a value)
- [x] Do **not** touch project rows — a project only stores an override when an admin explicitly set one

## 3. Part A — remaining Go call sites

- [x] Update the ladder in the error string at `api/pkg/org/infrastructure/runtime/helix/spectasks.go:207` (`"sandbox_vcpus must be 1, 4 or 8"`)
- [x] Update the ladder in the MCP tool description prose at `api/pkg/org/interfaces/mcptools/spec_tasks.go:56` — org Workers read this string
- [x] Update the doc comment at `api/pkg/org/infrastructure/runtime/runtime.go:263` (`"(1, 4 or 8; memory follows)"`)
- [x] Update the `"standard 4 vCPU / 8 GB preset"` comment at `api/pkg/types/project.go:246`
- [x] Extend the sandboxes-API rungs at `api/pkg/sandbox/controller_provision.go:42-43` and `api/pkg/cli/sandbox/sandbox.go:264-266` to the same five rungs, leaving that surface's default unchanged (see design A6)
- [x] Verify the `ValidPreset()` callers (`spec_driven_task_handlers.go:169`, `spec_task_execution_config_handlers.go:109`, `project_handlers.go:409,755`) and the rollback target (`spec_task_execution_config_handlers.go:130`) follow the new ladder symbolically, with no edit needed

## 4. Part A — frontend

- [ ] Create `frontend/src/constants/sandboxPresets.ts` exporting `SandboxPreset`, `SANDBOX_PRESETS` (five rungs, each with **both** `label` and `description`) and `DEFAULT_SANDBOX_PRESET = SANDBOX_PRESETS[3]`
- [ ] Point `components/tasks/SpecTaskExecutionControls.tsx:57-60` at the shared module; delete its local table and retype `selectSandbox`'s param as `SandboxPreset`
- [ ] Point `components/agent/CodeAgentExecutionControls.tsx:54-55` at the shared module; delete its local table
- [ ] Replace the literals in `pages/Home.tsx:138,174` with `DEFAULT_SANDBOX_PRESET`'s two numeric fields only (never spread `label`/`description` into an API payload)
- [ ] Replace the useState initial value (`components/tasks/NewSpecTaskForm.tsx:152`) and the `?.memory_mb || 8192` project fallback (`:339`)
- [ ] Replace the `?.memory_mb || 8192` fallback in `components/session/projectChatItemDetails.ts:65`
- [ ] Extend the size list in `components/sandboxes/CreateSandboxDialog.tsx:44-46` to match the shared rungs, leaving its default selection unchanged
- [ ] Confirm a task storing `{"vcpus": 12, "memory_mb": 24576}` now renders with the "12 CPU / 24 GB RAM" rung **selected** — meta's 31 backfilled tasks currently render blank because no 12-vCPU rung exists
- [ ] Make an unmatched stored value degrade gracefully (show the raw size, not blank), so a future hand-edited row is visible rather than silently unselected
- [ ] Grep `frontend/src` and confirm no sandbox-default literal survives outside `sandboxPresets.ts` and test fixtures (ignore unrelated `8192`/`16384` in `api_bindings.ts`, `profileBlocks.ts`)

## 5. Part A — OpenAPI and tests

- [ ] Run `./stack update_openapi` and commit all seven regenerated artifacts; re-grep `standard 4 vCPU` to confirm every copy moved
- [ ] Invert the materialized-default assertion in `api/pkg/services/spec_driven_task_service_test.go` (~129) — the row is now `nil`, not the default
- [ ] Re-point the contrasting project default in `api/pkg/org/infrastructure/runtime/helix/spectasks_sandbox_test.go` (~79) to `1/2048` so `TestSpecTasks_CreateFallsBackToProjectSandboxDefaults` is not vacuous
- [ ] Add a `sandboxResourceLimits` clamp case to `api/pkg/hydra/devcontainer_test.go`
- [ ] Add a `SetDefaultSpecTaskSandboxResources` test covering a valid pair, an invalid pair, and restore-after-test
- [ ] Add a frontend test: a stored `{vcpus: 12, memory_mb: 24576}` selects the 12-CPU rung, and a stored value matching no rung does not render blank
- [ ] Add a migration test (or documented manual check) that the predicate leaves `{"vcpus": 8, "memory_mb": 16384}` rows untouched
- [ ] Run `api/pkg/external-agent/task_overrides_test.go` to confirm the symbolic assertions still pass
- [ ] Run the touched vitest files; update `SpecTaskExecutionControls.test.tsx`, `ProjectChatItemTooltip.test.tsx` and `projectChatItemDetails` fixtures only where they actually fail

## 6. Part B — proxy replay

- [ ] Add the `SessionReplay` interface to `api/pkg/proxy/resilient.go` with a doc comment that names `CreateWebSocketUpgradeFunc`'s `extraHeaders` as its sibling and states the same reconnect rationale
- [ ] Add `Replay SessionReplay` to `ResilientProxyConfig` and `ResilientProxy` (nil = today's behaviour, so existing callers are unaffected)
- [ ] Call `p.replay.Observe(buf[:n])` in `copyClientToServer` on each successful read, before the write/buffer branch
- [ ] Write `p.replay.Frames()` to the new `serverConn` in `reconnect()` immediately after `upgradeFunc` succeeds and **before** flushing the input buffer
- [ ] Create `api/pkg/proxy/streaminit.go` with an incremental RFC 6455 client-frame parser (7/16/64-bit lengths, 4-byte mask key), capturing the first **text** frame and skipping binary keepalives, ping/pong and close
- [ ] Clear `user_retry` on the replayed payload by unmarshalling to `map[string]any` and deleting the key — not by round-tripping through `StreamConfig`, which would drop unknown fields
- [ ] Re-encode the replay as a freshly-masked client text frame; cache it and make `Observe` a no-op once captured
- [ ] Bound the buffered prefix (a few KB) so a client that never sends a text frame cannot grow it without limit
- [ ] Return nil from `Frames()` when no init has been seen, so a reconnect before init replays nothing rather than garbage
- [ ] Pass `Replay: proxy.NewStreamInitReplay()` into `ResilientProxyConfig` in `proxyStreamWebSocket` (`api/pkg/server/external_agent_handlers.go`), adjacent to the existing `UpgradeFunc` line
- [ ] Handle the partial-frame-across-drop hazard (design B7): expose the last complete frame boundary and discard a partially-written frame's remainder from the input buffer before flushing — or, if that exceeds ~20 lines, ship a `log.Warn` on detected desync and file a follow-up

## 7. Part B — unit tests in `api/pkg/proxy/`

- [ ] `reconnect replays init` — kill the server conn, assert the re-dialled server receives the init before any buffered input
- [ ] `replayed init has user_retry cleared` — `user_retry: true` in, absent out, every other field unchanged
- [ ] Parser tests for 7-bit, 16-bit and 64-bit payload length forms
- [ ] Binary keepalives before init are skipped; the first text frame wins; a later text frame does not overwrite the captured init
- [ ] The replayed frame is a well-formed masked client text frame
- [ ] `Frames()` is nil before any init is observed
- [ ] Replay composes with `extraHeaders` — a read-only reconnect still carries `X-Helix-Readonly` and the replayed payload carries no privilege field

## 8. Verification — required, not optional

- [ ] `cd api && go build ./pkg/...` and `cd frontend && yarn build`
- [ ] In the inner Helix at `http://localhost:8080`: register `test@helix.ml` / `helixtest`, complete onboarding, create a spec task, open its detail page, confirm video actually plays in the browser
- [ ] Fresh task: `docker exec helix-sandbox-nvidia-1 docker inspect -f '{{.HostConfig.Memory}} {{.HostConfig.NanoCpus}}' <container>` shows the new limits (`25769803776 12000000000`, or the clamped CPU value with its log line)
- [ ] Pre-existing task row (non-meta deployment): confirm the migration NULLed it, that starting it produces the new limits, and that the `8/16384` row count is unchanged
- [ ] On meta: open one of the 31 hand-backfilled tasks and confirm the selector shows "12 CPU / 24 GB RAM" selected rather than blank
- [ ] Config override: set `HELIX_SPEC_TASK_SANDBOX_DEFAULT_VCPUS=8` / `..._MEMORY_MB=16384`, restart, create a task, confirm the container follows; then set an invalid pair and confirm startup refuses
- [ ] Real reconnect: with a browser watching a playing stream, restart `desktop-bridge` in the desktop container; confirm video resumes unattended, `reconnect_count` increments, and **no** `failed to read init message` appears in the desktop-bridge log
- [ ] Read the desktop-bridge log for which `GetOrCreate` branch the replayed init took — existing source vs `Evicted dead source` (design B6 / Open Question 5)
- [ ] Test the next operation after the drop: click in the desktop and confirm input still works; confirm an embed-key read-only viewer still cannot input after a reconnect
- [ ] `helix spectask stream <session> --duration 30` returns ~1600 frames at 50–60 FPS at 1920x1080

## 9. Ship

- [ ] Push the single branch `feature/002936-right-size-spec-task` with Part A and Part B as separate commits, so either half can be reverted alone
- [ ] Write the PR body in two clearly separated sections, calling out the irreversible migration at the top
- [ ] Write `design/2026-08-24-sandbox-defaults-and-stream-init-replay.md` in the helix repo recording the chosen default and reasoning, the migration strategy, and the `user_retry`-on-replay decision
- [ ] State in PR1's body that the migration resets deliberate 4/8192 choices, and why that is acceptable
- [ ] `gh pr checks` on both PRs; fix CI failures without being asked
- [ ] Report PR links as full URLs (`https://github.com/helixml/helix/pull/NNN`)
- [ ] Close `spt_01m0evm3dpanc1sfktywbxhes4` as superseded by this task
