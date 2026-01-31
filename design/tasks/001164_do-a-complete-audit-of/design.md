# Design: Complete Dead Code Audit

## Architecture Overview

The Helix codebase consists of:
- **Backend (Go)**: `api/` - main API server, CLI, types, services
- **Frontend (TypeScript/React)**: `frontend/src/` - 311+ components, router5-based routing
- **Supporting services**: `operator/`, `haystack_service/`, `runner/`, `sandbox/`
- **Desktop streaming**: `api/pkg/desktop/`, `api/pkg/external-agent/`

## Static Analysis Tools

### Go (Backend)
1. **golangci-lint** (already configured in `.golangci.yml`)
   - `unused` linter enabled - detects unused code
   - `deadcode` can be added for additional coverage
   
2. **go-deadcode** (golang.org/x/tools/cmd/deadcode)
   - Whole-program analysis for unreachable functions
   - Run: `go install golang.org/x/tools/cmd/deadcode@latest && deadcode ./...`

### TypeScript (Frontend)
1. **ts-prune** - finds unused exports
   - Run: `npx ts-prune --project tsconfig.json`

2. **depcheck** - finds unused dependencies in package.json
   - Run: `npx depcheck`

3. **Manual grep analysis** - verify component imports

## Key Decisions

### Decision 1: Route Analysis Approach
**Choice**: Cross-reference `server.go` routes with `frontend/src/api/api.ts` (generated client) and CLI commands.

**Rationale**: The generated API client is the source of truth for frontend calls. Any route not in the client AND not called by CLI is potentially dead.

### Decision 2: Frontend Component Analysis
**Choice**: Build import graph from `router.tsx` and trace all component dependencies.

**Rationale**: Router defines all reachable pages. Components not transitively imported from these pages are unreachable.

### Decision 3: Output Format
**Choice**: Create a codebase map in `design/codebase-map.md` documenting:
- All API routes and their callers
- All frontend routes and their components
- Identified dead code with removal recommendations

**Rationale**: User requested a "complete map" - this provides documentation value beyond just deletion.

### Decision 4: Type Analysis
**Choice**: Focus on `api/pkg/types/` and compare with `frontend/src/api/api.ts` generated types.

**Rationale**: Generated API types should be the single source of truth. Manual frontend type definitions that duplicate these are candidates for removal.

## Discovered Patterns

### Already Deprecated Code (found via grep)
- `settings-sync-daemon`: `DEPRECATED_FIELDS` for Zed settings
- `desktop/ws_input.go`: Deprecated touch/mouse handlers
- `desktop/ws_stream.go`: Deprecated SPS patching function
- `external-agent/executor.go`: `ContainerAppID` field marked deprecated
- `memory/estimate.go`: `EstimateGPULayers` deprecated

### Potentially Dead Directories
- `runner/` - Python scripts for fine-tuning (may be superseded)
- `zed_integration/` - single test file, may be obsolete
- `examples/` - YAML examples that may reference old API patterns
- `demos/` - separate demo service

### Frontend Component Clusters (311+ files)
- `components/app/` - App/Agent configuration (54 files)
- `components/spec-tasks/` and `components/specTask/` - duplicate naming!
- `components/finetune/` - fine-tuning UI (may be dead if feature removed)
- `components/fleet/` - runner fleet management

## Implementation Strategy

1. **Phase 1: Analysis** (generate the map)
   - Run static analysis tools
   - Build route-to-caller mapping
   - Build component import graph
   
2. **Phase 2: Review** (human verification)
   - Review generated map with stakeholders
   - Mark items as "remove" or "keep with reason"

3. **Phase 3: Removal** (surgical deletion)
   - Remove confirmed dead code in small, reviewable PRs
   - Run tests after each removal batch

## Risks

| Risk | Mitigation |
|------|------------|
| False positives (code used via reflection/dynamic calls) | Manual review before deletion |
| Breaking external consumers | Routes not in generated client may still be called externally - check for `insecureRouter` routes |
| Runner-token authenticated routes | These are called by runners, not frontend/CLI - exclude from "dead" analysis |