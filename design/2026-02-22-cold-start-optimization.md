# Faster Docker Cold Starts for Agent Sessions

**Date**: 2026-02-22

## Context

With shared BuildKit + smart `--load` + registry-based transfer (PRs #1705-#1722), warm starts take **~23s**. But first sessions still have a cold-start penalty from image transfer. The blog's "10-minute" figure was measured before PR #1722 (registry-based `--load`), so the actual cold start is likely faster — but needs re-benchmarking with the current code.

Two remaining bottlenecks are called out in the blog:
1. **`docker compose build` bypasses the wrapper** — compose calls buildx bake internally (Go API, not CLI), so smart `--load` never fires. Cold-start compose build takes ~60s (tarball `--load`).
2. **Empty Docker daemon on first session** — even with registry-based `--load`, all images need full transfer on the first session.

---

## Step 1: Benchmark Current Cold Start

Before any code changes, measure the actual cold-start time with all existing optimizations (shared BuildKit, smart `--load`, registry-based transfer) deployed.

### Method

1. Start a fresh spectask (empty Docker daemon)
2. Run the full build pipeline (`./stack build && ./stack build-zed release && ./stack build-sandbox`)
3. Record per-phase timing with `BUILD_SANDBOX_TIMING_LOG`
4. Break down: which phases use smart `--load` (fast) vs tarball `--load` (slow)?
5. How much time is compose build vs individual docker builds?

### Results (2026-02-22, session ses_01kga54ay40ht22m28t5kcz7fd)

**Total cold start: ~10 minutes** (container start to build-sandbox complete)

| Phase | Duration | Notes |
|-------|----------|-------|
| Boot + workspace setup | 23s | Git clone, branch checkout |
| `./stack build` (compose) | **41s** | 4 images cold-loaded via registry |
| `./stack build-zed release` | ~19s | Binary check (pre-built in image) |
| `./stack build-sandbox` | **525s** | See breakdown below |
| — qwen-code build | 60s | npm install + build |
| — helix-ubuntu build + load | 86s | 7.24GB image via registry |
| — helix-sandbox build | 10s | |
| — Inner registry push | **227s** | Push 7.24GB desktop image |
| — Inner registry pull | **135s** | Pull in inner sandbox daemon |

**Key findings:**
1. Compose build is **41s** cold, not 60s — registry loading from PR #1722 already helps
2. **Dominant bottleneck: inner sandbox image transfer** (push+pull = **362s / 6 min**)
3. Compose interception would save ~20s cold, ~7s warm — modest but worthwhile

### Decision

- Proceed with Step 2 (compose build interception) — saves 7s on every warm start
- Step 3 (golden cache) is needed for dramatic cold start improvement but is a larger change

---

## Step 2: Make `docker compose build` use smart `--load`

### Problem

The docker wrapper at `/usr/local/bin/docker` intercepts `docker build` and `docker buildx build`, but when `$1=compose` it passes through unchanged (line 30: `exec "$REAL_DOCKER" "$@"`). Compose invokes buildx bake internally via Go APIs, so our smart `--load` and registry-based transfer never apply. Result: compose always does tarball `--load`, even when images are unchanged or only 1 layer changed.

### Approach: Intercept `docker compose ... build` in the wrapper

Extend the wrapper to detect `docker compose build` (or `docker compose ... build`) and decompose it into individual `docker buildx build` commands that go through our existing smart `--load` logic.

**Flow:**
```
docker compose -f docker-compose.dev.yaml build [service...]
  └── wrapper intercepts ($1 = "compose", finds "build" in args)
      1. Run: $REAL_DOCKER compose -f FILE config --format json
         → get services, their image names, build contexts, dockerfiles, args
      2. For each service with a build section:
         → docker buildx build -t $IMAGE -f $DOCKERFILE $CONTEXT [--build-arg ...]
         → this goes through our existing smart --load logic:
            a. Image unchanged? → skip load (314ms)
            b. Image changed? → registry push/pull (871ms)
            c. No registry? → tarball --load fallback (10s)
      3. Done. Compose up will find the images locally.
```

### Implementation in `docker-buildx-wrapper.sh`

Add a new branch before the existing pass-through:

```bash
# Detect 'docker compose ... build ...'
if [ "${1:-}" = "compose" ]; then
    # Check if "build" appears in the args
    has_compose_build=false
    for arg in "$@"; do
        if [ "$arg" = "build" ]; then
            has_compose_build=true
            break
        fi
    done

    if ! $has_compose_build || [ "$_DOCKER_WRAPPER_DRIVER" != "remote" ]; then
        # Not a compose build, or not using remote builder — pass through
        exec "$REAL_DOCKER" "$@"
    fi

    # Extract compose file flags (-f/--file) and service names
    # Run: $REAL_DOCKER compose [compose-flags] config --format json
    # Parse with jq to get: service name, image, build.context, build.dockerfile
    # For each service: run docker buildx build with smart --load
    # (reuses existing wrapper logic by calling $0 buildx build ...)
fi
```

**Parsing compose config**: Use `$REAL_DOCKER compose -f $FILE config --format json | jq` to extract:
- `.services[].image` — target image name
- `.services[].build.context` — build context path
- `.services[].build.dockerfile` — Dockerfile path
- `.services[].build.args` — build arguments
- `.services[].build.target` — build target stage

Only build services that have a `build:` section. Skip services without one (pre-built images).

If specific services are passed (`docker compose build api frontend`), only build those.

### Expected Impact

| Scenario | Before | After |
|----------|--------|-------|
| Cold start compose (images not local) | ~60s | ~10-15s (registry pull, layer dedup) |
| Warm start compose (images unchanged) | ~9.6s | ~2s (digest check per service, skip all) |
| Incremental (1 service changed) | ~9.6s | ~2s (skip unchanged, ~1s for changed) |

### Files to Modify

| File | Change |
|------|--------|
| `desktop/shared/docker-buildx-wrapper.sh` | Add compose build interception logic |

### Verification

1. **Re-benchmark cold start**: Start fresh spectask, run `./stack build`, measure compose build time
2. **Warm start**: Run `./stack build` again (no changes), verify compose build skips all loads
3. **Incremental**: Change one Go file, rebuild, verify only API image reloads
4. **Fallback**: Unset `HELIX_REGISTRY`, verify compose build falls back to normal behavior
5. **Non-build compose commands**: Verify `docker compose up`, `docker compose ps`, etc. still work unchanged

### Risks

1. **`jq` dependency**: The wrapper needs `jq` to parse compose config JSON. Check if `jq` is installed in desktop containers; if not, add it to the Dockerfile.
2. **Compose config format changes**: Parsing JSON output couples to Docker Compose's config format. Mitigate by handling missing fields gracefully.
3. **Build arg handling**: Compose supports build args from env vars, `.env` files, and compose file. Need to pass these through correctly to `docker buildx build`.
4. **Multi-service builds**: Must handle `docker compose build` (all services) and `docker compose build api frontend` (specific services).

---

## Step 3: Golden Docker Cache (if needed after Step 1)

Only pursue this if Step 1 + re-benchmarking shows cold start is still unacceptably slow.

### Goal

Make cold start match warm start (~23s) by pre-populating session Docker data from a "golden" directory. The golden is built generically by running the project's startup script — no app-specific knowledge of which images to build.

### Project Setting (generic, off by default)

Add `AutoWarmDockerCache bool` to `ProjectMetadata` (JSONB field, no migration):

```go
// api/pkg/types/project.go line 242
type ProjectMetadata struct {
    BoardSettings       *BoardSettings `json:"board_settings,omitempty"`
    AutoWarmDockerCache bool           `json:"auto_warm_docker_cache,omitempty"`
}
```

### Trigger: Main Branch Changes

Two code paths detect merges to main:

**A. PR merged (external repos)** — `spec_task_orchestrator.go:575-587`
- When `PullRequestStateMerged` detected → trigger golden build if setting enabled

**B. Internal merge (approve implementation)** — `spec_task_workflow_handlers.go:263-289`
- After fast-forward merge succeeds → trigger golden build if setting enabled

Both call `goldenBuildService.TriggerGoldenBuild(ctx, project)` which:
- Checks `project.Metadata.AutoWarmDockerCache`
- Checks no golden build is already running for this project (debounce)
- Creates a golden build session

### Golden Build Session

A real desktop container whose only job is running the startup script:

- Created via existing `StartDesktop()` flow with:
  - `BranchMode: "existing"`, `WorkingBranch: project.DefaultBranch` (checks out main)
  - `Env: ["HELIX_GOLDEN_BUILD=true"]`
  - Normal `RepositoryIDs`, `ProjectID`
- **No spec task** — visible in UI as "Docker Cache Warm-up"
- `helix-workspace-setup.sh` detects `HELIX_GOLDEN_BUILD=true`:
  - Clones repos, checks out main (same as normal)
  - Runs `.helix/startup.sh` in foreground (blocking)
  - Skips Zed IDE startup
  - Exits with startup.sh's exit code

### Golden Promotion

When golden build container exits with code 0:
1. Docker data is at `/container-docker/sessions/docker-data-{sessionID}/docker/`
2. Atomic rename to `/container-docker/golden/docker/`
3. Clean up session record (Docker data preserved as golden)

### Overlayfs COW Mount for New Sessions

When a new session starts and golden exists:

```
/container-docker/
├── golden/docker/           ← populated by golden build session (read-only lowerdir)
└── sessions/
    └── docker-data-{sessionID}/
        ├── upper/           ← per-session COW writes
        ├── work/            ← overlayfs workdir
        └── merged/          ← overlayfs mount → bind-mounted as /var/lib/docker
```

**`devcontainer.go:buildMounts()` (line ~635)**:
- If `/container-docker/golden/docker/` exists: create overlayfs mount
- If not: fall back to empty directory (current behavior)

Properties: O(1) mount, true COW, kernel 6.17 nested overlayfs support.

### Session Cleanup

`devcontainer.go:StopDevContainer()` (line ~899):
- `syscall.Unmount(merged, MNT_DETACH)`
- `os.RemoveAll(sessions/docker-data-{sessionID}/)`
- Fix existing bug: bind mount dirs never cleaned up

### Staleness Handling

Stale golden (images rebuilt after golden created):
- Session starts with old images from golden overlay
- Smart `--load` detects digest mismatch, updates via registry (~1s per image)
- Golden rebuilt on next main branch change

### Files to Modify (Step 3)

| File | Change |
|------|--------|
| `api/pkg/types/project.go` | Add `AutoWarmDockerCache` to `ProjectMetadata` |
| `api/pkg/hydra/golden.go` | **New** — overlay mount/unmount, golden promotion |
| `api/pkg/hydra/devcontainer.go` | Use overlay mount when golden exists; cleanup |
| `api/pkg/services/golden_build_service.go` | **New** — trigger/manage golden build sessions |
| `api/pkg/services/spec_task_orchestrator.go` | Trigger golden build on PR merge |
| `api/pkg/server/spec_task_workflow_handlers.go` | Trigger golden build on internal merge |
| `desktop/shared/helix-workspace-setup.sh` | Handle `HELIX_GOLDEN_BUILD=true` mode |

### Risks (Step 3)

1. **Nested overlayfs**: Docker overlay2 on our overlay mount. Supported on kernel 6.17 but needs testing.
2. **Golden update race**: Renaming golden while sessions use it as lowerdir. Accept for prototype; versioned goldens for production.
3. **Session as golden builder**: Runs normal dockerd, no conflict with sandbox's dockerd.

### Verification (Step 3)

1. Enable `auto_warm_docker_cache` on Helix project
2. Merge a PR → verify golden build session triggers
3. Golden build completes → startup script runs, images built, golden promoted
4. New cold-start session → uses golden overlay, matches warm start timing
5. Multiple concurrent sessions → independent overlay mounts
6. Session cleanup → overlay unmounted, dir removed
7. No golden → falls back to empty dir

---

## Process

1. Write plan to `design/2026-02-22-cold-start-optimization.md` before starting ✅
2. All code changes in a prototype branch — no merging until full review and testing
3. Each step is gated on the previous step's results
