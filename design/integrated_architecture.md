# Integrated Zed-Helix Architecture: Building on SpecTask

## Overview

This architecture integrates the sophisticated multi-session Zed-Helix workflow with the existing `SpecTask` system, which already implements Kiro-style spec-driven development. Instead of creating parallel systems, we extend the proven `SpecTask` foundation to support complex multi-session workflows.

## Current Foundation: SpecTask System

The existing system already provides:
- **Two-phase workflow**: Spec generation (Helix agent) → Implementation (Zed agent)
- **Human approval gates**: Specs must be reviewed and approved
- **Agent specialization**: Different agents for planning vs implementation
- **Status tracking**: Full lifecycle from backlog to done
- **Simple artifacts**: Human-readable specs in markdown

## Architecture Integration

### Core Principle: SpecTask + Multi-Session Execution

```
SpecTask ("Implement user authentication")
├── Planning Phase
│   ├── Planning Agent (existing SpecAgent field)
│   ├── Single Helix Session (existing SpecSessionID)
│   └── Outputs: RequirementsSpec, TechnicalDesign, ImplementationPlan
├── Approval Gate (existing human review process)
└── Implementation Phase
    ├── Implementation Agent (existing ImplementationAgent field)
    ├── Zed Instance (one per SpecTask)
    │   ├── WorkSession 1 → Zed Thread 1 ("Backend API")
    │   ├── WorkSession 2 → Zed Thread 2 ("Frontend UI") 
    │   └── WorkSession 3 → Zed Thread 3 ("Testing")
    └── Multiple Helix Sessions (1:1 with Zed threads)
```

## Database Schema Extensions

### Extend Existing SpecTask
```sql
-- Add multi-session support to existing spec_tasks table
ALTER TABLE spec_tasks 
ADD COLUMN zed_instance_id VARCHAR(255),
ADD COLUMN project_path VARCHAR(500),
ADD COLUMN workspace_config JSONB;

CREATE INDEX idx_spec_tasks_zed_instance ON spec_tasks(zed_instance_id);
```

### New Tables for Multi-Session Support
```sql
-- Work sessions within a spec task (implementation phase)
CREATE TABLE spec_task_work_sessions (
    id VARCHAR(255) PRIMARY KEY,
    spec_task_id VARCHAR(255) NOT NULL REFERENCES spec_tasks(id) ON DELETE CASCADE,
    helix_session_id VARCHAR(255) NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    
    -- Work session details
    name VARCHAR(255),
    description TEXT,
    phase VARCHAR(50) NOT NULL, -- 'planning', 'implementation'
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    
    -- Implementation task context (parsed from ImplementationPlan)
    implementation_task_title VARCHAR(255),
    implementation_task_description TEXT,
    implementation_task_index INTEGER, -- Order within the plan
    
    -- Relationships for spawning/branching
    parent_work_session_id VARCHAR(255) REFERENCES spec_task_work_sessions(id),
    spawned_by_session_id VARCHAR(255) REFERENCES spec_task_work_sessions(id),
    
    -- Configuration
    agent_config JSONB,
    environment_config JSONB,
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    
    UNIQUE(helix_session_id), -- 1:1 mapping
    INDEX idx_spec_work_sessions_spec_task (spec_task_id),
    INDEX idx_spec_work_sessions_phase (phase)
);

-- Zed thread mappings for implementation work sessions
CREATE TABLE spec_task_zed_threads (
    id VARCHAR(255) PRIMARY KEY,
    work_session_id VARCHAR(255) NOT NULL REFERENCES spec_task_work_sessions(id) ON DELETE CASCADE,
    spec_task_id VARCHAR(255) NOT NULL REFERENCES spec_tasks(id) ON DELETE CASCADE,
    zed_thread_id VARCHAR(255) NOT NULL,
    
    -- Thread-specific configuration
    thread_config JSONB,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    last_activity_at TIMESTAMP,
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    UNIQUE(work_session_id), -- 1:1 mapping work session to thread
    UNIQUE(spec_task_id, zed_thread_id), -- Unique thread ID within spec task
    INDEX idx_spec_zed_threads_spec_task (spec_task_id)
);
```

## Enhanced SpecTask Lifecycle

