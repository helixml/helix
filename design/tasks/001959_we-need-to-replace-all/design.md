# Design: Unified Helix Worker (compose-profile inference + Hydra agent desktops)

> **2026-04-28 architectural pivot:** the original draft of this design described a "compose-profile runner" replacing the existing runner while leaving Sandbox alone. After implementing the foundation layer we observed that the new runner and Sandbox are structurally the same thing — both are DinD wrappers around GPU containers — and decided to unify them into a single deployable artifact. This document captures the unified design. Decisions 12 and 13 explain the unification specifically; the rest of the doc reads naturally with "worker" = the unified artifact.


## Architecture Overview

```
┌─ API Server ──────────────────────────────────────────────────┐
│                                                                │
│  /v1/chat/completions ──► RunnerRouter (picks runner where    │
│                            active_profile.models contains M)  │
│                                │                               │
│                                ▼                               │
│  ProfileService (CRUD)    NATS: runner.{id}.cmd               │
└────────────────────────────────│───────────────────────────────┘
                                 │
       ┌─────────────────────────┴────────────────────┐
       ▼                                              ▼
┌─ Runner A (8xH100) ──────────────┐   ┌─ Runner B (4xL40S) ──┐
│ helix-runner (Go)                │   │ helix-runner (Go)    │
│  ├─ NATS client (status, cmds)   │   │  ├─ ...              │
│  ├─ HTTP server :8081            │   │  ├─ HTTP server      │
│  │   /v1/chat/completions ──┐    │   │                      │
│  │   /v1/embeddings         │    │   │                      │
│  │   /v1/images/generations │    │   │                      │
│  └─ ComposeManager          │    │   └─ ComposeManager      │
│       └─ docker compose ────┤    │        └─ docker compose │
│            up -d (in DinD)  │    │             ...          │
│                             ▼    │   └──────────────────────┘
│  ┌──── inner dockerd ───────┐    │
│  │ vllm-qwen3-embed :8000   │    │
│  │ vllm-qwen3-text  :8000   │◄───┤  reverse proxy by
│  │ vllm-qwen35-35b  :8000   │    │  body.model field
│  │ vllm-minimax-m2  :8000   │    │
│  │ vllm-gemma4-31b  :8000   │    │
│  └──────────────────────────┘    │
└──────────────────────────────────┘
```

## Key Design Decisions

### Decision 1: Reuse Sandbox's DinD Pattern Verbatim

The Sandbox container (`Dockerfile.sandbox`) already runs an inner `dockerd` to host isolated workloads (Wolf desktops, helix-sway). The pattern works in production. We copy it — including the persistent-volume layout and the registry-override mechanism (see Decision 9).

**Rationale:**
- Avoids reinventing GPU passthrough into nested Docker.
- Same operational story for ops (logs, debugging, restart semantics).
- The Hydra abstraction for per-scope dockerd isolation is *not* needed here — one runner has one inner dockerd hosting one compose stack.

**Consequence:** The runner image (`Dockerfile.runner`) shrinks dramatically. No more vLLM/Ollama bundled inside. It only contains: Go binary, docker CLI, dockerd, **both** nvidia-container-toolkit and AMD container runtime support (see Decision 10). All model runtimes come from the compose file's images. Model weights and image layers live in named volumes outside the image so they survive runner upgrades — see Decision 9.

### Decision 2: Profile = Compose YAML + Derived Metadata

A profile in the database has:

```go
type RunnerProfile struct {
    ID            string
    Name          string                  // "8xH100 production"
    Description   string
    ComposeYAML   string                  // raw text the operator wrote
    Models        []ProfileModel          // derived from YAML
    GPURequirement ProfileGPURequirement  // partly derived (Count), partly operator-declared
    CreatedAt, UpdatedAt time.Time
}

type ProfileModel struct {
    Name          string  // value of --served-model-name, or --model basename
    ContainerName string  // from compose service.container_name
    InternalPort  int     // first 8000-mapped port, or first ports[] entry
}

type ProfileGPURequirement struct {
    Count          int       // required; derived from union of device_ids across services
    Vendor         GPUVendor // optional: "nvidia" | "amd" | "" (any)
    Architectures  []string  // optional whitelist, e.g. ["hopper", "blackwell"]; empty = any
    ModelMatch     string    // optional regex against GPU marketing name
    MinVRAMBytes   int64     // optional
}
```

Derived fields are recomputed on save via a small parser (`api/pkg/runner/composeparse/`). Storing both raw YAML and derived metadata means:
- The router has fast lookups without re-parsing on every request.
- The UI can show models without parsing YAML in the browser.
- Hand-editing the YAML in the UI re-derives on save — no drift.

**Rationale for not using compose's own `profiles:` feature:** Compose profiles select a subset of services in *one* file at runtime. We want named, separately-stored, separately-versioned configurations. Different concept, same word — be careful in code.

### Decision 3: Reverse Proxy by Request Body, Not by URL Path

OpenAI-compatible clients put the model name in the request body's `model` field, not the URL. So the runner's HTTP handler:

1. Reads + buffers the request body.
2. JSON-decodes only the `model` field.
3. Looks up `model → ProfileModel.ContainerName + Port` from the active profile.
4. Forwards (with the original body) to `http://<container_name>:<port>/v1/...`.

Container names from the compose file are reachable via Docker's built-in DNS within the inner dockerd's default bridge network. No hardcoded IPs.

