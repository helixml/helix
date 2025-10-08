-- Create extension for UUID generation if not exists
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Streaming access grants table
CREATE TABLE IF NOT EXISTS streaming_access_grants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- What is being shared
    session_id TEXT,
    pde_id TEXT,

    -- Who owns it
    owner_user_id TEXT NOT NULL,

    -- Who can access it
    granted_user_id TEXT,
    granted_team_id TEXT,
    granted_role TEXT,

    -- What they can do
    access_level TEXT NOT NULL CHECK (access_level IN ('view', 'control', 'admin')),

    -- When
    granted_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE,

    -- Audit
    granted_by TEXT NOT NULL,
    revoked_at TIMESTAMP WITH TIME ZONE,
    revoked_by TEXT,

    -- Constraints
    CONSTRAINT check_streaming_target CHECK (
        (session_id IS NOT NULL AND pde_id IS NULL) OR
        (session_id IS NULL AND pde_id IS NOT NULL)
    ),
    CONSTRAINT check_streaming_grantee CHECK (
        (granted_user_id IS NOT NULL AND granted_team_id IS NULL AND granted_role IS NULL) OR
        (granted_user_id IS NULL AND granted_team_id IS NOT NULL AND granted_role IS NULL) OR
        (granted_user_id IS NULL AND granted_team_id IS NULL AND granted_role IS NOT NULL)
    )
);

-- Indexes for fast lookups
CREATE INDEX IF NOT EXISTS idx_streaming_access_session ON streaming_access_grants(session_id) WHERE session_id IS NOT NULL AND revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_streaming_access_pde ON streaming_access_grants(pde_id) WHERE pde_id IS NOT NULL AND revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_streaming_access_user ON streaming_access_grants(granted_user_id) WHERE granted_user_id IS NOT NULL AND revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_streaming_access_team ON streaming_access_grants(granted_team_id) WHERE granted_team_id IS NOT NULL AND revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_streaming_access_role ON streaming_access_grants(granted_role) WHERE granted_role IS NOT NULL AND revoked_at IS NULL;

-- Streaming access audit log
CREATE TABLE IF NOT EXISTS streaming_access_audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- What was accessed
    session_id TEXT,
    pde_id TEXT,
    wolf_lobby_id TEXT,

    -- Who accessed it
    user_id TEXT NOT NULL,
    access_level TEXT NOT NULL,

    -- How
    access_method TEXT NOT NULL CHECK (access_method IN ('owner', 'user_grant', 'team_grant', 'role_grant')),
    grant_id UUID,

    -- When
    accessed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    disconnected_at TIMESTAMP WITH TIME ZONE,
    session_duration_seconds INTEGER,

    -- Where from
    ip_address INET,
    user_agent TEXT,

    CONSTRAINT check_streaming_audit_target CHECK (
        (session_id IS NOT NULL AND pde_id IS NULL) OR
        (session_id IS NULL AND pde_id IS NOT NULL)
    )
);

-- Indexes for audit queries
CREATE INDEX IF NOT EXISTS idx_streaming_audit_user ON streaming_access_audit_log(user_id);
CREATE INDEX IF NOT EXISTS idx_streaming_audit_session ON streaming_access_audit_log(session_id) WHERE session_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_streaming_audit_pde ON streaming_access_audit_log(pde_id) WHERE pde_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_streaming_audit_time ON streaming_access_audit_log(accessed_at);
