# Implementation Tasks

## Phase 1: Setup & Tooling

- [ ] Install Go analysis tools:
  ```bash
  go install golang.org/x/tools/cmd/deadcode@latest
  go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
  go install github.com/ofabry/go-callvis@latest
  ```
- [ ] Run baseline golangci-lint: `./script/clippy 2>&1 | tee lint-baseline.txt`
- [ ] Run Go deadcode analysis: `cd api && deadcode -test ./... 2>&1 | tee ../deadcode-report.txt`
- [ ] Run gocyclo complexity: `gocyclo -over 15 api/ 2>&1 | tee complexity-report.txt`
- [ ] Install and run frontend tools:
  ```bash
  cd frontend
  npx ts-prune --project tsconfig.json > ../ts-prune-report.txt
  npx depcheck > ../depcheck-report.txt
  npx unimported > ../unimported-report.txt
  npx madge --circular --extensions ts,tsx src/ > ../circular-deps.txt
  ```

## Phase 2: Backend Code Mapping

### 2.1 Entry Point Analysis
- [ ] Document all CLI commands in `api/pkg/cli/` and what packages they import
- [ ] Document `main.go` and `api/cmd/` entry points
- [ ] Map `api/pkg/server/NewServer()` initialization and all dependencies
- [ ] Map background goroutines started during server initialization
- [ ] Map scheduled jobs via `scheduler` package

### 2.2 Route Analysis
- [ ] Extract ALL routes from `api/pkg/server/server.go` into structured list:
  - Route path
  - HTTP method
  - Handler function
  - Router type (authRouter, adminRouter, insecureRouter, subRouter, runnerRouter)
- [ ] For each handler, trace all called functions recursively
- [ ] Document which routes are runner-authenticated (external caller)
- [ ] Document which routes are webhooks (external caller)

### 2.3 Package Dependency Graph
- [ ] Map all 50+ packages in `api/pkg/`
- [ ] For each package, document:
  - Exported functions/types
  - Which packages import it
  - Whether it's reachable from any entry point
- [ ] Identify packages with no importers
- [ ] Identify exported functions never called externally

### 2.4 Backend Map Document
- [ ] Create `design/codebase-map-backend.md` with:
  - Package hierarchy
  - Route-to-handler-to-function traces
  - Marked dead code (functions, types, files)

## Phase 3: Frontend Code Mapping

### 3.1 Route Analysis
- [ ] Extract all routes from `frontend/src/router.tsx`
- [ ] Map each route to its page component
- [ ] Identify legacy/redirect routes

### 3.2 Component Import Graph
- [ ] Build import graph starting from `index.tsx` → `App.tsx` → `router.tsx`
- [ ] Trace all 311+ components in `frontend/src/components/`
- [ ] For each component, document:
  - Who imports it
  - Is it reachable from router
- [ ] Identify orphaned components (no importers from reachable code)

### 3.3 Hooks, Contexts, Utilities
- [ ] Map all hooks in `frontend/src/hooks/`
- [ ] Map all contexts in `frontend/src/contexts/`
- [ ] Map all utilities in `frontend/src/utils/` and `frontend/src/lib/`
- [ ] Identify unused hooks/contexts/utilities

### 3.4 Duplicate/Overlapping Directories
- [ ] Audit `components/spec-tasks/` vs `components/specTask/` duplication
- [ ] Identify any other duplicate component directories
- [ ] Document consolidation plan

### 3.5 Frontend Map Document
- [ ] Create `design/codebase-map-frontend.md` with:
  - Route → page → component hierarchy
  - Full component dependency graph
  - Marked dead components/hooks/utilities

## Phase 4: Service Inventory

### 4.1 Core Services
- [ ] Audit `api/` - main API (definitely active)
- [ ] Audit `frontend/` - web UI (definitely active)
- [ ] Audit `operator/` - Kubernetes operator, verify if actively maintained
- [ ] Audit `haystack_service/` - Python RAG service, verify usage

### 4.2 Runner/Sandbox Services
- [ ] Audit `runner/` - Python fine-tuning scripts, check if superseded
- [ ] Audit `runner-cmd/` - Go runner wrapper
- [ ] Audit `sandbox/` - container orchestration
- [ ] Audit `desktop/` - Rust gstreamer plugin, active for streaming

### 4.3 Supporting Services
- [ ] Audit `demos/` - demo data service
- [ ] Audit `searxng/` - search integration configs
- [ ] Audit `tts-server/` - text-to-speech, check if used
- [ ] Audit `zed-config/` - Zed configuration templates
- [ ] Audit `zed_integration/` - single test file, likely obsolete

