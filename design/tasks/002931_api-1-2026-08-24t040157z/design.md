# Design: Fix Kodit Embedding Dimension Mismatch After Switching Embedding Provider

## Where the fix belongs

In **`github.com/helixml/kodit`**, not in Helix. The Helix side is already correct:

| Helix file | What it does | Verdict |
|---|---|---|
| `api/pkg/server/kodit_embedding_decision.go` | Decides external vs built-in per embedding type | Correct |
| `api/pkg/server/kodit_init.go` `buildKoditOpts` | Wires `provider.NewOpenAIProviderFromConfig` with the `kodit-text-embedding` placeholder model | Correct |
| `api/pkg/server/kodit_init.go` `Reinit` | Rebuilds the client, swaps it atomically, closes the old one, then fires `RescanAllRepositories` | Correct |
| `api/pkg/server/system_settings_handlers.go:140-155` | Triggers `Reinit` in a goroutine when a kodit embedding setting changes | Correct |
| `api/pkg/server/openai_embeddings_handlers.go:65-85` | Substitutes `kodit-text-embedding` → the configured model | Correct |

Helix's only change is a dependency bump.

## The defect in kodit v1.3.8

`infrastructure/persistence/embedding_store_vectorchord.go`:

```go
func NewVectorChordEmbeddingStore(db, taskName, onRebuilt, logger) *VectorChordEmbeddingStore {
    ...
    s.DB(ctx).Raw("SELECT count(*) FROM pg_class WHERE relname = ? AND relkind = 'r'", tableName).Scan(&count)
    if count > 0 {
        s.tableReady.Store(true)      // (1) "exists" is treated as "usable"
    }
}

func (s *VectorChordEmbeddingStore) ensureTable(ctx context.Context, dimension int) error {
    s.tableMu.Lock()
    defer s.tableMu.Unlock()
    if s.tableReady.Load() {
        return nil                    // (2) short-circuits before the dimension check
    }
    ... CREATE TABLE ...
    ... SELECT a.atttypmod ... ; if dbDimension != dimension { DROP + recreate; onRebuilt(ctx) }
}
```

`tableReady` conflates *"the table exists"* (what `Find` / `Exists` / `DeleteBy` need,
so they can return empty instead of erroring on a missing relation) with *"the table
matches the dimension we are about to write"* (what `Index` needs). Because (1) sets
the flag for any pre-existing table, (2) makes the migration branch unreachable in the
exact case it was written for.

Why the tests missed it: `TestVectorChordEmbeddingStore_ExistingTable` pre-creates a
`VECTOR(4)` table and then only calls `Find`. No test pre-creates a table at one
dimension and then calls `Index` at another.

## Fix

Split the two meanings of the flag:

- `tableExists atomic.Bool` — set by the constructor probe; keeps guarding
  `Find` / `Exists` / `DeleteBy` / `Search` exactly as today.
- `readyDimension atomic.Int32` — the dimension this process has validated the table
  against; `0` means "not yet validated in this process".

`ensureTable(ctx, dimension)` early-returns **only** when
`readyDimension.Load() == int32(dimension)`. Otherwise it runs the existing
`CREATE TABLE IF NOT EXISTS` → `atttypmod` probe → drop-and-recreate path, then sets
`readyDimension = dimension` and `tableExists = true`. The existing `tableMu` still
serialises concurrent `Index` callers, and the existing idle-connection reset after the
drop stays as-is.

Net effect: the first `Index` call after a provider swap — whether from an in-process
reinit or a cold start — probes the real column type, sees `768 != 1536`, drops the
table, recreates it as `VECTOR(1536)`, fires `onRebuilt`, and the batch insert succeeds.
Subsequent calls in the same process cost one atomic load.

### Secondary fix: `onRebuilt` must force a re-embed, not a plain sync

