-- Between 1eff4e801 (2026-08-10) and this migration, CreateTaskFromPrompt wrote
-- the global default onto rows that had no explicit size, freezing those tasks
-- at 4 vCPU / 8 GB forever — a raised default could never reach them. NULL means
-- "no override, use the live default", which is what these rows meant all along.
--
-- Match the old default pair EXACTLY. Rows holding {"vcpus": 8, "memory_mb":
-- 16384} are deliberate user choices (178 of them on meta alone) and must never
-- be caught by this. Do not generalise the predicate to "any row equal to some
-- default": 8/16384 has been a selectable preset the whole time, so equality
-- with a default does not imply the value was materialized.
--
-- Verify with the same query that sized the problem, before and after:
--   SELECT sandbox_resource_overrides, count(*) FROM spec_tasks GROUP BY 1;
-- The 8/16384 count must be identical afterwards.
DO $$
BEGIN
    IF to_regclass('spec_tasks') IS NULL
        OR NOT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema = current_schema()
                AND table_name = 'spec_tasks'
                AND column_name = 'sandbox_resource_overrides'
        ) THEN
        RETURN;
    END IF;

    UPDATE spec_tasks
    SET sandbox_resource_overrides = NULL
    WHERE sandbox_resource_overrides = '{"vcpus": 4, "memory_mb": 8192}'::jsonb;
END $$;
