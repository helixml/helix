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

## Step 2 Results (2026-02-22)

Compose build interception implemented and tested live in session:

| Scenario | Before | After |
|----------|--------|-------|
| Warm compose build (4 services, unchanged) | 9.6s | **3.2s** |
| Single service warm check | ~2.4s | **0.7s** |

**Note**: The bash wrapper is getting complex (290 lines, jq pipelines, array parsing, recursive `$0`). If we keep this optimization, rewrite in Go for maintainability. Do that last, after golden cache results confirm we need it.

---

## Step 3: Golden Docker Cache

### Goal

Eliminate cold-start image transfer by pre-populating session Docker data from a project-specific "golden" snapshot. The golden is built generically by running the project's startup script — no app-specific knowledge of which images to build.

### Why the registry comes for free

Docker volumes live under `/var/lib/docker/volumes/`. By overlaying the entire `/var/lib/docker`, the golden captures everything:
- **Image layers** (`overlay2/`) — all built images
- **Docker volumes** (`volumes/`) — includes inner registry data, BuildKit state, sandbox Docker storage
- **Container metadata** — not useful (containers don't survive restart)

For Helix-in-Helix, the inner sandbox's Docker data is in a Docker volume within the session's daemon. The golden overlay provides this volume pre-populated, so the inner sandbox starts with images AND the inner registry already has layers. The `transfer-desktop-to-sandbox` function checks `IMAGE_HASH_BEFORE = IMAGE_HASH_AFTER` → skips the 362s transfer.

### Per-project golden directories

Each project has its own startup script building different images. Goldens are scoped per-project:

```
/container-docker/
├── golden/
│   ├── {projectID}/docker/    ← project A's golden (read-only lowerdir)
│   └── {projectID2}/docker/   ← project B's golden
└── sessions/
    └── docker-data-{sessionID}/
        ├── upper/              ← per-session COW writes
        ├── work/               ← overlayfs workdir
        └── merged/             ← overlayfs mount → bind-mounted as /var/lib/docker
```

**Disk pressure**: Each golden can be 15-20GB (Helix project with all images + volumes). For prototype, no eviction — single-tenant deployments won't have many projects. Production: add LRU eviction by last-used timestamp.

### Project Setting (generic, off by default)

Add `AutoWarmDockerCache bool` to `ProjectMetadata` (JSONB field, no migration):

```go
type ProjectMetadata struct {
    BoardSettings       *BoardSettings `json:"board_settings,omitempty"`
    AutoWarmDockerCache bool           `json:"auto_warm_docker_cache,omitempty"`
}
```

### Trigger: Main Branch Changes

Two code paths detect merges to main:

**A. PR merged (external repos)** — `spec_task_orchestrator.go`
- When `PullRequestStateMerged` detected → trigger golden build if setting enabled

**B. Internal merge (approve implementation)** — `spec_task_workflow_handlers.go`
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
2. Atomic rename to `/container-docker/golden/{projectID}/docker/`
3. Clean up session record (Docker data preserved as golden)

### Golden Copy for New Sessions (revised from overlayfs)

**Overlayfs didn't work**: Docker's overlay2 storage driver tries to create overlayfs mounts inside our overlayfs merged dir. Nested overlayfs fails with `invalid argument` because the upper dir can't be on an overlayfs filesystem, even on kernel 6.17.

**Approach**: `cp -a` the golden to each session's Docker data directory. The copy is fast enough (13.8s for 8.7 GB on SSD) and avoids the nested overlayfs issue entirely.

```
/container-docker/
├── golden/{projectID}/docker/   ← read-only reference (never mounted)
└── sessions/docker-data-{sessionID}/docker/  ← copied from golden, bind-mounted as /var/lib/docker
```

**`devcontainer.go:buildMounts()`**:
- Check `/container-docker/golden/{projectID}/docker/` exists
- If yes: `cp -a golden sessions/docker-data-{sessionID}/docker` (~14s)
- If not: create empty directory (current behavior)

### Session Cleanup

`devcontainer.go:DeleteDevContainer()`:
- `os.RemoveAll(sessions/docker-data-{sessionID}/)`
- For golden builds: skip cleanup (monitorGoldenBuild handles promotion)

### Staleness Handling

Stale golden (code changed since golden was built):
- Session starts with old images from golden overlay
- Smart `--load` detects digest mismatch on next build, updates via registry (~1s per image)
- Golden rebuilt automatically on next merge to main

### Files to Modify

| File | Change |
|------|--------|
| `api/pkg/types/project.go` | Add `AutoWarmDockerCache` to `ProjectMetadata` |
| `api/pkg/hydra/golden.go` | **New** — overlay mount/unmount, golden promotion |
| `api/pkg/hydra/devcontainer.go` | Use overlay mount when golden exists; cleanup |
| `api/pkg/services/golden_build_service.go` | **New** — trigger/manage golden build sessions |
| `api/pkg/services/spec_task_orchestrator.go` | Trigger golden build on PR merge |
| `api/pkg/server/spec_task_workflow_handlers.go` | Trigger golden build on internal merge |
| `desktop/shared/helix-workspace-setup.sh` | Handle `HELIX_GOLDEN_BUILD=true` mode |

### Risks

1. ~~**Nested overlayfs**: Docker overlay2 on our overlay mount.~~ **RESOLVED**: Switched to copy approach. Docker overlay2 works fine on the copied (ext4) directory.
2. **Golden update race**: `cp -a` reads golden while another process renames it during promotion. Low risk since promotion is atomic rename.
3. **Disk pressure**: 8-20 GB per project golden + copy per session. No eviction for prototype.
4. **Copy time scales with golden size**: 13.8s for 8.7 GB. Larger goldens (20+ GB) could take 30s+. Still far better than 10 min cold build.

### Verification

1. Enable `auto_warm_docker_cache` on Helix project
2. Merge a PR → verify golden build session triggers
3. Golden build completes → startup script runs, images built, golden promoted
4. New cold-start session → uses golden copy, images pre-populated
5. Inner dockerd starts successfully with copied data
6. Multiple concurrent sessions → independent copies (no shared state)
7. Session cleanup → session dir removed, golden untouched
8. No golden → falls back to empty dir (current behavior)

---

## Step 3 Test Results (2026-02-22)

### Overlayfs approach (FAILED)

Docker's overlay2 storage driver cannot run on an overlayfs mount:
```
failed to mount overlay: invalid argument
driver not supported: overlay2
```

Root cause: nested overlayfs requires that the upperdir is NOT on overlayfs. Our merged dir IS overlayfs, so Docker's overlay2 (which creates mounts inside `/var/lib/docker/overlay2/`) fails.

### Copy approach (WORKING)

Switched to `cp -a` of golden to session directory. Results:

| Metric | Value |
|--------|-------|
| Golden cache size | 8.7 GB (4 Docker images) |
| Golden copy time | **13.8 seconds** |
| Dockerd startup | First attempt, no errors |
| Pre-populated images | helix-haystack:dev (5.14GB), helix-api (619MB), helix-frontend (1.67GB), helix-typesense (996MB) |
| Session cleanup | Clean — session dir removed, golden intact |
| Cold build without golden | ~10 minutes |
| **Improvement** | **~43x faster cold start** |

### Bugs found and fixed during testing

1. **`ProjectID` not passed to `DesktopAgent`** — `spec_driven_task_service.go` created `DesktopAgent` without `ProjectID`, so Hydra always got empty project ID and `GoldenExists("")` returned false. Fixed in both `StartSpecGeneration` and `StartJustDoItMode`.

2. **Nested overlayfs fails** — Changed from overlayfs COW mount (`SetupGoldenOverlay`) to filesystem copy (`SetupGoldenCopy`). Renamed `CleanupGoldenOverlay` → `CleanupGoldenSession`. Removed `syscall` import.

3. **Golden builds started from scratch** — `!req.GoldenBuild` condition excluded golden builds from getting the golden copy, forcing a full cold build every time. Fixed: golden builds now start from the previous golden for incremental rebuilds (copy ~14s + only rebuild changed images).

4. **`DeleteDevContainer` destroyed golden build data** — Unconditionally called `CleanupSessionDockerDir` even for golden builds, deleting Docker data before `monitorGoldenBuild` could promote it. Fixed: skip cleanup for golden builds.

5. **`monitorGoldenBuild` had no timeout** — Used `context.Background()` indefinitely. Fixed: 30-minute `context.WithTimeout`.

### Incremental Golden Builds

Golden builds now use the previous golden as a starting point:

| Build | Duration | Notes |
|-------|----------|-------|
| First (no golden) | ~10 min | Full cold build + image transfer |
| Subsequent (from golden) | ~30s-2min | Copy previous golden (14s), only rebuild changed images |
| Normal session (from golden) | ~14s | Copy only, no build needed |

### Golden Build Status Tracking

`DockerCacheState` added to `ProjectMetadata` (JSONB, no migration):

```go
type DockerCacheState struct {
    Status         string     // "building", "ready", "failed", "none"
    SizeBytes      int64      // Golden cache size
    LastBuildAt    *time.Time // When current/last build started
    LastReadyAt    *time.Time // When golden was last promoted
    BuildSessionID string     // Session ID of running build (for debugging)
    Error          string     // Last error message
}
```

API-side goroutine polls session status every 15s to detect completion, then updates project metadata. This gives the frontend full visibility into golden build state.

---

## Staff Review: Architecture Gaps (2026-02-22)

### Critical: `DeleteDevContainer` destroys golden build data (Finding 1+2)

`devcontainer.go:963` calls `CleanupSessionDockerDir` unconditionally for ALL sessions, including golden builds. This deletes the Docker data before `monitorGoldenBuild` can promote it to golden.

**Race scenario**: Session reconciler or user clicks "stop" → `DeleteDevContainer` runs → deletes session Docker data → `monitorGoldenBuild` wakes up → tries to promote deleted data → error.

**Fix**: Skip `CleanupSessionDockerDir` for golden builds in `DeleteDevContainer`. Let `monitorGoldenBuild` handle promotion OR cleanup.

### Medium: `building` map never cleared on success (Finding 3)

`runGoldenBuild` never removes the `building[projectID]` entry after successfully launching the container. The 30-minute staleness check is the only recovery mechanism. If the golden build takes 5 minutes, the project is locked for the remaining 25 minutes — rapid merges silently drop golden builds.

**Fix**: Have `monitorGoldenBuild` call back to the API to clear the entry, or reduce the staleness timeout to match expected build duration.

### Medium: Golden build session visible to user, never cleaned from DB (Finding 6)

The golden build creates a real `Session` record with `Owner: project.UserID`. Users see "Docker Cache Warm-up: ..." in their session list. The session is never deleted after the build completes.

**Fix**: Delete or soft-delete the session in `monitorGoldenBuild` after promotion/cleanup. Or add a `Hidden` flag to sessions.

### Medium: Sandbox restart loses golden build state (Finding 9)

`RecoverDevContainersFromDocker` doesn't restore `IsGoldenBuild` or `ProjectID` on recovered containers. If sandbox restarts during a golden build, the monitoring goroutine is never re-launched, the lock file persists forever, and the Docker data leaks.

**Fix**: Store golden build metadata in container labels so it survives Docker-level recovery.

### Medium: No timeout on `monitorGoldenBuild` (Finding 7)

Uses `context.Background()` with no deadline. If the container hangs, the goroutine blocks forever.

**Fix**: Use `context.WithTimeout(ctx, 30*time.Minute)`.

### Low: `GoldenBuildRunning()` is dead code (Finding 4)

The file-based lock mechanism (`GoldenBuildRunning`/`SetGoldenBuildRunning`) is written to/cleared but never checked. The actual debouncing uses the in-memory `building` map in `GoldenBuildService` on the API side.

**Fix**: Either use the lock file for cross-process coordination (Hydra checks it) or remove it entirely.

### Low: Old golden `.old` dirs can leak (Finding 11)

`PromoteSessionToGolden` renames old golden to `.old` and deletes in a background goroutine. If deletion fails, `.old` dirs accumulate.

**Fix**: Check for and clean up `.old` dirs on Hydra startup.

---

## UI Design

### Where the setting lives

The setting belongs in the **Startup Script** section of Project Settings, NOT in Automations. Rationale:

1. Docker cache warming is directly tied to the startup script — the golden build runs the startup script to populate the cache
2. It only makes sense if the startup script uses Docker (builds images, runs compose, etc.)
3. Placing it under the startup script editor makes the relationship obvious

### Proposed UI

In `ProjectSettings.tsx`, below the `StartupScriptEditor` component within the same Paper section:

```
─────────────────────────────────────────────
Startup Script
─────────────────────────────────────────────
This script runs when an agent starts working
on this project. Use it to install dependencies,
start dev servers, etc.

┌─────────────────────────────────────────────┐
│  #!/bin/bash                                │
│  ./stack build                              │
│  ./stack start                              │
│                                             │
└─────────────────────────────────────────────┘
                            [History] [Test]

─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─

  Pre-warm Docker cache                   [ON]
  If your startup script builds Docker
  images, this keeps a warm cache so new
  sessions start in ~15 seconds instead
  of rebuilding from scratch. The cache
  updates automatically when code is
  merged to main.

  ┌─ Status ─────────────────────────────────┐
  │ ● Ready · 8.7 GB · Updated 2h ago       │
  │   Session: ses_01kj2cz8... [View]        │
  └──────────────────────────────────────────┘

─────────────────────────────────────────────
```

### Status display states

| `DockerCacheState.Status` | Visual | Details |
|---------------------------|--------|---------|
| `"none"` (no golden yet) | Gray dot | "No cache yet — will build on next merge" |
| `"building"` | Spinning indicator | "Building... Started 3 min ago [View] [Stop]" |
| `"ready"` | Green dot | "Ready · 8.7 GB · Updated 2h ago [Delete cache]" |
| `"failed"` | Red dot | "Failed: <error> [View session] [Retry]" |

### Inline session viewer

The **[View]** button opens a `DesktopStreamViewer` inline, identical to how "Test" works for startup scripts. This uses the existing `showTestSession` pattern:
- A second column appears alongside settings showing a 16:9 stream viewer
- Header: "Docker Cache Build" (distinct from "Test Session")
- Same `DesktopStreamViewer` component, using `DockerCacheState.BuildSessionID`
- User can watch the startup script running inside Ghostty in real time

This reuses the existing exploratory session infrastructure — the golden build IS a real desktop session, just with `HELIX_GOLDEN_BUILD=true`. The stream shows the same Ghostty terminal the user would see in a normal session.

Important: the golden build session is **separate** from the startup script test session. Both can exist simultaneously. The test session uses the exploratory session API; the golden build creates its own session with a different ID.

### Delete cache

A **[Delete cache]** button (shown when status is "ready" or "failed"):
- Calls `DELETE /api/v1/projects/{id}/docker-cache`
- API tells sandbox to `rm -rf /container-docker/golden/{projectID}/`
- Updates `DockerCacheState` to `{ status: "none" }`
- Useful when a bad dependency or stale artifact gets cached

### Retry

A **[Retry]** button (shown when status is "failed"):
- Triggers a new golden build immediately
- Same as toggling off and on, but without changing the setting

### Copy/labels

- **Toggle label**: "Pre-warm Docker cache"
- **Description**: "If your startup script builds Docker images, this keeps a warm cache so new sessions start in ~15 seconds instead of rebuilding from scratch. The cache updates automatically when code is merged to main."

### Behavior on toggle ON

1. Save `auto_warm_docker_cache: true` to project metadata
2. **Immediately trigger a golden build** — don't wait for a merge. The user wants the benefit now.
3. Show a brief toast: "Docker cache build started. This runs your startup script and takes a few minutes."
4. Status changes from "none" to "Building..."

### Behavior on toggle OFF

1. Save `auto_warm_docker_cache: false` to project metadata
2. Do NOT delete the existing golden (it's useful until it goes stale)
3. Stop triggering new golden builds on merge
4. Status remains showing last known state (e.g. "Ready · 8.7 GB")

### Disabled state

The toggle should be **disabled** if:
- No startup script is configured (empty `startupScript` state)

With helper text: "Add a startup script first — the Docker cache is built by running it."

### Files to modify

| File | Change |
|------|--------|
| `frontend/src/pages/ProjectSettings.tsx` | Add toggle + status + inline viewer in Startup Script section |
| `frontend/src/api/api.ts` | Regenerated via `./stack update_openapi` |
| `api/pkg/types/project.go` | Already has `AutoWarmDockerCache` + `DockerCacheState` |
| `api/pkg/server/project_handlers.go` | Handle update — trigger golden build on enable |
| `api/pkg/server/project_docker_cache_handlers.go` | **New** — DELETE endpoint for cache deletion |
| `api/pkg/hydra/golden.go` | Add `DeleteGolden(projectID)` function |
| `api/pkg/hydra/server.go` | Add Hydra endpoint to delete golden on sandbox |

---

## Process

1. Write plan to `design/2026-02-22-cold-start-optimization.md` before starting ✅
2. Step 1 benchmark: 10 min cold start, compose build is 41s (not dominant) ✅
3. Step 2 compose interception: implemented, 9.6s → 3.2s warm ✅
4. Step 3 golden cache: copy approach working, 13.8s for 8.7 GB (43x improvement) ✅
5. Staff review: 7 issues identified, critical ones fixed ✅
6. Golden builds now incremental (start from previous golden) ✅
7. Status tracking via `DockerCacheState` in project metadata ✅
8. Next: add UI toggle + status display, test golden build trigger end-to-end
9. All code in prototype branch — no merging until full review and testing
