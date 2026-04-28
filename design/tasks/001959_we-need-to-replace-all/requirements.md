# Requirements: Sandbox Grows Inference (deletes the runner image)

> **2026-04-28 architectural pivot:** the user observed that the new compose-based runner and the existing Sandbox container are converging structurally — both are DinD wrappers around GPU containers. The cleanest framing: **Sandbox just grows the ability to run LLMs and we wire in routing to them.** The runner image (`Dockerfile.runner`) is deleted entirely. Sandbox absorbs the role: two new Go binaries (`compose-manager`, `inference-proxy`) ship inside the existing `Dockerfile.sandbox`. Hydra and Sandbox's other features stay as-is. The API server's inference router picks a sandbox by model name. This is a smaller, lower-risk change than inventing a new "worker" image — Sandbox is already production-mature.



## Problem Statement (Unified Worker)

Helix today ships two GPU-bearing container artifacts that have evolved
into very similar shapes:

- **Sandbox** (`Dockerfile.sandbox`): inner dockerd + Hydra. Hosts dynamic
  agent desktop containers (Wolf, helix-sway). Operationally: pull images
  on startup from a registry, manage per-scope dockerds for isolation.
- **Runner** (`Dockerfile.runner`, current): custom Go binary that spawns
  vLLM/Ollama subprocesses. Operationally: scheduler over NATS decides
  what runs where; bin-packs GPUs; predicts memory.

The runner replacement we set out to design — operator-defined Docker
Compose profiles running inside DinD — is structurally a thin variation
on what Sandbox already does. Continuing to ship two artifacts is paying
twice for: image build pipelines, helm charts, registry credential
plumbing, GPU passthrough setup, multi-arch build matrices, security
review, ops monitoring. And it forecloses the option of mixing inference
and agent workloads on the same hardware, which is real money on the
table for operators with bursty inference loads.

We therefore unify into a single artifact, the **Helix worker**:

- One Docker image that bundles the inner dockerd, Hydra, and the
  compose-profile manager.
- One deployable unit (helm chart, compose service).
- Two coexisting subsystems sharing the same node hardware:
  - The **inference router** routes LLM requests by model name to
    workers whose active profile exposes that model.
  - **Hydra** routes agent session requests to workers and spawns
    per-scope desktop containers in the same inner dockerd that hosts
    inference services.
- Operators pick per worker (per profile) what mix runs there: pure
  inference node, pure agent node, or mixed (e.g. inference on GPUs 0–5,
  agent desktops on GPUs 6–7).

The deletion mandate (AC8) and rewrite framing (Decision 11 in
design.md) still apply to the runner-side code; the Sandbox-side code
stays largely intact and is integrated into the worker rather than
replaced.

### Original problem statement (scheduler complexity)

