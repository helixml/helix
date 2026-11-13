-- CreateInteractions previously rewrote the first interaction's created time
-- whenever an entire session transcript was saved. Scheduled retained the
-- original turn start, so repair only rows where created moved after it.
DO $$
BEGIN
    IF to_regclass('interactions') IS NULL
        OR NOT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema = current_schema()
                AND table_name = 'interactions'
                AND column_name = 'created'
        )
        OR NOT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema = current_schema()
                AND table_name = 'interactions'
                AND column_name = 'scheduled'
        ) THEN
        RETURN;
    END IF;

    UPDATE interactions
    SET created = scheduled
    WHERE scheduled > TIMESTAMPTZ '0001-01-01 00:00:00+00'
        AND created > scheduled;
END $$;
