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
As a Helix operator, when I assign a profile to a runner, I want the system to reject the assignment if the profile's GPU requirements do not match the runner's hardware (more GPUs requested than present, wrong GPU model), so I get a useful error instead of a half-started compose stack.

## Acceptance Criteria

### AC1: Profile CRUD
- [ ] Profiles are stored in the database with: name, description, compose YAML, declared GPU requirements (count, min VRAM per GPU, optional GPU model regex), set of model names exposed.
- [ ] Admin UI supports create/edit/delete via a new "Runner Profiles" tab in the dashboard.
- [ ] On save, the YAML is parsed and the model list + GPU requirement is extracted automatically and stored alongside the raw YAML.

### AC2: Runner Hardware Reporting
- [ ] On connect, runner reports: hostname, GPU count, per-GPU model name, per-GPU total VRAM, driver version. (This already exists in `TypesGPUStatus` — keep it.)

### AC3: Profile Assignment
- [ ] In the admin UI, each connected runner shows a "Profile" dropdown containing only profiles whose GPU requirements are satisfied by the runner's hardware.
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

### AC8: Migration
- [ ] All existing scheduler code (`api/pkg/scheduler/`), per-slot runtime code (`vllm_runtime.go`, `ollama_runtime.go`, slot CRUD handlers), and slot data model are removed in the same change set. We are not maintaining both systems.
- [ ] Helix-managed model registry (`HelixModelsTable`) becomes informational-only (lists models known to be available across profiles); model loading is no longer driven from it.

## Out of Scope

- **Auto-selecting a profile based on hardware.** v1 requires the operator to pick. A future "best fit" heuristic can come later.
- **Per-request load balancing across containers within a runner.** Each model has one container per profile. Operators wanting more throughput can add replicas to the compose file (treated as opaque).
- **Hot-reload of a single service in the compose stack.** Profile changes restart the whole stack.
- **GPU resource arbitration between models.** The compose author is responsible for setting `gpu-memory-utilization` and `device_ids` correctly. We do not bin-pack.
- **Fine-tuning workloads (Axolotl).** Out of scope for this change; if needed later, add to compose.
- **Image generation (Diffusers).** Same — declare in compose if needed.
- **Backwards compatibility with the old slot API.** Clients using `/api/v1/slots/...` directly will break. The user-facing OpenAI-compatible endpoint (`/v1/chat/completions`) is unchanged.
