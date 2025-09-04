-- Database schema extensions for Zed-Helix integration architecture
-- This extends the existing Helix database schema

-- Core task management
CREATE TABLE IF NOT EXISTS tasks (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    task_type VARCHAR(50) NOT NULL, -- 'interactive', 'batch', 'coding_session'
    status VARCHAR(50) NOT NULL DEFAULT 'pending', -- 'pending', 'active', 'paused', 'completed', 'failed', 'cancelled'
    priority INTEGER DEFAULT 0, -- Lower = higher priority
    
    -- Ownership
    user_id VARCHAR(255) NOT NULL,
    app_id VARCHAR(255),
    organization_id VARCHAR(255),
    
    -- Task configuration
    config JSONB, -- Task-specific configuration
    constraints_config JSONB, -- Resource constraints, time limits, etc.
    
    -- State tracking
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    deadline_at TIMESTAMP,
    
    -- Metadata
    metadata JSONB,
    labels JSONB, -- Tags for filtering/organization
    
    -- Parent/child relationships for task hierarchies
    parent_task_id VARCHAR(255) REFERENCES tasks(id),
    
    INDEX idx_tasks_user_id (user_id),
    INDEX idx_tasks_app_id (app_id),
    INDEX idx_tasks_status (status),
    INDEX idx_tasks_created_at (created_at)
);

-- Task coordination sessions (orchestration layer)
CREATE TABLE IF NOT EXISTS task_sessions (
    id VARCHAR(255) PRIMARY KEY,
    task_id VARCHAR(255) NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    helix_session_id VARCHAR(255) NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    
    status VARCHAR(50) NOT NULL DEFAULT 'active', -- 'active', 'paused', 'completed'
    coordinator_type VARCHAR(50) NOT NULL DEFAULT 'helix_coordinator', -- Agent type for coordination
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    UNIQUE(task_id), -- One coordination session per task
    INDEX idx_task_sessions_task_id (task_id),
    INDEX idx_task_sessions_helix_session (helix_session_id)
);

-- Individual work sessions within a task
CREATE TABLE IF NOT EXISTS work_sessions (
    id VARCHAR(255) PRIMARY KEY,
    task_id VARCHAR(255) NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    helix_session_id VARCHAR(255) NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    
    -- Work session details
    name VARCHAR(255),
    description TEXT,
    work_type VARCHAR(50) NOT NULL, -- 'zed_thread', 'direct_helix', 'subprocess'
    status VARCHAR(50) NOT NULL DEFAULT 'pending', -- 'pending', 'active', 'completed', 'failed', 'cancelled'
    
    -- Relationships
    parent_work_session_id VARCHAR(255) REFERENCES work_sessions(id), -- For branching
    spawned_by_session_id VARCHAR(255) REFERENCES work_sessions(id), -- Which session created this one
    
    -- Configuration
    agent_config JSONB, -- Agent-specific configuration
    environment_config JSONB, -- Environment setup (project path, etc.)
    
    -- State tracking
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    
    -- Metadata
    metadata JSONB,
    
    INDEX idx_work_sessions_task_id (task_id),
    INDEX idx_work_sessions_helix_session (helix_session_id),
    INDEX idx_work_sessions_status (status),
    INDEX idx_work_sessions_parent (parent_work_session_id)
);

-- Zed thread to work session mapping
CREATE TABLE IF NOT EXISTS zed_thread_mappings (
    id VARCHAR(255) PRIMARY KEY,
    work_session_id VARCHAR(255) NOT NULL REFERENCES work_sessions(id) ON DELETE CASCADE,
    zed_session_id VARCHAR(255) NOT NULL, -- Zed's session ID
    zed_thread_id VARCHAR(255) NOT NULL, -- Zed's thread ID within the session
    
    -- Zed-specific configuration
    project_path VARCHAR(500),
    workspace_config JSONB,
    
    -- Status tracking
    status VARCHAR(50) NOT NULL DEFAULT 'pending', -- 'pending', 'active', 'disconnected', 'completed'
    last_activity_at TIMESTAMP,
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    UNIQUE(zed_session_id, zed_thread_id),
    INDEX idx_zed_mappings_work_session (work_session_id),
    INDEX idx_zed_mappings_zed_session (zed_session_id),
    INDEX idx_zed_mappings_thread (zed_thread_id)
);

-- Shared context and state across task sessions
CREATE TABLE IF NOT EXISTS task_contexts (
    id VARCHAR(255) PRIMARY KEY,
    task_id VARCHAR(255) NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    
    -- Context data
    context_type VARCHAR(50) NOT NULL, -- 'file_state', 'decision', 'shared_memory', 'project_state'
    context_key VARCHAR(255) NOT NULL, -- Key for this context item
    context_data JSONB NOT NULL, -- The actual context data
    
    -- Metadata
    created_by_session_id VARCHAR(255) REFERENCES work_sessions(id),
    access_permissions JSONB, -- Which sessions can read/write this context
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP, -- Optional expiration
    
    UNIQUE(task_id, context_type, context_key),
    INDEX idx_task_contexts_task_id (task_id),
    INDEX idx_task_contexts_type (context_type),
    INDEX idx_task_contexts_created_by (created_by_session_id)
);

