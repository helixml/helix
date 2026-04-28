# Sandbox absorbs runner: compose-based inference, deletes Dockerfile.runner

Replaces the runner infrastructure (custom Go binary spawning vLLM/Ollama
subprocesses, scheduler bin-packing GPUs) with a sandbox-side compose
profile system. The runner image is deleted; Sandbox bundles two new
binaries (`compose-manager`, `inference-proxy`) that apply operator-
defined Docker Compose profiles and serve OpenAI-compatible inference by
forwarding to the matching container in the inner dockerd.

This is a large change set. It is shipped as one PR because the design
calls for an atomic switch (Decision 11 / AC8) and because the new code
needs to land alongside the data-model + HTTP-route additions to be
testable.

## Design references

- Requirements: `helix-specs/design/tasks/001959_we-need-to-replace-all/requirements.md`
- Design: `helix-specs/design/tasks/001959_we-need-to-replace-all/design.md`
- Tasks: `helix-specs/design/tasks/001959_we-need-to-replace-all/tasks.md`

## Summary

**New backend packages (all CGO-free, fully unit-tested):**

- `api/pkg/runner/gpuarch/` — NVIDIA compute-capability + AMD `gfx*` → canonical architecture string. Shared by the runner (label its GPUs) and API server (validate profile fit).
- `api/pkg/runner/composeparse/` — extract model list + GPU count from a profile's Docker Compose YAML, handling both NVIDIA-style and AMD-style GPU declarations + all five port forms.
- `api/pkg/runner/profile/` — parse-on-save service over the store; `Compatibility(req, gpus)` constraint check (count → vendor → arch → model_match → min_vram); `FilterCompatible(profiles, gpus)` for the dropdown.
- `api/pkg/inferencerouter/` — replaces the scheduler's request-routing role. `PickRunner(model)` round-robins among connected sandboxes whose active profile contains the model and are `running`.
- `api/pkg/composemgr/` — applies an assigned profile via `docker compose pull` (skipped if `HELIX_RUNNER_OFFLINE=true`) → down-old → up-new → poll readiness. Registry mirror via `HELIX_RUNNER_REGISTRY`. Periodic prune that NEVER runs inline with profile switches.
- `api/pkg/inferenceproxy/` — body-aware reverse proxy reading the `model` field from request bodies and forwarding to the matching container.

**New binaries (ship inside the Sandbox image):**

- `api/cmd/compose-manager/` — reconciliation loop polling `GET /api/v1/runners/{id}/assignment` and applying profiles via composemgr.
- `api/cmd/inference-proxy/` — HTTP server reading `/etc/helix/active.yaml`, reloading on SIGHUP + 30s mtime poll.

**Data model:**

- New `RunnerProfile` and `RunnerAssignment` GORM types. AutoMigrate-registered.
- `GPUStatus` extended with `Vendor`, `Architecture`, `ComputeCapability`.
- `SandboxInstance` and `SandboxHeartbeatRequest` extended with `GPUs`, `ActiveProfileID`, `ProfileStatus`, `ProfileError`, `ServiceHealth`.

**HTTP routes (all admin, swagger-annotated):**

- `GET / POST / PUT / DELETE /api/v1/runner-profiles[/{id}]` — profile CRUD.
- `GET /api/v1/runners/{id}/compatible-profiles` — server-side dropdown filter.
- `GET / POST /api/v1/runners/{id}/assignment` + `/assign-profile` + `/clear-profile` — assignment lifecycle. 422 with named-constraint detail on incompatibility.
- `GET /v1/models` — OpenAI-compatible, returns the union of model names across running profiles.

**Image / chart changes:**

- `Dockerfile.sandbox` extended with two new builder stages, COPYs into `/usr/local/bin/`, two new cont-init.d hooks, `/etc/helix` directory.
- `Dockerfile.runner`, `Dockerfile.runner.dockerignore`, `docker-compose.runner.yaml` deleted.
- `charts/helix-runner/` deleted entirely.

**Frontend:**

- New "Runner Profiles" admin tab + sidebar entry.
- `RunnerProfilesTable.tsx` (list with name / models / GPU req chips / actions).
- `EditRunnerProfile.tsx` (modal with sample compose pre-populated; vendor / architectures / model_match / min_vram fields).
- `runnerProfilesService.ts` React Query hooks. Uses raw axios pending `./stack update_openapi` regenerating from the new swagger annotations — TODO marker for swap-over.