**Rationale:** Avoids URL rewriting. Matches how `vllm` and other OpenAI-compatible servers work. Trivially extends to embeddings and images endpoints — same lookup, different upstream path.

### Decision 4: API Server Routing — Simple Map, Not Scheduler

The internal Helix OpenAI client (`api/pkg/openai/helix_openai_client.go`) currently calls `scheduler.Enqueue(work)` (lines 305, 399) for both chat and embedding requests. We change *only* the implementation of those methods to call into the new router; the client's public interface (the `go-openai` interface implementation) is unchanged. Every in-tree caller — sessions handlers, embeddings handler, summary service, cron triggers, provider manager — keeps working without touching their imports.

Replace `api/pkg/scheduler/scheduler.go` with a much smaller `api/pkg/runner/router.go`:

```go
type Router struct {
    runners map[string]*RunnerState  // by runner ID
}

type RunnerState struct {
    ID            string
    ActiveProfile *RunnerProfile
    Status        string          // "running" | "starting" | ...
    LastSeen      time.Time
}

func (r *Router) PickRunner(modelName string) (*RunnerState, error) {
    candidates := r.runnersWithModel(modelName)
    if len(candidates) == 0 { return nil, ErrNoRunner }
    return candidates[r.nextRR()%len(candidates)], nil  // round-robin
}
```

That is essentially the entire scheduling logic. The bin-packing scheduler (`scheduler.go`, `slot.go`, the global allocation decisions, the eviction logic) is deleted.

**Rationale:** The user explicitly framed this as a *simplification*. If we keep any of the slot-allocation machinery "just in case", we have done the wrong thing. Operators express intent in the compose file; we obey it.

**One intentional caller-visible behaviour change:** the old scheduler would queue a request for a registered-but-unloaded model and load it on demand. The new router has no concept of "load on demand" — if the model isn't in some currently-`running` profile, the request returns 503 immediately with the list of available models. This is the *only* behaviour change visible to callers of `helix_openai_client` and the OpenAI-compatible HTTP endpoints; everything else (signatures, streaming semantics, error envelopes for normal cases) is preserved.

### Decision 5: Runner ↔ API Communication — Keep NATS

Keep the existing NATS pub/sub between runner and API server. We narrow the message set:

| Subject | Direction | Payload |
|---------|-----------|---------|
| `runner.{id}.status` | runner → api | hardware info, active profile ID, per-service health |
| `runner.{id}.cmd` | api → runner | `{"action":"set_profile","profile_id":"..."}` or `{"action":"clear_profile"}` |

All slot-related subjects are removed. `RunnerController` shrinks accordingly.

**Rationale:** NATS is already deployed and stable. The HTTP path (inference) stays HTTP — runner publishes a NATS heartbeat that includes its address, API server caches it, then dials directly for inference. Same as today.

### Decision 6: Profile Compatibility Check

Each constraint in `ProfileGPURequirement` is optional except `Count`. Constraints compose with AND. Validation order (cheapest first, fail fast):

1. **Index existence** — for every GPU index referenced in the compose YAML, that index exists on the runner. (Catches `device_ids: ["7"]` against a 4-GPU box.)
2. **Vendor** — if set, every referenced GPU's vendor must match. This is the load-bearing one: a CUDA-image profile assigned to an AMD box won't even start.
3. **Architecture** — if non-empty, every referenced GPU's architecture canonical string must be in the list. (`["hopper", "blackwell"]` matches H100 and B200; rejects A100.)
4. **ModelMatch** — if set, every referenced GPU's marketing name must match the regex.
5. **MinVRAMBytes** — if set, every referenced GPU's `total_memory >= MinVRAMBytes`.

All five run in the API server, not the runner, so the admin UI can pre-filter the assignment dropdown to *only profiles a given runner could run*. Validation errors are returned with the failing constraint named ("profile requires Hopper or Blackwell; runner GPU 0 is Ampere") so operators can fix either side.

**Architecture canonicalization** lives in one Go file shared by runner (writer) and API server (reader). NVIDIA compute-capability mapping (12.x → blackwell, 9.x → hopper, 8.x → ampere, 8.9 → ada) and AMD `gfx*` mapping (gfx942 → cdna3, etc.) are both there. Adding a new architecture = one line in that file.

**Examples of how the four optional fields compose in practice:**

| Profile intent | `Vendor` | `Architectures` | `ModelMatch` | `MinVRAMBytes` |
|----------------|----------|-----------------|--------------|----------------|
| 8xH100 production (tight) | `nvidia` | `["hopper"]` | `^NVIDIA H100` | 80 GB |
| Any 4×NVIDIA Blackwell    | `nvidia` | `["blackwell"]` | (unset) | (unset) |
| Any NVIDIA, dev workload  | `nvidia` | (empty) | (unset) | 24 GB |
| AMD MI300X embedding      | `amd`    | `["cdna3"]` | `MI300X` | (unset) |
| Truly any GPU             | (unset)  | (empty) | (unset) | (unset) |

### Decision 7: Profile Switching is Not Zero-Downtime

`set_profile` semantics:
1. Runner runs `docker compose -f /etc/helix/active.yaml down --remove-orphans` (if any active).
2. Writes new YAML to `/etc/helix/active.yaml`.
3. Runs `docker compose -f /etc/helix/active.yaml pull`.
4. Runs `docker compose -f /etc/helix/active.yaml up -d`.
5. Polls each service's `/v1/models` endpoint until ready or timeout.
6. Reports `running` (or `failed` with logs).

