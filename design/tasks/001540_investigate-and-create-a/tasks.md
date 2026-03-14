# Implementation Tasks

## Phase 1 — Kodit: Add Document File Support (kodit repo work, blocks everything)

All changes are in `application/handler/indexing/chunk_files.go`.

**Confirmed by reading kodit v1.1.8 source:** `.txt`, `.pdf`, `.docx` are not in `indexableExtensions` (lines 218–274) and will be silently skipped. PDFs also contain null bytes and fail the `isBinary()` check (lines 283–290). These changes are required before the git paradigm can work at all.

- [ ] Add `.txt": true` to `indexableExtensions` — one line change, `.txt` is plain text so no converter needed
- [ ] Add `.pdf` to `indexableExtensions` and add a PDF text-extraction converter: intercept between `read bytes` and `isBinary()`, detect `.pdf` extension, run Go PDF parser, pass extracted text to `NewTextChunks()` (bypassing `isBinary()`)
- [ ] Add `.docx` to `indexableExtensions` with a DOCX text-extraction converter (same intercept pattern as PDF)
- [ ] Add `.pptx` to `indexableExtensions` with a PPTX text-extraction converter (optional stretch)
- [ ] Implement metadata sidecar reading: when kodit encounters `{filename}.meta.json` alongside an indexed file, merge that JSON into the stored enrichment metadata so it is searchable/filterable
- [ ] Write a test verifying `.txt`, `.pdf`, `.docx` files in a git repo are indexed and appear in search results (extend or mirror `TestChunkFiles_OnlyIndexesSourceAndDocFiles`)

## Phase 3 — KoditRAG Adapter in Helix

- [ ] Create `api/pkg/rag/rag_kodit.go` implementing `rag.RAG` interface (`Index`, `Query`, `Delete`)
- [ ] `Index()`: for each `SessionRAGIndexChunk`, write raw `Content` bytes to a synthetic git repo keyed by `DataEntityID`; commit; call `kodit.RegisterRepository` or `SyncRepository`; poll `GetRepositoryStatus()` until ready or error
- [ ] `Query()`: look up kodit repo ID for `q.DataEntityID`; call `koditSvc.SemanticSearch` + `KeywordSearch`; merge results with Reciprocal Rank Fusion (k=60); map `KoditFileResult` → `SessionRAGResult`
- [ ] `Delete()`: look up kodit repo ID; call `koditSvc.DeleteRepository`; delete the synthetic git repo
- [ ] Add `KoditRepoID int64` column to `KnowledgeVersion` table (migration) to persist the `DataEntityID → kodit repo ID` mapping
- [ ] Implement `KoditFileResult → SessionRAGResult` mapping in adapter: `Content` = full `Preview` text, `ContentOffset` = parsed `StartLine` from `Lines` field, `DocumentID` = `sha256(Content)`, `DocumentGroupID` = `sha256(Source)`, `Distance` = `1.0 - Score`
- [ ] Implement RRF (k=60) merge of semantic and keyword search results before returning from `Query()`
- [ ] Add `RAGProviderKodit = "kodit"` constant to `api/pkg/config/config.go`
- [ ] Wire `KoditRAG` into `serve.go` switch statement
- [ ] Add build-tag stub `api/pkg/rag/rag_kodit_nokodit.go` (returns error when kodit is disabled at compile time)
- [ ] Write unit tests for the adapter (mock KoditServicer and GitRepositoryServicer)
- [ ] Write integration test: upload files → Index → Query → Delete roundtrip

## Phase 4 — Knowledge Indexer Adjustments

- [ ] When kodit is the RAG provider, automatically set `DisableChunking=true` so whole files are sent (not pre-chunked text) — kodit handles chunking internally
- [ ] Commit per-file `.metadata.yaml` content from filestore as `{filename}.meta.json` into the synthetic git repo so kodit picks up custom metadata
- [ ] Handle async indexing: poll kodit repo status after sync with a configurable timeout; surface indexing errors back through the knowledge reconciler state machine
- [ ] Test re-indexing path: update a file in a knowledge base, verify the git repo is updated and kodit re-indexes the changed file

## Phase 5 — Validation

- [ ] Run haystack and KoditRAG in parallel (shadow mode) on the same documents; compare search result quality
- [ ] Validate metadata filtering: queries with `data_entity_id` return only documents from the correct knowledge entity
- [ ] Validate delete: after `Delete(dataEntityID)`, no results returned and git repo is removed from kodit
- [ ] Test PDF fidelity: index a complex multi-column PDF, check if key content is retrievable
- [ ] Test DOCX fidelity: index a Word document with headings and tables
- [ ] Load test: 10k+ document chunks, measure indexing throughput and query latency vs haystack baseline

## Phase 6 — Cutover

- [ ] Change `RAG_DEFAULT_PROVIDER` default to `kodit` in `config.go`
- [ ] Write migration runbook: mark all `KnowledgeVersion` records as Pending to trigger re-indexing through KoditRAG
- [ ] Update `docker-compose.yaml`: remove `haystack` service; kodit reuses existing `pgvector` container
- [ ] Update `docker-compose.dev.yaml` similarly
- [ ] Update Helm charts: remove haystack deployment, update kodit config

## Phase 7 — Haystack Removal

- [ ] Remove `haystack_service/` directory from repository
- [ ] Remove `api/pkg/rag/rag_haystack.go` and `api/pkg/rag/haystack_types.go`
- [ ] Remove `RAGProviderHaystack` from `config.go`, `serve.go`, and all references
- [ ] Remove haystack environment variables from docs and example configs
- [ ] Drop `haystack_documents` and `haystack_documents_vision` tables from VectorChord (post-cutover cleanup script)

## Vision Pipeline (Deferred)

- [ ] Assess kodit image embedding support (CLIP-style or DSE model)
- [ ] If feasible: add image file indexer to kodit and handle `Pipeline=VisionPipeline` in `KoditRAG.Index()`
- [ ] If not feasible: document vision RAG as unsupported and track separately
