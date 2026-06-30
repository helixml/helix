-- Merge the helix-org Role and Worker concepts into a single Bot.
--
-- helix-org is pre-release. The former org_roles + org_workers collapse
-- into one org_bots table, and the rows that referenced a worker
-- (subscriptions, reporting lines, runtime state) are re-keyed bot↔bot.
-- Rather than backfill across the schema change, drop the helix-org
-- tables that change shape and let GORM AutoMigrate recreate them with
-- the new shapes (org_bots, bot-keyed columns) on the next boot. The
-- operator recreates their bots manually after the change.
--
-- Each step is guarded so it's a no-op on fresh databases.

DO $$
BEGIN
    IF to_regclass('org_subscriptions') IS NOT NULL THEN
        DROP TABLE org_subscriptions CASCADE;
    END IF;
    IF to_regclass('org_reporting_lines') IS NOT NULL THEN
        DROP TABLE org_reporting_lines CASCADE;
    END IF;
    IF to_regclass('org_worker_runtime_state') IS NOT NULL THEN
        DROP TABLE org_worker_runtime_state CASCADE;
    END IF;
    IF to_regclass('org_workers') IS NOT NULL THEN
        DROP TABLE org_workers CASCADE;
    END IF;
    IF to_regclass('org_roles') IS NOT NULL THEN
        DROP TABLE org_roles CASCADE;
    END IF;
END $$;
