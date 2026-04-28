# Sandbox absorbs runner: compose-based inference, scheduler deleted, RunPod test harness scaffolded

Replaces the entire runner / scheduler / slot infrastructure with the
sandbox-absorbs-runner architecture. Sandbox bundles two new binaries
(`compose-manager`, `inference-proxy`) that apply operator-defined
Docker Compose profiles and serve OpenAI-compatible inference. The
legacy runner image, scheduler package, custom runtimes, slot CRUD, and
GGUF memory estimation are all deleted in this same PR per the
"do it all in one PR" mandate.

End-to-end inference is **validated on a real GPU** (RTX 2000 Ada): a
chat completion through `POST /v1/chat/completions {provider:helix}`
returns a valid OpenAI-shape response via the new path with no scheduler
involvement.

## What changed

**~31,000 lines deleted, ~140 + the new code added.**

### Backend additions
- `api/pkg/types/runner_profile.go` — new GORM types for compose-based profiles + assignments.
- `api/pkg/runner/composeparse/` — extracts model list + GPU count from compose YAML (NVIDIA + AMD declaration styles, all 5 port forms).
- `api/pkg/runner/gpuarch/` — NVIDIA compute-cap + AMD `gfx*` → canonical architecture string.
- `api/pkg/runner/profile/` — parse-on-save service + Compatibility check + FilterCompatible helper.
- `api/pkg/inferencerouter/` — replaces the scheduler's request-routing role. Round-robin per model, NoRunnerError carries available-models list.
- `api/pkg/composemgr/` — applies an assigned profile via `docker compose pull && up -d`. Registry mirror, offline mode, image-prune-on-low-water-mark.
- `api/pkg/inferenceproxy/` — body-aware reverse proxy mapping model names to inner-dockerd containers.
- `api/pkg/gpudetect/` — nvidia-smi / rocm-smi probe wired into sandbox-heartbeat.
- `api/cmd/compose-manager/`, `api/cmd/inference-proxy/` — sandbox-side process binaries.
- New HTTP routes: profile CRUD ×5, compatible-profiles, assign/clear, /v1/models.

### Backend deletions
- `api/pkg/scheduler/` (entire package — bin-pack-meets-tensor-parallel, slot lifecycle, prewarming, eviction, queue, workload, decisions; ~2000 lines + 16 test files).
- `api/pkg/runner/{vllm,ollama,axolotl,diffusers}_runtime.go` and the related per-slot supervision (commander, process_monitor, slot, controller, controller_nats, openai_chat_handlers, openai_embedding_handlers, openai_image_handlers, openai_finetuning_handlers, helix_finetuning_handlers, helix_image_handlers, openai_model_handlers, utils, files, server, gpu, gpu_memory_tracker, ollama_model_controller, axolotl_client, diffusers_client, stub_windows).
- Memory estimation (`memory_estimation_handlers.go` ×2 + the controller service + `types/memory.go`).
- Slot CRUD + `RunnerSlot` type + the `runner_slots` table.
- `runner-cmd/helix-runner/` (the standalone runner binary entrypoint).
- `Dockerfile.runner`, `Dockerfile.runner.dockerignore`, `docker-compose.runner.yaml`.
- `charts/helix-runner/` (entire helm chart).
- `github.com/ollama/ollama` dropped from `go.mod`.
- Type deletions covering: `RunnerSlot`, `RunnerModelStatus`, `CreateRunnerSlot*`, `DesiredRunnerSlot*`, `RunnerWorkload`, `RunnerActualSlot*`, `RunnerAttributes`, `Runner`, `GetRunnersResponse`, `DashboardRunner`, `DashboardData`, `WorkloadSummary`, `GlobalSchedulingDecision`, `GlobalAllocationDecision`, `AllocationPlanView`, `RunnerStateView`, `GPUState`, `SchedulingDecisionType`, `SchedulingDecision`, `GPUMemoryStats`, `GPUMemoryDataPoint`, `SchedulingEvent`, `GPUMemoryReading`, `GPUMemoryStabilizationEvent`.
- Endpoint removals: `/scheduler/heartbeats`, `/slots/{slot_id}`, `/dashboard`, `/logs`, `/logs/{slot_id}`, `/helix-models/memory-estimate(s)`.

### Sandbox image changes
- `Dockerfile.sandbox` extended with two new builder stages (compose-manager, inference-proxy) + COPYs into `/usr/local/bin/` + cont-init.d hooks (`80-start-compose-manager`, `85-start-inference-proxy`) + `/etc/helix` directory.
- Sandbox is **fully CGO-free** for all four binaries (hydra, sandbox-heartbeat, compose-manager, inference-proxy). The CGO-free win lands as a side effect of the runner-image deletion.

### Frontend additions
- `RunnerProfilesTable` + `EditRunnerProfile` + sidebar entry.
- `ProfileGallery` modal with **curated default profiles** (5 hand-tuned starting points with pros/cons cards + GPU-memory-budget bars) and a **"Build from blocks"** composer (chat tiny/7B/35B/72B-tp4, embeddings text/vision, desktop-headroom marker; live YAML preview; over-budget detection).
- HelixModels tab **integrated** with the inference router: each row shows "● Available now" badge if served by some sandbox's active profile, "○ Metadata only" otherwise. Memory column removed.
- `runnerProfilesService.ts` React Query hooks.

