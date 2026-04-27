# Implementation Tasks

## Spike (do first, may invalidate parts of the design)

- [ ] Confirm GPU passthrough into nested dockerd works on a real GPU host (`--gpus all` on outer + nvidia-container-toolkit in inner image). Run the user's sample compose end-to-end. If it doesn't work, revisit Decision 1 in `design.md`.
- [ ] Confirm the `helixml/helix` org's NATS deployment can survive removal of all slot-related subjects (no external consumers).

## Backend: Profile Storage & API

- [ ] Add `runner_profiles` and `runner_assignments` tables (migration in `api/pkg/store/`).
- [ ] Implement `api/pkg/runner/composeparse/parse.go`: extract `ProfileModel[]` and `ProfileGPURequirement` from a compose YAML string.
- [ ] Unit tests for `composeparse` covering: `--served-model-name`, `--model` fallback, multi-GPU `device_ids`, `tensor-parallel-size`.
- [ ] Implement `api/pkg/runner/profile/store.go` (CRUD against the new tables; re-derive metadata on save).
- [ ] Add HTTP routes in `api/pkg/server/`:
  - `GET    /api/v1/runner-profiles`
  - `POST   /api/v1/runner-profiles`
  - `GET    /api/v1/runner-profiles/{id}`
  - `PUT    /api/v1/runner-profiles/{id}`
  - `DELETE /api/v1/runner-profiles/{id}`
  - `POST   /api/v1/runners/{runner_id}/assign-profile` (body: `{"profile_id": "..."}`)
  - `POST   /api/v1/runners/{runner_id}/clear-profile`
- [ ] Add profile-compatibility check to the assign endpoint (count, VRAM, optional regex).

## Backend: Runner Router (replaces scheduler)

- [ ] Implement `api/pkg/runner/router.go` with `PickRunner(model)` (round-robin across runners whose active profile contains the model and are in `running` state).
- [ ] Wire `/v1/chat/completions`, `/v1/embeddings`, `/v1/images/generations` (and any other OpenAI-compatible endpoints currently routed via the scheduler) through the new router.
- [ ] Return HTTP 503 with a list of currently-available models when no runner qualifies.

## Backend: NATS Surface Reduction

- [ ] Narrow `api/pkg/runner/controller_nats.go` to only `runner.{id}.status` (in) and `runner.{id}.cmd` (out, with `set_profile` / `clear_profile` actions).
- [ ] Delete subjects + handlers for slot create/delete/list/inference.
- [ ] Persist the last-known assignment per runner; on runner reconnect, re-send `set_profile` so the runner re-applies after restart.

## Runner Binary

- [ ] Strip `Dockerfile.runner` to: golang build artifact + dockerd + docker CLI + nvidia-container-toolkit (no vLLM, no Ollama, no axolotl, no diffusers).
- [ ] Implement `api/pkg/runner/compose_manager.go`:
  - Apply `set_profile`: down current → write `/etc/helix/active.yaml` → pull → up -d → poll readiness.
  - Apply `clear_profile`: down current → delete file.
  - Stream concise progress events back via NATS status updates.
- [ ] Implement `api/pkg/runner/proxy.go`: body-buffered, model-aware reverse proxy. Returns 404 on unknown model.
- [ ] Replace runner's HTTP server (`api/pkg/runner/server.go`) routes with just: `POST /v1/chat/completions`, `POST /v1/embeddings`, `POST /v1/images/generations`, `GET /api/v1/status`, `GET /api/v1/services/{name}/logs`.
- [ ] Add startup behaviour: on boot, if the API has previously assigned a profile, fetch + apply it before reporting `running`.

## Backend: Deletions

- [ ] Delete `api/pkg/scheduler/` entirely.
- [ ] Delete `api/pkg/runner/{vllm,ollama,axolotl,diffusers}_runtime.go` and any helpers only used by them.
- [ ] Delete `api/pkg/runner/slot.go` and `RunnerSlot` from `api/pkg/types/runner.go`.
- [ ] Delete slot CRUD handlers and tests.
- [ ] Remove all imports + dead references; `go build ./...` and `go vet ./...` are clean.

## Frontend: Profile UI

- [ ] Add `runner_profiles` tab to `frontend/src/pages/Dashboard.tsx`.
- [ ] Build `RunnerProfilesTable.tsx` (mirror `HelixModelsTable.tsx` shape).
- [ ] Build `EditRunnerProfile.tsx` modal with Monaco YAML editor; on save, POST to backend (which re-derives metadata).
- [ ] Show derived models + GPU requirement read-only beneath the editor as confirmation.

## Frontend: Runner Assignment UI

- [ ] In `RunnerSummary.tsx`, add a "Profile" dropdown showing only profiles whose GPU requirements fit the runner's reported hardware.
- [ ] On change, call `POST /api/v1/runners/{id}/assign-profile`.
- [ ] Replace the slot list with a list of services from the active profile, rendered via a modified `ModelInstanceSummary.tsx` (status, health, "View Logs" button).
- [ ] Keep the per-GPU memory chart unchanged.

## Frontend: Generated Client

- [ ] Regenerate `frontend/src/api/api.ts` after backend route changes (`update_openapi`).
- [ ] Remove now-dead hooks (`useDeleteSlot`, slot list queries).

## Sample Profiles

- [ ] Commit the user's example compose as `design/sample-profiles/8xH100-vllm.yaml`.
- [ ] Add at least one smaller form factor (e.g. `2xL40S-qwen-only.yaml`) so operators have starting points.

## Manual Verification (no automated coverage possible — flag as user-tested)

- [ ] Bring up a runner against a real GPU host. Assign the 8xH100 profile. Verify all five containers come up.
- [ ] Send a chat completion for `qwen3.5-35b` to the API and confirm it routes through.
- [ ] Switch the runner to a different profile. Verify the previous stack is torn down and the new one comes up.
- [ ] Assign a profile that requires more GPUs than the runner has and confirm a clear error.
- [ ] Restart the runner; confirm it re-applies its assigned profile automatically on boot.
- [ ] Confirm the admin dashboard correctly lists active services and per-service logs.

## Documentation

- [ ] Update `docs/` runner setup pages: replace per-slot scheduler explanation with the profile model.
- [ ] Add a short operator guide: "How to write a runner profile" (compose conventions, model name extraction, GPU requirement fields).
- [ ] Note in release notes: this is a breaking change for anyone calling `/api/v1/slots/*` directly.
