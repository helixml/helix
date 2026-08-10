-- Remove prompt-library metadata while preserving prompt_history_entries as
-- the durable delivery queue for pending, sending, failed, and sent messages.

DO $$
BEGIN
    IF to_regclass('prompt_history_entries') IS NOT NULL THEN
        ALTER TABLE prompt_history_entries
            DROP COLUMN IF EXISTS organization_id,
            DROP COLUMN IF EXISTS pinned,
            DROP COLUMN IF EXISTS usage_count,
            DROP COLUMN IF EXISTS last_used_at,
            DROP COLUMN IF EXISTS tags,
            DROP COLUMN IF EXISTS is_template;
    END IF;
END $$;
