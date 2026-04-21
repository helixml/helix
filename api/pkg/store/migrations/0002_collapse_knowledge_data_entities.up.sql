-- Collapse per-version data_entity rows into one per knowledge source.
--
-- Background: before this change, each re-index of a knowledge source
-- produced a new data_entity row with ID "<knowledge_id>-<version>" (e.g.
-- "kno_01k...-2026-04-21_09-09-59"). Every row pointed at the same
-- underlying kodit repository (kodit tracks commits over time itself —
-- there was no need for Helix to duplicate the concept). Pruning old
-- versions would delete the shared kodit repo and orphan the newer ones.
--
-- After this change, each knowledge source has a single data_entity row
-- keyed on just the knowledge_id. This migration consolidates any existing
-- versioned rows: keep the most recently created row per knowledge, rename
-- its ID to the bare knowledge_id, drop the rest.
--
-- The KnowledgePrefix is "kno_". The underscore after "kno" is part of the
-- prefix, not a split boundary — split on '-' to lop off the version
-- suffix. Canonical knowledge IDs contain no dashes.

-- Guard on the table existing: on a fresh database migrations run before
-- GORM AutoMigrate creates the schema, so data_entities may not exist yet.
-- When it doesn't, there's nothing to consolidate and this migration is a
-- no-op. On existing deployments the table is already there.
DO $$
DECLARE
    r RECORD;
    new_id TEXT;
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_tables WHERE tablename = 'data_entities') THEN
        RETURN;
    END IF;

    FOR r IN
        SELECT DISTINCT split_part(id, '-', 1) AS knowledge_id
        FROM data_entities
        WHERE id LIKE 'kno\_%-%' ESCAPE '\'
    LOOP
        new_id := r.knowledge_id;

        -- If a canonical row already exists, leave it alone — the newer
        -- code will have written directly to it.
        CONTINUE WHEN EXISTS (SELECT 1 FROM data_entities WHERE id = new_id);

        -- Promote the most recently created versioned row to the canonical
        -- ID; this preserves the live kodit_repository_id / filestore_path.
        UPDATE data_entities
        SET id = new_id
        WHERE id = (
            SELECT id FROM data_entities
            WHERE id LIKE r.knowledge_id || '-%'
            ORDER BY created DESC
            LIMIT 1
        );
    END LOOP;

    -- Anything still matching the versioned pattern is now stale history;
    -- safe to drop.
    DELETE FROM data_entities WHERE id LIKE 'kno\_%-%' ESCAPE '\';
END $$;
