# Design: Haystack → Kodit RAG Migration

## Current State

### Haystack RAG Stack

```
helix-api (Go)
  └── rag.HaystackRAG (HTTP client)
        └── haystack_service (Python FastAPI, port 8000)
              ├── LocalUnstructuredConverter  (PDF, DOCX, TXT, HTML)
              ├── DocumentSplitter            (500-char chunks, 50-char overlap)
              ├── EmbeddingProvider           (VLLM or UNIX socket)
              └── VectorchordDocumentStore    (PostgreSQL 17 + pgvector + BM25)
                    └── table: haystack_documents
```

The haystack service is a separate container. The Go side calls it over HTTP. The `rag.RAG` interface (`Index`, `Query`, `Delete`) is the only coupling point.

### Kodit Stack (existing)

```
helix-api (Go)
  └── kodit.Client (in-process Go library)
        ├── ONNX embedder (local model, hugot)
        ├── VectorChord PostgreSQL (same tech, separate DB or tables)
        └── BM25 + semantic hybrid search
```

Kodit is already embedded as a Go library and already uses VectorChord with ONNX embeddings. However, it currently only indexes **git repositories** (code snippets), not arbitrary documents.

---

## Target Architecture

```
helix-api (Go)
  └── rag.KoditRAG  (new, implements rag.RAG)
        └── kodit.Client (extended to support arbitrary document indexing)
              ├── DocumentConverter  (Go: PDF, DOCX, TXT, HTML support)
              ├── Chunker            (configurable chunk size + overlap)
              ├── ONNX embedder      (same model as code intelligence)
              └── VectorChord        (shared PostgreSQL instance)
                    ├── table: kodit_code_snippets   (existing)
                    └── table: kodit_documents       (new, for RAG)
```

The haystack Python service is eliminated. All document indexing and search runs in the helix Go binary, using kodit's embedding and storage infrastructure.

---

## Required Changes in Kodit (`github.com/helixml/kodit`)

These are **blockers** — kodit must provide these before the adapter can be built.

### 1. Arbitrary Document Indexing API
Currently kodit only accepts git repositories. It needs a new API surface for indexing document chunks:

```go
// New kodit API needed
type DocumentIndexRequest struct {
    DataEntityID    string
    DocumentID      string
    DocumentGroupID string
    Source          string
    Filename        string
    ContentOffset   int
    Content         string
    Metadata        map[string]string
}

type DocumentQueryRequest struct {
    Query        string
    DataEntityID string
    DocumentIDs  []string
    MaxResults   int
    Threshold    float64
}

type DocumentQueryResult struct {
    DocumentID      string
    DocumentGroupID string
    Source          string
    Filename        string
    ContentOffset   int
    Content         string
    Score           float64
    Metadata        map[string]string
}

type DocumentDeleteRequest struct {
    DataEntityID string
}
```

### 2. Metadata-Filtered Delete
Kodit needs `DeleteDocuments(ctx, DataEntityID)` to remove all chunks belonging to a knowledge entity.

### 3. File Type Converters in Go
Currently the haystack Python service uses the `unstructured` library for PDF/DOCX parsing. Kodit (or helix) needs Go-based equivalents:
- **PDF**: `pdfcpu`, `ledongthuc/pdfcontent`, or calling `pdftotext`
- **DOCX**: `gooxml` or `docconv`
- **TXT/MD**: direct read
- **HTML**: `golang.org/x/net/html` stripper

This is the most significant engineering effort. Options:
- A. Embed lightweight Go PDF/DOCX parsers in kodit
- B. Keep a minimal Python converter sidecar just for format conversion (pre-conversion, not full haystack)
- C. Send raw bytes to kodit and let kodit handle conversion

**Recommendation**: Option A for TXT/MD/HTML (easy), Option C with kodit owning converters for PDF/DOCX.

### 4. Separate VectorChord Table Namespace
Kodit's existing code snippet table must not conflict with document RAG tables. Kodit should use a configurable table prefix (e.g., `kodit_documents` vs `kodit_code_snippets`).

### 5. BM25 Hybrid Search for Documents
Kodit already implements hybrid search for code. This should be reusable for documents with the same VectorChord BM25 + pgvector approach.

---

## Helix-Side Changes (new `KoditRAG` adapter)

File: `api/pkg/rag/rag_kodit.go`

```go
type KoditRAG struct {
    client kodit.Client  // extended to support document RAG
}

func (r *KoditRAG) Index(ctx context.Context, chunks ...*types.SessionRAGIndexChunk) error
func (r *KoditRAG) Query(ctx context.Context, q *types.SessionRAGQuery) ([]*types.SessionRAGResult, error)
func (r *KoditRAG) Delete(ctx context.Context, req *types.DeleteIndexRequest) error
```

- `Index`: converts `SessionRAGIndexChunk` → `kodit.DocumentIndexRequest`, calls kodit
- `Query`: converts `SessionRAGQuery` → `kodit.DocumentQueryRequest`, maps results back to `SessionRAGResult`
- `Delete`: calls `kodit.DeleteDocuments(dataEntityID)`

Add `RAGProviderKodit = "kodit"` to `config.go` and wire into `serve.go` switch statement.

---

## File Storage Integration

Helix filestore (local disk, S3, GCS) holds raw uploaded files. The knowledge indexer (`knowledge_indexer.go`) already:
1. Fetches raw bytes from filestore
2. Creates `SessionRAGIndexChunk` with content pre-extracted (if chunking enabled) or raw bytes
3. Calls `ragClient.Index()`

