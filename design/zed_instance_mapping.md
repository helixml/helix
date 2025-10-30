# Zed Agent Instance Mapping Architecture

## Overview

This document clarifies how Zed agent instances map to tasks and work sessions in the Helix integration architecture.

## Core Mapping: One Zed Instance per Task

```
Task ("Implement user authentication")
├── Zed Agent Instance (single process)
│   ├── WorkSession 1 ("Backend API") → Zed Thread 1
│   ├── WorkSession 2 ("Frontend UI") → Zed Thread 2
│   ├── WorkSession 3 ("Tests") → Zed Thread 3
│   └── WorkSession 4 ("Documentation") → Zed Thread 4
└── WorkSession 5 ("Code Review") → Direct Helix (helix_agent)
```

## Key Principles

### 1. Task-Level Zed Instance
- **One Zed agent instance per task** when any work session in the task uses `agent_type = "zed_agent"`
- All Zed-based work sessions within a task share the same Zed instance
- Non-Zed work sessions (`helix_agent`, `simple`) run independently

### 2. WorkSession-Level Zed Threads
- Each `zed_agent` work session gets its own thread within the shared Zed instance
- Threads can collaborate and share project context
- Each thread maintains its own conversation history with Helix

### 3. Project Context Sharing
- All Zed threads in a task work on the same project/workspace
- File changes are visible across all threads
- Shared terminal sessions and project state

## Database Schema Updates

### ZedInstanceMapping (New Table)
```sql
CREATE TABLE zed_instance_mappings (
    id VARCHAR(255) PRIMARY KEY,
    task_id VARCHAR(255) NOT NULL UNIQUE REFERENCES tasks(id),
    zed_instance_id VARCHAR(255) NOT NULL, -- External Zed process ID
    project_path VARCHAR(500),
    workspace_config JSONB,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    last_activity_at TIMESTAMP,
    
    INDEX idx_zed_instance_task (task_id),
    INDEX idx_zed_instance_status (status)
);
```

### Updated ZedThreadMapping
```sql
-- Links specific work sessions to threads within the Zed instance
CREATE TABLE zed_thread_mappings (
    id VARCHAR(255) PRIMARY KEY,
    work_session_id VARCHAR(255) NOT NULL UNIQUE REFERENCES work_sessions(id),
    zed_instance_id VARCHAR(255) NOT NULL, -- References zed_instance_mappings
    zed_thread_id VARCHAR(255) NOT NULL,   -- Thread ID within the instance
    thread_config JSONB,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    last_activity_at TIMESTAMP,
    
    UNIQUE(zed_instance_id, zed_thread_id),
    INDEX idx_zed_thread_work_session (work_session_id),
    INDEX idx_zed_thread_instance (zed_instance_id)
);
```

## Implementation Flow

### 1. Task Creation with Zed Work
```
1. User creates Task
2. First WorkSession with agent_type="zed_agent" triggers:
   a. Create ZedInstanceMapping for the task
   b. Launch Zed agent instance
   c. Create ZedThreadMapping for the work session
   d. Create thread within Zed instance
```

### 2. Additional Zed WorkSessions
```
1. User creates another WorkSession with agent_type="zed_agent"
2. System finds existing ZedInstanceMapping for task
3. Create new ZedThreadMapping
4. Create new thread in existing Zed instance
```

### 3. Non-Zed WorkSessions
```
1. User creates WorkSession with agent_type="helix_agent"
2. No Zed integration required
3. Standard Helix session created
4. Can coordinate with Zed threads through task context
```

## Go Types

```go
// New type for Zed instance management
type ZedInstanceMapping struct {
    ID              string    `json:"id" gorm:"primaryKey;size:255"`
    TaskID          string    `json:"task_id" gorm:"not null;size:255;uniqueIndex"`
    ZedInstanceID   string    `json:"zed_instance_id" gorm:"not null;size:255;index"`
    ProjectPath     string    `json:"project_path,omitempty" gorm:"size:500"`
    WorkspaceConfig datatypes.JSON `json:"workspace_config,omitempty" gorm:"type:jsonb"`
    Status          ZedInstanceStatus `json:"status" gorm:"not null;size:50;default:pending;index"`
    CreatedAt       time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP"`
    LastActivityAt  *time.Time `json:"last_activity_at,omitempty"`
    
    // Relationships
    Task         *Task              `json:"task,omitempty" gorm:"foreignKey:TaskID"`
    ThreadMappings []ZedThreadMapping `json:"thread_mappings,omitempty" gorm:"foreignKey:ZedInstanceID"`
}

