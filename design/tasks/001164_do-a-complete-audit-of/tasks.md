# Implementation Tasks

## Phase 1: Setup & Tooling

- [ ] Install Go analysis tools:
  ```bash
  go install golang.org/x/tools/cmd/deadcode@latest
  go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
  ```
- [ ] Install and run frontend tools:
  ```bash
  cd frontend
  npx ts-prune --project tsconfig.json > ../ts-prune-report.txt
  npx depcheck > ../depcheck-report.txt
  npx unimported > ../unimported-report.txt
  npx madge --circular --extensions ts,tsx src/ > ../circular-deps.txt
  ```
- [ ] Run baseline golangci-lint: `./script/clippy 2>&1 | tee lint-baseline.txt`

## Phase 2: Frontend Dead Code Cleanup (DO THIS FIRST)

### 2.1 Route & Navigation Analysis
- [ ] Extract all routes from `frontend/src/router.tsx`
- [ ] Map which routes are accessible from main navigation bar
- [ ] Map which routes are accessible via in-app links
- [ ] Identify routes that are only reachable via direct URL (no navigation path)
- [ ] Identify legacy/redirect routes

### 2.2 Build Component Reachability Graph
- [ ] Trace from `index.tsx` → `App.tsx` → `router.tsx` → page components
- [ ] For each of the 311+ components in `frontend/src/components/`:
  - Document who imports it
  - Document if it's reachable from any route
- [ ] Identify orphaned components (not imported by any reachable code)

### 2.3 Audit Hooks, Contexts, Utilities
- [ ] Map all hooks in `frontend/src/hooks/` and their usage
- [ ] Map all contexts in `frontend/src/contexts/` and their usage
- [ ] Map all utilities in `frontend/src/utils/` and `frontend/src/lib/`
- [ ] Identify unused hooks/contexts/utilities

### 2.4 Handle Duplicate Directories
- [ ] Audit `components/spec-tasks/` vs `components/specTask/` duplication
- [ ] Identify which is actively used, which is dead
- [ ] Document consolidation plan

### 2.5 DELETE Unreachable Frontend Code
- [ ] Remove orphaned page components
- [ ] Remove orphaned components
- [ ] Remove unused hooks/contexts/utilities
- [ ] Remove duplicate directories
- [ ] Run `cd frontend && yarn test && yarn build` to verify
- [ ] Create `design/codebase-map-frontend.md` documenting what was removed

## Phase 3: Map Actual API Calls from Clean Frontend

### 3.1 Find All API Call Patterns
- [ ] Grep for React Query hooks: `useQuery`, `useMutation`, `useInfiniteQuery`
- [ ] Grep for generated API client calls: `apiClient.v1` patterns
- [ ] Grep for manual fetch calls: `fetch('/api/`, `fetch(\`/api/`
- [ ] Grep for manual axios calls: `api.get`, `api.post`, `api.put`, `api.delete`
- [ ] Check custom hooks in `frontend/src/hooks/` for wrapped API calls

### 3.2 Extract API Endpoint List
- [ ] For each API call found, extract the endpoint being called
- [ ] Normalize to route format (e.g., `/api/v1/sessions/{id}`)
- [ ] Create list: `frontend-api-calls.txt`

### 3.3 Flag Bad Patterns
- [ ] Document any manual fetch/axios calls that bypass generated client
- [ ] These should be refactored to use React Query + generated client (side quest)

## Phase 4: Map CLI API Calls

- [ ] Scan all files in `api/pkg/cli/` for API calls
- [ ] Extract HTTP client calls and their endpoints
- [ ] Create list: `cli-api-calls.txt`

## Phase 5: Identify Dead Backend Routes

### 5.1 Extract All Backend Routes
- [ ] Parse `api/pkg/server/server.go` for all route registrations
- [ ] For each route, document:
  - Path and HTTP method
  - Handler function
  - Router type (authRouter, adminRouter, insecureRouter, subRouter, runnerRouter)

