# Implementation Tasks

## Phase 1 — Kodit Library: Document RAG API (kodit repo work)

- [ ] Add `DocumentIndexer` interface to kodit with `IndexDocument(ctx, DocumentIndexRequest)` and `IndexDocuments(ctx, ...DocumentIndexRequest)` methods
- [ ] Add `DocumentSearcher` interface to kodit with `SearchDocuments(ctx, DocumentQueryRequest) ([]DocumentQueryResult, error)`
- [ ] Add `DeleteDocuments(ctx, dataEntityID string) error` to kodit for metadata-filtered bulk delete
- [ ] Implement separate VectorChord table namespace for document RAG (e.g. `kodit_documents` vs `kodit_code_snippets`) with configurable table prefix
- [ ] Implement hybrid search (BM25 + semantic, RRF k=60) for documents — reuse existing code intelligence hybrid search logic
- [ ] Store arbitrary `map[string]string` metadata in JSONB on document table and support filter-by-metadata on search and delete
- [ ] Expose document RAG API from `kodit.Client` so helix can call it in-process

## Phase 2 — File Type Converters

- [ ] Implement Go-based TXT/MD/HTML → plain text converter (in helix knowledge indexer or kodit)
- [ ] Integrate a Go PDF parser (evaluate `pdfcpu` or `ledongthuc/pdfcontent`) and document fidelity limitations vs Python unstructured
- [ ] Integrate a Go DOCX parser (`gooxml` or `docconv`) for Word documents
- [ ] Decide and document whether PDF/DOCX conversion stays in helix (knowledge indexer, pre-extraction) or moves into kodit
- [ ] Validate chunking quality of existing Go splitter (`text.DataPrepTextSplitterChunk`) against haystack's sentence-boundary-aware splitter

## Phase 3 — KoditRAG Adapter in Helix

- [ ] Create `api/pkg/rag/rag_kodit.go` implementing `rag.RAG` interface (`Index`, `Query`, `Delete`)
- [ ] Map `SessionRAGIndexChunk` → `kodit.DocumentIndexRequest` and vice versa in the adapter
- [ ] Map `SessionRAGQuery` → `kodit.DocumentQueryRequest`, including `data_entity_id` filter, `document_id` list filter, `max_results`, `distance_threshold`
- [ ] Map `kodit.DocumentQueryResult` → `SessionRAGResult`
- [ ] Add `RAGProviderKodit = "kodit"` constant to `api/pkg/config/config.go`
- [ ] Wire `KoditRAG` into `serve.go` switch statement (alongside existing Typesense, Llamaindex, Haystack cases)
- [ ] Add build-tag stub (`rag_kodit_nokodit.go`) returning an error if kodit is disabled at compile time
- [ ] Write unit tests for the adapter (mock kodit client)
- [ ] Write integration tests verifying Index → Query → Delete roundtrip

## Phase 4 — Validation

- [ ] Run haystack and KoditRAG in parallel (shadow mode): index same documents with both, compare query results
- [ ] Validate metadata filtering: queries with `data_entity_id` return only correct documents
- [ ] Validate delete: after `Delete(dataEntityID)`, confirm no results returned for that entity
- [ ] Validate re-indexing: mark knowledge versions pending, confirm reconciler re-indexes via KoditRAG
- [ ] Test PDF and DOCX conversion fidelity with representative real-world documents
- [ ] Load test: index 10k+ document chunks and measure query latency vs haystack baseline

## Phase 5 — Cutover

- [ ] Change `RAG_DEFAULT_PROVIDER` default from `haystack` to `kodit` in `config.go`
- [ ] Update docker-compose.yaml: remove `haystack` service, keep `pgvector` (now shared by kodit code + document RAG)
- [ ] Update docker-compose.dev.yaml similarly
- [ ] Write migration runbook: mark all KnowledgeVersions as Pending to trigger re-indexing through KoditRAG
- [ ] Update Helm charts: remove haystack deployment, update kodit config to enable document RAG

## Phase 6 — Haystack Removal

- [ ] Remove `haystack_service/` directory from repository
- [ ] Remove `api/pkg/rag/rag_haystack.go` and `haystack_types.go`
- [ ] Remove `RAGProviderHaystack` from config, serve.go, and all references
- [ ] Remove haystack-related environment variables from documentation and example configs
- [ ] Drop `haystack_documents` and `haystack_documents_vision` tables from VectorChord (post-cutover cleanup)
- [ ] Archive or remove `design/2025-01-07-haystack-rag-integration.md`

## Vision Pipeline (Deferred)

- [ ] Assess whether kodit can support image embeddings (DSE model or CLIP-style)
- [ ] If yes: add vision document indexing to kodit document API
- [ ] If no: document that vision RAG is not supported and track as separate backlog item
