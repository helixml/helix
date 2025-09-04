# Implementation Plan: SpecTask Multi-Session Architecture

## Overview

This document outlines the implementation plan for extending the existing SpecTask system to support sophisticated multi-session workflows. The existing SpecTask system already provides Kiro-style spec-driven development with two agents (planning + implementation). We're adding multi-session coordination during the implementation phase.

## Current Foundation: SpecTask System ✅

**Already implemented and working:**
- Two-phase workflow: Planning agent → Human approval → Implementation agent
- Human approval gates for specifications
- Agent specialization (planning vs implementation)
- Status tracking from backlog to done
- Simple, human-readable specifications in markdown
- Integration with existing Helix session management

## Architecture Extension: Multi-Session Implementation

**What we're adding:**
- Multiple work sessions per SpecTask during implementation phase
- One Zed instance per SpecTask with multiple threads
- Session spawning and coordination during implementation
- Shared project context across all work sessions in a SpecTask

## Implementation Phases

### Phase 1: GORM Models and Database (Week 1)

#### 1.1 New GORM Models ✅ (Partially Complete)
- [x] `SpecTaskWorkSession` - Individual work units within SpecTask
- [x] `SpecTaskZedThread` - Zed thread mappings for work sessions  
- [x] `SpecTaskImplementationTask` - Parsed implementation tasks
- [x] Extend existing `SpecTask` with multi-session fields
- [x] Extend existing `Session` with SpecTask context in metadata

**Files completed:**
- ✅ `helix/api/pkg/types/spec_task_multi_session.go`
- ✅ `helix/api/pkg/types/simple_spec_task.go` (extended)
- ✅ `helix/api/pkg/types/types.go` (SessionMetadata extended)

#### 1.2 Store Interface Extensions ✅ (Complete)
- [x] Add multi-session methods to Store interface
- [x] CRUD operations for all new entities
- [x] Complex queries for overview and progress tracking

**Files completed:**
- ✅ `helix/api/pkg/store/store.go` (interface extended)

#### 1.3 PostgreSQL Implementation ✅ (Complete)
- [x] Implement all store methods for new entities
- [x] Add implementation plan parsing logic
- [x] Add multi-session creation and coordination
- [x] Add GORM automigrate updates

**Files completed:**
- ✅ `helix/api/pkg/store/store_spec_task_multi_session.go`
- ✅ `helix/api/pkg/store/postgres.go` (automigrate updated)

#### 1.4 Service Layer ✅ (Complete)
- [x] `SpecTaskMultiSessionManager` for coordinating work sessions
- [x] Integration with existing `SpecDrivenTaskService`
- [x] Work session lifecycle management
- [x] Zed instance and thread management

**Files completed:**
- ✅ `helix/api/pkg/services/spec_task_multi_session_manager.go`
- ✅ `helix/api/pkg/services/spec_driven_task_service.go` (extended)

#### 1.5 API Handlers ✅ (Complete)
- [x] Multi-session endpoints for SpecTask management
- [x] Work session CRUD and status management
- [x] Zed thread integration endpoints
- [x] Progress and overview endpoints

**Files completed:**
- ✅ `helix/api/pkg/server/spec_task_multi_session_handlers.go`
- ✅ `helix/api/pkg/server/server.go` (routes added)

#### 1.6 Testing ✅ (Basic Complete)
- [x] Unit tests for multi-session manager
- [x] Test coverage for major workflows
- [x] Mock-based testing approach

**Files completed:**
- ✅ `helix/api/pkg/services/spec_task_multi_session_manager_test.go`

### Phase 2: Zed Infrastructure Integration (Week 2) ✅ COMPLETE

#### 2.1 Update Existing Zed Agent Integration ✅
- [x] Modified `agent_session_manager.go` to handle SpecTask context
- [x] Updated Zed agent launcher to work with SpecTask-level instances
- [x] Added SpecTask ID and work session context to Zed agent protocol
- [x] Ensured existing single-session Zed workflows continue working

**Files completed:**
- ✅ `helix/api/pkg/controller/agent_session_manager.go`
- ✅ `helix/api/pkg/external-agent/executor.go`
- ✅ `helix/api/pkg/pubsub/zed_protocol.go`