During steps 1–5 the runner reports a non-`running` state and the API router excludes it. Other runners keep serving. **Caller is responsible** for not assigning incompatible profiles to all runners simultaneously.

**Rationale:** Hot-swapping individual services in a compose stack is fragile (port conflicts, GPU memory not released cleanly). A clean down/up is more honest.

## Implementation Sketch

### New / Modified Files

**Backend (Go):**
- `api/pkg/runner/composeparse/parse.go` — parse compose YAML, extract models + GPU requirements.
- `api/pkg/runner/gpuarch/canonical.go` — vendor-specific GPU identifier → canonical architecture string mapping (NVIDIA compute capability + AMD `gfx*`).
- `api/pkg/runner/profile/store.go` — DB CRUD for profiles + parse-on-save service.
- `api/pkg/runner/profile/compatibility.go` — `Compatibility(req, gpus)` constraint check + `FilterCompatible(profiles, gpus)` helper.
- `api/pkg/runnerrouter/router.go` — replaces the scheduler's request-routing role. **Lives in its own package** (not `api/pkg/runner/router.go` as originally drafted) because the existing `api/pkg/runner/` package contains code destined for deletion (Ollama imports etc.) that breaks compilation in CGO-disabled environments. Decoupling lets the router build and test independently of the runner-package deletion timeline; routing is logically distinct from runner-binary code anyway.
- `api/pkg/runner/controller_nats.go` — narrowed to status + set_profile.
- `api/pkg/runner/compose_manager.go` — runs `docker compose` against the inner dockerd.
- `api/pkg/runner/proxy.go` — body-based reverse proxy (replaces the per-slot proxy).
- `api/cmd/helix/migrate-runner-config.go` — one-off migrator (see below).

**Backend (deleted):**
- `api/pkg/scheduler/` — entire package.
- `api/pkg/runner/vllm_runtime.go`, `ollama_runtime.go`, `axolotl_runtime.go`, `diffusers_runtime.go`.
- `api/pkg/runner/slot.go` and slot CRUD handlers in `server.go`.
- `api/pkg/types/runner.go` `RunnerSlot` struct.

**Runner image:**
- `Dockerfile.runner` — strip down to: golang build + dockerd + docker CLI + nvidia-container-toolkit. No vLLM/Ollama installs.

**Frontend:**
- New: `frontend/src/components/dashboard/RunnerProfilesTable.tsx` — CRUD list, similar shape to `HelixModelsTable.tsx`.
- New: `frontend/src/components/dashboard/EditRunnerProfile.tsx` — modal with YAML editor (Monaco, already in deps).
- Modified: `frontend/src/components/session/RunnerSummary.tsx` — replace slot list with profile-services list. Reuse the GPU memory chart unchanged.
- Modified: `frontend/src/components/session/ModelInstanceSummary.tsx` — render a compose service (status, health, logs link) instead of a slot.
- Modified: `frontend/src/pages/Dashboard.tsx` — add `runner_profiles` tab.

### Data Model

```sql
CREATE TABLE runner_profiles (
  id              TEXT PRIMARY KEY,
  name            TEXT NOT NULL UNIQUE,
  description     TEXT,
  compose_yaml    TEXT NOT NULL,
  models_json     TEXT NOT NULL,  -- []ProfileModel as JSON
  gpu_req_json    TEXT NOT NULL,  -- ProfileGPURequirement as JSON
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE runner_assignments (
  runner_id       TEXT PRIMARY KEY,        -- NATS-reported runner ID
  profile_id      TEXT REFERENCES runner_profiles(id) ON DELETE SET NULL,
  assigned_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  assigned_by     TEXT                     -- user ID for audit
);
```

The API server holds runner liveness in memory (rebuilt from NATS heartbeats on startup); the assignment is durable so a restarted runner re-applies its assigned profile automatically.

### Migration

There is no clean migration of existing slots → profiles. Instead:

1. Ship the new code behind no flag.
2. On API startup, log a warning if the old `slots` table is non-empty and skip reading it.
3. Provide a sample profile (`design/sample-profiles/8xH100-vllm.yaml`) matching the user's example compose, plus 1–2 smaller form factors.
4. Operators write profiles for their own hardware and assign them post-deploy.

This is acceptable because runners themselves are not user data — they reconnect and accept new commands. The downtime for an operator is "redeploy + click profile in UI".

## Decision 9: Persistent Storage & Offline Operation

The runner must support fully air-gapped deployments and must not re-download multi-GB model weights on every restart. The Sandbox already solves both problems with a layout we copy.

### Two named volumes mounted into the runner container

| Volume | Mount inside runner | Purpose | Survives restart? | Survives image upgrade? |
|--------|---------------------|---------|-------------------|--------------------------|
| `helix-runner-docker-storage` | `/var/lib/docker` | Inner dockerd state — image layers, stopped containers, build cache | Yes | Yes |
| `helix-runner-models` | `/models` | Shared HuggingFace / model-weight cache for every container the inner dockerd runs | Yes | Yes |

These exactly mirror Sandbox's `sandbox-docker-storage` and `~/.cache/huggingface` patterns (`docker-compose.dev.yaml` lines 268–303 and line 15). Operators upgrading the runner image keep all their pulled images and downloaded weights.

