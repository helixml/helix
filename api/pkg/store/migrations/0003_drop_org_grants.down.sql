-- Recreate the org_grants table shape AutoMigrate previously produced.
-- Schema-only — original grant rows cannot be recovered from this code,
-- they were a derived view of Role.Tools after the refactor.
CREATE TABLE IF NOT EXISTS org_grants (
    id text NOT NULL,
    org_id text NOT NULL,
    worker_id text NOT NULL,
    tool_name text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    PRIMARY KEY (id, org_id)
);
CREATE INDEX IF NOT EXISTS idx_org_grants_org_id ON org_grants (org_id);
CREATE INDEX IF NOT EXISTS idx_org_grants_worker_id ON org_grants (worker_id);
