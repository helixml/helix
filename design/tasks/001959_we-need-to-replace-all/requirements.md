# Requirements: Replace Runner Infrastructure with Compose Profiles

## Problem Statement

The current runner implementation is a sophisticated dynamic scheduler:

- A central scheduler (`api/pkg/scheduler/scheduler.go`) bin-packs models onto GPUs at request time.
- The runner (`api/pkg/runner/`) spawns vLLM/Ollama subprocesses on demand via custom Go runtimes (`vllm_runtime.go`, `ollama_runtime.go`), each on a random localhost port.
- Slot lifecycle (create/delete) is driven over NATS from the scheduler.
- Per-slot HTTP proxying inside the runner forwards inference to the right local port.

This is a lot of code to maintain for what — in practice — operators want to express as: *"on this 8×H100 box, run these five models from this Docker Compose file."*

We are replacing it with a profile-driven Compose runner. Each runner runs a `docker compose up` inside Docker-in-Docker (the same DinD pattern Sandbox uses for desktop environments). Models are declared statically in a `docker-compose.yaml` that the operator writes for each GPU "form factor" (e.g. `8xH100.yaml`, `4xA100.yaml`, `2xL40S.yaml`). An operator picks which profile a connected runner should run; the runner pulls images and starts containers. A reverse proxy inside the runner routes by model name to the right container's OpenAI-compatible port.

## User Stories

### US1: Operator Defines Compose Profiles
As a Helix operator, I want to define named "runner profiles" — each one a Docker Compose file describing which models to run on which GPUs — so that I can encode the model layout for each server form factor I operate.

### US2: Operator Assigns a Profile to a Runner
As a Helix operator, when a new runner connects and reports its hardware (GPU count, GPU model, total VRAM), I want to pick a compatible profile from a dropdown of profiles whose GPU requirements fit, and have the runner start that profile, so I can bring capacity online without writing custom config per box.

### US3: Operator Switches Profiles
As a Helix operator, I want to switch a runner from one profile to another (e.g. swap the 35B model for two smaller models), and have the runner cleanly stop the old compose stack and start the new one, so I can re-tune capacity without SSHing to the box.

### US4: Operator Sees What's Running
As a Helix operator, I want the admin dashboard to show, per runner: the active profile, each model in that profile, each model's container health, and per-GPU memory/utilization, so I can confirm the rollout is healthy.

### US5: Inference Requests Route to the Right Model
As a developer using the Helix API, when I send a chat completion for `qwen3.5-35b`, I want the API server to route it to a runner whose active profile includes `qwen3.5-35b`, and to the right container inside that runner, so that the experience is identical to today.

### US6: Profiles are Validated Before Assignment
As a Helix operator, when I assign a profile to a runner, I want the system to reject the assignment if the profile's GPU requirements do not match the runner's hardware, so I get a useful error instead of a half-started compose stack. "Match" must be expressive enough to cover:
- **Vendor** (NVIDIA vs AMD) — mandatory because compose images target one or the other (`vllm/vllm-openai` is CUDA, `rocm/vllm` is ROCm). A profile must never be assigned across vendors.
- **Architecture family** (Blackwell, Hopper, Ampere, Ada Lovelace; CDNA3, RDNA3) — required when the compose images contain kernels compiled for a specific arch (e.g. FP8 paths for Hopper/Blackwell, Flash Attention 3 for Hopper+).
- **Specific GPU model** (e.g. only H100 80GB) — required when memory budgets or kernel choices in the compose file are tight.
- **"Any NVIDIA" / "Any AMD"** — permissive profiles that should match any GPU of the right vendor, useful for small/dev workloads.

## Acceptance Criteria

### AC1: Profile CRUD
- [ ] Profiles are stored in the database with: name, description, compose YAML, set of model names exposed, **and an operator-declared GPU compatibility specification** (see AC1a).
- [ ] Admin UI supports create/edit/delete via a new "Runner Profiles" tab in the dashboard.
- [ ] On save, the YAML is parsed and the model list + the count of GPUs referenced is extracted automatically. Vendor / architecture / model-match rules are entered separately by the operator (they cannot be inferred from a compose file alone).

