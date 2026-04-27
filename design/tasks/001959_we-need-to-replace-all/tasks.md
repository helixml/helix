# Implementation Tasks

## Spike (do first, may invalidate parts of the design)

- [ ] Confirm GPU passthrough into nested dockerd works (`--gpus all` on outer + nvidia-container-toolkit in inner image). **Use a tiny model** — the dev hardware is a single 16 GB GPU shared with desktop workloads, so the user's full sample compose (8 GPUs, ~700 GB total VRAM) is irrelevant for derisking. Pick something like `Qwen/Qwen2.5-0.5B-Instruct` on vLLM with `--gpu-memory-utilization 0.2 --max-model-len 4096` on `device_ids: ["0"]`. The spike is "does the GPU show up inside the inner container and produce one valid completion?" — nothing more. If GPU passthrough doesn't work, revisit Decision 1 in `design.md` before any other implementation. Save the working tiny-spike compose as `design/sample-profiles/dev-spike-tiny.yaml` so future agents on similar hardware can re-run it.
- [ ] Confirm the `helixml/helix` org's NATS deployment can survive removal of all slot-related subjects (no external consumers).

## Backend: Profile Storage & API

- [ ] Add `runner_profiles` and `runner_assignments` tables (migration in `api/pkg/store/`).
- [ ] Implement `api/pkg/runner/composeparse/parse.go`: extract `ProfileModel[]` and the `Count` (union of `device_ids`) from a compose YAML string. Vendor/architecture/model-match/min-VRAM are operator inputs, not parsed.
- [ ] Unit tests for `composeparse` covering: `--served-model-name`, `--model` fallback, multi-GPU `device_ids`, `tensor-parallel-size`, services with no GPU reservation (count=0).
- [ ] Implement `api/pkg/runner/gpuarch/canonical.go`: shared mapping for NVIDIA compute capability → architecture canonical string and AMD `gfx*` → architecture string. One file, used by both runner (to label its GPUs) and API server (to validate profiles). Add table-driven tests.
- [ ] Implement `api/pkg/runner/profile/store.go` (CRUD against the new tables; re-derive `Count` + `Models` on save; persist vendor/architectures/model_match/min_vram_bytes verbatim from the request).
- [ ] Add HTTP routes in `api/pkg/server/`:
  - `GET    /api/v1/runner-profiles`
  - `POST   /api/v1/runner-profiles`
  - `GET    /api/v1/runner-profiles/{id}`
  - `PUT    /api/v1/runner-profiles/{id}`
  - `DELETE /api/v1/runner-profiles/{id}`
  - `POST   /api/v1/runners/{runner_id}/assign-profile` (body: `{"profile_id": "..."}`)
  - `POST   /api/v1/runners/{runner_id}/clear-profile`
- [ ] Add profile-compatibility check to the assign endpoint: index existence → vendor → architecture → model_match regex → min VRAM, returning a single named-constraint failure on mismatch.
- [ ] Filter the assignment dropdown server-side: `GET /api/v1/runners/{id}/compatible-profiles` returns only profiles that pass all five checks against the runner's reported hardware.

## Backend: Runner Router (replaces scheduler)

- [ ] Implement `api/pkg/runner/router.go` with `PickRunner(model)` (round-robin across runners whose active profile contains the model and are in `running` state).
- [ ] Wire `/v1/chat/completions`, `/v1/embeddings`, `/v1/images/generations` (and any other OpenAI-compatible endpoints currently routed via the scheduler) through the new router.
- [ ] **Repoint `api/pkg/openai/helix_openai_client.go`** so the two `scheduler.Enqueue` call sites (lines 305 and 399 today — chat completions and embeddings) call the new router instead. Public method signatures must not change. Drop the `scheduler` import; add a router dependency to the client constructor and update wherever the client is instantiated.
- [ ] Wire `/v1/models` to return the union of model names across all currently-`running` profiles.
- [ ] Return HTTP 503 with a list of currently-available models when no runner qualifies. Use the same error shape from both the HTTP path and the in-process `helix_openai_client` path so callers see consistent errors.

## Backend: NATS Surface Reduction

- [ ] Narrow `api/pkg/runner/controller_nats.go` to only `runner.{id}.status` (in) and `runner.{id}.cmd` (out, with `set_profile` / `clear_profile` actions).
- [ ] Delete subjects + handlers for slot create/delete/list/inference.
- [ ] Persist the last-known assignment per runner; on runner reconnect, re-send `set_profile` so the runner re-applies after restart.

