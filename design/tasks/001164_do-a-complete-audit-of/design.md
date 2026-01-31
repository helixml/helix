# Design: Complete Dead Code Audit

## Architecture Overview

The Helix monorepo contains multiple services and components:

### Core Services
| Directory | Language | Purpose | Build Artifact |
|-----------|----------|---------|----------------|
| `api/` | Go | Main API server, CLI, all business logic | `helix` binary |
| `frontend/` | TypeScript/React | Web UI | Static assets |
| `operator/` | Go | Kubernetes operator for Helix deployment | `helix-operator` |
| `haystack_service/` | Python | RAG/document processing service | Docker image |
| `runner/` | Python | Fine-tuning scripts (axolotl, SDXL) | Used by runner image |
| `runner-cmd/` | Go | Runner command wrapper | Part of runner image |

### Desktop/Sandbox Services
| Directory | Language | Purpose |
|-----------|----------|---------|
| `sandbox/` | Shell/Config | Container sandbox orchestration |
| `desktop/` | Rust/Config | GStreamer plugin for video streaming |
| `api/pkg/desktop/` | Go | Desktop streaming server (WebSocket H.264) |
| `api/pkg/hydra/` | Go | Dev container lifecycle management |
| `api/pkg/external-agent/` | Go | Zed agent communication |

### Supporting Services
| Directory | Language | Purpose | Status |
|-----------|----------|---------|--------|
| `demos/` | Go | Demo data generator service | Active |
| `searxng/` | Config | SearXNG search integration | Active |
| `tts-server/` | Unknown | Text-to-speech | Needs audit |
| `zed-config/` | Config | Zed IDE configuration templates | Active |
| `zed_integration/` | Go | Integration tests for Zed | Needs audit |

### Build Artifacts
| Dockerfile | Purpose | Used By |
|------------|---------|---------|
| `Dockerfile` | Main API image | docker-compose.yaml |
| `Dockerfile.runner` | Runner image | docker-compose.runner.yaml |
| `Dockerfile.sandbox` | Sandbox container | docker-compose.dev.yaml |
| `Dockerfile.ubuntu-helix` | Desktop Ubuntu image | Sandbox |
| `Dockerfile.sway-helix` | Desktop Sway image | Sandbox |
| `Dockerfile.zed-build` | Zed IDE build | Desktop images |
| `Dockerfile.qwen-code-build` | Qwen Code agent build | Desktop images |
| `Dockerfile.demos` | Demo service | docker-compose.demos.yaml |
| `Dockerfile.typesense` | Search service | Unknown |
| `Dockerfile.lint` | Linting container | CI |
| `Dockerfile.hyprland-helix` | Hyprland desktop | Needs audit |

## Static Analysis Tools

### Go (Backend)

1. **golangci-lint** (already configured)
   - `unused` linter - detects unused code
   - `deadcode` - additional dead code detection
   ```bash
   ./script/clippy
   ```

2. **go-deadcode** (whole-program analysis)
   ```bash
   go install golang.org/x/tools/cmd/deadcode@latest
   cd api && deadcode -test ./...
   ```

3. **go-callvis** (call graph visualization)
   ```bash
   go install github.com/ofabry/go-callvis@latest
   go-callvis -group pkg ./api/...
   ```

4. **gocyclo** (complexity analysis)
   ```bash
   go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
   gocyclo -over 15 api/
   ```

### TypeScript (Frontend)

1. **ts-prune** (unused exports)
   ```bash
   cd frontend && npx ts-prune --project tsconfig.json
   ```

2. **depcheck** (unused npm packages)
   ```bash
   cd frontend && npx depcheck
   ```

3. **madge** (dependency graph)
   ```bash
   cd frontend && npx madge --circular --extensions ts,tsx src/
   ```

4. **unimported** (unused files)
   ```bash
   cd frontend && npx unimported
   ```

### General

1. **tokei** (lines of code by language)
   ```bash
   tokei .
   ```

2. **Custom scripts** for cross-referencing routes with API client

## Mapping Strategy

### Phase 1: Entry Point Identification

