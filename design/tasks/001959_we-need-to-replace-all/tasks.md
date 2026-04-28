# Implementation Tasks

> **2026-04-28 architectural pivot:** the task list originally talked about replacing the Runner. We refined: **Sandbox absorbs the runner role.** `Dockerfile.runner` is deleted entirely; `Dockerfile.sandbox` gains two new binaries (`compose-manager`, `inference-proxy`). No new image type. See Decision 12 in design.md. Where a task says "runner image," read "delete it." Where it says "runner binary rewrite," read "compose-manager + inference-proxy added to Sandbox."
> - `runnerrouter` Go package → `inferencerouter` (already done — routes inference to sandboxes).
> - `Dockerfile.runner` → **deleted**.
> - GPU sharing is implicit: GPUs claimed by inference services in compose are off-limits to Hydra; the rest are Hydra's. No `x-helix` extension.


## Spike (do first, may invalidate parts of the design)

- [x] **VALIDATED on RTX 2000 Ada (16 GiB).** GPU passthrough into nested dockerd works. The full chain works end-to-end: docker compose pull/up of `dev-spike-tiny.yaml` inside the sandbox's inner dockerd; vLLM serves Qwen2.5-0.5B; chat completion roundtrips through the new API server → inferencerouter → dispatchHTTPToRunner → sandbox inference-proxy → vLLM path with no scheduler involvement. See "Spike Result" in design.md for the full chain.
- [~] Confirm the `helixml/helix` org's NATS deployment can survive removal of all slot-related subjects — **deferred** to scheduler-deletion follow-up PR.

## Backend: Profile Storage & API

- [x] Add `runner_profiles` and `runner_assignments` tables (migration in `api/pkg/store/`). Implementation note: project uses GORM AutoMigrate (per `api/pkg/store/migrations/README.md` — explicit SQL migrations are reserved for renames/alters), so this is just adding GORM types + registering in `postgres.go` AutoMigrate call. Types live in new file `api/pkg/types/runner_profile.go`.
- [x] Implement `api/pkg/runner/composeparse/parse.go`: extract `ProfileModel[]` and the `Count` (union of `device_ids`) from a compose YAML string. Vendor/architecture/model-match/min-VRAM are operator inputs, not parsed.
- [x] Unit tests for `composeparse` covering: `--served-model-name`, `--model` fallback, multi-GPU `device_ids`, `tensor-parallel-size`, services with no GPU reservation (count=0). Plus AMD device passthrough, mixed-vendor rejection, flag=value syntax, string command form, sidecar services skipped, all 5 port forms, NVIDIA `count:` fallback.
- [x] Bonus: `sample_profiles_test.go` validates the five committed `design/sample-profiles/*.yaml` parse cleanly with expected model + GPU counts.
- [x] Implement `api/pkg/runner/gpuarch/canonical.go`: shared mapping for NVIDIA compute capability → architecture canonical string and AMD `gfx*` → architecture string. One file, used by both runner (to label its GPUs) and API server (to validate profiles). Add table-driven tests. Adds `IsNVIDIA`/`IsAMD` predicates as a side-bonus for compatibility checks.
- [x] Implement `api/pkg/runner/profile/store.go` (CRUD against the new tables; re-derive `Count` + `Models` on save; persist vendor/architectures/model_match/min_vram_bytes verbatim from the request). Also added store-level CRUD in `store_runner_profiles.go` and `RunnerProfilePrefix=rprof_` to `system/uuid.go`.
- [~] Add HTTP routes in `api/pkg/server/`:
  - [x] `GET    /api/v1/runner-profiles`
  - [x] `POST   /api/v1/runner-profiles`
  - [x] `GET    /api/v1/runner-profiles/{id}`
  - [x] `PUT    /api/v1/runner-profiles/{id}`
  - [x] `DELETE /api/v1/runner-profiles/{id}`
  - [x] `POST   /api/v1/runners/{runner_id}/assign-profile` (body: `{"profile_id": "..."}`) — wired with compatibility check (returns 422 with named-constraint failure on mismatch).
  - [x] `POST   /api/v1/runners/{runner_id}/clear-profile` — idempotent, returns 204.
  - [x] `GET    /api/v1/runners/{runner_id}/compatible-profiles` — server-side filtering for the dropdown.
  - [x] `GET    /api/v1/runners/{runner_id}/assignment` — current assignment for a runner.