#### 2.2 Multi-Session Zed Coordination ✅
- [x] Implemented one Zed instance per SpecTask model
- [x] Created multiple threads within single Zed instance
- [x] Mapped each work session to specific Zed thread
- [x] Handled Zed instance lifecycle (create, manage, cleanup)

#### 2.3 Communication Protocol Extensions ✅
- [x] Extended Zed-Helix protocol with complete message types
- [x] Added SpecTask ID for instance-level coordination
- [x] Added work session ID for thread-level routing
- [x] Added implementation task context
- [x] Added multi-session status reporting

#### 2.4 Zed-to-Helix Session Creation ✅
- [x] Implemented automatic Helix session creation when Zed threads are created
- [x] Added `ZedToHelixSessionService` for reverse flow management
- [x] Added context propagation and validation
- [x] Enabled dynamic session spawning from Zed interface

### Phase 3: Git Integration and Spec Document Management (Week 3)

#### 3.1 Git-Based Spec Document Handoff ✅
- [x] Implemented Kiro-style spec document generation (requirements.md, design.md, tasks.md)
- [x] Added `SpecDocumentService` for git integration
- [x] Created EARS notation conversion for requirements
- [x] Added automatic commit and branch management
- [x] Integrated with SpecTask approval workflow

**Files completed:**
- ✅ `helix/api/pkg/services/spec_document_service.go`

**Key features:**
- **requirements.md**: User stories with EARS notation
- **design.md**: Technical architecture and multi-session context
- **tasks.md**: Implementation plan with trackable tasks
- **spec-metadata.json**: Tooling integration metadata

#### 3.2 Session Context and Coordination ✅
- [x] Implemented `SessionContextService` for inter-session coordination
- [x] Added coordination event logging and management
- [x] Created shared state management across work sessions
- [x] Added session spawning notifications and tracking

**Files completed:**
- ✅ `helix/api/pkg/services/session_context_service.go`
- ✅ `helix/api/pkg/services/zed_to_helix_session_service.go`

#### 3.3 SpecTask Workflow Integration ✅
- [x] Updated spec approval flow to trigger multi-session implementation
- [x] Integrated all services with existing `SpecDrivenTaskService`
- [x] Maintained backward compatibility for simple implementations
- [x] Added git document generation on spec approval

### Phase 4: User Interface Implementation (Week 4) ❌ NOT IMPLEMENTED

#### 4.1 Multi-Session SpecTask Dashboard (Required)
- [ ] **SpecTask Overview Page**: Real-time progress visualization
  - Work session hierarchy tree (parent/child relationships)
  - Zed integration status with direct links
  - Coordination timeline showing inter-session communication
  - Session spawning controls for creating new work sessions
- [ ] **Progress Visualization**: Interactive progress tracking
  - Overall SpecTask progress (% complete)
  - Individual work session progress
  - Implementation task completion status
  - Real-time activity indicators

#### 4.2 Work Session Management Interface (Required)
- [ ] **Session Detail Pages**: Comprehensive session information
  - Session status and progress tracking
  - Activity timeline with real-time updates
  - Related session navigation (parent/child links)
  - Coordination controls for inter-session communication
- [ ] **Session Spawning Interface**: Modal for creating new sessions
  - Session name and description input
  - Agent type selection (Zed vs Helix)
  - Configuration options (environment, priority)
  - Dependency management and relationship linking

#### 4.3 Zed Integration Monitoring (Required)
- [ ] **Zed Instance Dashboard**: Instance health and monitoring
  - Real-time resource monitoring (CPU, memory, disk)
  - Thread activity visualization with file change tracking
  - Direct Zed integration with deep links
  - Instance control buttons for management
- [ ] **Session Coordination Center**: Real-time coordination dashboard
  - Active coordination events with action buttons
  - Visual session relationship mapping
  - Notification center for important events
  - Quick action controls for emergency situations

**UI Framework Requirements:**
- WebSocket integration for real-time updates
- React/Vue components for session visualization
- State management for complex session hierarchies
- Mobile-responsive design for developer workflows

### Phase 5: Testing and Validation (Week 5)

#### 5.1 Backend Integration Testing ✅
- [x] End-to-end tests with SpecTask workflows
- [x] Multi-session coordination testing
- [x] Service integration validation
- [x] Database schema and migration testing