### AC1a: GPU Compatibility Specification
A profile's GPU compatibility has these fields, all optional except `count`:
- [ ] `count` (int, required) — number of GPUs the compose file expects to use, derived from the union of `device_ids` across all services.
- [ ] `vendor` (`nvidia` | `amd` | unset) — when set, every GPU on the runner must match.
- [ ] `architectures` (list of strings, e.g. `["hopper", "blackwell"]`) — when non-empty, every referenced GPU's architecture must be in the list. Empty list = any architecture of the chosen vendor.
- [ ] `model_match` (regex, optional) — when set, every referenced GPU's marketing name must match (e.g. `^NVIDIA H100`).
- [ ] `min_vram_bytes` (int, optional) — when set, every referenced GPU's total VRAM must be ≥ this.

A profile satisfying *only* `vendor: nvidia` is the "any NVIDIA" case. A profile setting `vendor`, `architectures`, *and* `model_match` is the "tight Blackwell-only" case. The four fields compose freely.

### AC2: Runner Hardware Reporting
- [ ] On connect, runner reports per GPU: index, marketing model name, total VRAM, driver version (already in `TypesGPUStatus`).
- [ ] **New fields:** vendor (`nvidia` | `amd`), architecture (canonical string, e.g. `hopper`, `blackwell`, `ampere`, `ada`, `cdna3`, `rdna3`), and — for NVIDIA — compute capability (e.g. `9.0`).
- [ ] Vendor/architecture/compute-capability are derived on the runner: NVIDIA via `nvidia-smi --query-gpu=name,compute_cap`; AMD via `rocm-smi`. A small lookup table in the runner maps compute capability → architecture canonical string so the API server doesn't have to know the mapping.

### AC3: Profile Assignment
- [ ] In the admin UI, each connected runner shows a "Profile" dropdown containing **only profiles whose GPU compatibility specification (AC1a) is satisfied by every GPU referenced in the profile's compose YAML against the runner's reported hardware**. Filtering happens server-side via `GET /api/v1/runners/{id}/compatible-profiles` so the UI cannot accidentally show an incompatible option.
- [ ] Profiles excluded from a given runner's dropdown can still be inspected from the global Profiles tab — but the assignment endpoint will reject the same profile with a named-constraint error if a client tries to bypass the filter (defence in depth).
- [ ] Setting the profile triggers the runner to: stop the previous compose stack (if any), pull images, and `docker compose up -d` the new one.
- [ ] Runner reports the assignment state: `assigning`, `pulling`, `starting`, `running`, `failed` with error message.

### AC4: Compose Runs in Docker-in-Docker
- [ ] The runner container runs an inner dockerd (same DinD pattern as Sandbox uses today via `Dockerfile.sandbox`).
- [ ] All compose services run inside that inner dockerd; the host docker daemon is not touched.
- [ ] GPUs are passed through to the inner dockerd so `device_ids: ["0"]` etc. works as in the user's example.

### AC5: Reverse Proxy Routes by Model Name
- [ ] Runner exposes a single HTTP endpoint (e.g. `POST /v1/chat/completions`, `POST /v1/embeddings`) to the API server.
- [ ] Internally the runner inspects the `model` field in the request body and forwards to the matching container's port (resolved from the active profile's compose YAML).
- [ ] If the requested model is not in the active profile, return HTTP 404 with a clear error.

### AC6: API Server Routing
- [ ] When the API server receives an inference request for model `M`, it picks a runner whose active profile includes `M` and is in `running` state.
- [ ] If multiple runners qualify, distribute requests (round-robin is acceptable for v1).
- [ ] If no runner qualifies, return HTTP 503 with a clear error listing which models are currently available.