- [x] Add profile-compatibility check (`profile.Compatibility()`): count → vendor → architecture → model_match regex → min VRAM, returning a single named-constraint failure on mismatch via `*IncompatibilityReason`. Index-existence check belongs at the assignment layer (operates on parsed compose, not declared count) — to be wired when the assign endpoint is implemented.
- [x] Filter the assignment dropdown server-side: implemented as `profile.FilterCompatible()` helper. HTTP route `GET /api/v1/runners/{id}/compatible-profiles` to be wired in the next task block.

## Backend: Inference Router (replaces scheduler)

- [x] Implement `api/pkg/inferencerouter/router.go` (renamed twice — first from `api/pkg/runner/router.go` in design.md, then from `runnerrouter` for the sandbox-absorbs-runner pivot). `PickRunner(model)` round-robins across sandboxes whose active profile contains the model and are `running`. Includes `NoRunnerError` carrying available-models list, `AvailableModels()` for the `/v1/models` endpoint, and per-model round-robin counters.
- [x] Wire `/v1/models` — returns the union of model names across all sandboxes whose active profile is `running`. OpenAI-compatible response shape.
- [~] **Repoint `api/pkg/openai/helix_openai_client.go`** — done as transitional repoint. `enqueueRequest` first tries `inferenceRouter.PickRunner(model)`; if a connected sandbox can serve, calls `dispatchHTTPToRunner` (currently a stub returning error → falls back to scheduler path). Once GPU validation confirms HTTP-via-router works end-to-end, the stub becomes a real implementation and the scheduler fallback is deleted. The split is a deliberate transitional state — keeps the API server building and the existing path serving while the new one is brought online.
- [~] Return HTTP 503 with available-models list — implemented inside the inference proxy + `NoRunnerError`. The transitional helix_openai_client path still uses the scheduler error shape; converges when scheduler is removed.

## Backend: NATS Surface Reduction

- [ ] Narrow `api/pkg/runner/controller_nats.go` to only `runner.{id}.status` (in) and `runner.{id}.cmd` (out, with `set_profile` / `clear_profile` actions).
- [ ] Delete subjects + handlers for slot create/delete/list/inference.
- [ ] Persist the last-known assignment per runner; on runner reconnect, re-send `set_profile` so the runner re-applies after restart.

## Runner Binary (post-pivot: this is now sandbox-side compose-manager + inference-proxy)

> **Sandbox-absorbs-runner pivot:** the sections below were originally about
> rewriting the standalone runner binary. After the pivot the work splits
> into two new binaries that ship inside Sandbox: `compose-manager`
> applies profiles via docker compose, `inference-proxy` does model-name
> routing. The "rewrite, not refactor" framing still applies: the existing
> `api/pkg/runner/` package is destined for deletion (AC8). Items below
> tagged `[~]` are the "transitional" state (the new binaries are shipped;
> the old api/pkg/runner code lingers as fallback). Items tagged `[ ]`
> remain genuinely outstanding.



**Framing reminder (see Decision 11 in design.md):** the runner Go code is rewritten in the same change set as the deletions, not evolved in place. Roughly 6% of the existing runner code survives — copied forward as utilities. Don't try to thread changes through existing files; delete the package contents per AC8 and write new ones. The new package keeps the name `api/pkg/runner/` but its contents are entirely new.