The runner replacement still removes the sophisticated dynamic scheduler:

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
- [ ] **Both NVIDIA and AMD GPU passthrough work** (see AC12). NVIDIA via nvidia-container-toolkit and `deploy.resources.reservations.devices` (the user's example syntax). AMD via device passthrough — `/dev/kfd` and `/dev/dri` mounted in, `group_add: [video, render]`. The compose parser handles either declaration style; the inner dockerd has both runtimes registered if the host supports them.

### AC12: Dual GPU Vendor + Multi-Arch
- [ ] The runner image is **a single multi-arch manifest covering `linux/amd64` and `linux/arm64`**. Reasons: NVIDIA ships on both (Jetson, Grace Hopper), Apple Silicon dev machines need arm64 to run the runner without a GPU profile, deploys shouldn't need to know what they're pulling.
- [ ] The runner image installs **both** `nvidia-container-toolkit` and AMD container runtime support side-by-side (`amd-container-toolkit` where packaged, manual `/dev/kfd` + `/dev/dri` + `group_add` configuration where not). Vendor is implicit at profile-assignment time, not at runner-image-build time. One image, either vendor.
- [ ] On arm64 the AMD toolkit install is skipped (ROCm is x86-only in practice) with a clear log: "AMD runtime omitted on arm64; arm64 runners cannot host AMD GPU profiles." This is logged once at runner startup, not on every profile assignment.
- [ ] The compose parser (`composeparse/parse.go`) accepts both NVIDIA-style (`deploy.resources.reservations.devices`) and AMD-style (`devices: [/dev/kfd, /dev/dri/...]` + `group_add`) GPU declarations. Mixing both styles in one service is rejected with a clear error.
- [ ] Pre-flight: when applying a profile, the runner verifies the inner dockerd has the required runtime registered for the profile's vendor; otherwise fails fast with a clear message ("profile requires the nvidia runtime but it is not registered on this runner") rather than producing an opaque `docker compose up` error.

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

### AC8: Migration & Exhaustive Dead-Code Removal (Rewrite, Not Refactor)
The runner Go code is **rewritten from scratch** in the same change set as the deletions, not evolved in place — see Decision 11 in `design.md` for the rationale (roughly 6% of today's runner code survives; the rest is genuinely new; threading changes through old files leaks the old design's shape into the new responsibilities).

A handful of utilities are copied forward as plain new files in the rewritten package: NATS connection plumbing, HTTP server scaffolding, GPU detection for vendor/arch reporting (slimmed), the `RunnerStatus` type minus dead fields, and `runner-cmd/helix-runner/main.go`'s flag parsing / log setup / signal handling. Everything else under `api/pkg/runner/` is deleted and replaced.

We are not leaving skeletons. Everything below is gone in the same change set. A reviewer should be able to `git grep` for any of these names and find no live references — *and* a reviewer should be able to `git grep` the *new* `api/pkg/runner/` for old internal abstractions (`Runtime`, `VLLMRuntime`, `OllamaRuntime`, `slotState`, `perSlotProxy`) and find none. If they appear, the rewrite framing was violated.

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

### AC11: Persistent Caching & Offline / Air-Gapped Operation
The runner must be usable in fully air-gapped deployments and must not re-download multi-GB model weights or container images on every restart. We mirror the Sandbox patterns.

- [ ] **Two named volumes** are mounted into the runner container by default:
  - `helix-runner-docker-storage` → `/var/lib/docker` (inner dockerd state, image layers, build cache).
  - `helix-runner-models` → `/models` (HuggingFace / model-weight cache shared across all containers in any profile).
  - Both volumes survive runner container restart *and* runner image upgrade.
- [ ] **Convention:** profile compose files mount their model cache from `/models` (e.g. `volumes: - /models:/root/.cache/huggingface`), and operators are documented on this in the operator guide. The runner does not perform automatic path substitution.
- [ ] **Three registry modes** are supported:
  - *Default (online):* `docker compose pull && docker compose up -d` against image refs as written in the profile.
  - *Mirror:* setting `HELIX_RUNNER_REGISTRY=mirror.corp.example.com` rewrites the leading registry portion of every `image:` field before pull/up. Same `sed`-style substitution Sandbox already uses (`sandbox/04-start-dockerd.sh` lines 205–235).
  - *Offline:* setting `HELIX_RUNNER_OFFLINE=true` skips the `pull` step entirely; relies on images already present in `/var/lib/docker`. If a referenced image is absent, the profile assignment fails with a message listing which images are missing.
- [ ] The three modes compose: a typical air-gapped deployment sets both `HELIX_RUNNER_REGISTRY` and `HELIX_RUNNER_OFFLINE=true`.
- [ ] **Image cleanup ordering** matches Sandbox: when switching profiles, pull-new → down-old → up-new, *then* (on a separate low-water-mark trigger) prune images that are no longer referenced. Never prune between `down` and `up` — that destroys shared layers and forces a full re-download.
- [ ] **HF_TOKEN passthrough:** the `HUGGING_FACE_HUB_TOKEN` env var on the runner container is forwarded into every compose service that declares it (matching the user's example compose). Operators configure the secret once, on the runner.
- [ ] **Offline + HF_HUB_OFFLINE=1** in the compose env (already in the user's example) gives true offline operation: no registry access, no HuggingFace access. This combination is what we test for AC11 sign-off.

### Non-Goals for AC11
- The runner does **not** download or pre-stage model weights for operators. The `helix-runner-models` volume is populated out-of-band (network filesystem, scp, snapshot restore, or first-online compose up) and operators use whatever tooling they already use.
- The runner does **not** distribute images via tarballs. Sandbox already moved away from this pattern (`design/2026-01-12-sandbox-registry-based-images.md`); operators wanting offline image distribution run a local registry mirror.
- Per-profile cache isolation is not provided. The model and image caches are shared across all profiles on a runner — this is the right default (same weights are useful to multiple profiles); operators wanting isolation mount a subpath.

### AC10: CGO-Free Runner Build (Adopt After Deletions)
**API server keeps CGO=1.** The `-tags ORT` build tag and `github.com/yalue/onnxruntime_go` dependency are required by **Kodit**, not by anything in the runner stack. `api/pkg/server/kodit_init.go:261` (`preflightORT()`) checks for `libonnxruntime.so` at runtime when Kodit's local-ONNX embedding model is in use; this is a hard dependency for Kodit's code-intelligence indexing. We do not touch this.

**Runner flips to CGO=0.** Today `Dockerfile.runner` uses `CGO_ENABLED=1` solely because `memory_estimation_handlers.go` and `ollama_runtime.go` import `github.com/ollama/ollama/{api,discover,fs/ggml,llm}` (which are CGO-heavy via llama.cpp bindings). After AC8's deletions the runner has no CGO drivers left.

- [ ] After the deletions land, run `git grep -E '^import \"C\"' runner-cmd/ api/pkg/runner/` — should return nothing.
- [ ] Flip `Dockerfile.runner` to `CGO_ENABLED=0` and drop the `-tags "!rocm"` tag (it was paired with the runtime split that no longer exists).
- [ ] Confirm the runner image builds clean and a smoke test passes (`ldd /helix-runner` should show no surprising dynamic links — ideally a static binary).
- [ ] If an indirect dep on the runner side *does* still pull CGO (e.g. via `provider_manager` shared with the API server), document the dep in `design/2026-MM-DD-cgo-after-runner-rewrite.md` and leave CGO=1. Don't ship a half-disabled state.
- [ ] **Out of scope:** the API server `Dockerfile` (CGO=1 / `-tags ORT` stays for Kodit), and the desktop/sandbox binaries (`desktop-bridge`, etc., keep CGO=1 for xkb/wayland/pipewire). This AC is about the runner image only.

The win is narrower than initially scoped (just the runner, not the API server), but still real: smaller runner image, faster runner CI, simpler runner cross-compilation, no glibc/musl coupling on the runner side.

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