#### 5.2 Backward Compatibility Validation
- [ ] Ensure existing SpecTask workflows work unchanged
- [ ] Validate single-session implementation still works
- [ ] Test existing Zed integration scenarios
- [ ] Verify no breaking changes to API contracts

#### 5.3 Error Handling and Edge Cases
- [ ] Test Zed disconnection scenarios
- [ ] Handle partial implementation completion
- [ ] Test work session spawning edge cases
- [ ] Validate proper cleanup of failed sessions

### Phase 6: Git Integration and Documentation (Week 6)

#### 6.1 Automatic Session History Recording
- [ ] **Session History as Markdown**: Record all session/thread interactions in git
  - Conversation logs saved as markdown files per session
  - Code changes and file modifications tracked
  - Decision points and reasoning documented
  - Coordination events and handoffs recorded
- [ ] **Git-Based Documentation**: Living documentation updated with progress
  - Automatic updates to tasks.md with completion status
  - Session activity logs committed to repository
  - Branch management for feature development tracking
  - Pull request integration with review workflows

#### 6.2 Enhanced Git Integration
- [ ] **Real-time Spec Updates**: Update spec documents as implementation progresses
  - Progress commits to tasks.md with completion percentages
  - Design updates when architecture decisions are made
  - Requirements refinement based on implementation learnings
  - Traceability links between specs and actual code changes
- [ ] **Branch Management**: Sophisticated git workflow integration
  - Feature branches per SpecTask with proper naming
  - Automatic merge conflict resolution for spec documents
  - Integration with existing git workflows and CI/CD
  - Tag management for spec versions and milestones

## Future Roadmap (Beyond Phase 6)

### Phase 7: Advanced Workflow Features (Month 2)

#### 7.1 Session Dependencies and Workflow Templates
- [ ] **Complex Dependencies**: Advanced dependency management between work sessions
  - Conditional dependencies based on implementation outcomes
  - Parallel execution with synchronization points
  - Dependency visualization and management tools
  - Automatic blocking and unblocking based on completion
- [ ] **Workflow Templates**: Predefined templates for common development patterns
  - Feature development templates (frontend + backend + testing)
  - Bug fix workflows (investigation + fix + validation)
  - Refactoring templates (analysis + implementation + testing)
  - Custom workflow creation and sharing

#### 7.2 Advanced Coordination Features
- [ ] **Real-time Collaboration**: Enhanced coordination between sessions
  - Shared whiteboards and design documents
  - Real-time code review within session contexts
  - Pair programming coordination across sessions
  - Knowledge sharing and decision documentation
- [ ] **AI-Powered Coordination**: Intelligent session management
  - Automatic work session spawning based on code analysis
  - Smart dependency detection and management
  - Predictive resource allocation and scheduling
  - Intelligent merge conflict resolution

### Phase 8: Enterprise Features (Month 3+)

#### 8.1 External System Integration
- [ ] **GitHub Integration**: Deep integration with GitHub workflows
  - Automatic issue creation from SpecTask implementation tasks
  - Pull request management and review coordination
  - GitHub Actions integration for testing and deployment
  - Issue linking and traceability
- [ ] **Jira/Project Management**: Integration with project management tools
  - Automatic ticket creation and status synchronization
  - Epic and story management coordination
  - Sprint planning integration with SpecTask timelines
  - Resource allocation and capacity planning

#### 8.2 Advanced Analytics and Reporting
- [ ] **Development Metrics**: Comprehensive analytics on development workflows
  - Code quality metrics across sessions
  - Development velocity and efficiency tracking
  - Resource utilization optimization
  - Bottleneck identification and resolution
- [ ] **Predictive Analytics**: AI-powered insights for development planning
  - Effort estimation improvement based on historical data
  - Risk prediction for complex SpecTasks
  - Optimal session allocation recommendations
  - Timeline prediction and adjustment

### Phase 9: Innovation Features (Month 4+)

#### 9.1 Advanced AI Coordination
- [ ] **Cross-Session Learning**: AI agents learn from each other's work
  - Pattern recognition across similar SpecTasks
  - Best practice identification and sharing
  - Code style and architecture consistency
  - Automated code review and suggestions
- [ ] **Intelligent Task Breakdown**: Dynamic task decomposition
  - Real-time task refinement based on implementation progress
  - Automatic sub-task creation for complex problems
  - Adaptive planning based on emerging requirements
  - Context-aware work session spawning

