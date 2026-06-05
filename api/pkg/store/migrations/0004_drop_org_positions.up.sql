-- Drop the org_positions table and recreate org_workers + org_subscriptions
-- with the new schema where:
--
--   * Worker holds role_id and parent_id directly (was on Position).
--   * Subscription is keyed by (org_id, worker_id, stream_id) and dies
--     with the worker on fire (was position-anchored).
--
-- helix-org is pre-release; existing dev rows are wiped. GORM
-- AutoMigrate recreates org_workers and org_subscriptions with the new
-- columns on next boot. The org_positions table is dropped entirely
-- (no more Position concept).
--
-- Each step is guarded so this is a no-op on fresh databases where
-- the tables never existed in the current search_path.

DO $$
BEGIN
    IF to_regclass('org_positions') IS NOT NULL THEN
        DROP TABLE org_positions CASCADE;
    END IF;
    IF to_regclass('org_subscriptions') IS NOT NULL THEN
        DROP TABLE org_subscriptions CASCADE;
    END IF;
    IF to_regclass('org_workers') IS NOT NULL THEN
        DROP TABLE org_workers CASCADE;
    END IF;
END $$;
