# Implementation Tasks

## Phase 1 — Validate Kodit File Type Support (blocks everything)

- [ ] Inspect kodit v1.1.8 source to confirm `.txt` and `.md` files are not filtered out by language detection during repo indexing
- [ ] Create a test git repo with `.txt` and `.pdf` (raw) files, register with kodit, verify they appear in semantic/keyword search results
- [ ] If `.txt` is filtered: add a kodit change to allow arbitrary text files before any other work proceeds

## Phase 2 — Kodit File-Type Indexers (kodit repo work)

- [ ] Add PDF indexer to kodit: extract text per page from `.pdf` files when encountered during repo clone/index
- [ ] Add DOCX indexer to kodit: extract paragraphs and headings from `.docx` files
- [ ] Add HTML indexer to kodit: strip tags and extract body text from `.html` files
- [ ] Add PPTX indexer (optional stretch): extract slide text from `.pptx` files
- [ ] Implement metadata sidecar reading: when kodit finds `{filename}.meta.json` alongside a file, merge that JSON into the document's stored metadata in VectorChord
- [ ] Verify end-to-end pipeline: file → indexer → chunker (SimpleChunking) → ONNX embedder → VectorChord → searchable

## Phase 3 — KoditRAG Adapter in Helix

- [ ] Create `api/pkg/rag/rag_kodit.go` implementing `rag.RAG` interface (`Index`, `Query`, `Delete`)
- [ ] `Index()`: for each `SessionRAGIndexChunk`, write raw `Content` bytes to a synthetic git repo keyed by `DataEntityID`; commit; call `kodit.RegisterRepository` or `SyncRepository`; poll `GetRepositoryStatus()` until ready or error
- [ ] `Query()`: look up kodit repo ID for `q.DataEntityID`; call `koditSvc.SemanticSearch` + `KeywordSearch`; merge results with Reciprocal Rank Fusion (k=60); map `KoditFileResult` → `SessionRAGResult`
- [ ] `Delete()`: look up kodit repo ID; call `koditSvc.DeleteRepository`; delete the synthetic git repo
- [ ] Add `KoditRepoID int64` column to `KnowledgeVersion` table (migration) to persist the `DataEntityID → kodit repo ID` mapping
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
