# SpecTask Orchestrator Implementation Tasks

## Status: In Progress
**Started**: 2025-10-08
**Goal**: Complete SpecTask orchestrator with live fleet dashboard

---

## Phase 1: Design & Planning ✅
- [x] Capture complete vision in design document
- [x] Analyze existing SpecTask implementation
- [x] Analyze git repository service
- [x] Analyze agent dashboard
- [x] Create comprehensive architecture document

## Phase 2: Backend Core Components ✅
- [x] DesignDocsWorktreeManager service
  - [x] Create/setup git worktree for helix-design-docs branch
  - [x] Initialize design doc templates
  - [x] Parse progress.md task lists
  - [x] Mark tasks in-progress/complete with commits
- [x] SpecTaskOrchestrator service
  - [x] Main orchestration loop
  - [x] State machine for workflow transitions
  - [x] Task progress tracking
  - [x] Live progress broadcasting
- [x] ExternalAgentPool service
  - [x] Agent session allocation
  - [x] Agent reuse across Helix sessions
  - [x] Agent lifecycle management

## Phase 3: API Endpoints ✅
- [x] GET /api/v1/agents/fleet/live-progress
- [x] POST /api/v1/spec-tasks/from-demo
- [x] GET /api/v1/spec-tasks/{id}/design-docs
- [x] Routes registered in server.go

## Phase 4: Frontend Components ✅
- [x] LiveAgentFleetDashboard component
- [x] AgentTaskCard component
- [x] TaskListItem component with fade/highlight
- [x] Live progress polling (5s interval)
- [x] Demo repo selector integration in AgentDashboard
- [x] Added "Live Agent Fleet" tab to Fleet page

## Phase 5: Integration ✅
- [x] Connect orchestrator to SpecDrivenTaskService
- [x] Wire up agent pool to external agents
- [x] Initialize services in server startup
- [x] Register routes in server.go
- [x] Update OpenAPI spec and generate TypeScript client
- [x] Fix all compilation errors
- [x] Verify API builds successfully

## Phase 6: Testing & Polish 🔄
- [ ] Test complete workflow end-to-end
- [ ] Test parallel agent execution
- [ ] Verify design docs persistence
- [ ] Test demo repo flows
- [x] Commit and push changes regularly

---

## Current Task
**Working on**: Final integration testing and deployment

## Notes
- Using separate task tracker to avoid conflicts with parallel agent
- Implementing complete vision in one go as requested
- Focus on reusing existing SpecTask infrastructure where possible