### 5.2 Categorize Routes by Caller Type
- [ ] Routes called by frontend (from Phase 3)
- [ ] Routes called by CLI (from Phase 4)
- [ ] Runner-authenticated routes (called by runners, NOT dead)
- [ ] Webhook routes (called externally, NOT dead)
- [ ] WebSocket endpoints (special handling)
- [ ] Insecure routes (public endpoints)

### 5.3 Identify Dead Routes
- [ ] Routes with NO callers from any category = DEAD
- [ ] Document each dead route with confidence level

## Phase 6: Trace Dead Backend Code

### 6.1 Handler Analysis
- [ ] For each dead route, identify its handler function
- [ ] Trace all functions called by the handler
- [ ] Identify functions ONLY called by dead handlers

### 6.2 Package Analysis
- [ ] Map all 50+ packages in `api/pkg/`
- [ ] Identify packages only used by dead code
- [ ] Identify exported functions never called

### 6.3 Type Analysis
- [ ] Identify types only used by dead code
- [ ] Identify types duplicated across packages

### 6.4 Create Backend Map
- [ ] Create `design/codebase-map-backend.md` with:
  - All routes and their status (KEEP/DEAD)
  - Package dependency graph
  - Dead functions/types identified

## Phase 7: Delete Dead Backend Code

- [ ] Remove dead route registrations from `server.go`
- [ ] Remove dead handler functions
- [ ] Remove functions only called by dead handlers
- [ ] Remove types only used by dead code
- [ ] Remove packages with no remaining usage
- [ ] Run `cd api && go test ./...` after each batch
- [ ] Run `./script/clippy` to verify

## Phase 8: Service & Build Artifact Audit

### 8.1 Service Inventory
- [ ] Audit each service directory for active usage:
  - `operator/` - Kubernetes operator
  - `haystack_service/` - Python RAG
  - `runner/` - Python fine-tuning
  - `runner-cmd/` - Go runner wrapper
  - `sandbox/` - container orchestration
  - `desktop/` - Rust gstreamer plugin
  - `demos/` - demo data service
  - `searxng/` - search configs
  - `tts-server/` - text-to-speech
  - `zed-config/` - Zed configs
  - `zed_integration/` - test file

### 8.2 Dockerfile Audit
- [ ] Map each Dockerfile to docker-compose and CI usage
- [ ] Identify unused Dockerfiles (e.g., `Dockerfile.hyprland-helix`?)

### 8.3 Script & Config Audit
- [ ] Map scripts in `scripts/` and root `*.sh`
- [ ] Audit `examples/` YAML files against current API
- [ ] Audit `charts/` Helm charts

### 8.4 Create Services/Build Map
- [ ] Create `design/codebase-map-services.md`
- [ ] Create `design/codebase-map-build.md`

## Phase 9: Type Consolidation

- [ ] List all types in `api/pkg/types/`
- [ ] Identify duplicates within Go codebase
- [ ] Compare with generated frontend types
- [ ] Create `design/type-consolidation.md`

## Phase 10: Code Complexity Report

- [ ] Run gocyclo: `gocyclo -over 15 api/`
- [ ] Document top 20 most complex functions
- [ ] Create `design/complexity-report.md`

## Phase 11: Final Cleanup & Verification

- [ ] Run full test suite: `cd api && go test ./...`
- [ ] Run frontend tests: `cd frontend && yarn test && yarn build`
- [ ] Run linter: `./script/clippy`
- [ ] Manual smoke test of main UI flows
- [ ] Create summary of lines/files removed
- [ ] Create `design/dead-code-removal-plan.md` with final state

## Deliverables Checklist

- [ ] `design/codebase-map-frontend.md` - Frontend cleanup documentation
- [ ] `design/codebase-map-backend.md` - Backend route/package analysis
- [ ] `design/codebase-map-services.md` - Service inventory
- [ ] `design/codebase-map-build.md` - Build artifact mapping
- [ ] `design/dead-code-removal-plan.md` - Master removal summary
- [ ] `design/type-consolidation.md` - Type overlap analysis
- [ ] `design/complexity-report.md` - Complexity findings
- [ ] PR(s) with dead code removed