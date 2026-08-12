-- Tasks created before execution ownership was enforced may not have an
-- assignee. Preserve the recorded starter where available; otherwise the
-- creator is the only known actor for the legacy task.
DO $$
BEGIN
    IF to_regclass('spec_tasks') IS NULL
        OR NOT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema = current_schema()
                AND table_name = 'spec_tasks' AND column_name = 'assignee_id'
        )
        OR NOT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema = current_schema()
                AND table_name = 'spec_tasks' AND column_name = 'created_by'
        ) THEN
        RETURN;
    END IF;

    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
            AND table_name = 'spec_tasks' AND column_name = 'planning_started_by'
    ) THEN
        UPDATE spec_tasks
        SET assignee_id = COALESCE(NULLIF(planning_started_by, ''), created_by)
        WHERE assignee_id IS NULL OR assignee_id = '';
    ELSE
        UPDATE spec_tasks
        SET assignee_id = created_by
        WHERE assignee_id IS NULL OR assignee_id = '';
    END IF;
END $$;