// Updated ZedThreadMapping
type ZedThreadMapping struct {
    ID            string    `json:"id" gorm:"primaryKey;size:255"`
    WorkSessionID string    `json:"work_session_id" gorm:"not null;size:255;uniqueIndex"`
    ZedInstanceID string    `json:"zed_instance_id" gorm:"not null;size:255;index"`
    ZedThreadID   string    `json:"zed_thread_id" gorm:"not null;size:255;index"`
    ThreadConfig  datatypes.JSON `json:"thread_config,omitempty" gorm:"type:jsonb"`
    Status        ZedThreadStatus `json:"status" gorm:"not null;size:50;default:pending;index"`
    CreatedAt     time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP"`
    LastActivityAt *time.Time `json:"last_activity_at,omitempty"`
    
    // Relationships
    WorkSession   *WorkSession        `json:"work_session,omitempty" gorm:"foreignKey:WorkSessionID"`
    ZedInstance   *ZedInstanceMapping  `json:"zed_instance,omitempty" gorm:"foreignKey:ZedInstanceID"`
}

type ZedInstanceStatus string
const (
    ZedInstanceStatusPending      ZedInstanceStatus = "pending"
    ZedInstanceStatusActive       ZedInstanceStatus = "active"
    ZedInstanceStatusDisconnected ZedInstanceStatus = "disconnected"
    ZedInstanceStatusCompleted    ZedInstanceStatus = "completed"
    ZedInstanceStatusFailed       ZedInstanceStatus = "failed"
)
```

## Example Scenarios

### Scenario 1: Feature Development Task
```
Task: "Add user profile page"
├── Zed Instance: project-auth (shared)
    ├── Thread 1: "Backend API" (WorkSession 1)
    ├── Thread 2: "Frontend UI" (WorkSession 2)
    └── Thread 3: "Tests" (WorkSession 3)
```

### Scenario 2: Mixed Agent Types
```
Task: "Implement authentication system"
├── WorkSession: "Research & design" (helix_agent) → Standard Helix
├── Zed Instance: auth-project (shared)
│   ├── Thread 1: "Backend implementation" (zed_agent)
│   ├── Thread 2: "Frontend implementation" (zed_agent)
│   └── Thread 3: "Integration tests" (zed_agent)
└── WorkSession: "Security review" (helix_agent) → Standard Helix
```

### Scenario 3: Interactive Coding Session
```
Task: "User coding session" (long-running)
├── Zed Instance: user-workspace (persistent)
    ├── Thread 1: "Main development" (initial)
    ├── Thread 2: "Debug performance" (spawned later)
    └── Thread 3: "Refactor components" (spawned later)
```

## Benefits

### 1. Resource Efficiency
- One Zed process per task instead of per work session
- Shared project loading and indexing
- Reduced memory and CPU usage

### 2. Natural Collaboration
- Threads can see each other's file changes
- Shared project context and state
- Natural workflow for developers

### 3. Simplified Management
- Task-level Zed lifecycle management
- Easier cleanup when task completes
- Centralized workspace configuration

### 4. Scalability
- Tasks can have many work sessions without spawning many Zed instances
- Work sessions can be added/removed dynamically
- Efficient use of system resources

## Implementation Considerations

### 1. Zed Instance Lifecycle
- Create when first `zed_agent` work session is added to task
- Keep alive while any Zed work sessions are active
- Clean up when all Zed work sessions complete or task ends

### 2. Thread Management
- Each work session gets unique thread ID within instance
- Thread cleanup when work session completes
- Handle thread disconnections gracefully

### 3. Project Context
- All threads in instance share same project path
- Workspace configuration applied at instance level
- File changes visible across all threads

### 4. Communication Protocol
- Extend existing Zed-Helix protocol to include:
  - Task ID and instance ID
  - Thread ID for routing messages
  - Work session context

This architecture provides the best balance of resource efficiency, natural workflow, and implementation simplicity while supporting the complex multi-session scenarios described in the requirements.