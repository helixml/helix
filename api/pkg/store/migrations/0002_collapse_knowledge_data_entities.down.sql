-- No-op down migration. The up migration lossily collapses multiple
-- per-version data_entity rows into one per knowledge_id, discarding
-- the original timestamps in the IDs. We cannot recover them.
SELECT 1;
