# Requirements: Fix Kodit Embedding Dimension Mismatch After Switching Embedding Provider

## Background

After an admin switched Kodit's text embedding from the built-in ONNX (CPU) provider
to an external OpenAI provider in System Settings, indexing fails on every batch:

```
ERR gorm query error error="ERROR: expected 768 dimensions, not 1536 (SQLSTATE 22000)"
    sql="INSERT INTO \"vectorchord_code_embeddings\" (\"snippet_id\",\"embedding\") VALUES ..."
ERR embedding batch failed error="ERROR: expected 768 dimensions, not 1536 (SQLSTATE 22000)"
    batch_end=27963 operation=create_code_embeddings
```

The built-in ONNX model emits 768-dim vectors; `text-embedding-3-small` emits 1536-dim.
The `vectorchord_code_embeddings.embedding` column is still `VECTOR(768)`, so every
insert is rejected by Postgres.

**Root cause (confirmed by reading kodit v1.3.8 source):**
`VectorChordEmbeddingStore.ensureTable` — which contains the drop-and-recreate-on-
dimension-change logic — is unreachable whenever the table already exists:

- `NewVectorChordEmbeddingStore` probes `pg_class` and sets `tableReady = true` if the
  table exists (`infrastructure/persistence/embedding_store_vectorchord.go:73-81`).
- `ensureTable` returns early on `if s.tableReady.Load()` **before** running the
  `atttypmod` dimension check (same file, `:169-175`).

So the dimension-change branch only ever runs when the table did *not* exist at
construction time — in which case `CREATE TABLE` has just created it at the correct
dimension and the check is a no-op. In practice the migration path is dead code.
Helix's own comments (`api/pkg/server/kodit_init.go:109-115`,
`api/pkg/server/system_settings_handlers.go:140-142`) correctly assume kodit handles
the rebuild, so the Helix-side reinit + rescan-all flow is sound — it is the library
that never rebuilds the table.

Secondary defect in the same failure mode: `onRebuilt` (kodit.go:289-304) enqueues
`CloneRepository` + `SyncRepository` after the rebuild. A plain Sync reuses existing
enrichment IDs, and `filterNewEnrichments` skips snippets already present in the
embedding store, so for a non-Helix caller the freshly recreated table can stay
empty. Helix avoids this by calling `RescanAllRepositories` after reinit.

## User Stories

**US-1 — As a Helix admin, when I switch Kodit's embedding provider or model in
System Settings, code indexing continues to work without manual DB surgery.**

Acceptance criteria:
- Changing `KoditTextEmbeddingProvider` / `KoditTextEmbeddingModel` (or the vision
  equivalents) from a 768-dim provider to a 1536-dim provider results in the
  `vectorchord_*_embeddings` table being dropped and recreated at the new dimension
  on the first indexing run after the change.
- No `expected N dimensions, not M (SQLSTATE 22000)` errors appear in the API log
  after the switch.
- Code search returns results against the new embeddings once re-indexing completes.
- The same holds on a cold restart (settings already saved, stale table on disk) —
  not just on the in-process reinit path.

**US-2 — As a Helix admin, a dimension change does not silently leave a repository
indexed with a partially-populated vector table.**

Acceptance criteria:
- After the rebuild fires, every registered repository is re-indexed and its snippets
  are re-embedded (the "all snippets already have embeddings" skip must not suppress
  the re-embed of a table that was just dropped).
- Repository status in the Code Intelligence admin UI reflects the re-index.

**US-3 — As an operator, I can recover an environment already stuck in this state.**

Acceptance criteria:
- Deploying the fixed build is sufficient — the next indexing run repairs the table.
- No manual `DROP TABLE` is required, and the design documents the manual SQL as an
  emergency-only escape hatch.

## Non-Goals

- Changing the default embedding provider or model.
- Forcing OpenAI's `dimensions` parameter down to 768 to match the old table.
- Preserving existing embeddings across a provider change — they are unusable in a
  different vector space and must be recomputed.
- Adding a Helix-side workaround that issues DDL against the kodit database directly.

## Open Questions

1. **Do we have write access to `github.com/helixml/kodit` and the ability to cut a
   `v1.3.9` tag?** The fix must land there — `v1.3.8` is the latest published version
   (verified against `proxy.golang.org`), so no upstream fix exists to just bump to.
   If the kodit repo is not reachable from the implementation sandbox, this task
   blocks on that access rather than being reworked into a Helix-side workaround.
2. **Confirm the failing environment is the Meta deployment and it is safe to lose the
   existing code index there.** The fix drops and rebuilds `vectorchord_code_embeddings`
   (and `vectorchord_text_embeddings` if its dimension also changed). Re-indexing all
   repositories will re-run embeddings against the paid OpenAI endpoint — the log shows
   ~29,338 snippets for repository 15 alone. Assumed acceptable; flagging the cost.
3. **Should the vision embedding table be treated identically?** Assumed yes — the same
   store type backs `vectorchord_vision_embeddings`, so the fix covers it automatically,
   and no separate handling is planned.
4. **Should Helix pin the fixed kodit by tag or by pseudo-version for verification?**
   Assumed: verify locally with a `replace` directive against a checkout, then pin the
   released tag before opening the Helix PR.
