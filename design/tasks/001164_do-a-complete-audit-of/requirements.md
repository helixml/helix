# Requirements: Complete Dead Code Audit

## Overview
Perform a comprehensive audit of the Helix codebase to identify and remove dead code, unreachable frontend components, unused files, and consolidate overlapping types.

## User Stories

### US1: Dead Backend Routes
**As a** maintainer  
**I want** all unused API routes removed  
**So that** the codebase is cleaner and easier to maintain

**Acceptance Criteria:**
- [ ] Map all API routes defined in `api/pkg/server/server.go`
- [ ] Identify which routes are called by frontend (via generated API client)
- [ ] Identify which routes are called by CLI (`api/pkg/cli/`)
- [ ] Document and remove routes with no callers

### US2: Dead Frontend Code
**As a** maintainer  
**I want** unreachable frontend components removed  
**So that** bundle size decreases and code is navigable

**Acceptance Criteria:**
- [ ] Map all routes in `frontend/src/router.tsx`
- [ ] Verify each page component is reachable from navigation
- [ ] Identify orphaned components not imported anywhere
- [ ] Remove unused pages and components

### US3: Unused Files
**As a** maintainer  
**I want** unused files of all types removed  
**So that** the repository is lean

**Acceptance Criteria:**
- [ ] Identify unused Go files (via static analysis)
- [ ] Identify unused TypeScript/TSX files
- [ ] Identify unused config files, scripts, Dockerfiles
- [ ] Remove or document reason to keep each file

### US4: Type Consolidation
**As a** maintainer  
**I want** overlapping types consolidated  
**So that** there's a single source of truth

**Acceptance Criteria:**
- [ ] Identify duplicate type definitions in `api/pkg/types/`
- [ ] Identify frontend types that duplicate backend types
- [ ] Consolidate where appropriate (frontend should use generated API types)

### US5: Code Simplification
**As a** maintainer  
**I want** overly complex code identified  
**So that** it can be refactored

**Acceptance Criteria:**
- [ ] Identify functions with high cyclomatic complexity
- [ ] Identify deeply nested code
- [ ] Document candidates for refactoring (implementation deferred)

## Out of Scope
- Actual refactoring of complex code (only identification)
- Removing deprecated code still in active deprecation period
- Changes to external integrations (operator, haystack_service)