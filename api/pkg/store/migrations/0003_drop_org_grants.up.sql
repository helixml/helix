-- Drop the org_grants table.
--
-- The org-graph runtime previously stored per-Worker tool grants in
-- org_grants. As of this revision, Worker MCP surfaces are derived
-- live from their Position's Role.Tools: capability is a property of
-- the Role, not a per-Worker attribute. Without the table, the gorm
-- AutoMigrate cycle no longer recreates it and the schema-reset path
-- has nothing to drop next time round.
--
-- Guarded so it's a no-op on fresh databases where org_grants never
-- existed in the current search_path.
DO $$
BEGIN
    IF to_regclass('org_grants') IS NOT NULL THEN
        DROP TABLE org_grants;
    END IF;
END $$;