#### 9.2 Developer Experience Innovation
- [ ] **Immersive Development**: Advanced developer experience features
  - 3D visualization of session hierarchies and dependencies
  - Voice control for session management and coordination
  - AR/VR integration for complex system visualization
  - Natural language session spawning and coordination
- [ ] **Autonomous Development**: Semi-autonomous development capabilities
  - Self-organizing work sessions for complex SpecTasks
  - Automatic testing and validation workflows
  - Intelligent code refactoring and optimization
  - Autonomous documentation generation and maintenance

## Architecture Summary

### Core Flow
```
1. User creates SpecTask → Planning agent generates specs
2. Human approves specs → Implementation phase begins
3. SpecTaskMultiSessionManager:
   a. Parses implementation plan into discrete tasks
   b. Creates work sessions for each implementation task
   c. Creates single Zed instance for SpecTask (if using Zed agent)
   d. Creates Zed threads (1:1 with work sessions)
   e. Starts implementation sessions with proper context
4. Work sessions execute in parallel within shared Zed instance
5. Sessions can spawn additional work sessions as needed
6. Infrastructure tracks progress and handles coordination
7. SpecTask marked complete when all work sessions done
```

### Key Principles
- **Build on proven SpecTask foundation** - No reinventing wheels
- **Infrastructure-level coordination** - Not agent tools
- **One Zed instance per SpecTask** - Shared project context
- **One Zed thread per work session** - Clear 1:1 mapping
- **Backward compatibility** - Existing workflows unchanged
- **Agent type determines behavior** - Not artificial work types

## Success Criteria

### Phase 1 Complete ✅
- [x] All GORM models compile and migrate successfully
- [x] Store methods work correctly
- [x] API endpoints respond properly
- [x] Basic service integration functions
- [x] Unit tests pass

### Phase 2 Success
- [ ] SpecTask approval triggers multi-session implementation
- [ ] Zed instance created per SpecTask
- [ ] Multiple Zed threads created within instance
- [ ] Work sessions map correctly to Zed threads

### Phase 3 Success  
- [ ] Complete end-to-end workflow from SpecTask to implementation
- [ ] Work sessions can be spawned during implementation
- [ ] Session coordination works properly
- [ ] Existing single-session workflows unaffected

### Phase 4 Success
- [ ] System tested and validated in realistic scenarios
- [ ] Performance acceptable under load
- [ ] Error handling robust and graceful
- [ ] Documentation complete and accurate

## Current Status

**Phase 1: COMPLETE ✅**
All database models, store methods, services, and API handlers implemented.

**Next: Phase 2 - Zed Infrastructure Integration**
Focus on updating the existing Zed agent integration to work with SpecTask-level instances and work session coordination.

## Files Implemented ✅

1. **Database Models:**
   - `helix/api/pkg/types/spec_task_multi_session.go`
   - `helix/api/pkg/types/simple_spec_task.go` (extended)
   - `helix/api/pkg/types/types.go` (SessionMetadata extended)

2. **Store Layer:**
   - `helix/api/pkg/store/store.go` (interface extended)
   - `helix/api/pkg/store/store_spec_task_multi_session.go`
   - `helix/api/pkg/store/postgres.go` (automigrate updated)

3. **Service Layer:**
   - `helix/api/pkg/services/spec_task_multi_session_manager.go`
   - `helix/api/pkg/services/spec_driven_task_service.go` (extended)

4. **API Layer:**
   - `helix/api/pkg/server/spec_task_multi_session_handlers.go`
   - `helix/api/pkg/server/server.go` (routes added)

5. **Testing:**
   - `helix/api/pkg/services/spec_task_multi_session_manager_test.go`

## Next Implementation Steps

1. **Update Zed Agent Integration** (Phase 2.1)
   - Modify `launchZedAgent` to handle SpecTask context
   - Add SpecTask-level instance management
   - Update communication protocol for multi-threading

2. **Test End-to-End Flow** (Phase 2.2)
   - Create test SpecTask with complex implementation plan
   - Verify multi-session creation works
   - Test Zed instance and thread creation

3. **Add Missing Infrastructure** (Phase 2.3)
   - Session spawning from running work sessions
   - Proper session lifecycle management
   - Status synchronization between systems

The foundation is solid - now we need to connect it to the Zed infrastructure properly.