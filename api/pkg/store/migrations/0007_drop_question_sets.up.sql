-- Drop the question sets feature.
--
-- Question sets (a set of prompts fan-executed against an app, each
-- answer landing in its own session) has been removed from the product.
-- The tables and the two session columns that back-referenced an
-- execution are no longer written or read by any code path, so drop
-- them rather than leaving AutoMigrate-orphaned schema behind.
--
-- Each step is guarded so it's a no-op on fresh databases.

DO $$
BEGIN
    IF to_regclass('question_set_executions') IS NOT NULL THEN
        DROP TABLE question_set_executions CASCADE;
    END IF;
    IF to_regclass('question_sets') IS NOT NULL THEN
        DROP TABLE question_sets CASCADE;
    END IF;
    IF to_regclass('sessions') IS NOT NULL THEN
        ALTER TABLE sessions DROP COLUMN IF EXISTS question_set_id;
        ALTER TABLE sessions DROP COLUMN IF EXISTS question_set_execution_id;
    END IF;
END $$;
