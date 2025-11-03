# Spec-Driven Development Architecture for Helix-Zed Integration

## Overview

This architecture implements spec-driven development similar to Kiro.dev, where tasks are structured into two distinct phases managed by separate agents: **Planning** and **Implementation**. This brings structure to AI coding by separating the "what" from the "how."

## Core Philosophy

> "Bring structure to the chaos before you write a single line" - Kiro.dev

Instead of jumping straight into code ("vibe coding"), we use a structured approach:
1. **Planning Agent**: Turns prompts into clear requirements, system design, and discrete tasks
2. **Implementation Agent**: Executes the spec with focused coding work

## Architecture Overview

```
Task: "Implement user authentication"
├── Planning Phase
│   ├── Planning Agent (Helix App)
│   ├── Planning Session (requirements, design, specs)
│   └── Outputs: Specification Document + Implementation Tasks
└── Implementation Phase
    ├── Implementation Agent (Helix App, often Zed-based)
    ├── Multiple Work Sessions based on planning output
    └── Outputs: Working code that meets the spec
```

## Core Entities

### Task
- **Two-Agent Structure**: Every task has a planning agent and an implementation agent
- **Lifecycle**: Planning → Review/Approval → Implementation → Validation
- **Specification Storage**: Maintains the living spec document throughout the task

### Planning Agent (Helix App)
- **Purpose**: Requirements analysis, system design, task breakdown
- **Skills**: Research, analysis, system design, specification writing
- **Output**: Detailed specification with implementation tasks
- **Agent Type**: Typically `helix_agent` with specialized planning skills

### Implementation Agent (Helix App) 
- **Purpose**: Code implementation following the specification
- **Skills**: Coding, testing, debugging, integration
- **Input**: Specification and implementation tasks from planning phase
- **Agent Type**: Often `zed_agent` for hands-on coding work

### Specification Document
- **Living Document**: Updated throughout task lifecycle
- **Contains**: Requirements, system design, API specs, implementation tasks
- **Format**: Structured markdown with clear sections
- **Versioning**: Tracked changes and approvals

## Database Schema

```sql
-- Core task with two-agent structure
CREATE TABLE tasks (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    task_type VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'planning',
    priority INTEGER DEFAULT 0,
    
    -- Ownership
    user_id VARCHAR(255) NOT NULL,
    app_id VARCHAR(255),
    organization_id VARCHAR(255),
    
    -- Two-agent configuration
    planning_agent_id VARCHAR(255) NOT NULL,     -- References apps table
    implementation_agent_id VARCHAR(255) NOT NULL, -- References apps table
    
    -- Specification management
    current_spec_version INTEGER DEFAULT 1,
    spec_approved BOOLEAN DEFAULT FALSE,
    spec_approval_date TIMESTAMP,
    
    -- Phase tracking
    planning_completed_at TIMESTAMP,
    implementation_started_at TIMESTAMP,
    
    -- Standard fields
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP,
    
    INDEX idx_tasks_user_id (user_id),
    INDEX idx_tasks_status (status),
    INDEX idx_tasks_planning_agent (planning_agent_id),
    INDEX idx_tasks_implementation_agent (implementation_agent_id)
);

-- Specification documents with versioning
CREATE TABLE task_specifications (
    id VARCHAR(255) PRIMARY KEY,
    task_id VARCHAR(255) NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    
    -- Specification content
    requirements_section TEXT,
    system_design_section TEXT,
    api_specifications JSONB,
    implementation_tasks JSONB,
    acceptance_criteria TEXT,
    
    -- Metadata
    created_by_agent_id VARCHAR(255),
    status VARCHAR(50) NOT NULL DEFAULT 'draft', -- 'draft', 'review', 'approved', 'superseded'
    approved_by_user_id VARCHAR(255),
    approved_at TIMESTAMP,
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    UNIQUE(task_id, version),
    INDEX idx_spec_task_id (task_id),
    INDEX idx_spec_status (status)
);

-- Work sessions with phase context
CREATE TABLE work_sessions (
    id VARCHAR(255) PRIMARY KEY,
    task_id VARCHAR(255) NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    helix_session_id VARCHAR(255) NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    
    -- Session details
    name VARCHAR(255),
    description TEXT,
    phase VARCHAR(50) NOT NULL, -- 'planning', 'implementation', 'validation'
    agent_type VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    
    -- Specification context
    spec_version INTEGER, -- Which spec version this session is working with
    implementation_task_id VARCHAR(255), -- Specific task from spec this session addresses
    
    -- Relationships
    parent_work_session_id VARCHAR(255) REFERENCES work_sessions(id),
    spawned_by_session_id VARCHAR(255) REFERENCES work_sessions(id),
    
    -- Configuration
    agent_config JSONB,
    environment_config JSONB,
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    
    INDEX idx_work_sessions_task_id (task_id),
    INDEX idx_work_sessions_phase (phase),
    INDEX idx_work_sessions_spec_version (spec_version)
);

-- Implementation tasks from specifications
CREATE TABLE implementation_tasks (
    id VARCHAR(255) PRIMARY KEY,
    task_id VARCHAR(255) NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    spec_version INTEGER NOT NULL,
    
    -- Task details
    title VARCHAR(255) NOT NULL,
    description TEXT,
    acceptance_criteria TEXT,
    estimated_effort VARCHAR(50), -- 'small', 'medium', 'large'
    priority INTEGER DEFAULT 0,
    dependencies JSONB, -- Array of other task IDs this depends on
    
    -- Implementation tracking
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    assigned_work_session_id VARCHAR(255) REFERENCES work_sessions(id),
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP,
    
    INDEX idx_impl_tasks_task_id (task_id),
    INDEX idx_impl_tasks_status (status),
    INDEX idx_impl_tasks_assigned (assigned_work_session_id)
);
```

