# Simplified Zed-Helix Integration Architecture

## Overview

This design simplifies the integration between Zed and Helix by building on existing infrastructure and avoiding over-engineering. The core principle is that **each Zed thread maps 1:1 to a Helix session**, with tasks providing coordination across multiple sessions.

## Core Entities

### Task
- **Purpose**: High-level work coordination (e.g., "Implement user authentication", "User does coding")
- **Lifecycle**: Can be long-running and interactive
- **Relationship**: Can spawn and coordinate multiple work sessions
- **Examples**:
  - `interactive`: "User does coding" - ongoing session that spawns work as needed
  - `batch`: "Implement feature X" - specific deliverable with defined scope
  - `coding_session`: "Debug performance issue" - focused debugging session

### WorkSession
- **Purpose**: Individual work unit within a task
- **Mapping**: 1:1 with Helix Session (reuse existing infrastructure)
- **Agent Configuration**: Each work session has a configured agent type
- **Spawning**: Can be spawned by task coordination or by other work sessions
- **Agent Types**:
  - `simple`: Basic Helix agent
  - `helix_agent`: Standard Helix agent with skills
  - `zed_agent`: Zed-integrated agent (with optional future variants like `zed_claude_code`, `zed_gemini_cli`, `zed_qwen_code`)

### ZedThreadMapping
- **Purpose**: Maps Zed threads to work sessions when `zed_agent` type is used
- **Scope**: Links specific Zed session/thread combinations to Helix work sessions
- **Configuration**: Stores Zed-specific config (project path, workspace settings)
- **Note**: Subprocess work (testing, deployment, etc.) happens naturally within the Zed agent

## Simplified Schema

```sql
-- Core task management
CREATE TABLE tasks (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    task_type VARCHAR(50) NOT NULL, -- 'interactive', 'batch', 'coding_session'
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    priority INTEGER DEFAULT 0,
    
    user_id VARCHAR(255) NOT NULL,
    app_id VARCHAR(255),
    organization_id VARCHAR(255),
    
    config JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    
    INDEX idx_tasks_user_id (user_id),
    INDEX idx_tasks_status (status)
);

-- Individual work sessions (maps 1:1 to helix sessions)
CREATE TABLE work_sessions (
    id VARCHAR(255) PRIMARY KEY,
    task_id VARCHAR(255) NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    helix_session_id VARCHAR(255) NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    
    name VARCHAR(255),
    description TEXT,
    agent_type VARCHAR(50) NOT NULL, -- 'simple', 'helix_agent', 'zed_agent'
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    
    parent_work_session_id VARCHAR(255) REFERENCES work_sessions(id),
    spawned_by_session_id VARCHAR(255) REFERENCES work_sessions(id),
    
    agent_config JSONB,
    environment_config JSONB,
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    
    UNIQUE(helix_session_id), -- 1:1 mapping
    INDEX idx_work_sessions_task_id (task_id)
);

-- Zed integration mapping (only for agent_type = 'zed_agent')
CREATE TABLE zed_thread_mappings (
    id VARCHAR(255) PRIMARY KEY,
    work_session_id VARCHAR(255) NOT NULL REFERENCES work_sessions(id) ON DELETE CASCADE,
    zed_session_id VARCHAR(255) NOT NULL,
    zed_thread_id VARCHAR(255) NOT NULL,
    
    project_path VARCHAR(500),
    workspace_config JSONB,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    last_activity_at TIMESTAMP,
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    UNIQUE(work_session_id), -- 1:1 mapping
    UNIQUE(zed_session_id, zed_thread_id),
    INDEX idx_zed_mappings_zed_session (zed_session_id)
);

-- Extend existing sessions table
ALTER TABLE sessions 
ADD COLUMN task_id VARCHAR(255) REFERENCES tasks(id),
ADD COLUMN work_session_id VARCHAR(255) REFERENCES work_sessions(id);
```

## Integration Flow