### Convention for compose authors: `/models` is the cache root

Operators write their profile compose files using `/models` (or `${HF_HOME:-/models}`) as the cache mount path:

```yaml
volumes:
  - /models:/root/.cache/huggingface
environment:
  - HF_HOME=/root/.cache/huggingface
  - HF_HUB_OFFLINE=1
```

Because the inner dockerd runs *inside* the runner container, its host filesystem is the runner container's filesystem — so `/models` from the operator's perspective resolves to the `helix-runner-models` named volume. No path rewriting, no magic. The user's example compose can be ported by changing `/prod/models` → `/models`.

We document the convention in the operator guide; we do *not* implement automatic path substitution. Magic substitution is a footgun (operators paste a working compose, it silently breaks because we rewrite a path they meant literally).

### Three modes of registry access

To match what Sandbox already supports plus an explicit offline mode:

1. **Default (online, public registry)** — runner does `docker compose pull` then `docker compose up -d`. Image refs in the profile YAML are taken as-is (e.g. `vllm/vllm-openai:latest`).
2. **Registry mirror (`HELIX_RUNNER_REGISTRY=mirror.corp.example.com`)** — copy of `HELIX_SANDBOX_REGISTRY` (`Dockerfile.sandbox` line 240, `sandbox/04-start-dockerd.sh` lines 205–235). Before each `pull`/`up`, the runner rewrites the leading registry portion of every `image:` field so `vllm/vllm-openai:latest` becomes `mirror.corp.example.com/vllm/vllm-openai:latest`. Same `sed`-style substitution Sandbox uses today.
3. **Offline (`HELIX_RUNNER_OFFLINE=true`)** — runner skips `docker compose pull` entirely; relies on images already being in the inner dockerd's `/var/lib/docker`. If a referenced image is missing, the runner fails the profile assignment with a clear message listing which images are absent. Combined with `HF_HUB_OFFLINE=1` in the operator's compose env (already in the user's example), this gives true air-gapped operation.

The three modes compose: an air-gapped deployment typically sets *both* `HELIX_RUNNER_REGISTRY` (so any rebuild that does need a pull goes to the internal mirror) and `HELIX_RUNNER_OFFLINE=true` (so no accidental external calls).

### Image cleanup ordering

Sandbox's startup script enforces "pull all new images BEFORE pruning old ones" because pruning first loses shared layers and forces a full re-download of the new images (`sandbox/04-start-dockerd.sh` lines 269–360, plus the rationale in `design/2026-01-12-sandbox-registry-based-images.md`). The runner's profile-switch logic must do the same:

1. (Online modes) `docker compose -f new.yaml pull`
2. `docker compose -f old.yaml down --remove-orphans`
3. `docker compose -f new.yaml up -d`
4. (Eventually, on a low-water mark) `docker image prune` of images no longer referenced by any known profile

**Don't** prune between steps 2 and 3. It costs gigabytes of re-pulls.

### What we do NOT do

- **No model preloading by the runner.** The runner does not download model weights on the operator's behalf. The operator either populates the `helix-runner-models` volume out-of-band (network filesystem, scp, restic snapshot, whatever they prefer), or relies on the first compose `up` to download with `HF_HUB_OFFLINE` unset. Trying to add a "pre-stage these weights" feature in the runner duplicates work the operator's existing tooling already does.
- **No tarball-based image distribution.** Sandbox already moved away from this (`design/2026-01-12-sandbox-registry-based-images.md`); we follow suit. Operators wanting offline image distribution run a local registry mirror.
- **No per-profile model isolation.** Both the model cache and the image cache are shared across all profiles on a given runner. This is the right default — the same `Qwen3.5-35B` weights are useful to multiple profiles. If a profile's compose author wants isolation, they can mount a sub-path.

### Registry credentials

Same plumbing as Sandbox today: Docker config on the host (`~/.docker/config.json`), or `imagePullSecrets` for the helm chart. No new mechanism. If the operator's mirror needs credentials, they configure it the same way they would for any DinD setup.

## Decision 11: Total Rewrite of the Runner Binary, Not Incremental Refactor

The runner Go code is rewritten from scratch in the same change set as the deletions. We do *not* try to evolve the existing `api/pkg/runner/` package in place. The package keeps its name (`api/pkg/runner/`); its contents are entirely new.

### Why a rewrite, not a refactor

Honest accounting of how much of today's runner code survives:

| Survives | What | Roughly how much |
|----------|------|------------------|
| Yes, copied forward verbatim or near-verbatim | NATS connection plumbing — connect/reconnect, heartbeat cadence, subject naming pattern | ~100 lines |
| Yes, reused as a shape | HTTP server scaffolding — mux setup, middleware order, log/auth wiring | ~50 lines |
| Yes, slimmed down | `gpu.go` — vendor/arch/total-VRAM detection (AC8 already says "slim down, don't delete") | ~150 lines after slimming |
| Yes, fields removed | `RunnerStatus` type — keep the struct, drop `AllocatedMemory`, `Models`, `GPUMemoryStats` | ~30 lines |
| Yes, mostly intact | `runner-cmd/helix-runner/main.go` — flag parsing, log setup, signal handling | ~80 lines |
| **No** | Slot lifecycle, slot CRUD handlers, vLLM/Ollama/Axolotl/Diffusers runtimes, process supervision, GGUF memory estimation, per-slot reverse proxy, scheduler client, slot state machine | ~6,000+ lines |

That's roughly 400 lines of ~6,500 surviving — 6%. Calling that a "refactor" is dishonest. It's a rewrite that copies a few load-bearing utilities forward.

### Why this matters for *how* the work is done

If we frame this as "modify the existing runner package", the implementer's reflex is to thread changes through existing files, preserve existing structure, and make targeted edits. The result is new responsibilities shaped by old defaults: the ComposeManager ends up looking like a Runtime because Runtime is what the surrounding scaffolding expects; the new proxy reuses the per-slot proxy's URL conventions because those URLs are still in adjacent files; the DinD lifecycle hooks into the slot state machine because the state machine is right there. Six months from now, "compose runner" reads like a layer over "slot runner" with awkward seams.

If we frame this as "delete the package contents and write new ones in the same change set", the new code is shaped by the new responsibilities. The compose manager is what it is; the proxy is what it is; the state machine is the simple `assigning → pulling → starting → running → failed` loop the design calls for. The handful of surviving utilities get copied forward as utilities, with no implication that the surrounding architecture should match.

### How this maps to the work

- The change set deletes every file currently under `api/pkg/runner/` (per AC8) and adds the new files in the same change set. Mid-PR, it's fine for the package to be empty for a few commits.
- The few surviving pieces are copied forward as plain files in the new package, not preserved as edits-in-place. Use `git mv` only if the file truly is unchanged (e.g. a NATS helper); otherwise it's a delete + rewrite.
- The new package has a different shape: `compose_manager.go`, `proxy.go`, `controller_nats.go` (narrowed), `pre_flight.go`, `gpu_inventory.go` (the slimmed-down GPU file), `server.go`, plus tests. No `slot.go`, no `*_runtime.go`, no `process_monitor.go`, no `commander.go`.
- Reviewing the resulting PR is "read the new files top-to-bottom", not "diff against the old structure". This is a feature.

### Implications for AC8 and the verification step

AC8's `git grep` litmus test (no live references to `scheduler.`, `RunnerSlot`, `GGUF`, etc.) becomes stricter under a rewrite framing: there should also be no references to the *internal abstractions* of the old runner that we used to lean on (`Runtime` interface, slot state machine types, per-slot URL builders). If an implementer finds themselves importing one of those into the new code, that's a signal they're regressing toward the old design.

## Decision 10: Dual GPU Vendor + Multi-Arch Runner Image

### Both NVIDIA and AMD runtimes in the same image

NVIDIA and AMD have completely different containerised-GPU mechanisms. NVIDIA uses `nvidia-container-toolkit` and a `nvidia` Docker runtime, with the `--gpus all` (or `deploy.resources.reservations.devices` in compose) syntax. AMD uses device passthrough — `/dev/kfd` (kernel fusion driver) and `/dev/dri` (DRM render nodes) — with `group_add: [video, render]`. The newer `amd-container-toolkit` automates this similar to how nvidia-container-toolkit does, but it's relatively new and not on every base image.

We install **both** in the runner image. Vendor selection is implicit: the operator's profile compose file declares the syntax (NVIDIA-style or AMD-style), and the inner dockerd uses whatever runtime it finds registered. A runner on a host with only NVIDIA hardware will simply never get assigned an AMD-style profile (filtered out at AC1a's vendor check); the AMD runtime support sits dormant. Same the other way round. There is no per-vendor build of the runner image — one image works for either, which keeps deploys simple.

