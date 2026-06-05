-- Wipe all helix-org state in preparation for the position-less schema.
--
-- helix-org is pre-release. Position is being removed from the domain;
-- Workers now hold role_id + parent_id directly, and subscriptions are
-- worker-anchored. Rather than backfill across the schema change, drop
-- every helix-org row and let GORM AutoMigrate recreate the tables with
-- the new shapes on the next boot. The next /helix-org request per org
-- re-bootstraps the owner Role + Worker.
--
-- Each step is guarded so it's a no-op on fresh databases.

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
    -- Wipe everything else helix-org owns so re-bootstrap from a fresh
    -- /helix-org request succeeds without "already exists" conflicts.
    IF to_regclass('org_roles') IS NOT NULL THEN
        DROP TABLE org_roles CASCADE;
    END IF;
    IF to_regclass('org_streams') IS NOT NULL THEN
        DROP TABLE org_streams CASCADE;
    END IF;
    IF to_regclass('org_events') IS NOT NULL THEN
        DROP TABLE org_events CASCADE;
    END IF;
    IF to_regclass('org_environments') IS NOT NULL THEN
        DROP TABLE org_environments CASCADE;
    END IF;
    IF to_regclass('org_worker_runtime_state') IS NOT NULL THEN
        DROP TABLE org_worker_runtime_state CASCADE;
    END IF;
    IF to_regclass('org_activations') IS NOT NULL THEN
        DROP TABLE org_activations CASCADE;
    END IF;
    IF to_regclass('org_configs') IS NOT NULL THEN
        DROP TABLE org_configs CASCADE;
    END IF;
END $$;