### 1. Task Creation
```
User/System creates Task
├── Task coordinator session (regular Helix session with special role)
└── Initial work session spawned based on task type
```

### 2. Work Session Lifecycle
```
WorkSession Created with Agent Type
├── Helix Session created (1:1 mapping) with specified agent
├── [If agent_type = 'zed_agent'] Zed Thread spawned
└── [If Zed] ZedThreadMapping created for integration
```

### 3. Session Coordination
- Task coordination happens through the main Helix session
- Work sessions can spawn other work sessions
- Zed threads communicate back to Helix through existing integration
- Task status updated based on work session completion

## API Design

### Task Management
```go
POST /api/v1/tasks
GET /api/v1/tasks
GET /api/v1/tasks/{id}
PUT /api/v1/tasks/{id}/status
DELETE /api/v1/tasks/{id}
```

### Work Session Management
```go
POST /api/v1/tasks/{task_id}/work-sessions
GET /api/v1/tasks/{task_id}/work-sessions
GET /api/v1/work-sessions/{id}
PUT /api/v1/work-sessions/{id}/status
```

### Zed Integration
```go
POST /api/v1/work-sessions/{id}/zed-thread
GET /api/v1/work-sessions/{id}/zed-thread
DELETE /api/v1/work-sessions/{id}/zed-thread
```

## Implementation Phases

### Phase 1: Core Infrastructure
1. Add Task and WorkSession tables
2. Extend existing Session table with task/work_session references
3. Basic task and work session CRUD operations
4. Task coordination through existing session infrastructure

### Phase 2: Agent Type Integration
1. Add ZedThreadMapping table for `zed_agent` type work sessions
2. Extend existing Zed agent launching to work with work sessions
3. Implement work session spawning with proper agent configuration
4. Update agent communication to include work session context

### Phase 3: Advanced Features
1. Work session dependencies (simple blocking/waiting)
2. Session branching and merging
3. Task templates for common workflows
4. Enhanced task dashboard

## Future Considerations (Not Implemented Yet)

### TaskContext (Future Phase)
- Shared state across work sessions in a task
- File state synchronization
- Decision tracking
- Cross-session memory

### Advanced Dependencies
- Complex dependency graphs
- Conditional dependencies
- Parallel execution with synchronization points

### Task Templates
- Predefined task workflows
- Template-based work session spawning
- Common development patterns (feature development, bug fixing, code review)

## Benefits of This Approach

1. **Leverages Existing Infrastructure**: Builds on proven Helix session management
2. **Simple Mental Model**: Each Zed thread = one Helix session = one work unit
3. **Incremental Implementation**: Can be built and deployed in phases
4. **Backward Compatibility**: Existing Zed-Helix integration continues to work
5. **Scalable**: Can handle both simple 1:1 mappings and complex multi-session tasks

## Example Scenarios

### Scenario 1: Simple Feature Development
```
Task: "Add user profile page"
└── WorkSession: "Implement profile page" (agent_type: zed_agent)
    └── ZedThread: Single thread in user's project
```

### Scenario 2: Complex Feature with Review
```
Task: "Implement authentication system"
├── WorkSession: "Research & design" (agent_type: helix_agent)
├── WorkSession: "Implement backend" (agent_type: zed_agent)
├── WorkSession: "Implement frontend" (agent_type: zed_agent)  
└── WorkSession: "Integration testing" (agent_type: zed_agent)
    └── Note: Testing/deployment happens within Zed agent naturally
```

### Scenario 3: Interactive Coding Session
```
Task: "User coding session" (interactive, long-running)
├── WorkSession: "Main development" (agent_type: zed_agent)
├── WorkSession: "Debug performance issue" (agent_type: zed_agent, spawned from main)
└── WorkSession: "Create tests" (agent_type: zed_agent, spawned from main)
```

This simplified architecture provides the foundation for sophisticated task management while remaining implementable and maintainable.