## Task Lifecycle

### Phase 1: Planning
```
1. User creates Task with planning and implementation agents
2. Planning Work Session created with planning agent
3. Planning agent:
   - Analyzes requirements
   - Creates system design
   - Breaks down into implementation tasks
   - Writes specification document
4. Specification goes to review status
5. User reviews and approves specification
```

### Phase 2: Implementation
```
1. Implementation phase begins after spec approval
2. Implementation Work Sessions created based on spec tasks
3. Implementation agent(s):
   - Follow the approved specification
   - Implement discrete tasks
   - Create working code
   - Run tests and validation
4. Work sessions can spawn additional sessions as needed
5. Progress tracked against specification
```

### Phase 3: Validation
```
1. Implementation completion triggers validation
2. Validation sessions verify code meets specification
3. User acceptance testing
4. Task marked as complete
```

## Agent Configuration

### Planning Agents
```yaml
# Example planning agent configuration
agent_type: "helix_agent"
skills:
  - requirements_analysis
  - system_design  
  - specification_writing
  - task_breakdown
  - research
model: "claude-3-5-sonnet-20241022"
system_prompt: |
  You are a senior software architect specializing in requirements analysis 
  and system design. Your role is to turn user prompts into clear, 
  implementable specifications.
```

### Implementation Agents  
```yaml
# Example implementation agent configuration
agent_type: "zed_agent"
skills:
  - coding
  - testing
  - debugging
  - integration
model: "claude-3-5-sonnet-20241022"  
system_prompt: |
  You are a senior software engineer who implements code following 
  detailed specifications. You write clean, tested, production-ready code.
```

## Specification Document Structure

```markdown
# Task Specification: [Task Name]

## 1. Requirements
### Functional Requirements
- [Detailed functional requirements]

### Non-Functional Requirements  
- [Performance, security, scalability requirements]

## 2. System Design
### Architecture Overview
- [High-level system design]

### Component Design
- [Detailed component specifications]

### Data Model
- [Database/data structure design]

## 3. API Specifications
### Endpoints
- [Detailed API endpoint specifications]

### Data Formats
- [Request/response formats]

## 4. Implementation Tasks
### Task 1: [Title]
- **Description**: [Detailed description]
- **Acceptance Criteria**: [Clear success criteria]
- **Dependencies**: [Other tasks this depends on]
- **Estimated Effort**: [Small/Medium/Large]

### Task 2: [Title]
[... additional tasks ...]

## 5. Testing Strategy
### Unit Tests
- [Unit testing requirements]

### Integration Tests  
- [Integration testing approach]

### Acceptance Tests
- [User acceptance criteria]

## 6. Deployment & Operations
### Deployment Strategy
- [How to deploy the implementation]

### Monitoring
- [What to monitor in production]
```

## API Design

### Task Management
```go
POST /api/v1/tasks
{
  "name": "Implement user authentication",
  "description": "Add secure user login/signup",
  "planning_agent_id": "agent_planning_123",
  "implementation_agent_id": "agent_zed_456",
  "priority": 1
}

GET /api/v1/tasks/{id}
GET /api/v1/tasks/{id}/specification
POST /api/v1/tasks/{id}/approve-specification
```

### Work Session Management
```go
POST /api/v1/tasks/{id}/planning-session
POST /api/v1/tasks/{id}/implementation-sessions
GET /api/v1/work-sessions/{id}/specification-context
```

### Specification Management
```go
GET /api/v1/tasks/{id}/specifications
POST /api/v1/tasks/{id}/specifications/{version}/approve
GET /api/v1/tasks/{id}/implementation-tasks
PUT /api/v1/implementation-tasks/{id}/assign
```

## Benefits

### 1. Structured Development
- Clear separation between planning and implementation
- Reduced ambiguity and scope creep
- Better estimation and project management

### 2. Quality Assurance
- Specifications serve as contract between planning and implementation
- Clear acceptance criteria for each task
- Systematic validation against requirements

### 3. Scalability
- Planning can happen in parallel with other work
- Implementation tasks can be distributed across multiple agents
- Clear handoff points and dependencies

### 4. Traceability
- Full audit trail from requirements to implementation
- Version control of specifications
- Clear rationale for design decisions

## Implementation Examples

### Example 1: Feature Development
```
Task: "Add user profile management"

Planning Phase:
├── Planning Agent: "Product requirements analyst"
├── Planning Session: Requirements gathering and design
└── Output: 
    ├── User stories and acceptance criteria
    ├── API design for profile endpoints
    ├── Database schema changes
    └── 5 implementation tasks

Implementation Phase:
├── Implementation Agent: "Full-stack developer (Zed)"
├── Task 1: Database schema migration (WorkSession 1)
├── Task 2: Backend API endpoints (WorkSession 2)  
├── Task 3: Frontend UI components (WorkSession 3)
├── Task 4: Integration tests (WorkSession 4)
└── Task 5: End-to-end testing (WorkSession 5)
```

### Example 2: Bug Fix with Investigation
```
Task: "Fix performance issue in user dashboard"

Planning Phase:
├── Planning Agent: "Performance specialist"
├── Planning Session: Investigation and root cause analysis
└── Output:
    ├── Performance analysis report
    ├── Root cause identification
    ├── Solution approach
    └── 3 implementation tasks

Implementation Phase:
├── Implementation Agent: "Backend optimization expert (Zed)"
├── Task 1: Database query optimization (WorkSession 1)
├── Task 2: Caching implementation (WorkSession 2)
└── Task 3: Performance testing validation (WorkSession 3)
```

This spec-driven architecture provides the structured approach to AI coding that Kiro.dev advocates, while building on Helix's existing agent and session infrastructure.