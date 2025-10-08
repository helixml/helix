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

## Phase 2: Backend Core Components 🔄
- [x] DesignDocsWorktreeManager service
  - [x] Create/setup git worktree for helix-design-docs branch
  - [x] Initialize design doc templates
  - [x] Parse progress.md task lists
  - [x] Mark tasks in-progress/complete with commits
- [ ] SpecTaskOrchestrator service
  - [ ] Main orchestration loop
  - [ ] State machine for workflow transitions
  - [ ] Task progress tracking
  - [ ] Live progress broadcasting
- [x] ExternalAgentPool service
  - [x] Agent session allocation
  - [x] Agent reuse across Helix sessions
  - [x] Agent lifecycle management

## Phase 3: API Endpoints
- [ ] GET /api/v1/agents/fleet/live-progress
- [ ] POST /api/v1/spec-tasks/from-demo
- [ ] GET /api/v1/spec-tasks/{id}/design-docs
- [ ] PUT /api/v1/spec-tasks/{id}/design-docs

## Phase 4: Frontend Components
- [ ] LiveAgentFleetDashboard component
- [ ] AgentTaskCard component
- [ ] TaskListItem component with fade/highlight
- [ ] Live progress polling hook
- [ ] Demo repo selector integration

## Phase 5: Integration
- [ ] Connect orchestrator to SpecDrivenTaskService
- [ ] Wire up agent pool to external agents
- [ ] Setup worktree on SpecTask creation
- [ ] Parse and broadcast progress from commits
- [ ] Handle errors and retries

## Phase 6: Testing & Polish
- [ ] Test complete workflow end-to-end
- [ ] Test parallel agent execution
- [ ] Verify design docs persistence
- [ ] Test demo repo flows
- [ ] Commit and push all changes

---

## Current Task
**Working on**: Backend Core Components - DesignDocsWorktreeManager

## Notes
- Using separate task tracker to avoid conflicts with parallel agent
- Implementing complete vision in one go as requested
- Focus on reusing existing SpecTask infrastructure where possible
