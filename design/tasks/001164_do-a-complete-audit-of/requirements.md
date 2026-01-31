# Requirements: Complete Dead Code Audit

## Overview
Perform a comprehensive audit of the **entire** Helix codebase to create a detailed map of all code, identify dead/unreachable code at every level, and remove it. This includes the main API, frontend, CLI, and all supporting services.

## User Stories

### US1: Complete Backend Code Map
**As a** maintainer  
**I want** a detailed map of all backend Go code and its dependencies  
**So that** I understand what code is actually used vs dead

**Acceptance Criteria:**
- [ ] Map all packages in `api/pkg/` and their internal dependencies
- [ ] Trace from entry points: `main.go`, server startup, CLI commands
- [ ] Identify code reached via routes, startup initialization, background jobs, etc.
- [ ] Mark functions/types/files that are never called from any entry point
- [ ] Include code used via reflection or dynamic dispatch (document separately)

### US2: Complete Frontend Code Map
**As a** maintainer  
**I want** a detailed map of all frontend code and its dependencies  
**So that** I understand what components/utilities are actually used

**Acceptance Criteria:**
- [ ] Map all routes in `frontend/src/router.tsx` and their page components
- [ ] Build full import graph from entry point (`index.tsx`, `App.tsx`)
- [ ] Map all 311+ components and trace which are reachable
- [ ] Identify orphaned components, hooks, utilities, contexts
- [ ] Identify unused exports in each file
- [ ] Mark files/exports that can be removed

### US3: All Services Audit
**As a** maintainer  
**I want** all supporting services mapped and audited  
**So that** I know which services are actively used vs obsolete

**Services to audit:**
- [ ] `operator/` - Kubernetes operator
- [ ] `haystack_service/` - Python RAG service
- [ ] `runner/` - Python fine-tuning scripts
- [ ] `runner-cmd/` - Go runner command
- [ ] `sandbox/` - Container sandbox code
- [ ] `desktop/` - Desktop streaming (Rust gstreamer plugin, configs)
- [ ] `demos/` - Demo service
- [ ] `searxng/` - Search integration
- [ ] `tts-server/` - Text-to-speech
- [ ] `zed-config/` - Zed configuration
- [ ] `zed_integration/` - Zed integration tests

**Acceptance Criteria:**
- [ ] Document purpose and current status of each service
- [ ] Identify services that are actively built/deployed
- [ ] Identify services that are obsolete or superseded
- [ ] Mark entire services that can be removed

### US4: Build Artifacts & Docker Images
**As a** maintainer  
**I want** all Dockerfiles and build scripts mapped  
**So that** I know which build artifacts are used

**Acceptance Criteria:**
- [ ] Map all Dockerfiles and what they build
- [ ] Cross-reference with `docker-compose*.yaml` files
- [ ] Cross-reference with CI/CD (`.drone.yml`, `cloudbuild.yaml`)
- [ ] Identify Dockerfiles not used in any compose/CI config
- [ ] Identify build scripts (`scripts/`, root `*.sh`) and their usage

### US5: Configuration & Example Files
**As a** maintainer  
**I want** unused config and example files removed  
**So that** the repo is lean and examples are current

**Acceptance Criteria:**
- [ ] Audit `examples/` YAML files against current API
- [ ] Audit config files (`.yaml`, `.json`, `.toml`) for relevance
- [ ] Identify orphaned migration files, test fixtures, sample data
- [ ] Mark files that reference deprecated APIs or patterns

### US6: Type Consolidation
**As a** maintainer  
**I want** overlapping types consolidated  
**So that** there's a single source of truth

**Acceptance Criteria:**
- [ ] Map all types in `api/pkg/types/` 
- [ ] Identify duplicate/overlapping type definitions
- [ ] Compare backend types with generated frontend types (`api/api.ts`)
- [ ] Identify manual frontend types that duplicate generated ones
- [ ] Document consolidation plan

### US7: Code Complexity Report
**As a** maintainer  
**I want** overly complex code identified  
**So that** it can be prioritized for refactoring

**Acceptance Criteria:**
- [ ] Run cyclomatic complexity analysis on Go code
- [ ] Identify functions with complexity > 15
- [ ] Identify deeply nested code (> 4 levels)
- [ ] Document top candidates for simplification (not implement)

## Deliverables

The audit must produce these map documents in `design/`:

1. **`codebase-map-overview.md`** - High-level architecture and service inventory
2. **`codebase-map-backend.md`** - Detailed Go package/function dependency graph
3. **`codebase-map-frontend.md`** - Detailed component/hook/utility dependency graph
4. **`codebase-map-services.md`** - Status of each supporting service
5. **`codebase-map-build.md`** - Dockerfiles, scripts, CI/CD mapping
6. **`dead-code-removal-plan.md`** - Consolidated list of all code marked for removal
7. **`type-consolidation.md`** - Type overlap analysis
8. **`complexity-report.md`** - Code complexity findings

Each map document must clearly mark items as:
- ✅ **KEEP** - Actively used
- ⚠️ **REVIEW** - Potentially dead, needs human verification
- ❌ **REMOVE** - Confirmed dead, safe to delete

## Out of Scope
- Actual refactoring of complex code (only identification)
- Performance optimization
- Security audit
- External API consumers we can't trace (document as risk)