**Sample profiles:** five YAML files under `design/sample-profiles/` (8xH100, any-Blackwell-4-GPU, any-NVIDIA-dev, AMD MI300X, dev-spike-tiny) + README. Validated by `composeparse/sample_profiles_test.go`.

**Internal openai client:** `helix_openai_client` repointed transitionally — `enqueueRequest` tries `inferencerouter.PickRunner(model)` first, falls back to scheduler when no sandbox can serve. `dispatchHTTPToRunner` is currently a stub returning error so the scheduler fallback kicks in. Once GPU validation confirms the HTTP-via-router path works end-to-end on real hardware, the stub becomes a real implementation and the scheduler fallback is deleted in a follow-up PR.

## What this PR does NOT do (deliberate, deferred to follow-up)

- **Does not delete `api/pkg/scheduler/`, the per-runtime files (`vllm_runtime.go` etc.), `memory_estimation_handlers.go`, slot CRUD, or the `RunnerSlot` type.** Decision 11 / AC8 mandate this deletion, but doing it before the HTTP-via-router path is validated against real GPUs would leave the API server with no working inference path. Sequence: this PR ships the new path alongside the old; a follow-up PR (after GPU validation) implements `dispatchHTTPToRunner` for real and atomically deletes the scheduler-related code.
- **Does not implement the runner-side GPU detection** that populates `SandboxInstance.GPUs` with vendor + arch + VRAM. Until this lands, the compatibility check returns "no GPUs known," which makes vendor / arch / VRAM checks fail-closed (refuses non-trivial profiles). Safe default. Add `nvidia-smi --query-gpu=name,compute_cap,memory.total / rocm-smi` parsing into the heartbeat publisher in a follow-up.
- **Does not extend the `AgentSandboxes` admin table** to show profile / service columns. The data is reported in heartbeats; the table just needs new columns.
- **Does not validate end-to-end on a GPU host.** The spike (GPU passthrough into nested dockerd, run a tiny model, send a chat completion) has not been done — this dev box has no GPU. Marked `BLOCKED — needs GPU host` in tasks.md.
- **Does not run `./stack update_openapi`** to regenerate the frontend API client. Frontend uses raw axios with a clear TODO; type-checks pass when run against installed deps.

## Test plan

```
# Build everything new + touched
go build \
  ./api/pkg/types/ \
  ./api/pkg/store/ \
  ./api/pkg/system/ \
  ./api/pkg/runner/gpuarch/ \
  ./api/pkg/runner/composeparse/ \
  ./api/pkg/runner/profile/ \
  ./api/pkg/inferencerouter/ \
  ./api/pkg/composemgr/ \
  ./api/pkg/inferenceproxy/ \
  ./api/pkg/server/ \
  ./api/pkg/openai/ \
  ./api/cmd/compose-manager/ \
  ./api/cmd/inference-proxy/

# Run new unit tests (50+ tests)
go test \
  ./api/pkg/runner/gpuarch/ \
  ./api/pkg/runner/composeparse/ \
  ./api/pkg/runner/profile/ \
  ./api/pkg/inferencerouter/ \
  ./api/pkg/composemgr/ \
  ./api/pkg/inferenceproxy/

# Manual verification (post-merge, on a GPU host):
# - assign design/sample-profiles/dev-spike-tiny.yaml; compose-manager pulls + ups
# - send chat completion for `qwen2.5-0.5b`; inference-proxy routes
# - admin UI: create / edit / delete a profile via the new tab
# - assign incompatible profile; receive 422 with named-constraint error
# - restart sandbox; profile re-applies on heartbeat
```

## Implementation notes worth highlighting

- The `inferencerouter` package lives outside `api/pkg/runner/` because the existing runner package can't compile in CGO-disabled environments (Ollama Go SDK + memory_estimation imports). Decoupling lets the router and its tests build and run independently of the runner-package deletion timeline.
- `SandboxInstance.GPUs` is a `datatypes.JSON` field rather than `[]GPUStatus` directly because GORM handles jsonb cleanly that way. The heartbeat path serializes via the `SandboxHeartbeatRequest.GPUs []GPUStatus` typed field.
- The compose-manager polls HTTP rather than receiving NATS commands. This is a deliberate simplification — the existing sandbox already has an HTTP path back to the API server, and inference profile state changes are infrequent enough that polling is fine.
- The sandbox-absorbs-runner pivot was not in the original design; see design.md Decision 12 for the rationale and the abbreviated implementation impact (smaller / safer than the originally-planned new "worker" image).
