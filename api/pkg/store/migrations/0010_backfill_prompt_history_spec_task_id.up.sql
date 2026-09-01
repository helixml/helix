-- Prompts queued through POST /api/v1/sessions/{id}/messages were written with an
-- empty spec_task_id, because the generic session-messages handler had no spec
-- task to hand and passed "". The prompt-queue UI queries BY spec_task_id, so
-- those rows were invisible even though they were correctly queued and would be
-- dispatched — 53 approvals vanished from a user's view this way.
--
-- The handler now resolves the owning spec task itself. This repairs the rows
-- written before that fix, by the same rule: a session is owned by the spec task
-- whose planning_session_id points at it.
--
-- Idempotent — the guard no longer matches once a row is stamped — and it never
-- overwrites a non-empty spec_task_id.
DO $$
BEGIN
    IF to_regclass('prompt_history_entries') IS NULL
        OR to_regclass('spec_tasks') IS NULL
        OR NOT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema = current_schema()
                AND table_name = 'prompt_history_entries' AND column_name = 'spec_task_id'
        )
        OR NOT EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema = current_schema()
                AND table_name = 'spec_tasks' AND column_name = 'planning_session_id'
        ) THEN
        RETURN;
    END IF;

    UPDATE prompt_history_entries p
    SET spec_task_id = t.id
    FROM spec_tasks t
    WHERE p.session_id = t.planning_session_id
        AND t.planning_session_id <> ''
        AND (p.spec_task_id IS NULL OR p.spec_task_id = '')
        AND p.deleted_at IS NULL;
END $$;