The compose parser (`composeparse/parse.go`) handles both declaration styles when extracting GPU count:
- **NVIDIA:** `deploy.resources.reservations.devices` with `driver: nvidia` and `device_ids: [...]`. Count = `len(device_ids)`.
- **AMD:** top-level `devices:` containing `/dev/dri/renderD*` entries plus `group_add: [video, render]`. Count = number of distinct render-node entries. (`/dev/kfd` is shared and not counted.)
- A single service with both styles is rejected as ambiguous.

Pre-flight: when applying a profile, the runner checks the inner dockerd has the required runtime registered for the profile's vendor. If a vendor=nvidia profile is assigned to an inner dockerd without the `nvidia` runtime, fail fast with a clear message rather than producing an opaque `docker compose up` error.

### Multi-arch build: `linux/amd64` and `linux/arm64`

The runner image must be a multi-arch manifest covering both. Reasons:
- NVIDIA ships GPUs on both x86 (datacenter, workstation) and arm64 (Jetson, Grace Hopper).
- AMD ROCm is x86-only in practice, but operators on Apple Silicon dev machines need an arm64 runner image to run the runner without attaching a GPU profile (or with a CPU-only profile).
- A unified multi-arch manifest means deploy commands don't need to know what they're pulling.

Build: `docker buildx build --platform linux/amd64,linux/arm64 -f Dockerfile.runner -t ... --push`. The Go build line uses `GOOS=linux GOARCH=$TARGETARCH` so the right binary lands in each layer.

**Caveat:** AMD's `amd-container-toolkit` likely lacks arm64 packaging (since ROCm is x86-only). The `Dockerfile.runner` should skip the AMD-toolkit install on arm64 with a logged note: "AMD runtime omitted on arm64; arm64 runners cannot host AMD GPU profiles." NVIDIA-toolkit ships arm64 packages and stays on both architectures.