-- Session dependencies and relationships
CREATE TABLE IF NOT EXISTS session_dependencies (
    id VARCHAR(255) PRIMARY KEY,
    task_id VARCHAR(255) NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    dependent_session_id VARCHAR(255) NOT NULL REFERENCES work_sessions(id) ON DELETE CASCADE,
    dependency_session_id VARCHAR(255) NOT NULL REFERENCES work_sessions(id) ON DELETE CASCADE,
    
    dependency_type VARCHAR(50) NOT NULL, -- 'blocks', 'waits_for', 'merges_with', 'branches_from'
    status VARCHAR(50) NOT NULL DEFAULT 'pending', -- 'pending', 'satisfied', 'failed'
    
    -- Conditions for dependency satisfaction
    condition_config JSONB, -- What needs to happen for dependency to be satisfied
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMP,
    
    INDEX idx_session_deps_task (task_id),
    INDEX idx_session_deps_dependent (dependent_session_id),
    INDEX idx_session_deps_dependency (dependency_session_id),
    INDEX idx_session_deps_type (dependency_type)
);

-- Task execution logs and events
CREATE TABLE IF NOT EXISTS task_execution_logs (
    id VARCHAR(255) PRIMARY KEY,
    task_id VARCHAR(255) NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    work_session_id VARCHAR(255) REFERENCES work_sessions(id) ON DELETE SET NULL,
    
    event_type VARCHAR(50) NOT NULL, -- 'session_spawned', 'session_completed', 'dependency_resolved', 'context_updated'
    event_data JSONB NOT NULL,
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    INDEX idx_task_logs_task (task_id),
    INDEX idx_task_logs_session (work_session_id),
    INDEX idx_task_logs_type (event_type),
    INDEX idx_task_logs_created (created_at)
);

-- Extend existing sessions table to reference tasks
ALTER TABLE sessions 
ADD COLUMN IF NOT EXISTS task_id VARCHAR(255) REFERENCES tasks(id),
ADD COLUMN IF NOT EXISTS work_session_id VARCHAR(255) REFERENCES work_sessions(id),
ADD COLUMN IF NOT EXISTS session_role VARCHAR(50) DEFAULT 'standalone'; -- 'standalone', 'task_coordinator', 'work_session'

CREATE INDEX IF NOT EXISTS idx_sessions_task_id ON sessions(task_id);
CREATE INDEX IF NOT EXISTS idx_sessions_work_session ON sessions(work_session_id);

-- Extend existing agent_work_items to reference tasks
ALTER TABLE agent_work_items 
ADD COLUMN IF NOT EXISTS task_id VARCHAR(255) REFERENCES tasks(id),
ADD COLUMN IF NOT EXISTS work_session_id VARCHAR(255) REFERENCES work_sessions(id);

CREATE INDEX IF NOT EXISTS idx_agent_work_items_task_id ON agent_work_items(task_id);
CREATE INDEX IF NOT EXISTS idx_agent_work_items_work_session ON agent_work_items(work_session_id);

-- Views for common queries
CREATE OR REPLACE VIEW task_overview AS
SELECT 
    t.id as task_id,
    t.name as task_name,
    t.status as task_status,
    t.task_type,
    COUNT(DISTINCT ws.id) as work_session_count,
    COUNT(DISTINCT ztm.id) as zed_thread_count,
    COUNT(DISTINCT tc.id) as context_items,
    t.created_at,
    t.updated_at
FROM tasks t
LEFT JOIN work_sessions ws ON t.id = ws.task_id
LEFT JOIN zed_thread_mappings ztm ON ws.id = ztm.work_session_id
LEFT JOIN task_contexts tc ON t.id = tc.task_id
GROUP BY t.id, t.name, t.status, t.task_type, t.created_at, t.updated_at;

CREATE OR REPLACE VIEW active_work_sessions AS
SELECT 
    ws.id as work_session_id,
    ws.task_id,
    ws.name as work_session_name,
    ws.status as work_session_status,
    s.id as helix_session_id,
    s.name as helix_session_name,
    ztm.zed_session_id,
    ztm.zed_thread_id,
    ztm.status as zed_status,
    ws.created_at,
    ws.started_at,
    ztm.last_activity_at
FROM work_sessions ws
JOIN sessions s ON ws.helix_session_id = s.id
LEFT JOIN zed_thread_mappings ztm ON ws.id = ztm.work_session_id
WHERE ws.status IN ('pending', 'active');