## Runner Binary

- [ ] Strip `Dockerfile.runner` to: golang build artifact + dockerd + docker CLI + nvidia-container-toolkit (no vLLM, no Ollama, no axolotl, no diffusers).
- [ ] Implement `api/pkg/runner/compose_manager.go`:
  - Apply `set_profile`: pull-new (unless offline) → down-old → up-new → poll readiness. **Never** prune between down-old and up-new.
  - Apply `clear_profile`: down current → delete `/etc/helix/active.yaml`.
  - Honour `HELIX_RUNNER_REGISTRY` (rewrite leading registry portion of `image:` fields) and `HELIX_RUNNER_OFFLINE` (skip pull; fail fast if any referenced image is absent from `/var/lib/docker`).
  - Stream concise progress events back via NATS status updates.
- [ ] Implement `api/pkg/runner/proxy.go`: body-buffered, model-aware reverse proxy. Returns 404 on unknown model.
- [ ] Replace runner's HTTP server (`api/pkg/runner/server.go`) routes with just: `POST /v1/chat/completions`, `POST /v1/embeddings`, `POST /v1/images/generations`, `GET /api/v1/status`, `GET /api/v1/services/{name}/logs`.
- [ ] Add startup behaviour: on boot, if the API has previously assigned a profile, fetch + apply it before reporting `running`.

## Runner Persistence & Offline Operation

- [ ] In `docker-compose.yaml` and `docker-compose.dev.yaml`, declare two named volumes for the runner service:
  - `helix-runner-docker-storage:/var/lib/docker:rw`
  - `helix-runner-models:/models:rw`
  - Same lifecycle conventions as Sandbox's `sandbox-docker-storage` and HF cache mounts (`docker-compose.dev.yaml` lines 268–303 and line 15) — survives container restart and image upgrade.