### AC7: Admin Dashboard
- [ ] Existing runner state UI (`RunnerSummary`, `FloatingRunnerState`, `ModelInstanceSummary`) is retained but its data model swaps from "slots" to "compose services".
- [ ] Each runner card shows: active profile name, list of services in the profile, health status per service, per-GPU memory chart (already exists, keep).
- [ ] Logs for each service are viewable (tail of `docker compose logs <service>` in the runner's inner dockerd).

### AC8: Migration & Exhaustive Dead-Code Removal
We are not leaving skeletons. Everything below is gone in the same change set. A reviewer should be able to `git grep` for any of these names and find no live references.

**Backend Go code to delete in full:**
- [ ] `api/pkg/scheduler/` — entire package (~28 files including `scheduler.go`, `global_allocator.go`, `slot.go`, `slot_store.go`, `cache.go`, `queue.go`, `workload.go`, `runner.go`, `model_allocation.go`, `decisions.go`, `errors.go`, `util.go`, `test_helpers.go`, and all `*_test.go`). This includes the GPU bin-packing logic, the multi-GPU eviction logic, the prewarming cache, the workload queue, and **specifically all the code that tries to bin-pack while also handling tensor parallelism** (which was the source of the `multi_gpu_eviction_test.go` and `memory_calculation_inconsistency_test.go` complexity).
- [ ] `api/pkg/runner/vllm_runtime.go`, `ollama_runtime.go`, `axolotl_runtime.go`, `diffusers_runtime.go` — every custom runtime. All model serving moves to stock images (`vllm/vllm-openai`, `rocm/vllm`, etc.) declared in compose.
- [ ] `api/pkg/runner/ollama_model_controller.go` — Ollama model pulling/caching helpers.
- [ ] `api/pkg/runner/memory_estimation_handlers.go` — **delete in full**. This is the file that imports `github.com/ollama/ollama/{api,discover,fs/ggml,llm}` and goes to great lengths to estimate Ollama model memory by parsing GGUF files. With compose, the operator declares GPU memory budget via `--gpu-memory-utilization` directly; we do no estimation.
- [ ] `api/pkg/runner/gpu_memory_tracker.go` — per-slot GPU memory accounting.
- [ ] `api/pkg/runner/gpu.go` — only the slot-allocation parts; keep whatever is needed to *report* GPU inventory to the API server (vendor / arch / VRAM) for AC2.
- [ ] `api/pkg/runner/process_monitor.go`, `commander.go`, `commander_mocks.go` — process spawning DSL used only by the deleted runtimes.
- [ ] `api/pkg/runner/openai_finetuning_handlers.go`, `helix_finetuning_handlers.go`, `helix_image_handlers.go` — slot-based fine-tuning + Helix-native image endpoints. (OpenAI-compatible image generation goes through the proxy like chat/embeddings.)
- [ ] `api/pkg/runner/slot.go` and slot CRUD route registrations in `api/pkg/runner/server.go`.
- [ ] `api/pkg/server/memory_estimation_handlers.go` — the API-server-side counterpart.
- [ ] `api/pkg/server/handlers.go` — `deleteSlot()` handler and `getSchedulerHeartbeats()` handler.
- [ ] `api/pkg/controller/handlers.go` — `DeleteSlotFromScheduler()`, `RunnerSlots()`, and any other slot-listing methods.
- [ ] `api/pkg/store/store_slots.go` — entire file (CreateSlot, GetSlot, UpdateSlot, DeleteSlot, ListSlots, ListAllSlots) and the slot methods on the `Store` interface.
- [ ] `api/pkg/openai/helix_openai_server.go` — if this exists only to bridge the scheduler. Inspect; keep only what `helix_openai_client` legitimately needs.

**Types to delete:**
- [ ] `api/pkg/types/runner.go`: `RunnerSlot`, `CreateRunnerSlotRequest`, `CreateRunnerSlotAttributes`, `ListRunnerSlotsResponse`, `RunnerModelStatus`, the `Runtime` enum (no longer meaningful — the container image declares the runtime).
- [ ] `api/pkg/types/types.go`: `SchedulingDecisionType`, `SchedulingDecision`, `GlobalSchedulingDecision`, `GlobalAllocationDecision`, `AllocationPlanView`, `GPUMemoryStats`.
- [ ] `api/pkg/types/memory.go` — Ollama memory-estimation types.
- [ ] `RunnerStatus.AllocatedMemory`, `RunnerStatus.Models`, `RunnerStatus.GPUMemoryStats` fields (the new equivalent is "active profile + per-service health").
- [ ] `HelixModel.Prewarm` field — prewarming is gone.

**Database:**
- [ ] Drop the `runner_slots` table via an explicit migration (don't rely on GORM's autoMigrate ignoring orphaned tables).
- [ ] Same for any scheduling-decision/allocation-history tables if they exist.

**Frontend dead code:**
- [ ] `frontend/src/components/dashboard/GlobalSchedulingVisualization.tsx`, `SchedulingDecisionsTable.tsx`, `SchedulerHealthIndicators.tsx` — all visualisations of scheduler decisions.
- [ ] Any React Query hooks targeting deleted endpoints (`useDeleteSlot`, slot list queries, `v1SchedulerHeartbeatsList`, `v1MemoryEstimationsList`).
- [ ] `MemoryEstimateCell` in `HelixModelsTable.tsx` and any helpers behind it.
- [ ] Generated types for deleted endpoints in `frontend/src/api/api.ts` (auto-removed by `update_openapi`, but spot-check).

**Docker / CI / Charts:**
- [ ] Strip `Dockerfile.runner` to: golang build + dockerd + docker CLI + nvidia-container-toolkit. Remove vLLM CUDA venv setup, vLLM ROCm venv setup, Ollama binary install, Axolotl fake venv, Diffusers, the model preload cache, all related Python and CUDA layer installs.
- [ ] Remove `docker-compose.runner.yaml` if it's purely for the standalone runner.
- [ ] Strip `charts/helix-runner/` of vLLM/Ollama/Axolotl env vars, model-preload values, scheduling-strategy values. Confirm the chart still produces a working pod with the new minimal image.

**Config / env vars to remove from `api/pkg/config/config.go` and `.env.example`:**
- [ ] `HELIX_MODEL_TTL`, `HELIX_SLOT_TTL`, `HELIX_SCHEDULING_STRATEGY`, `HELIX_QUEUE_SIZE`, and any other scheduler-tuning knobs.

**CLI wiring:**
- [ ] `api/cmd/helix/serve.go`: remove `NewScheduler()` call site and the `PrewarmNewRunner` callback wiring.

**Docs:**
- [ ] Any `helix/design/` docs explaining scheduler semantics, slot allocation, prewarming, multi-GPU eviction, GGUF memory estimation. Either delete or replace with a redirect note pointing at the new design.
- [ ] Update `docs/` operator-facing pages.
- [ ] `charts/helix-runner/README.md`.

**Verification (must pass before merge):**
- [ ] `go build ./...` clean.
- [ ] `go vet ./...` clean.
- [ ] `git grep -nE "scheduler\.|Scheduler\b|RunnerSlot\b|GGUF|memory_estimation|ollama/ollama|axolotl|diffusers_runtime|SchedulingDecision|GlobalAllocationDecision|Prewarm"` returns *only* legitimate hits (e.g. release-notes mentioning what was removed). No live references.

### AC10: CGO-Free Build (Investigate; Adopt If Possible)
The current main `Dockerfile` builds the API server with `CGO_ENABLED=1 -tags ORT` because of `github.com/yalue/onnxruntime_go` (embedding fallback) and the runner uses `CGO_ENABLED=1` because of the `github.com/ollama/ollama/{api,discover,fs/ggml,llm}` imports in the deleted memory-estimation code.

After AC8's deletions:
- [ ] Audit remaining CGO requirements: `git grep -E '^import \"C\"'` in `api/` (should now find only `pkg/desktop/*` which builds separate binaries) and `git grep -nE 'CGO_ENABLED=1'` in `Dockerfile*`.
- [ ] If nothing in the API or runner build paths still requires CGO, switch `Dockerfile` and `Dockerfile.runner` to `CGO_ENABLED=0` and drop the `-tags ORT` build tag. Pure-Go builds give us: smaller images, faster CI, no glibc/musl version coupling, simpler cross-compilation.
- [ ] If something *does* still require CGO (e.g. an indirect dependency from `provider_manager` or rate limiter), document what and why in `design/2026-MM-DD-cgo-after-runner-rewrite.md` and leave CGO on. Don't ship a half-disabled state.
- [ ] The desktop / sandbox binaries (`desktop-bridge`, etc.) keep `CGO_ENABLED=1` — they need xkb/wayland/pipewire bindings. This AC is about the API server and the runner only.

### AC9: Caller-Facing API Surface is Preserved
The internal switch from scheduler-to-runners to router-to-runners must be invisible to every existing caller. Specifically:

- [ ] **Internal Go OpenAI client (`api/pkg/openai/helix_openai_client.go`)** — its public method signatures (`CreateChatCompletion`, `CreateChatCompletionStream`, `CreateEmbeddings`, etc., implementing the `go-openai` interface) remain unchanged. Internally it stops calling `scheduler.Enqueue` and starts calling the new `Router`. Every existing in-tree consumer of this client keeps working without modification:
  - `api/pkg/server/openai_chat_handlers.go` (session/agent inference)
  - `api/pkg/server/openai_embeddings_handlers.go`
  - `api/pkg/server/openai_model_handlers.go`
  - `api/pkg/server/summary_service.go` (auto-titling, summarisation)
  - `api/pkg/server/provider_handlers.go`
  - `api/pkg/trigger/cron/` (scheduled inference)
  - `api/pkg/openai/manager/provider_manager.go` (the "helix" provider entry)
- [ ] **External OpenAI-compatible HTTP endpoints** continue to work with no client-side changes:
  - `POST /v1/chat/completions` (incl. streaming)
  - `POST /v1/embeddings`
  - `POST /v1/images/generations` (if any profile exposes an image model)
  - `GET  /v1/models` — returns the union of model names across all currently-`running` profiles
- [ ] **Sessions endpoint and the full chat-session flow keep working end-to-end.** A user creating a session via `POST /api/v1/sessions`, sending messages, and receiving streamed responses must see no behavioural difference (other than the obvious: only models in some currently-assigned profile are available; previously the scheduler could load any registered model on demand).
- [ ] **Provider selection is unchanged.** `provider_manager.go` continues to route requests for the `"helix"` provider to the in-process Helix client; other providers (OpenAI, Anthropic, Google, custom OpenAI-compatible endpoints) are untouched by this change.
- [ ] **Error semantics for unavailable models change in one specific way:** previously, requesting an unloaded but registered model would queue and eventually load it. Now, requesting a model not in any currently-`running` profile returns HTTP 503 immediately with a list of currently-available models. This is the only intentional caller-visible behaviour change, and is documented in release notes.

## Out of Scope

- **Auto-*selecting* a profile and applying it without operator action.** v1 requires the operator to click. The dropdown they pick from is *already filtered* to only profiles that fit the runner's hardware (see AC3 and AC6) — what's out of scope is the system deciding for them which of the eligible profiles to apply. A future "best fit" heuristic that pre-selects an eligible profile (e.g. the one that exposes the most models, or the one most recently used on similar hardware) can come later.
- **Per-request load balancing across containers within a runner.** Each model has one container per profile. Operators wanting more throughput can add replicas to the compose file (treated as opaque).
- **Hot-reload of a single service in the compose stack.** Profile changes restart the whole stack.
- **GPU resource arbitration between models.** The compose author is responsible for setting `gpu-memory-utilization` and `device_ids` correctly. We do not bin-pack.
- **Fine-tuning workloads (Axolotl).** Out of scope for this change; if needed later, add to compose.
- **Image generation (Diffusers).** Same — declare in compose if needed.
- **Backwards compatibility with the old slot API.** Clients using `/api/v1/slots/...` directly (slot create/list/delete, per-slot proxy paths) will break — this was an internal API between the scheduler and the runner and is gone. *All caller-facing surface — the OpenAI-compatible endpoints, sessions, the internal Helix OpenAI Go client — is preserved; see AC9.*
