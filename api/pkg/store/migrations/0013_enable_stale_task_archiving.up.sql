-- Stale-task archiving was opt-in while it was new and the reaper that deletes
-- the freed workspaces did not work at all (see #3628): enabling it archived
-- tasks but reclaimed nothing, so nobody turned it on. With the reaper fixed
-- and implementation tasks no longer exempt from the sweep (#3629), the chain
-- stale -> archived -> past grace -> deleted now actually runs, and leaving it
-- off means workspaces accumulate forever.
--
-- The gorm tag `default:true` only affects rows created after the deploy, so
-- existing projects must be moved explicitly. Without this, turning the
-- default on would reach nobody: 141 of 145 projects store `false`.
--
-- Scoped to the exact value the old default wrote. A project that had it
-- turned on keeps it on; the four that already opted in are untouched.
--
-- Verify with the same query that sized the problem, before and after:
--   SELECT archive_stale_tasks_enabled, count(*) FROM projects
--   WHERE deleted_at IS NULL GROUP BY 1;
-- Every row must read `t` afterwards.
DO $$
BEGIN
    IF to_regclass('projects') IS NULL
        OR NOT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema = current_schema()
                AND table_name = 'projects'
                AND column_name = 'archive_stale_tasks_enabled'
        ) THEN
        RETURN;
    END IF;

    UPDATE projects
    SET archive_stale_tasks_enabled = true
    WHERE archive_stale_tasks_enabled IS NULL
       OR archive_stale_tasks_enabled = false;
END $$;