**Surviving pieces to copy forward (as plain new files in the rewritten package, not edits-in-place):**
- [ ] NATS connection plumbing — connect/reconnect, heartbeat cadence (~100 lines from today's `controller_nats.go`).
- [ ] HTTP server scaffolding — mux, middleware, log/auth wiring (~50 lines from today's `server.go`).
- [ ] GPU detection for vendor/arch/total-VRAM reporting — slimmed subset of today's `gpu.go` (~150 lines after slimming).
- [ ] `RunnerStatus` type minus dead fields (`AllocatedMemory`, `Models`, `GPUMemoryStats`).
- [ ] `runner-cmd/helix-runner/main.go` — flag parsing, log setup, signal handling.

If you find yourself importing `Runtime`, slot state machine types, per-slot URL builders, or any other internal abstraction of the old runner into the new code, **stop** — that's a signal you're regressing toward the old design.

**New files (everything else) — give them their own shape, not the old one's shape:**
- [x] `api/pkg/composemgr/manager.go` — calls docker CLI, manages YAML, polls health, registry rewrite, offline mode. Replaces the originally-planned `api/pkg/runner/compose_manager.go`. 9 tests.
- [x] `api/pkg/inferenceproxy/proxy.go` — body-buffered, model-aware reverse proxy. Replaces `api/pkg/runner/proxy.go`. 6 tests.
- [~] `api/pkg/runner/controller_nats.go` — narrowed surface — **deferred**: the existing api/pkg/runner package is on the deletion list, narrowing it makes no sense. The new model is: compose-manager polls HTTP, inference-proxy serves HTTP. No NATS for the inference subsystem; Sandbox keeps using NATS only for the agent-desktop subsystem.
- [~] Pre-flight runtime registration check — **deferred to follow-up PR** (compose-manager logs and fails on `up -d` if runtime missing; explicit pre-flight is a UX improvement, not a correctness requirement).
- [~] GPU inventory — **deferred to follow-up PR**: GPUStatus already has the fields; the sandbox heartbeat is the right time to populate them via `nvidia-smi --query-gpu=...` / `rocm-smi`. Currently GPUs[] is reported empty by the sandbox heartbeat, which makes the compatibility check fail-closed (refuses non-trivial profiles) — the safe default until GPU detection lands.
- [x] Inference HTTP routes — implemented in `api/pkg/inferenceproxy/proxy.go` (Handler() serves `/v1/chat/completions`, `/v1/embeddings`, `/v1/images/generations`, `/v1/models`).
- [x] Profile state machine: `assigning → pulling → starting → running → failed` implemented in `api/pkg/composemgr` (setStatus + setFailed methods). Lifecycle is exactly the design.
- [x] Tests for each new file (composemgr: 9 tests, inferenceproxy: 6 tests).

**Image build (post-pivot — Sandbox is now the GPU-bearing image):**
- [x] `Dockerfile.runner` deleted. Sandbox image (`Dockerfile.sandbox`) extended with two new builder stages (compose-manager, inference-proxy) + COPYs into `/usr/local/bin/` + cont-init.d hooks + `/etc/helix` directory.
- [~] Multi-arch build for `linux/amd64` + `linux/arm64` — Sandbox already builds for both; the new binaries inherit that. **Verification deferred** to first CI build of Sandbox after this PR.
- [ ] Implement `api/pkg/runner/compose_manager.go`:
  - Apply `set_profile`: pull-new (unless offline) → down-old → up-new → poll readiness. **Never** prune between down-old and up-new.
  - Apply `clear_profile`: down current → delete `/etc/helix/active.yaml`.
  - Honour `HELIX_RUNNER_REGISTRY` (rewrite leading registry portion of `image:` fields) and `HELIX_RUNNER_OFFLINE` (skip pull; fail fast if any referenced image is absent from `/var/lib/docker`).
  - Stream concise progress events back via NATS status updates.
- [ ] Implement `api/pkg/runner/proxy.go`: body-buffered, model-aware reverse proxy. Returns 404 on unknown model.
- [ ] Replace runner's HTTP server (`api/pkg/runner/server.go`) routes with just: `POST /v1/chat/completions`, `POST /v1/embeddings`, `POST /v1/images/generations`, `GET /api/v1/status`, `GET /api/v1/services/{name}/logs`.
- [ ] Add startup behaviour: on boot, if the API has previously assigned a profile, fetch + apply it before reporting `running`.

## AMD GPU Support (parity with NVIDIA)

AMD's containerised GPU story is different from NVIDIA's: there is no single `--gpus all` flag. The standard pattern is to mount `/dev/kfd` (the kernel fusion driver) and `/dev/dri` (DRM render nodes) into the container and add the user to the `video` and `render` groups. The newer AMD Container Toolkit (`amd-container-toolkit`) automates this in a way analogous to nvidia-container-toolkit, but it's relatively new — the runner must work on hosts that have *either*.

- [ ] In `Dockerfile.runner`: install **both** runtimes side-by-side:
  - `nvidia-container-toolkit` (configured via `nvidia-ctk runtime configure --runtime=docker`)
  - `amd-container-toolkit` if available on the base image's package source; otherwise document the manual fallback (mount `/dev/kfd` and `/dev/dri`, `group_add: [video, render]`) and ensure the inner dockerd is launched with permission to do that.
- [ ] In the inner dockerd's `daemon.json`, register both `nvidia` and (if present) `amd` runtimes so compose files can reference either.
- [ ] In `composeparse/parse.go`: handle **both** GPU declaration styles when extracting the `Count` of GPUs a profile requests:
  - NVIDIA style (the user's example): `deploy.resources.reservations.devices` with `driver: nvidia` and `device_ids: [...]`.
  - AMD style: top-level `devices: [/dev/kfd, /dev/dri/renderDN]` plus `group_add: [video, render]`. Count is inferred from the number of distinct `/dev/dri/renderD*` entries (or `/dev/dri/card*` if used). If no GPU device entries are present, count=0 and the profile is treated as CPU-only.
  - Reject ambiguous/mixed declarations (a single service with both styles) with a clear error.
- [ ] On the runner, when applying a profile, ensure the inner dockerd has the right runtime registered for the profile's vendor (if vendor=nvidia, fail fast if `nvidia` runtime not registered; same for `amd`). This catches "wrong base image / missing toolkit" before `docker compose up` produces an opaque error.
- [ ] Sample profile `design/sample-profiles/amd-mi300x-vllm.yaml` must use the AMD-style declaration (devices + group_add + `image: rocm/vllm:...`), not the NVIDIA style. This is the reference operators copy when writing their own AMD profiles.
- [ ] Verify `gpuarch/canonical.go` covers both vendor codepaths: NVIDIA compute-capability → arch and AMD `gfx*` → arch (gfx906→vega20, gfx90a→cdna2, gfx942→cdna3, gfx1100→rdna3, etc.). Cite the source of mappings (AMD's LLVM target docs) in a comment so future agents know where to update.

### Multi-Arch Build (linux/amd64 + linux/arm64)

- [ ] Configure CI (`cloudbuild.yaml` or equivalent) to build `Dockerfile.runner` for both `linux/amd64` and `linux/arm64` and push a multi-arch manifest. NVIDIA Jetson/Grace ship arm64; AMD ROCm is x86-only in practice; operators on Apple Silicon dev machines need arm64 to run the runner without a GPU profile.
- [ ] Confirm the Go build line uses `GOOS=linux GOARCH=$TARGETARCH` (or equivalent buildx-aware) so the binary is the right arch in each layer.
- [ ] Spot-check the inner dockerd, docker CLI, and nvidia/amd container toolkit packages all have arm64 variants in their respective apt sources. If amd-container-toolkit lacks arm64 packaging (likely — AMD doesn't ship ROCm for arm64), allow the build to skip its install on arm64 with a clear log message: "AMD runtime omitted on arm64; arm64 runners cannot host AMD GPU profiles."
- [ ] Add a CI smoke test that pulls the multi-arch image on both architectures and runs `helix-runner --version`.

### Verification

- [ ] On an NVIDIA host: assign a profile written in NVIDIA style; confirm GPU passthrough works.
- [ ] On an AMD host (if available): assign a profile written in AMD style; confirm `/dev/kfd` and `/dev/dri` are present in the container and rocm-smi inside the container sees the GPU.
- [ ] On an arm64 dev machine (no GPU): runner starts, registers with API, can be assigned a CPU-only profile (or rejects GPU profiles with the clear "no GPU available" message from AC1a's vendor check).

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
- [ ] **Integrate (don't kill) the "Helix Models" tab.** Original AC8 said "informational-only." Refined: the tab evolves into a unified model registry where the *list* of models comes from active profiles' derived `Models` field (the source of truth — what's actually running), and the existing `HelixModel` records overlay metadata that doesn't fit on a compose service: pricing per-token, display name, marketing description, provider routing hints. UI shows a row per model with a clear distinction: "from profile X" badge if backed by a running profile, "registered metadata only (not currently served)" if there's a HelixModel without a matching profile. Deletes only the parts that the scheduler drove (memory estimation, prewarm flag, runtime field — the operator's compose declares the runtime).
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
- [ ] **Rewrite-not-refactor litmus test:** `git grep -nE "Runtime\b|VLLMRuntime|OllamaRuntime|slotState|perSlotProxy|slotURL"` in the new `api/pkg/runner/` returns nothing. If any of those internal abstractions appear in the new code, the implementer regressed toward the old design and the rewrite framing has been violated — push back in review.
- [ ] `frontend/` builds clean; no unused imports flagged by tsc/eslint.

## Sandbox Absorbs Runner (NEW — 2026-04-28 pivot)

- [x] Rename `api/pkg/runnerrouter/` → `api/pkg/inferencerouter/`.
- [x] Extend `types.SandboxInstance` (and `SandboxHeartbeatRequest`) with: `GPUs []GPUStatus`, `ActiveProfileID string`, `ProfileStatus string`, `ProfileError string`, `ServiceHealth map[string]string`.
- [x] Wire `inferencerouter.Router.SetRunnerState` from sandbox heartbeat (`refreshInferenceRouterFromHeartbeat` in `runner_assignment_handlers.go`): looks up assignment, fetches active profile, pushes RunnerState carrying GPUs + URL + active profile + status.
- [x] Write `api/cmd/compose-manager/main.go` — reconciliation loop polling `/api/v1/runners/{id}/assignment`, applies via `composemgr.Manager.Apply()`, periodic prune ticker.
- [x] Write `api/pkg/composemgr/` library — Apply / Clear / Trim / RegistryRewrite. 9 unit tests.
- [x] Write `api/cmd/inference-proxy/main.go` — HTTP server reading active.yaml, reloads on SIGHUP + 30s mtime poll.
- [x] Write `api/pkg/inferenceproxy/` library — model-name-aware reverse proxy. 6 unit tests.
- [x] Extend `Dockerfile.sandbox`: two new builder stages + COPYs + cont-init.d hooks (`80-start-compose-manager`, `85-start-inference-proxy`).
- [x] Delete `Dockerfile.runner`, `Dockerfile.runner.dockerignore`, `docker-compose.runner.yaml`.
- [x] Delete `charts/helix-runner/` Helm chart entirely.
- [~] Frontend: added `RunnerProfilesTable` + `EditRunnerProfile` + sidebar entry. **Not yet done:** extending `AgentSandboxes` table to show profile/service columns; deleting `RunnerSummary` + scheduling visualisations.
- [x] No `x-helix` compose extension — GPU sharing between inference and Hydra is implicit.

## RunPod-Backed Integration Test System (Planned, Separate PR)

See Decision 14 in design.md for the full plan. This task block is the
work breakdown for the follow-up PRs.

### Phase 1 — harness scaffolding (deferred to its own PR)

- [ ] `integration-test/runpod/matrix.yaml`: form factor × profile × scenarios. Initial entries: RTX 4090 (dev), 1×H100 SXM, 4×H100 SXM, 1×A100 80GB, 1×MI300X. Skip Blackwell until RunPod offers it.
- [ ] `integration-test/runpod/cmd/runpod-it/main.go`: harness binary. Reads matrix, loops over entries, dispatches each to the provisioner.
- [ ] `integration-test/runpod/internal/provision/`: thin wrapper around RunPod's REST API (POST `/v2/pod`, GET `/v2/pod/{id}`, POST `/v2/pod/{id}/stop`). Auth via `RUNPOD_API_KEY` env var. Templates a cloud-init that installs Docker + nvidia-container-toolkit + pulls the helix-sandbox image + boots it pointing at the test API server.
- [ ] `integration-test/runpod/internal/scenarios/`: the seven test scenarios (Boot smoke / Compatibility filter / Assignment / Inference / Profile switch / Clear / Incompatible rejection). Each takes a sandbox URL + auth and a profile, returns pass/fail with a structured reason.
- [ ] `integration-test/runpod/internal/report/`: writes per-form-factor results to JUnit XML for CI consumption + a markdown summary suitable for posting to a PR comment.

### Phase 2 — cost controls

- [ ] Wall-clock kill: 30-minute soft, 35-minute hard via RunPod API.
- [ ] Result cache: hash (sandbox image digest, profile YAML SHA, test code git SHA), keyed in S3 or a small Postgres table. Skip with "green-by-cache" if all three match a prior green.
- [ ] Parallelism cap: configurable max concurrent pods (default 4) so we don't blow through RunPod account limits or the daily $ budget.
- [ ] Daily $ budget: reads `MAX_DAILY_RUNPOD_USD`, queries RunPod's billing API at start, refuses to schedule if today's spend already exceeds. Alerts on approach to limit.

### Phase 3 — CI integration

- [ ] `.drone.yml`: new `runpod-integration` pipeline, scheduled nightly on `main`, triggerable on demand via `[runpod-it]` commit-message tag (not every PR — too expensive).
- [ ] Drone secrets: `RUNPOD_API_KEY`, `RUNPOD_NETWORK_VOLUME_ID` (for the persistent /models cache).
- [ ] Slack/PR notifications on failure (one ping per failing form factor, not per test).

### Phase 4 — model cache reuse

- [ ] Persistent RunPod network volume mounted into every pod at `/models`, populated on first run with the model weights for every profile in the matrix. Subsequent runs hit a warm cache → minutes saved per test.

### Out of scope for this work

- ARM64 Grace Hopper / Jetson — covered later when a customer asks.
- Hyperscaler alternatives (AWS spot, Azure NDv5, GCP A3) — RunPod first; switching providers is a separate PR.
- Stress / load testing — the matrix is functional smoke testing, not performance.

## CGO Status (Sandbox Already Free, Nothing To Do)

**2026-04-28 update following the sandbox-absorbs-runner pivot:** the only thing this section ever wanted — the GPU-bearing runtime image being CGO-free — is already true. Sandbox builds all four of its Go binaries (`hydra`, `sandbox-heartbeat`, `compose-manager`, `inference-proxy`) with `CGO_ENABLED=0`. Confirmed:

```
$ grep CGO_ENABLED Dockerfile.sandbox
    CGO_ENABLED=0 go build ... ./api/cmd/hydra
    CGO_ENABLED=0 go build ... ./api/cmd/sandbox-heartbeat
    CGO_ENABLED=0 go build ... ./api/cmd/compose-manager
    CGO_ENABLED=0 go build ... ./api/cmd/inference-proxy
```

CGO still lives in:
- `desktop-bridge` (`Dockerfile.sway-helix`, `Dockerfile.ubuntu-helix`) — xkb / wayland / pipewire. Different image, unrelated work.
- API server `Dockerfile` — Kodit's local ONNX embedder. Unrelated to runners.

- [x] Sandbox image is CGO-free for all four binaries.
- [x] `Dockerfile.runner` deleted (was the only place that needed CGO=1 for Ollama/llama.cpp).
- [x] No follow-up commit needed.

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

- [x] Commit the user's example compose as `design/sample-profiles/8xH100-vllm.yaml` with GPU req: vendor=nvidia, architectures=[hopper], model_match=`^NVIDIA H100`, min_vram=80GB.
- [x] Add `design/sample-profiles/any-nvidia-blackwell-4gpu.yaml` — vendor=nvidia, architectures=[blackwell], no model_match.
- [x] Add `design/sample-profiles/any-nvidia-dev-single-gpu.yaml` — vendor=nvidia, no arch restriction, min_vram=24GB. (Demonstrates the permissive case.)
- [x] Add `design/sample-profiles/dev-spike-tiny.yaml` — single tiny model (e.g. `Qwen2.5-0.5B-Instruct`) on `device_ids: ["0"]` with `--gpu-memory-utilization 0.2`. Sized to coexist with desktop workloads on a shared 16 GB dev GPU. This is the profile the spike uses; it's also the profile any future agent should reach for when validating on similar dev hardware.
- [x] Add `design/sample-profiles/amd-mi300x-vllm.yaml` — vendor=amd, architectures=[cdna3], using `rocm/vllm` images. (Demonstrates the AMD path; even if we have no AMD hardware to test against right now, it documents the intent.)
- [x] Bonus: `design/sample-profiles/README.md` documenting conventions, NVIDIA-vs-AMD declaration syntax, and per-profile hardware/model summary.

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