## What Dies (Deletion Catalogue)

This is a *simplification*, and the test of whether we got it right is whether the codebase shrinks substantially. The full file-level list lives in `requirements.md` AC8; this section captures the *categories* of code that disappear and why.

### Category 1: The bin-pack-meets-tensor-parallel scheduler
The current `api/pkg/scheduler/` package solves a hard problem: pick GPUs for a model that may need 1, 2, 4, or 8 GPUs (tensor parallel), while also bin-packing smaller models onto the GPUs that are left, while also evicting stale slots when the queue gets congested, while also keeping a mathematically-proven invariant that allocated memory ≤ total memory. The implementation is correct but expensive to maintain — the `multi_gpu_eviction_test.go`, `memory_calculation_inconsistency_test.go`, `model_allocation_integration_test.go` files exist because every interaction between bin-packing, eviction, and tensor parallelism is a corner case.

In the new world, the operator declares the layout in compose: `device_ids: ["2","3","4","5"]` + `--tensor-parallel-size 4` says "this model owns these four GPUs." There is no allocator. The whole package — `global_allocator.go`, `model_allocation.go`, the eviction logic, the queue, the prewarming cache — goes.

### Category 2: GGUF / Ollama memory estimation
`api/pkg/runner/memory_estimation_handlers.go` (and its API-server-side counterpart `api/pkg/server/memory_estimation_handlers.go`) imports `github.com/ollama/ollama/{api,discover,fs/ggml,llm}` to parse GGUF files and predict how much VRAM a given Ollama model + context length will consume. This was load-bearing for the scheduler — it had to predict before allocating. With compose, the operator types `--gpu-memory-utilization 0.45` and that's the budget. We don't predict; we don't need to. Both files plus the `api/pkg/types/memory.go` types disappear.

This deletion is what lets us drop the `github.com/ollama/ollama` dependency entirely (combined with deleting `ollama_runtime.go` and `ollama_model_controller.go`).

### Category 3: Custom Go runtimes
Four files (`vllm_runtime.go`, `ollama_runtime.go`, `axolotl_runtime.go`, `diffusers_runtime.go`) plus their helpers (`process_monitor.go`, `commander.go`, `ollama_model_controller.go`) exist to spawn and supervise model server subprocesses with carefully-crafted command lines, random localhost ports, and per-process lifecycle. All of that becomes `docker compose up -d` against the operator's YAML.

### Category 4: Per-slot HTTP proxy + slot CRUD
The runner's HTTP surface today is mostly slot-shaped (`POST /api/v1/slots`, `DELETE /api/v1/slots/{id}`, `POST /api/v1/slots/{id}/v1/chat/completions`). The new surface is profile-shaped (`POST /v1/chat/completions` with model name in the body, plus a status endpoint). The slot URL space and the per-slot proxy logic in `api/pkg/runner/server.go` and `openai_*_handlers.go` go.

### Category 5: Frontend scheduler visualisations
`GlobalSchedulingVisualization.tsx`, `SchedulingDecisionsTable.tsx`, `SchedulerHealthIndicators.tsx` — these visualise things that no longer happen. The runner card components (`RunnerSummary`, `ModelInstanceSummary`, `FloatingRunnerState`) are *kept* and adapted; the scheduler-decision components are deleted outright. Any `MemoryEstimateCell` references in `HelixModelsTable.tsx` go too.

### Category 6: DB tables, env vars, CLI flags
`runner_slots` table dropped via explicit migration. `HELIX_MODEL_TTL`, `HELIX_SLOT_TTL`, `HELIX_SCHEDULING_STRATEGY`, `HELIX_QUEUE_SIZE` env vars + corresponding fields in `api/pkg/config/config.go` removed. `NewScheduler()` and `PrewarmNewRunner` callback wiring in `api/cmd/helix/serve.go` removed.

### Verification of completeness
A reviewer should be able to run `git grep -nE "scheduler\.|RunnerSlot\b|GGUF|memory_estimation|ollama/ollama|axolotl|diffusers_runtime|SchedulingDecision|GlobalAllocationDecision|Prewarm"` and find nothing live. This is the litmus test. If something remains, either it's load-bearing for something legitimate (document why) or we missed it (delete it).

## Decision 8: CGO Off — Runner Only

Earlier draft of this doc speculated that both the API server and the runner could flip to `CGO_ENABLED=0` after the deletions. **That was wrong on the API server side.** The API server's `-tags ORT` build tag and `github.com/yalue/onnxruntime_go` dependency exist for **Kodit** (code-intelligence indexing), not for any runner-related embedding fallback. `api/pkg/server/kodit_init.go:261` defines `preflightORT()` which fails fast if `libonnxruntime.so` isn't present whenever Kodit is enabled with a local-ONNX embedding model. Kodit is wholly orthogonal to this work and we do not touch it.

So the actual situation:

| Binary | CGO today | CGO after this change | Reason |
|--------|-----------|-----------------------|--------|
| API server (`Dockerfile`) | `=1`, `-tags ORT` | **unchanged** | onnxruntime for **Kodit** local embedding (`api/pkg/server/kodit_init.go:261`) |
| Runner (`Dockerfile.runner`) | `=1`, `-tags "!rocm"` | **`=0`, no tags** | only driver was Ollama Go SDK (`fs/ggml`, `llm`) for memory estimation; deleted in Category 2 |
| Sandbox helpers (`hydra`, `sandbox-heartbeat`) | `=0` already | unchanged | n/a |
| Desktop binaries (`desktop-bridge`) | `=1` | unchanged | xkb / wayland / pipewire bindings — separate concern |