### Phase 1: Planning (Existing, Enhanced)
```
1. User creates SpecTask with prompt
2. Planning agent (SpecAgent) generates specs in single session (SpecSessionID)
3. Human reviews and approves (existing approval flow)
4. ImplementationPlan parsed into discrete implementation tasks
```

### Phase 2: Implementation (New Multi-Session)
```
1. SpecTask status changes to 'implementation_queued'
2. System creates:
   a. Single Zed instance for the SpecTask
   b. Multiple WorkSessions based on ImplementationPlan
   c. ZedThread mappings for each WorkSession
3. Implementation agent coordinates across multiple threads
4. Progress tracked against original specification
```

## Key Components

### SpecTaskWorkSessionManager
```go
type SpecTaskWorkSessionManager struct {
    store      store.Store
    controller *controller.Controller
}

// Creates work sessions from approved implementation plan
func (m *SpecTaskWorkSessionManager) CreateImplementationSessions(
    ctx context.Context, 
    specTask *types.SpecTask,
) ([]*SpecTaskWorkSession, error)

// Spawns additional work sessions during implementation
func (m *SpecTaskWorkSessionManager) SpawnWorkSession(
    ctx context.Context,
    parentSessionID string,
    description string,
) (*SpecTaskWorkSession, error)
```

### ZedInstanceManager
```go
type ZedInstanceManager struct {
    store      store.Store
    controller *controller.Controller
}

// Creates single Zed instance for a SpecTask
func (m *ZedInstanceManager) CreateZedInstance(
    ctx context.Context,
    specTask *types.SpecTask,
) (string, error) // Returns zed instance ID

// Creates new thread within existing instance
func (m *ZedInstanceManager) CreateZedThread(
    ctx context.Context,
    workSession *SpecTaskWorkSession,
) (*SpecTaskZedThread, error)
```

## Go Types (Extensions)

```go
// Extend existing SpecTask
type SpecTask struct {
    // ... existing fields ...
    
    // Multi-session support
    ZedInstanceID     string                   `json:"zed_instance_id,omitempty" gorm:"size:255;index"`
    ProjectPath       string                   `json:"project_path,omitempty" gorm:"size:500"`
    WorkspaceConfig   datatypes.JSON           `json:"workspace_config,omitempty" gorm:"type:jsonb"`
    
    // Relationships (loaded via joins)
    WorkSessions      []SpecTaskWorkSession    `json:"work_sessions,omitempty" gorm:"foreignKey:SpecTaskID"`
    ZedThreads        []SpecTaskZedThread      `json:"zed_threads,omitempty" gorm:"foreignKey:SpecTaskID"`
}

// New types for multi-session support
type SpecTaskWorkSession struct {
    ID                          string `json:"id" gorm:"primaryKey;size:255"`
    SpecTaskID                  string `json:"spec_task_id" gorm:"not null;size:255;index"`
    HelixSessionID              string `json:"helix_session_id" gorm:"not null;size:255;uniqueIndex"`
    
    Name                        string `json:"name,omitempty" gorm:"size:255"`
    Description                 string `json:"description,omitempty" gorm:"type:text"`
    Phase                       string `json:"phase" gorm:"not null;size:50;index"` // 'planning', 'implementation'
    Status                      string `json:"status" gorm:"not null;size:50;default:pending;index"`
    
    // Implementation context
    ImplementationTaskTitle     string `json:"implementation_task_title,omitempty" gorm:"size:255"`
    ImplementationTaskDescription string `json:"implementation_task_description,omitempty" gorm:"type:text"`
    ImplementationTaskIndex     int    `json:"implementation_task_index,omitempty"`
    
    // Relationships
    ParentWorkSessionID string `json:"parent_work_session_id,omitempty" gorm:"size:255;index"`
    SpawnedBySessionID  string `json:"spawned_by_session_id,omitempty" gorm:"size:255;index"`
    
    // Configuration
    AgentConfig         datatypes.JSON `json:"agent_config,omitempty" gorm:"type:jsonb"`
    EnvironmentConfig   datatypes.JSON `json:"environment_config,omitempty" gorm:"type:jsonb"`
    
    CreatedAt           time.Time  `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP"`
    StartedAt           *time.Time `json:"started_at,omitempty"`
    CompletedAt         *time.Time `json:"completed_at,omitempty"`
    
    // Relationships
    SpecTask            *SpecTask            `json:"spec_task,omitempty" gorm:"foreignKey:SpecTaskID"`
    HelixSession        *Session             `json:"helix_session,omitempty" gorm:"foreignKey:HelixSessionID"`
    ZedThread           *SpecTaskZedThread   `json:"zed_thread,omitempty" gorm:"foreignKey:WorkSessionID"`
}