- [ ] In `charts/helix-runner/`, add equivalent PVCs for the two volumes; ensure the helm chart preserves them across pod restarts and image bumps.
- [ ] Forward `HUGGING_FACE_HUB_TOKEN` from the runner container env into compose services that declare it (the user's example compose pattern).
- [ ] Implement registry-rewrite in `compose_manager.go`: when `HELIX_RUNNER_REGISTRY` is set, rewrite the leading registry portion of every `image:` field in the active YAML before invoking docker compose. Mirror the substitution from `sandbox/04-start-dockerd.sh` lines 205–235 (use the same regex if practical so behaviour is identical).
- [ ] Implement `HELIX_RUNNER_OFFLINE=true`: skip `docker compose pull`; before `up -d`, query the inner dockerd for each image referenced in the YAML and fail the assignment with a clear list if any are absent.
- [ ] Implement image-prune-on-low-water-mark as a *separate* periodic task in the runner — never inline with profile switches. Use `docker image prune --filter "until=72h"` or similar; prune must never run between `down-old` and `up-new`.
- [ ] Document `/models` as the canonical compose-side mount path in the operator guide; provide a sample profile (the user's example, with `/prod/models` swapped to `/models`) to make this obvious.

### Verification (manual, requires GPU host)

- [ ] Configure runner with `HELIX_RUNNER_OFFLINE=true` *without* pre-populating any images; assign a profile; confirm assignment fails with a clear list of missing images.
- [ ] Pre-populate the inner dockerd with required images (via `docker pull` against an online runner first); set `HELIX_RUNNER_OFFLINE=true`; confirm profile assignment succeeds with no network access.
- [ ] Pre-populate `helix-runner-models` with a model's weights; set `HF_HUB_OFFLINE=1` in the compose env; confirm vLLM container starts without contacting HuggingFace.
- [ ] Restart the runner container; confirm both image cache (`docker images` inside inner dockerd) and model cache (contents of `/models`) are intact.
- [ ] Upgrade the runner image (`docker compose pull && up -d` on the *outer* compose); confirm both caches survive.
- [ ] With `HELIX_RUNNER_REGISTRY=mirror.local`, assign a profile; confirm `image:` references in the active YAML are rewritten to the mirror; confirm pull goes to the mirror.
- [ ] Switch between two profiles that share an image; confirm no re-pull happens (image cache shared); confirm prune does not run between switches.

## Backend: Deletions (do all of this in the same change set as the new code)

### Scheduler package — delete in full
- [ ] `api/pkg/scheduler/` — every file: `scheduler.go`, `global_allocator.go`, `slot.go`, `slot_store.go`, `cache.go`, `queue.go`, `workload.go`, `runner.go`, `model_allocation.go`, `decisions.go`, `errors.go`, `util.go`, `test_helpers.go`, and every `*_test.go`. **Specifically including** the bin-pack-meets-tensor-parallel logic (`global_allocator.go`, `multi_gpu_eviction_test.go`, `memory_calculation_inconsistency_test.go`, `model_allocation_integration_test.go`) — that combined complexity is the whole point of replacing this.

### Runtime files & process supervision — delete in full
- [ ] `api/pkg/runner/vllm_runtime.go`
- [ ] `api/pkg/runner/ollama_runtime.go`
- [ ] `api/pkg/runner/axolotl_runtime.go`
- [ ] `api/pkg/runner/diffusers_runtime.go`
- [ ] `api/pkg/runner/ollama_model_controller.go`
- [ ] `api/pkg/runner/process_monitor.go`, `commander.go`, `commander_mocks.go`

### Memory estimation — delete in full
- [ ] `api/pkg/runner/memory_estimation_handlers.go` (the file that imports `github.com/ollama/ollama/{api,discover,fs/ggml,llm}` for GGUF parsing)
- [ ] `api/pkg/server/memory_estimation_handlers.go`
- [ ] `api/pkg/types/memory.go`
- [ ] Drop `github.com/ollama/ollama` from `go.mod` and run `go mod tidy`.

### GPU helpers — slim down, don't delete
- [ ] `api/pkg/runner/gpu.go` / `gpu_memory_tracker.go` — keep only what's needed to *report* per-GPU inventory (vendor, arch, total VRAM, used VRAM) for AC2. Delete per-slot allocation logic.

### Slot CRUD & per-slot proxy
- [ ] `api/pkg/runner/slot.go` and slot route registrations in `api/pkg/runner/server.go`.
- [ ] `api/pkg/runner/openai_finetuning_handlers.go`, `helix_finetuning_handlers.go`, `helix_image_handlers.go`.
- [ ] `api/pkg/server/handlers.go` — remove `deleteSlot()` (line ~1090) and `getSchedulerHeartbeats()` (line ~405) handlers, and their `@Router` annotations.
- [ ] `api/pkg/controller/handlers.go` — remove `DeleteSlotFromScheduler()`, `RunnerSlots()`, and any other slot-listing methods.
- [ ] `api/pkg/openai/helix_openai_server.go` — inspect; delete if it exists only to bridge the scheduler.

### Types
- [ ] `api/pkg/types/runner.go`: delete `RunnerSlot`, `CreateRunnerSlotRequest`, `CreateRunnerSlotAttributes`, `ListRunnerSlotsResponse`, `RunnerModelStatus`, `Runtime` enum.
- [ ] `api/pkg/types/types.go`: delete `SchedulingDecisionType`, `SchedulingDecision`, `GlobalSchedulingDecision`, `GlobalAllocationDecision`, `AllocationPlanView`, `GPUMemoryStats`.
- [ ] `api/pkg/types/runner.go` `RunnerStatus`: drop `AllocatedMemory`, `Models`, `GPUMemoryStats` fields.
- [ ] `api/pkg/types/models.go` `HelixModel`: drop `Prewarm` field.

### Database
- [ ] `api/pkg/store/store_slots.go` — delete the file and remove its methods from the `Store` interface.
- [ ] Add an explicit migration that drops the `runner_slots` table (don't rely on GORM AutoMigrate to ignore orphaned tables).
- [ ] Drop any scheduling-decision/allocation-history tables if they exist.

### Config / env vars
- [ ] `api/pkg/config/config.go` `Helix` struct: remove `ModelTTL`, `SlotTTL`, `SchedulingStrategy`, `QueueSize`.
- [ ] Remove `HELIX_MODEL_TTL`, `HELIX_SLOT_TTL`, `HELIX_SCHEDULING_STRATEGY`, `HELIX_QUEUE_SIZE` from `.env.example` and any sample configs.

### CLI wiring
- [ ] `api/cmd/helix/serve.go`: remove `NewScheduler()` call site (~line 332) and the `PrewarmNewRunner` callback wiring (~line 375). Wire the new `Router` in their place.

### Frontend dead code
- [ ] Delete `frontend/src/components/dashboard/GlobalSchedulingVisualization.tsx`.
- [ ] Delete `frontend/src/components/dashboard/SchedulingDecisionsTable.tsx`.
- [ ] Delete `frontend/src/components/dashboard/SchedulerHealthIndicators.tsx`.
- [ ] Remove `MemoryEstimateCell` from `HelixModelsTable.tsx` and its helpers.
- [ ] Remove dead React Query hooks: `useDeleteSlot`, slot list queries, `v1SchedulerHeartbeatsList`, `v1MemoryEstimationsList`.
- [ ] Remove the `Dashboard.tsx` tabs that hosted the deleted components.
- [ ] After `update_openapi`, spot-check `frontend/src/api/api.ts` to confirm `TypesRunnerSlot`, `TypesSchedulingDecision`, etc. are gone.

### Docker / Helm
- [ ] Strip `Dockerfile.runner`: remove vLLM CUDA venv setup, vLLM ROCm venv setup, Ollama binary install, Axolotl fake venv, Diffusers, Python toolchain, model preload cache, all `wget`/`pip` lines tied to those. End state: golang build + dockerd + docker CLI + nvidia-container-toolkit only.
- [ ] Delete `docker-compose.runner.yaml` (was for the standalone runner).
- [ ] `charts/helix-runner/values.yaml` and `templates/deployment.yaml`: remove vLLM env vars, Ollama config, model-preload values, scheduling-strategy values. Confirm chart still produces a working pod.

### Docs
- [ ] Delete or rewrite `helix/design/` docs that explain the scheduler/slot/prewarming model. Add a note pointing at the new design doc.
- [ ] Update `docs/` operator pages: replace per-slot scheduler explanation with profile model.
- [ ] Rewrite `charts/helix-runner/README.md`.

### Verification
- [ ] `go build ./...` clean.
- [ ] `go vet ./...` clean.
- [ ] `git grep -nE "scheduler\.|Scheduler\b|RunnerSlot\b|GGUF|memory_estimation|ollama/ollama|axolotl|diffusers_runtime|SchedulingDecision|GlobalAllocationDecision|Prewarm|tensor.*binpack|HELIX_SLOT_TTL|HELIX_MODEL_TTL|HELIX_SCHEDULING_STRATEGY|HELIX_QUEUE_SIZE"` returns only legitimate hits (release notes / migration mentions). No live references.
- [ ] `frontend/` builds clean; no unused imports flagged by tsc/eslint.

## CGO-Free Runner Build (Adopt After Deletions)

**Important:** the API server keeps `CGO_ENABLED=1` and `-tags ORT`. `github.com/yalue/onnxruntime_go` is required by **Kodit** (`api/pkg/server/kodit_init.go:261` — `preflightORT()` checks for `libonnxruntime.so` when Kodit's local ONNX embedder is in use). Kodit is unrelated to runners and we do not touch it. **Do not** drop the ORT dep, do not remove `-tags ORT`, do not flip the API server's `Dockerfile` to CGO=0.

The runner is a different story: its only CGO drivers are the Ollama Go SDK imports we are deleting in Category 2.

- [ ] After the deletions above, run `git grep -E '^import \"C\"' runner-cmd/ api/pkg/runner/` — should return nothing.
- [ ] Flip `Dockerfile.runner` to `CGO_ENABLED=0` and drop the `-tags "!rocm"` tag (paired with the runtime split that no longer exists).
- [ ] Confirm clean runner image build; `ldd /helix-runner` should show no surprising dynamic links (ideally a static binary).
- [ ] If an indirect runner dep still requires CGO (e.g. via shared `provider_manager` code), write `design/2026-MM-DD-cgo-after-runner-rewrite.md` documenting the dep + reason and leave runner CGO=1. Do not ship a half-disabled state. Do not go hunting for the indirect dep to swap it out — scope creep.
- [ ] Out of scope: API server `Dockerfile` (CGO=1 stays for Kodit), desktop binaries (`desktop-bridge` etc., keep CGO=1 for xkb/wayland/pipewire).

## Frontend: Profile UI

- [ ] Add `runner_profiles` tab to `frontend/src/pages/Dashboard.tsx`.
- [ ] Build `RunnerProfilesTable.tsx` (mirror `HelixModelsTable.tsx` shape).
- [ ] Build `EditRunnerProfile.tsx` modal with Monaco YAML editor; on save, POST to backend (which re-derives metadata).
- [ ] Show derived models + GPU requirement read-only beneath the editor as confirmation.

## Frontend: Runner Assignment UI

- [ ] In `RunnerSummary.tsx`, add a "Profile" dropdown populated from `GET /api/v1/runners/{id}/compatible-profiles` (server-filtered).
- [ ] Display per-runner the inferred vendor + architecture + per-GPU model so operators can see *why* a given profile is or isn't shown.
- [ ] On change, call `POST /api/v1/runners/{id}/assign-profile`. On error, surface the named-constraint failure verbatim.
- [ ] Replace the slot list with a list of services from the active profile, rendered via a modified `ModelInstanceSummary.tsx` (status, health, "View Logs" button).
- [ ] Keep the per-GPU memory chart unchanged.

## Frontend: Generated Client

- [ ] Regenerate `frontend/src/api/api.ts` after backend route changes (`update_openapi`).
- [ ] Remove now-dead hooks (`useDeleteSlot`, slot list queries).

## Sample Profiles

- [ ] Commit the user's example compose as `design/sample-profiles/8xH100-vllm.yaml` with GPU req: vendor=nvidia, architectures=[hopper], model_match=`^NVIDIA H100`, min_vram=80GB.
- [ ] Add `design/sample-profiles/any-nvidia-blackwell-4gpu.yaml` — vendor=nvidia, architectures=[blackwell], no model_match.
- [ ] Add `design/sample-profiles/any-nvidia-dev-single-gpu.yaml` — vendor=nvidia, no arch restriction, min_vram=24GB. (Demonstrates the permissive case.)
- [ ] Add `design/sample-profiles/dev-spike-tiny.yaml` — single tiny model (e.g. `Qwen2.5-0.5B-Instruct`) on `device_ids: ["0"]` with `--gpu-memory-utilization 0.2`. Sized to coexist with desktop workloads on a shared 16 GB dev GPU. This is the profile the spike uses; it's also the profile any future agent should reach for when validating on similar dev hardware.
- [ ] Add `design/sample-profiles/amd-mi300x-vllm.yaml` — vendor=amd, architectures=[cdna3], using `rocm/vllm` images. (Demonstrates the AMD path; even if we have no AMD hardware to test against right now, it documents the intent.)

## Manual Verification (no automated coverage possible — flag as user-tested)

- [ ] Bring up a runner. On dev hardware (single 16 GB GPU shared with desktops), assign `dev-spike-tiny.yaml` and verify the one container comes up. On a production GPU host with hardware to match, assign the 8xH100 profile and verify all five containers come up. Pick the profile that fits the hardware in front of you — don't try to run the 8xH100 profile on dev kit.
- [ ] Send a chat completion for the model exposed by the assigned profile (`Qwen2.5-0.5B-Instruct` on dev hardware, `qwen3.5-35b` on the 8xH100 profile) and confirm it routes through.
- [ ] Send an embeddings request for `Qwen/Qwen3-VL-Embedding-8B` and confirm it routes through.
- [ ] Create a session via `POST /api/v1/sessions`, send messages, confirm streaming responses work end-to-end (this exercises `helix_openai_client.go` → router path).
- [ ] Trigger summary/auto-titling on a session and confirm it works (exercises `summary_service.go` → `helix_openai_client` → router).
- [ ] Request a model that isn't in any currently-running profile; confirm HTTP 503 with the list of available models, and the same error class returned via the in-process client.
- [ ] Switch the runner to a different profile. Verify the previous stack is torn down and the new one comes up.
- [ ] Assign a profile that requires more GPUs than the runner has and confirm a clear error.
- [ ] Try to assign an `vendor=amd` profile to an NVIDIA runner; confirm rejection names the failing constraint.
- [ ] Try to assign an `architectures=[blackwell]` profile to a Hopper runner; confirm rejection names the failing constraint.
- [ ] Confirm `GET /api/v1/runners/{id}/compatible-profiles` excludes profiles whose constraints don't match.
- [ ] Restart the runner; confirm it re-applies its assigned profile automatically on boot.
- [ ] Confirm the admin dashboard correctly lists active services and per-service logs.

## Documentation

- [ ] Update `docs/` runner setup pages: replace per-slot scheduler explanation with the profile model.
- [ ] Add a short operator guide: "How to write a runner profile" (compose conventions, model name extraction, GPU requirement fields).
- [ ] Note in release notes: this is a breaking change for anyone calling `/api/v1/slots/*` directly.