**Backend Entry Points:**
- `main.go` → `api/cmd/` commands
- `api/pkg/server/server.go` → HTTP routes
- `api/pkg/cli/` → CLI commands
- Background goroutines started in `NewServer()`
- Scheduled jobs via `scheduler`
- WebSocket handlers
- Webhook handlers

**Frontend Entry Points:**
- `index.tsx` → `App.tsx` → `router.tsx`
- All routes in `router.tsx` are reachable pages

**Service Entry Points:**
- Each Dockerfile's `ENTRYPOINT`/`CMD`
- docker-compose service definitions

### Phase 2: Dependency Graph Building

For each entry point, trace all dependencies:

```
Entry Point
    └── Package A
        ├── Function A1 (used)
        │   └── Type T1 (used)
        └── Function A2 (unused - no callers)
    └── Package B
        └── ...
```

### Phase 3: Cross-Reference Analysis

**Backend routes ↔ Frontend API client:**
- Parse `api/pkg/server/server.go` for all route definitions
- Parse `frontend/src/api/api.ts` (generated) for all API methods
- Identify routes not called by frontend or CLI

**Backend routes ↔ CLI:**
- Parse `api/pkg/cli/` for API calls
- Match against route definitions

**Backend routes ↔ External callers:**
- `insecureRouter` routes (webhooks, callbacks)
- Runner-authenticated routes
- WebSocket endpoints
- Document these as "externally called"

### Phase 4: Service Dependency Mapping

```
docker-compose.yaml
    └── api (Dockerfile)
        └── depends on: postgres, vectorchord
    └── frontend (proxied by api in dev)
    └── ...

docker-compose.dev.yaml
    └── sandbox-nvidia (Dockerfile.sandbox)
        └── pulls: helix-ubuntu, helix-sway
    └── ...
```

## Key Decisions

### Decision 1: Definition of "Dead Code"
Code is dead if it cannot be reached from ANY entry point:
- Not called from HTTP routes
- Not called from CLI commands
- Not called from startup/initialization
- Not called from background jobs
- Not used via reflection (document separately)

Code is NOT dead if:
- Called by runners (runner-token authenticated)
- Called by webhooks (externally triggered)
- Called by scheduled jobs
- Used in tests (but test-only code should be in `_test.go`)

### Decision 2: Marking Convention
Each item in the codebase map gets one of:
- ✅ **KEEP** - Confirmed active, traced from entry point
- ⚠️ **REVIEW** - No direct trace found, may be reflection/dynamic
- ❌ **REMOVE** - Confirmed unreachable, safe to delete

### Decision 3: Removal Strategy
1. Generate complete map first (don't delete anything)
2. Human review of all ⚠️ REVIEW items
3. Small, focused PRs for removal (one service/package at a time)
4. Run full test suite after each removal

## Discovered Patterns

### Already Deprecated (found via grep)
- `settings-sync-daemon`: `DEPRECATED_FIELDS` for Zed settings
- `desktop/ws_input.go`: Deprecated touch/mouse handlers  
- `desktop/ws_stream.go`: Deprecated SPS patching function
- `external-agent/executor.go`: `ContainerAppID` field deprecated
- `memory/estimate.go`: `EstimateGPULayers` deprecated
- `cli/spectask/spectask.go`: `getContainerAppID` deprecated

### Duplicate Naming (needs consolidation)
- `frontend/src/components/spec-tasks/` AND `frontend/src/components/specTask/`

### Potential Dead Services (needs verification)
- `tts-server/` - No references found in main codebase
- `zed_integration/` - Single test file only
- `Dockerfile.hyprland-helix` - May be superseded by sway/ubuntu

### Large Component Counts (need pruning)
- `frontend/src/components/` - 311+ files
- `api/pkg/` - 50+ packages

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| False positives from reflection | Manual review, search for string-based function calls |
| External API consumers | Document all public routes, consider deprecation period |
| Runner-authenticated routes | Trace from runner codebase separately |
| Webhook endpoints | Mark as externally-called, don't remove |
| Test-only code | Allow `_test.go` files, but audit test utilities |
| Build-time only code | Trace from Dockerfiles and CI configs |