type SpecTaskZedThread struct {
    ID               string     `json:"id" gorm:"primaryKey;size:255"`
    WorkSessionID    string     `json:"work_session_id" gorm:"not null;size:255;uniqueIndex"`
    SpecTaskID       string     `json:"spec_task_id" gorm:"not null;size:255;index"`
    ZedThreadID      string     `json:"zed_thread_id" gorm:"not null;size:255;index"`
    
    ThreadConfig     datatypes.JSON `json:"thread_config,omitempty" gorm:"type:jsonb"`
    Status           string     `json:"status" gorm:"not null;size:50;default:pending;index"`
    LastActivityAt   *time.Time `json:"last_activity_at,omitempty"`
    
    CreatedAt        time.Time  `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP"`
    
    // Relationships
    WorkSession      *SpecTaskWorkSession `json:"work_session,omitempty" gorm:"foreignKey:WorkSessionID"`
    SpecTask         *SpecTask            `json:"spec_task,omitempty" gorm:"foreignKey:SpecTaskID"`
}
```

## API Extensions

### SpecTask Multi-Session Endpoints
```go
// Existing SpecTask endpoints continue to work
POST /api/v1/spec-tasks/from-prompt
GET /api/v1/spec-tasks/{id}
POST /api/v1/spec-tasks/{id}/approve-specs

// New multi-session endpoints
POST /api/v1/spec-tasks/{id}/implementation-sessions  // Create work sessions from plan
GET /api/v1/spec-tasks/{id}/work-sessions             // List work sessions
POST /api/v1/spec-tasks/{id}/work-sessions/{sessionId}/spawn // Spawn new session
GET /api/v1/spec-tasks/{id}/zed-instance              // Get Zed instance info
```

## Implementation Benefits

### 1. Builds on Proven Foundation
- Leverages existing SpecTask workflow that users understand
- Maintains existing API compatibility
- Proven two-phase approach with human approval gates

### 2. Gradual Enhancement
- Existing simple SpecTasks continue to work unchanged
- Multi-session capability added incrementally
- No breaking changes to current workflows

### 3. Natural Workflow Integration
- Planning phase remains single-session (focused spec generation)
- Implementation phase becomes multi-session (parallel coding work)
- Human review gates prevent runaway automation

### 4. Resource Efficiency
- One Zed instance per SpecTask (shared project context)
- Multiple threads within instance (parallel work streams)
- Subprocess work happens naturally within Zed threads

## Example Scenarios

### Simple SpecTask (Existing Behavior)
```
SpecTask: "Add user profile page"
├── Planning: Single Helix session generates simple spec
├── Approval: Human reviews and approves
└── Implementation: Single Zed thread implements feature
```

### Complex SpecTask (New Multi-Session)
```
SpecTask: "Implement authentication system"
├── Planning: Single Helix session generates comprehensive spec
├── Approval: Human reviews multi-part implementation plan
└── Implementation: Single Zed instance, multiple threads:
    ├── Thread 1: "Database schema migration"
    ├── Thread 2: "Backend API endpoints" 
    ├── Thread 3: "Frontend login components"
    ├── Thread 4: "Integration tests"
    └── Thread 5: "Security audit" (spawned during implementation)
```

### Interactive SpecTask (Advanced)
```
SpecTask: "Performance optimization" (ongoing)
├── Planning: Analysis and optimization plan
├── Approval: Strategy approved
└── Implementation: Adaptive work sessions:
    ├── Thread 1: "Database query optimization" 
    ├── Thread 2: "Frontend bundle optimization" (spawned)
    ├── Thread 3: "Cache implementation" (spawned)
    └── Thread 4: "Load testing validation" (spawned)
```

This architecture provides sophisticated multi-session coordination while building on the solid foundation of the existing SpecTask system, ensuring backward compatibility and leveraging proven workflows.