### 4.4 Services Map Document
- [ ] Create `design/codebase-map-services.md` with:
  - Service inventory with status (ACTIVE/OBSOLETE/UNKNOWN)
  - Dependencies between services
  - Recommended removals

## Phase 5: Build & Config Mapping

### 5.1 Dockerfile Audit
- [ ] Map all Dockerfiles to their purpose:
  - `Dockerfile` - main API
  - `Dockerfile.runner` - runner
  - `Dockerfile.sandbox` - sandbox
  - `Dockerfile.ubuntu-helix` - desktop Ubuntu
  - `Dockerfile.sway-helix` - desktop Sway
  - `Dockerfile.zed-build` - Zed build
  - `Dockerfile.qwen-code-build` - Qwen agent
  - `Dockerfile.demos` - demos
  - `Dockerfile.typesense` - search
  - `Dockerfile.lint` - CI linting
  - `Dockerfile.hyprland-helix` - Hyprland desktop (check if used)
- [ ] Cross-reference with docker-compose files
- [ ] Cross-reference with CI configs (`.drone.yml`, `cloudbuild.yaml`)
- [ ] Identify unused Dockerfiles

### 5.2 Script Audit
- [ ] Map all scripts in `scripts/` directory
- [ ] Map root-level scripts (`stack`, `*.sh`)
- [ ] Identify scripts not called by any CI/compose/docs

### 5.3 Config File Audit
- [ ] Audit `examples/` YAML files - do they work with current API?
- [ ] Audit `charts/` Helm charts - are they maintained?
- [ ] Audit `.github/` workflows
- [ ] Audit various config files (`.golangci.yml`, `tsconfig.json`, etc.)

### 5.4 Build Map Document
- [ ] Create `design/codebase-map-build.md` with:
  - Dockerfile → compose → CI mapping
  - Script inventory with usage status
  - Config file relevance

## Phase 6: Type Consolidation Analysis

- [ ] List all types in `api/pkg/types/` (30+ files)
- [ ] Compare with generated types in `frontend/src/api/api.ts`
- [ ] Identify duplicate type definitions across Go files
- [ ] Identify manual frontend types duplicating generated API types
- [ ] Create `design/type-consolidation.md` with consolidation plan

## Phase 7: Code Complexity Analysis

- [ ] Parse gocyclo output for functions with complexity > 15
- [ ] Identify deeply nested code (> 4 indentation levels)
- [ ] Document top 20 most complex functions
- [ ] Create `design/complexity-report.md`

## Phase 8: Master Dead Code List

- [ ] Consolidate all findings into `design/dead-code-removal-plan.md`:
  - Backend: dead routes, functions, packages, files
  - Frontend: dead components, hooks, utilities, files
  - Services: obsolete services
  - Build: unused Dockerfiles, scripts, configs
- [ ] Categorize each item as:
  - ✅ KEEP (with reason)
  - ⚠️ REVIEW (needs human verification)
  - ❌ REMOVE (safe to delete)

## Phase 9: Dead Code Removal

- [ ] Create removal PRs in logical batches:
  - PR 1: Remove confirmed dead backend routes/handlers
  - PR 2: Remove confirmed dead backend packages/functions
  - PR 3: Remove confirmed dead frontend components
  - PR 4: Remove confirmed dead frontend utilities/hooks
  - PR 5: Remove obsolete services/directories
  - PR 6: Remove unused Dockerfiles/scripts/configs
  - PR 7: Consolidate duplicate directories (spec-tasks)
- [ ] Each PR must pass:
  - `cd api && go test ./...`
  - `cd frontend && yarn test && yarn build`
  - `./script/clippy`

## Phase 10: Verification & Documentation

- [ ] Run full test suite after all removals
- [ ] Manual smoke test of main UI flows
- [ ] Update architecture documentation if needed
- [ ] Create summary of what was removed (lines of code, file counts)
- [ ] Archive generated analysis reports

## Deliverables Checklist

- [ ] `design/codebase-map-overview.md` - High-level architecture
- [ ] `design/codebase-map-backend.md` - Go package/function graph
- [ ] `design/codebase-map-frontend.md` - Component dependency graph
- [ ] `design/codebase-map-services.md` - Service inventory
- [ ] `design/codebase-map-build.md` - Build artifact mapping
- [ ] `design/dead-code-removal-plan.md` - Master removal list
- [ ] `design/type-consolidation.md` - Type overlap analysis
- [ ] `design/complexity-report.md` - Complexity findings
- [ ] PR(s) removing dead code with itemized changes