The current haystack service does the document conversion (PDF → text) inside its `/process` endpoint. With kodit, there are two options:

**Option A (preferred)**: Keep pre-processing in the knowledge indexer (Go side). The indexer already does chunking via `text.DataPrepTextSplitterChunk`. Add Go-based format converters here, so raw bytes become text before reaching kodit. Kodit receives text-only chunks.

**Option B**: Push file bytes to kodit and let kodit convert. Requires kodit to bundle converters.

Option A is simpler and keeps conversion logic in helix where it already partially lives.

### Re-indexing Strategy
Existing haystack vector data does **not** need to be migrated. Instead:
- On deprecation cutover, mark all existing knowledge versions as "Pending"
- The knowledge reconciler will re-index everything through kodit
- Haystack tables can be dropped after re-indexing completes

---

## Interfaces to Maintain (no upstream changes)

These types in `api/pkg/types/types.go` must remain unchanged:

| Type | Fields |
|------|--------|
| `SessionRAGIndexChunk` | DataEntityID, Source, Filename, DocumentID, DocumentGroupID, ContentOffset, Content, Metadata, Pipeline |
| `SessionRAGQuery` | Prompt, DataEntityID, DistanceThreshold, DistanceFunction, MaxResults, ExhaustiveSearch, DocumentIDList, Pipeline |
| `SessionRAGResult` | ID, SessionID, InteractionID, DocumentID, DocumentGroupID, Filename, Source, ContentOffset, Content, Distance, Metadata |
| `DeleteIndexRequest` | DataEntityID |

The `rag.RAG` interface (`Index`, `Query`, `Delete`) is the sole integration point and must remain as-is.

HTTP API endpoints for knowledge (`/api/v1/knowledge/...`) are unchanged.

---

## Hard Migration Challenges

1. **Vision Pipeline**: Haystack has a separate vision pipeline (image embeddings via DSE model). Kodit has no image embedding support. This is a blocker for full parity — either defer vision RAG or add vision embedding to kodit separately.

2. **PDF/DOCX Conversion**: The `unstructured` Python library handles messy real-world PDFs well (tables, headers, footnotes). Go PDF libraries are less capable. The hardest documents to migrate are complex PDFs with mixed layouts. Short-term mitigation: accept reduced PDF fidelity, or keep a minimal Python converter as a preprocessing step.

3. **Chunking Quality**: Haystack's `DocumentSplitter` respects sentence boundaries. The existing Go splitter in helix (`text.DataPrepTextSplitterChunk`) must be validated to produce equivalent quality chunks.

4. **RRF Hybrid Search**: Haystack uses Reciprocal Rank Fusion (k=60) to merge semantic + BM25 results. Kodit must implement the same (or equivalent) for document search.

5. **Metadata YAML**: The knowledge indexer loads per-file `.metadata.yaml` from filestore and passes custom metadata to the indexer. Kodit must store and filter on arbitrary `map[string]string` metadata.

---

## Migration Order

**Phase 1 — Kodit gains document RAG API** (kodit library work)
- Add `DocumentIndexer` and `DocumentSearcher` interfaces to kodit
- Implement metadata-filtered delete
- Separate VectorChord table namespace for documents

**Phase 2 — File type converters** (kodit or helix)
- TXT, MD, HTML: trivial (Go)
- PDF: Go-native parser (accept reduced fidelity initially)
- DOCX: `gooxml` or `docconv`

**Phase 3 — KoditRAG adapter in helix**
- Implement `api/pkg/rag/rag_kodit.go`
- Wire up `RAGProviderKodit` in config and serve.go
- Unit + integration tests

**Phase 4 — Validation**
- Run both haystack and kodit in parallel on test data
- Compare search quality (recall, precision) on known document sets
- Validate metadata filtering, deletes, re-indexing

**Phase 5 — Cutover**
- Set `RAG_DEFAULT_PROVIDER=kodit` as default
- Mark all knowledge versions pending for re-indexing
- Monitor error rates

**Phase 6 — Haystack removal**
- Remove `haystack_service/` directory
- Remove `RAGProviderHaystack` from config
- Remove haystack from docker-compose
- Remove `api/pkg/rag/rag_haystack.go`

---

## Configuration Changes

New config (in `config.go` Kodit struct, or new RAG sub-config):
```
KODIT_RAG_ENABLED=true
KODIT_RAG_TABLE_PREFIX=kodit_rag  # separate from code intelligence tables
```

The existing `KODIT_DB_URL` (VectorChord) can be shared between code intelligence and document RAG — same PostgreSQL instance, different tables.

---

## Patterns Found in This Codebase

- RAG providers are selected by `RAG_DEFAULT_PROVIDER` env var, switch-cased in `serve.go`
- All RAG implementations must satisfy the `rag.RAG` interface (`api/pkg/rag/rag.go`)
- Knowledge indexer at `api/pkg/controller/knowledge/knowledge_indexer.go` is the orchestrator; it's RAG-provider-agnostic
- Kodit uses build tags (`//go:build !nokodit`) — new KoditRAG adapter should follow the same pattern
- VectorChord (PostgreSQL 17 with pgvector + BM25) is already operational for kodit code intelligence
- File metadata is loaded from `{filename}.metadata.yaml` in filestore and passed as `map[string]string` to the RAG index call