`kodit.go:289-304` enqueues `CloneRepository` + `SyncRepository` after a rebuild.
`create_embeddings.go` calls `filterNewEnrichments` → `search.ExistingSnippetIDs`, and
Sync reuses existing enrichment IDs, so for a caller that relies on `onRebuilt` alone
the recreated table can end up permanently empty ("All snippets already have code
embeddings"). Change `onRebuilt` to enqueue the same rescan path
`Repositories.RescanAll` uses, so enrichments are cleared and re-embedded.

Helix is not exposed to this today because `KoditResult.Reinit` already calls
`RescanAllRepositories` itself (`api/pkg/server/kodit_init.go:148-159`) — but the
library should be correct standalone, and this also covers the cold-start path where
Helix never calls `Reinit`.

## Helix-side change

`api/go.mod`: `github.com/helixml/kodit v1.3.8` → `v1.3.9` (plus `go.sum`). Nothing else.

For local verification before the tag exists, use a temporary
`replace github.com/helixml/kodit => ../kodit` in `api/go.mod` and **revert it before
committing** — CI has no sibling checkout.

## Alternatives rejected

- **Send OpenAI's `dimensions: 768` parameter from Helix's embeddings proxy** so the
  vectors fit the existing column. Silently degrades retrieval quality, only works for
  providers that support the parameter, and breaks the moment someone picks a model
  that doesn't — it hides the bug rather than fixing it.
- **Have Helix run the DDL itself** (probe the embedding dimension, `ALTER`/`DROP` the
  `vectorchord_*` tables over `KODIT_DB_URL` before `kodit.New`). Duplicates kodit's
  private schema in Helix and creates two owners of the same tables. Rejected per the
  repo's "root cause it, no workarounds" rule.
- **Version the table name by dimension** (`vectorchord_code_embeddings_1536`). Leaves
  orphaned tables accumulating and changes every read path. Not worth it — a dimension
  change always invalidates every stored vector, so dropping is the honest operation.

## Testing

**kodit (primary).** Add an integration test alongside the existing ones in
`infrastructure/persistence/embedding_store_vectorchord_integration_test.go`
(`//go:build integration`, gated on `VECTORCHORD_TEST_URL`):

1. Pre-create `vectorchord_<task>_embeddings` as `VECTOR(768)` with a row.
2. Construct a **fresh** store (this is what simulates the restart / reinit).
3. `Index` documents carrying 1536-dim vectors.
4. Assert: no error; `atttypmod` is now 1536; the old row is gone; the new rows are
   findable; the `onRebuilt` callback fired exactly once.
5. Index again at 1536 and assert no second rebuild.

Run it against the dev stack's vectorchord:
`VECTORCHORD_TEST_URL="postgresql://postgres:postgres@localhost:5434/kodit" go test -tags integration -run TestVectorChordEmbeddingStore ./infrastructure/persistence/`
(port per `docker-compose.dev.yaml` service `vectorchord-kodit`).

Also add a unit test next to `TestVectorChordEmbeddingStore_TableReadyFlag` covering
that `tableExists` and `readyDimension` are independent.

**Helix (end-to-end, in the inner Helix at `localhost:8080`).** This is the acceptance
gate, not the unit tests:

1. Register / log in, create an org and project, enable kodit, index a repo with the
   default built-in ONNX provider. Confirm `vectorchord_code_embeddings` is
   `VECTOR(768)` and code search returns results.
2. In System Settings, set the Kodit text embedding provider to OpenAI with
   `text-embedding-3-small`. Save.
3. Watch `docker compose -f docker-compose.dev.yaml logs -f api` — expect
   `embedding dimension changed, dropping old table for re-indexing`, then
   `kodit reinit: rescan-all enqueued`, and **no** `SQLSTATE 22000`.
4. Confirm the column is now `VECTOR(1536)`:
   `docker exec <vectorchord-kodit> psql -U postgres -d kodit -c "\d vectorchord_code_embeddings"`
5. Wait for re-index, run a code search, confirm results.
6. Restart the API (`down` + `up`) and confirm no dimension errors on the cold path.

## Gotchas / learnings for future agents

- **`atttypmod` is the vector dimension** for pgvector/VectorChord columns — that is how
  kodit reads the current width (`vcCheckDimensionTemplate`). There is no `information_schema`
  equivalent.
- **The batch failure is not silent, but it is expensive.** `EmbeddingService.Index`
  (`domain/service/embedding.go:186-192`) tolerates up to `MaxFailureRate` (default 5%)
  of failed batches; at 100% failure the task does error out. But every batch still calls
  the embedding endpoint *before* hitting the DB error, so a broken dimension burns the
  full OpenAI cost of the index on every attempt (~29k snippets for repo 15 in the
  reported log). Fixing the table is also a cost fix.
- **`Sync` ≠ `Rescan` in kodit.** Sync skips commits that already have enrichments;
  only Rescan clears them and forces a re-embed. Any "the vectors need rebuilding"
  recovery path must use Rescan.
- **kodit's dimension handling is lazy by design** — no probe at `kodit.New`, so an
  external embedding provider that calls back into Helix's own `/v1/embeddings` does
  not deadlock at startup (see the comment at `api/pkg/server/kodit_init.go:44-48`).
  Keep it that way; do not "fix" this by probing eagerly at construction.
- Reading a pinned Go dependency's source without a checkout:
  `curl -o /tmp/kodit.zip https://proxy.golang.org/github.com/helixml/kodit/@v/v1.3.8.zip && unzip`.
  `https://proxy.golang.org/<module>/@latest` tells you whether a newer release exists.