### Frontend deletions
- `RunnerSummary`, `ModelInstanceSummary`, `FloatingRunnerState`, `MemoryEstimateCell`, `GlobalSchedulingVisualization`, `SchedulingDecisionsTable`, `SchedulerHealthIndicators`, `dashboardService`, `floatingRunnerState` context.
- 386-line "runners" tab in Dashboard.tsx replaced with a brief stub pointing operators at the new Agent Sandboxes + Runner Profiles tabs.

### Sample profiles (operator templates)
- `8xH100-vllm.yaml` — 5-service production stack (the user's original example, ported to use `/models` mount path).
- `any-nvidia-blackwell-4gpu.yaml` — 4×Blackwell tensor-parallel chat.
- `any-nvidia-dev-single-gpu.yaml` — single-GPU 7B chat.
- `amd-mi300x-vllm.yaml` — AMD device passthrough + ROCm vLLM.
- `dev-spike-tiny.yaml` — Qwen2.5-0.5B at 20% VRAM (the spike profile).
- `README.md` documenting conventions.

### RunPod-backed integration test harness (Decision 14)
Full scaffolding shipped, **awaiting `RUNPOD_API_KEY` for live runs**:
- `integration-test/runpod/matrix.yaml` — 5 form-factor entries enabled (RTX 4090, 1×H100 SXM, 4×H100 SXM, 1×A100, 1×MI300X) + 1 deferred (Blackwell).
- `cmd/runpod-it/main.go` — harness binary with `--dry-run`, `--only`, `--no-cache`, `--parallel`, `--max-daily-usd` flags.
- `internal/provision/` — RunPod REST API client + dry-run stub.
- `internal/scenarios/` — seven scenarios (boot smoke, compatibility filter, assignment+apply, inference roundtrip, profile switch, clear, incompatible rejection).
- `internal/cache/` — green-result cache keyed on (entry-id + profile-yaml-sha + harness-build-sha); 7-day stale cutoff.
- `internal/report/` — JUnit XML for CI + Markdown for PR comments.
- Cost controls: 30/35min wall-clock, parallelism cap, daily $ budget that queries RunPod billing API at start.
- README documenting matrix entries, scenarios, cost controls, and CI integration plan.
- Dry-run verified: 5 enabled entries listed cleanly.

## End-to-end validation on RTX 2000 Ada (real hardware)

Full chain works **post-deletion**:
```
POST /v1/chat/completions {model:qwen2.5-0.5b, provider:helix}
  → API server (no scheduler)
  → inferencerouter.PickRunner
  → dispatchHTTPToRunner
  → sandbox inference-proxy (port 8090)
  → 127.0.0.1:8000 (vllm-tiny in inner dockerd)
  → vLLM
  → "Yes, it continues to function."  ← real response from running stack
```

Sandbox heartbeats correctly report:
- GPU vendor + arch + ComputeCapability (`vendor: nvidia, architecture: ada, compute_capability: 8.9`)
- Profile status: `running`
- Service health: `vllm-tiny: healthy`

Compatibility check verified locally:
- `dev-spike-tiny` profile (any NVIDIA arch, ≥4 GiB) — accepted.
- `hopper-only` profile (architectures=[hopper]) — rejected with **`422 incompatible: architecture — profile requires one of [hopper], runner GPU 0 is "ada"`**.

## Test plan

```bash
# Build (CGO_ENABLED=0)
go build ./...

# Unit tests for new packages (50+ tests across these)
go test ./api/pkg/runner/... ./api/pkg/inferencerouter/ \
        ./api/pkg/composemgr/ ./api/pkg/inferenceproxy/ ./api/pkg/gpudetect/

# RunPod harness dry-run
go run ./integration-test/runpod/cmd/runpod-it --dry-run
```

Live RunPod runs: blocked on `RUNPOD_API_KEY` provisioning. Once available:
```bash
export RUNPOD_API_KEY=...
export HELIX_API_URL=https://test.helix.example.com
export RUNNER_TOKEN=...
go run ./integration-test/runpod/cmd/runpod-it --only rtx4090
```

## Design references

- Requirements: `helix-specs/design/tasks/001959_we-need-to-replace-all/requirements.md`
- Design: `helix-specs/design/tasks/001959_we-need-to-replace-all/design.md` (Decisions 1–14)
- Tasks: `helix-specs/design/tasks/001959_we-need-to-replace-all/tasks.md` (all items closed or `[~]` with reasoning)
- Sample profiles: `design/sample-profiles/`
- RunPod harness: `integration-test/runpod/`
- Spike result: design.md "Spike Result (2026-04-28)"

## Notes for reviewers

- The `Runtime` enum on `HelixModel` is preserved as a string alias for DB-column backward compat; its scheduler-input role is dead. The HelixModels tab now shows it as informational alongside the new "Available now" badge.
- `provider_manager.SetRunnerController` is now a no-op (interface kept for backwards compatibility); the helix provider is always listed and individual model availability flows through `/v1/models` from the router.
- The `external_agent` model name special case in the openai server is gone — external agents go through `externalAgentExecutor` directly, never the inference path. Verified with `git grep "external_agent"` (no live use of the inference dispatch path).
- Frontend uses raw axios for the new endpoints with a clear TODO; openapi regeneration via `./stack update_openapi` swaps these for typed client methods later. Type-checks and Vite hot-reload are happy.
- HelixModels tab is **integrated, not killed** (per user feedback): the model list is overlaid with the inference router's view of currently-served models. The Memory column is gone but pricing, type, etc. remain.
