# Implementation Tasks

## Phase 1: Setup & Analysis Tools

- [ ] Install Go deadcode analyzer: `go install golang.org/x/tools/cmd/deadcode@latest`
- [ ] Run existing golangci-lint to capture current warnings: `./script/clippy 2>&1 | tee lint-baseline.txt`
- [ ] Run Go deadcode analysis: `cd api && deadcode ./... 2>&1 | tee ../deadcode-report.txt`
- [ ] Install and run ts-prune for frontend: `cd frontend && npx ts-prune --project tsconfig.json > ../ts-prune-report.txt`
- [ ] Run depcheck for unused npm packages: `cd frontend && npx depcheck > ../depcheck-report.txt`

## Phase 2: Backend Route Analysis

- [ ] Extract all routes from `api/pkg/server/server.go` into a list
- [ ] Parse `frontend/src/api/api.ts` to identify which API endpoints are called
- [ ] Scan `api/pkg/cli/` to identify CLI-called endpoints
- [ ] Cross-reference to identify routes with no frontend/CLI callers
- [ ] Document runner-authenticated routes (called by runners, not dead)
- [ ] Document webhook routes (called externally, not dead)
- [ ] Create `design/codebase-map-backend-routes.md` with findings

## Phase 3: Frontend Route & Component Analysis

- [ ] Extract all routes from `frontend/src/router.tsx`
- [ ] Build component import graph starting from router page components
- [ ] Identify page components not reachable from any route
- [ ] Identify components not imported by any reachable page
- [ ] Check for duplicate component directories (`spec-tasks/` vs `specTask/`)
- [ ] Create `design/codebase-map-frontend.md` with findings

## Phase 4: Unused Files Analysis

- [ ] Find Go files not imported by any package in the module
- [ ] Find TypeScript/TSX files not imported anywhere
- [ ] Review `runner/` directory for obsolete Python scripts
- [ ] Review `examples/` YAML files for outdated API patterns
- [ ] Review `zed_integration/` for obsolete test files
- [ ] Check for orphaned Dockerfiles (not referenced in docker-compose or CI)
- [ ] Check for orphaned shell scripts in root and `scripts/`

## Phase 5: Type Consolidation Analysis

- [ ] List all types in `api/pkg/types/` directory
- [ ] Compare with generated types in `frontend/src/api/api.ts`
- [ ] Identify duplicate type definitions across Go files
- [ ] Identify manual TypeScript types that duplicate generated API types
- [ ] Document consolidation opportunities in `design/type-consolidation.md`

## Phase 6: Code Complexity Analysis

- [ ] Run gocyclo or similar for Go cyclomatic complexity: `go install github.com/fzipp/gocyclo/cmd/gocyclo@latest && gocyclo -over 15 api/`
- [ ] Identify deeply nested functions (>4 levels)
- [ ] Document top 20 most complex functions for future refactoring
- [ ] Note any obvious simplification opportunities

## Phase 7: Dead Code Removal

- [ ] Remove confirmed dead backend routes (after review approval)
- [ ] Remove confirmed dead frontend pages/components
- [ ] Remove unused Go files
- [ ] Remove unused TypeScript files
- [ ] Remove unused config/script files
- [ ] Remove deprecated code past its deprecation period
- [ ] Clean up duplicate component directories

## Phase 8: Verification

- [ ] Run full test suite: `cd api && go test ./...`
- [ ] Run frontend tests: `cd frontend && yarn test`
- [ ] Run frontend build: `cd frontend && yarn build`
- [ ] Run golangci-lint and verify no new errors
- [ ] Manual smoke test of main UI flows
- [ ] Update `design/codebase-map.md` with final state

## Deliverables

- [ ] `design/codebase-map-backend-routes.md` - API route analysis
- [ ] `design/codebase-map-frontend.md` - Frontend component analysis
- [ ] `design/type-consolidation.md` - Type overlap analysis
- [ ] `design/complexity-report.md` - Code complexity findings
- [ ] PR(s) removing dead code with itemized changes