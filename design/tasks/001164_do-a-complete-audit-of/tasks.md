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

## Phase 2: Frontend Dead Code Analysis

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

### 2.5 Create Frontend Analysis Document
- [ ] Create `design/codebase-map-frontend.md` with all findings
- [ ] List all code marked for removal with justification

---

## 🛑 USER REVIEW CHECKPOINT 1: Frontend Dead Code

**STOP HERE and present findings to user before proceeding.**

Present:
- List of orphaned page components to remove
- List of orphaned components to remove  
- List of unused hooks/contexts/utilities to remove
- Duplicate directory consolidation plan

**Wait for user approval before proceeding to Phase 3.**

---

## Phase 3: Delete Approved Frontend Code

- [ ] Remove orphaned page components (as approved)
- [ ] Remove orphaned components (as approved)
- [ ] Remove unused hooks/contexts/utilities (as approved)
- [ ] Remove/consolidate duplicate directories (as approved)
- [ ] Run `cd frontend && yarn test && yarn build` to verify
- [ ] Update `design/codebase-map-frontend.md` with what was removed

## Phase 4: Map Actual API Calls from Clean Frontend

### 4.1 Find All API Call Patterns
- [ ] Grep for React Query hooks: `useQuery`, `useMutation`, `useInfiniteQuery`
- [ ] Grep for generated API client calls: `apiClient.v1` patterns
- [ ] Grep for manual fetch calls: `fetch('/api/`, `fetch(\`/api/`
- [ ] Grep for manual axios calls: `api.get`, `api.post`, `api.put`, `api.delete`
- [ ] Check custom hooks in `frontend/src/hooks/` for wrapped API calls

### 4.2 Extract API Endpoint List
- [ ] For each API call found, extract the endpoint being called
- [ ] Normalize to route format (e.g., `/api/v1/sessions/{id}`)
- [ ] Create list: `frontend-api-calls.txt`

### 4.3 Flag Bad Patterns
- [ ] Document any manual fetch/axios calls that bypass generated client
- [ ] These should be refactored to use React Query + generated client (side quest)

## Phase 5: Map CLI API Calls

- [ ] Scan all files in `api/pkg/cli/` for API calls
- [ ] Extract HTTP client calls and their endpoints
- [ ] Create list: `cli-api-calls.txt`

## Phase 6: Identify Dead Backend Routes

### 6.1 Extract All Backend Routes
- [ ] Parse `api/pkg/server/server.go` for all route registrations
- [ ] For each route, document:
  - Path and HTTP method
  - Handler function
  - Router type (authRouter, adminRouter, insecureRouter, subRouter, runnerRouter)

### 6.2 Categorize Routes by Caller Type
- [ ] Routes called by frontend (from Phase 4)
- [ ] Routes called by CLI (from Phase 5)
- [ ] Runner-authenticated routes (called by runners, NOT dead)
- [ ] Webhook routes (called externally, NOT dead)
- [ ] WebSocket endpoints (special handling)
- [ ] Insecure routes (public endpoints)

### 6.3 Identify Dead Routes
- [ ] Routes with NO callers from any category = DEAD
- [ ] Document each dead route with confidence level

## Phase 7: Trace Dead Backend Code

### 7.1 Handler Analysis
- [ ] For each dead route, identify its handler function
- [ ] Trace all functions called by the handler
- [ ] Identify functions ONLY called by dead handlers

### 7.2 Package Analysis
- [ ] Map all 50+ packages in `api/pkg/`
- [ ] Identify packages only used by dead code
- [ ] Identify exported functions never called

### 7.3 Type Analysis
- [ ] Identify types only used by dead code
- [ ] Identify types duplicated across packages

### 7.4 Create Backend Analysis Document
- [ ] Create `design/codebase-map-backend.md` with:
  - All routes and their status (KEEP/DEAD)
  - Package dependency graph
  - Dead functions/types identified

---

## 🛑 USER REVIEW CHECKPOINT 2: Backend Dead Code

**STOP HERE and present findings to user before proceeding.**

Present:
- List of dead routes to remove
- List of dead handler functions to remove
- List of functions only called by dead handlers
- List of types only used by dead code
- List of packages to remove entirely

**Wait for user approval before proceeding to Phase 8.**

---

## Phase 8: Delete Approved Backend Code

- [ ] Remove dead route registrations from `server.go` (as approved)
- [ ] Remove dead handler functions (as approved)
- [ ] Remove functions only called by dead handlers (as approved)
- [ ] Remove types only used by dead code (as approved)
- [ ] Remove packages with no remaining usage (as approved)
- [ ] Run `cd api && go test ./...` to verify
- [ ] Run `./script/clippy` to verify

## Phase 9: Service & Build Artifact Audit

### 9.1 Service Inventory
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

### 9.2 Dockerfile Audit
- [ ] Map each Dockerfile to docker-compose and CI usage
- [ ] Identify unused Dockerfiles (e.g., `Dockerfile.hyprland-helix`?)

### 9.3 Script & Config Audit
- [ ] Map scripts in `scripts/` and root `*.sh`
- [ ] Audit `examples/` YAML files against current API
- [ ] Audit `charts/` Helm charts

### 9.4 Create Services/Build Analysis Documents
- [ ] Create `design/codebase-map-services.md`
- [ ] Create `design/codebase-map-build.md`

---

## 🛑 USER REVIEW CHECKPOINT 3: Services & Build Artifacts

**STOP HERE and present findings to user before proceeding.**

Present:
- List of services recommended for removal
- List of unused Dockerfiles
- List of unused scripts
- List of outdated example/config files

**Wait for user approval before proceeding to Phase 10.**

---

## Phase 10: Delete Approved Services & Build Artifacts

- [ ] Remove obsolete service directories (as approved)
- [ ] Remove unused Dockerfiles (as approved)
- [ ] Remove unused scripts (as approved)
- [ ] Remove outdated configs/examples (as approved)
- [ ] Verify docker-compose files still work
- [ ] Verify CI still passes

## Phase 11: Type Consolidation

- [ ] List all types in `api/pkg/types/`
- [ ] Identify duplicates within Go codebase
- [ ] Compare with generated frontend types
- [ ] Create `design/type-consolidation.md`

## Phase 12: Code Complexity Report

- [ ] Run gocyclo: `gocyclo -over 15 api/`
- [ ] Document top 20 most complex functions
- [ ] Create `design/complexity-report.md`

## Phase 13: Final Verification

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
- [ ] Single branch with all dead code removed