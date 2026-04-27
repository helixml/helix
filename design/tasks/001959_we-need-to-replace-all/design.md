# Design: Compose-Profile Runner

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

The Sandbox container (`Dockerfile.sandbox`) already runs an inner `dockerd` to host isolated workloads (Wolf desktops, helix-sway). The pattern works in production. We copy it.

**Rationale:**
- Avoids reinventing GPU passthrough into nested Docker.
- Same operational story for ops (logs, debugging, restart semantics).
- The Hydra abstraction for per-scope dockerd isolation is *not* needed here — one runner has one inner dockerd hosting one compose stack.

**Consequence:** The runner image (`Dockerfile.runner`) shrinks dramatically. No more vLLM/Ollama bundled inside. It only contains: Go binary, docker CLI, dockerd, nvidia-container-toolkit. All model runtimes come from the compose file's images.

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
- `api/pkg/runner/profile/store.go` — DB CRUD for profiles.
- `api/pkg/runner/router.go` — replaces the scheduler's request-routing role.
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

## Open Questions

1. **Do we need to namespace inner-dockerd networks per runner?** Probably not — each runner has its own inner dockerd, so the default bridge network is already isolated. Confirm during implementation.
2. **GPU passthrough into nested dockerd: any extra setup beyond `--gpus all` on the outer container?** Sandbox doesn't run GPU workloads inside DinD currently. Spike before committing to the full design — if this turns out to require kernel module gymnastics on the host, fall back to running compose against the *outer* dockerd (less isolation but simpler).
3. **HF token + image registry secrets:** plumbed to the inner dockerd via environment variables on the runner container (existing pattern). Confirm with ops there are no secret-rotation concerns.
4. **Per-service log retention:** logs come from `docker compose logs`. Truncate to last N lines on retrieval; don't try to stream long-lived logs over NATS.
