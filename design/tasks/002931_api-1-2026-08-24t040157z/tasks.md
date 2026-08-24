# Implementation Tasks: Fix Kodit Embedding Dimension Mismatch After Switching Embedding Provider

## kodit (github.com/helixml/kodit)

- [ ] Clone `github.com/helixml/kodit` and branch off the `v1.3.8` tag point / current main
- [ ] In `infrastructure/persistence/embedding_store_vectorchord.go`, replace `tableReady atomic.Bool` with `tableExists atomic.Bool` (read/delete guard) plus `readyDimension atomic.Int32` (validated write dimension)
- [ ] Update `NewVectorChordEmbeddingStore` to set only `tableExists` from the `pg_class` probe
- [ ] Update `ensureTable` to early-return only when `readyDimension == dimension`, otherwise run the existing create → `atttypmod` probe → drop-and-recreate path, then set `readyDimension` and `tableExists`
- [ ] Update `Find` / `Exists` / `DeleteBy` / `Search` guards to use `tableExists`
- [ ] Change `onRebuilt` in `kodit.go` to enqueue a rescan (clears enrichments, forces re-embed) instead of `CloneRepository` + `SyncRepository`
- [ ] Add integration test: pre-create table at `VECTOR(768)`, construct a fresh store, `Index` 1536-dim docs, assert rebuild + `atttypmod == 1536` + rows present + `onRebuilt` fired once + no second rebuild on re-index
- [ ] Add unit test asserting `tableExists` and `readyDimension` are independent
- [ ] Run the integration suite against the dev vectorchord (`VECTORCHORD_TEST_URL=... go test -tags integration ./infrastructure/persistence/`) and confirm green
- [ ] Open the kodit PR, get it merged, and cut tag `v1.3.9`

## helix

- [ ] Verify locally first with a temporary `replace github.com/helixml/kodit => ../kodit` in `api/go.mod`
- [ ] End-to-end in the inner Helix: index a repo on the built-in ONNX provider, confirm `vectorchord_code_embeddings` is `VECTOR(768)` and search works
- [ ] Switch System Settings to the OpenAI text embedding provider (`text-embedding-3-small`), confirm the API log shows `embedding dimension changed, dropping old table` and `rescan-all enqueued`, with no `SQLSTATE 22000`
- [ ] Confirm the column is `VECTOR(1536)` via `psql \d vectorchord_code_embeddings`, wait for re-index, and confirm code search returns results
- [ ] Restart the API (`down` + `up`) and confirm the cold-start path is clean
- [ ] Remove the `replace` directive and bump `api/go.mod` to `github.com/helixml/kodit v1.3.9` (+ `go.sum`)
- [ ] `cd api && go build ./...` and run `go test ./pkg/server/... ./pkg/services/...` (CGO_ENABLED=1 for tree-sitter)
- [ ] Open the Helix PR, check CI with `gh pr checks`, fix any failures
- [ ] Confirm on the affected deployment that indexing recovers without manual `DROP TABLE`
