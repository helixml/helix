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

## Critical: Analysis Order

**The generated API client (`frontend/src/api/api.ts`) contains ALL backend routes, not just the ones the frontend uses.** Therefore, we cannot simply compare backend routes against the generated client.

### Correct Order of Operations

```
┌─────────────────────────────────────────────────────────────────┐
│  STEP 1: Clean Frontend First                                   │
│  ─────────────────────────────────────────────────────────────  │
│  • Trace from router.tsx to find all accessible pages           │
│  • Build import graph from accessible pages                     │
│  • Identify components NOT reachable from navigation            │
│  • DELETE unreachable frontend code (including their API calls) │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  STEP 2: Map Actual API Calls from Clean Frontend               │
│  ─────────────────────────────────────────────────────────────  │
│  • Scan REMAINING frontend code for all API calls:              │
│    - React Query hooks (useQuery, useMutation)                  │
│    - Generated API client calls (apiClient.v1Xxx)               │
│    - Manual fetch/axios calls (should not exist, but check)     │
│    - Any custom hooks that wrap API calls                       │
│  • Output: List of backend routes actually called by frontend   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  STEP 3: Map CLI API Calls                                      │
│  ─────────────────────────────────────────────────────────────  │
│  • Scan api/pkg/cli/ for all API calls                          │
│  • Output: List of backend routes called by CLI                 │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  STEP 4: Identify Dead Backend Routes                           │
│  ─────────────────────────────────────────────────────────────  │
│  • Compare all backend routes against:                          │
│    - Frontend calls (from Step 2)                               │
│    - CLI calls (from Step 3)                                    │
│    - Runner-authenticated routes (external caller)              │
│    - Webhook routes (external caller)                           │
│    - WebSocket endpoints                                        │
│  • Routes with NO callers = DEAD                                │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  STEP 5: Trace Dead Backend Code                                │
│  ─────────────────────────────────────────────────────────────  │
│  • For each dead route, trace handler → called functions        │
│  • Identify functions ONLY called by dead routes                │
│  • Identify types ONLY used by dead code                        │
│  • This reveals entire chunks of dead backend code              │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  STEP 6: Remove Dead Backend Code                               │
│  ─────────────────────────────────────────────────────────────  │
│  • Remove dead routes                                           │
│  • Remove orphaned handlers                                     │
│  • Remove orphaned functions/types                              │
│  • Remove orphaned packages                                     │
└─────────────────────────────────────────────────────────────────┘
```

## Frontend API Call Patterns to Search

The frontend may call APIs in several ways. All must be traced:

### 1. React Query with Generated Client (correct pattern)
```typescript
const apiClient = api.getApiClient();
const { data } = useQuery({
  queryKey: ['sessions'],
  queryFn: () => apiClient.v1SessionsList().then(r => r.data)
});
```

### 2. React Query Mutations
```typescript
const mutation = useMutation({
  mutationFn: (data) => apiClient.v1SessionsCreate(data)
});
```

### 3. Direct API Client Calls (outside React Query)
```typescript
// May exist in event handlers, useEffect, etc.
await apiClient.v1SessionsResumeCreate(sessionId);
```

### 4. Manual fetch/axios Calls (should not exist, but check)
```typescript
// Bad pattern - should use generated client
await fetch('/api/v1/sessions');
await api.post('/api/v1/sessions', data);
```

### 5. Custom Hooks Wrapping API Calls
```typescript
// Check hooks in frontend/src/hooks/
function useSession(id) {
  return useQuery({
    queryFn: () => apiClient.v1SessionsRead(id)
  });
}
```

## Key Decisions

### Decision 1: Frontend Cleanup First
**Choice**: Delete unreachable frontend code BEFORE analyzing API usage.

**Rationale**: If we analyze API calls first, we'd include calls from dead frontend code, leading us to think those backend routes are needed when they're not.

### Decision 2: Trace Actual Calls, Not Generated Client
**Choice**: Grep/AST-parse the frontend code for actual API method invocations, not just compare against the generated client.

**Rationale**: The generated client contains methods for ALL backend routes. We need to find which methods are actually called by accessible frontend code.

### Decision 3: Definition of "Dead Code"
Code is dead if it cannot be reached from ANY entry point:
- Not called from HTTP routes that have callers
- Not called from CLI commands
- Not called from startup/initialization
- Not called from background jobs
- Not used via reflection (document separately)

Code is NOT dead if:
- Called by runners (runner-token authenticated)
- Called by webhooks (externally triggered)
- Called by scheduled jobs
- Used in tests (but test-only code should be in `_test.go`)

### Decision 4: Marking Convention
Each item in the codebase map gets one of:
- ✅ **KEEP** - Confirmed active, traced from entry point
- ⚠️ **REVIEW** - No direct trace found, may be reflection/dynamic
- ❌ **REMOVE** - Confirmed unreachable, safe to delete

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
| Manual API calls bypassing generated client | Grep for fetch/axios patterns, flag as tech debt |