**The runner change is the win we get for free.** Once `memory_estimation_handlers.go`, `ollama_runtime.go`, and `ollama_model_controller.go` are gone, `git grep '^import \"C\"' runner-cmd/ api/pkg/runner/` should return nothing, and we can ship a static Go runner binary: smaller image, faster CI, simpler cross-compilation, no glibc/musl coupling for the runner.

**Risk:** an indirect runner dependency might still pull a CGO-requiring package. If so, document it (`design/2026-MM-DD-cgo-after-runner-rewrite.md`) and leave runner CGO=1. Don't ship a half-disabled state, and don't go hunting for the indirect dep to swap it out — that's scope creep.

## Implementation Notes (as we go)

### Foundation layer landed (PR 1)
What's done and what subsequent agents should build on:

- **Data model:** `types.RunnerProfile` and `types.RunnerAssignment` are GORM types registered in `store.postgres.AutoMigrate`. Per `api/pkg/store/migrations/README.md` this codebase uses AutoMigrate for new tables — explicit SQL migrations are reserved for renames/alters.
- **Compose parsing:** `api/pkg/runner/composeparse/` is the source of truth for "what does this YAML expose?". Validated against all five `design/sample-profiles/*.yaml` via `sample_profiles_test.go` — that test breaks loudly if the parser regresses against any committed sample. Parser handles both NVIDIA-style (`deploy.resources.reservations.devices`) and AMD-style (`devices: [/dev/kfd, /dev/dri/renderD*]` + `group_add`) GPU declarations and rejects mixing them in one service.
- **Architecture mapping:** `api/pkg/runner/gpuarch/canonical.go` is the single shared mapping file — adding a new architecture is one entry. Used by both runner (label its GPUs) and API server (validate profile fit). NVIDIA mapping is by compute capability major version with a special case for 8.9 = Ada.
- **Profile service:** `api/pkg/runner/profile/` enforces the parse-on-save invariant. Callers must go through this package to write profiles; calling the store directly bypasses re-derivation of `Models` + `GPURequirement.Count`.
- **Compatibility check:** `profile.Compatibility(req, gpus)` returns nil or `*IncompatibilityReason` naming the failing constraint. Index-existence (does the YAML reference a GPU index that doesn't exist on this runner) is deliberately NOT here — it operates on the parsed compose, not on the profile's declared count, and lives at the assignment layer.
- **Router:** `api/pkg/runnerrouter/` rather than `api/pkg/runner/router.go` as originally planned. The existing `api/pkg/runner/` package can't compile without CGO + Ollama deps that are due for deletion (see AC8 / Decision 11). Decoupling lets the router build and test independently of the runner-package deletion timeline. Routing is logically distinct from runner-binary code anyway.
- **HTTP routes:** Five admin endpoints for profile CRUD live in `api/pkg/server/runner_profile_handlers.go`. Validation/parse errors → 400; missing IDs → 404. Assign-profile / clear-profile / compatible-profiles routes are NOT yet wired — they need `RunnerStatus` to carry `vendor` and `architecture` per-GPU, which lands with the runner-binary rewrite (AC2).

### Gotchas surfaced during foundation work

- **`api/pkg/runner/` won't compile in CGO_ENABLED=0 environments** until the deletion of `memory_estimation_handlers.go` and the Ollama Go SDK imports (AC8 + Decision 8). This is why the router lives in `api/pkg/runnerrouter/` rather than as a sibling file. Future agents adding more API-server-side code that's logically "part of the runner subsystem" should put it under a new sibling package (or `api/pkg/runnerXXX/`) rather than inside `api/pkg/runner/` until the rewrite lands.
- **`store_mocks.go` must be regenerated** with `mockgen -source store.go -destination store_mocks.go -package store` whenever the `Store` interface gains methods. Caught by `*store.MockStore does not implement store.Store` build errors.
- **The CLAUDE.md test pattern requires CGo** for tree-sitter and other packages. Foundation packages in this PR were intentionally written CGO-free (no Ollama, no tree-sitter, no other native deps) so they test cleanly without `gcc`/`libc6-dev` on the host. Keep new foundation code CGO-free where possible.

## Decision 12: Unify Runner and Sandbox into a Single "Worker" Image

**Context:** today's Sandbox container (`Dockerfile.sandbox`) and the new
compose-profile runner are converging structurally. Sandbox runs an inner
dockerd to host dynamic agent desktop containers via Hydra. The new runner
runs an inner dockerd to host static LLM inference containers via the
compose manager. They differ in *what they orchestrate* (dynamic desktops
vs. declarative inference), not in *how they orchestrate* (DinD with GPU
passthrough).

**Decision:** ship one Docker image — `Dockerfile.worker` — that bundles
both subsystems. Operators deploy one pod type. Per worker (per assigned
profile) they choose what mix runs there: pure inference, pure agent
desktops, or mixed.

### What lives in the worker image

| Component | Source | Role |
|-----------|--------|------|
| Inner dockerd | from Sandbox base | host all GPU containers |
| nvidia-container-toolkit + AMD container runtime | shared | dual-vendor GPU passthrough |
| Hydra (Go binary) | unchanged from today | per-scope desktop dockerd isolation; spawn agent containers on demand |
| Compose manager (Go binary, new) | this work | apply assigned inference profile; pull / up / down compose stacks |
| Inference proxy (Go binary, new) | this work | body-aware reverse proxy: model name → container in inner dockerd |
| Status reporter (Go) | shared | report per-GPU inventory + active services + active desktop sessions |

The two binaries — `hydra` and the new compose manager — coexist as
separate processes managed by `s6-overlay` (or whatever supervisor
Sandbox uses today). They share the inner dockerd and the host GPU
inventory; they don't talk to each other directly. The API server has
both subsystems active simultaneously and routes accordingly.

### What "mixed workload" looks like in a profile

A worker profile is still a Docker Compose YAML. Inference services are
declared as before:

```yaml
services:
  vllm-qwen:
    image: vllm/vllm-openai:latest
    deploy: { resources: { reservations: { devices: [{driver: nvidia, device_ids: ["0","1"]}]}}}
```

For mixed-mode profiles, the operator additionally declares the GPU
pool that Hydra is allowed to use for dynamic agent desktops:

```yaml
services:
  vllm-qwen:
    deploy: { resources: { reservations: { devices: [{driver: nvidia, device_ids: ["0","1"]}]}}}
  vllm-embedding:
    deploy: { resources: { reservations: { devices: [{driver: nvidia, device_ids: ["2"]}]}}}

x-helix:
  hydra-gpu-pool: ["3", "4", "5", "6", "7"]
```

The `x-helix` extension is a top-level compose key that holds Helix-
specific metadata that doesn't fit the standard schema. The compose
parser extracts `hydra-gpu-pool` and exposes it to Hydra at profile-apply
time so Hydra knows which GPU indices it may schedule against.

If `x-helix.hydra-gpu-pool` is absent, the worker is "pure inference" —
Hydra is present but receives no GPU pool and refuses to spawn
GPU-requiring agent sessions on this worker. Conversely, an empty
`services:` map means "pure agent" — no inference, all GPUs go to Hydra.

### Why this is cheap to do now

- `runnerrouter` (already shipped) routes inference. Hydra (already
  shipped) routes agent sessions. They don't compete; they consume
  the same node inventory in non-overlapping ways.
- `composeparse` (already shipped) only needs an `x-helix` extraction
  pass, which is one extra YAML field to read.
- `profile.RunnerProfile` (already shipped) gains an optional
  `HydraGPUPool []string` field. Compatibility checks unchanged.
- `Dockerfile.worker` is `Dockerfile.sandbox` with the new compose
  manager and inference proxy binaries copied in.

### What the unification does NOT change

- The compose-based inference path (Decisions 1–11): still applies,
  still validated by the same tests.
- Hydra's per-scope dockerd isolation: unchanged.
- The deletion mandate (AC8) for the existing runner Go code: still
  applies — the old `api/pkg/runner/` is gone, replaced by the new
  worker-side code.
- The CGO investigation (Decision 8): still applies to the worker
  image, with the wrinkle that Hydra's existing CGO-free build is
  preserved.

## Decision 13: Unified Worker Status, Not Two Status Surfaces

The API server currently surfaces two separate status types in the admin
UI: `RunnerStatus` (per-GPU memory, slots, models) and `SandboxInstance`
(connected sandboxes, sessions, health). After unification these collapse
into one `WorkerStatus`:

```go
type WorkerStatus struct {
    ID            string                  // worker ID (NATS-reported)
    URL           string                  // address for proxying
    Version       string
    LastSeen      time.Time

    // Hardware inventory (was RunnerStatus.GPUs).
    GPUs          []GPUStatus             // per-GPU: vendor, arch, total/used VRAM, model name

    // Inference subsystem state (replaces RunnerStatus.Slots / .Models).
    ActiveProfile *RunnerProfile          // nil if no profile assigned
    ProfileStatus string                  // "running" | "starting" | "pulling" | ...
    ServiceHealth map[string]string       // service name → "healthy" | "starting" | "failed"

    // Agent subsystem state (was SandboxInstance fields).
    HydraGPUPool  []string                // GPU indices reserved for Hydra (from profile x-helix)
    Sessions      []DesktopSessionSummary // active agent desktops
}
```

The frontend gets one `WorkerSummary` card that shows per-GPU usage,
inference services, and active desktop sessions. The two existing UI
trees (`RunnerSummary`, `AgentSandboxes`) collapse into one.

## Open Questions

1. **Do we need to namespace inner-dockerd networks per runner?** Probably not — each runner has its own inner dockerd, so the default bridge network is already isolated. Confirm during implementation.
2. **GPU passthrough into nested dockerd: any extra setup beyond `--gpus all` on the outer container?** Sandbox doesn't run GPU workloads inside DinD currently. Spike before committing to the full design — if this turns out to require kernel module gymnastics on the host, fall back to running compose against the *outer* dockerd (less isolation but simpler).
3. **HF token + image registry secrets:** plumbed to the inner dockerd via environment variables on the runner container (existing pattern). Confirm with ops there are no secret-rotation concerns.
4. **Per-service log retention:** logs come from `docker compose logs`. Truncate to last N lines on retrieval; don't try to stream long-lived logs over NATS.
