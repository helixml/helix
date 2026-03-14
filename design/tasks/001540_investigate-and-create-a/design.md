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

The haystack service is a separate container. The Go side calls it over HTTP. The `rag.RAG` interface (`Index`, `Query`, `Delete`) is the only coupling point between the knowledge indexer and the RAG provider.

### Kodit Stack (existing)

```
helix-api (Go)
  └── kodit.Client (in-process Go library, github.com/helixml/kodit v1.1.8)
        ├── SimpleChunking strategy (text-based, not AST-specific)
        ├── ONNX embedder (local hugot model)
        ├── VectorChord PostgreSQL (same tech, separate tables)
        └── BM25 + semantic hybrid search
```

Kodit is already embedded as a Go library. Helix runs a native git HTTP server at `/git/{repo_id}` with token authentication. Kodit clones from this server via `http://api:KEY@api:8080/git/{repo_id}`. Currently kodit only indexes git repositories (code files).

---

## Target Architecture: Filestore-as-Git-Repo

Rather than adding a document push API to kodit, we expose each knowledge entity's files as a synthetic git repository and let kodit index it using its existing git-clone pipeline. Kodit gains dedicated file-type indexers (PDF, DOCX, etc.) that it runs on files as it encounters them during repo indexing.

```
helix-api (Go)
  ├── knowledge_indexer.go
  │     └── writes raw files → git repo (one repo per knowledge entity)
  │           └── triggers KoditRAG.Index() → kodit.RegisterRepository(cloneURL)
  │
  └── KoditRAG (implements rag.RAG)
        └── kodit.Client
              ├── clones git repo via helix HTTP git server
              ├── PDF indexer    (new in kodit)
              ├── DOCX indexer   (new in kodit)
              ├── TXT/MD/HTML indexer (new in kodit)
              ├── SimpleChunking → ONNX embedder
              └── VectorChord (shared PostgreSQL)
                    ├── kodit_code_snippets  (existing, code intelligence)
                    └── kodit_rag_documents  (same infra, separate table or repo-scoped)
```

The haystack Python service is eliminated. File conversion, chunking, and embedding all run inside the kodit Go library.

---

## How the Indexing Flow Changes

### Current flow (haystack)

```
1. knowledge_indexer fetches raw bytes from filestore
2. (Optionally) pre-chunks text in helix via text.DataPrepTextSplitterChunk
3. Calls ragClient.Index(chunks...) — pushes content over HTTP to haystack
4. Haystack: PDF/DOCX → text, chunks, embeds, stores in VectorChord
```

### New flow (kodit git approach)

```
1. knowledge_indexer fetches raw bytes from filestore
2. Writes raw files into a git repo (one repo per knowledge entity, keyed by DataEntityID)
   — using helix's existing git HTTP server
3. Calls ragClient.Index(files...) → KoditRAG.Index()
   a. Creates or updates the git repo via helix GitRepositoryService
   b. Commits raw files (PDF, DOCX, TXT, etc.) to the repo
   c. Calls kodit.RegisterRepository(cloneURL) or kodit.SyncRepository(repoID)
   d. Polls kodit.GetRepositoryStatus() until indexing completes (or errors)
4. Kodit (internal, git-clone-based):
   — Clones repo, detects file types
   — Runs PDF/DOCX/TXT/HTML indexer (new kodit code)
   — Extracts text, chunks via SimpleChunking, embeds via ONNX
   — Stores in VectorChord
```

Key difference: helix sends **raw files**, not pre-chunked text. Kodit owns chunking and conversion.

### Implication for the knowledge indexer

The knowledge indexer should skip pre-chunking when using kodit. The `DisableChunking` flag already exists on `RAGSettings`; for the kodit provider it should default to true (kodit chunks internally). The indexer would call `indexDataDirectly()` which sends whole files — the KoditRAG adapter receives them and writes to git rather than pushing to a vector store.

To avoid the KoditRAG adapter having to re-assemble chunked content, the `rag.RAG` interface's `Index()` semantics shift for this provider: each `SessionRAGIndexChunk` received by `KoditRAG.Index()` is treated as a whole file (one chunk = one file to write to git), not a sub-document chunk. The `Source`/`Filename` fields identify the file; `Content` carries raw bytes.

---

## Required Changes in Kodit (`github.com/helixml/kodit`)

### Source of truth: `chunk_files.go`

The entire file-selection logic lives in one place:
`application/handler/indexing/chunk_files.go`

The pipeline per file is:
```
isIndexable(path)    → extension whitelist check (lines 276–281)
  ↓ pass
read bytes           → FileContent()
  ↓
isBinary(content)    → null-byte probe of first 8KB (lines 283–290)
  ↓ pass
NewTextChunks(text)  → SimpleChunking into fixed-size rune chunks
  ↓
save to VectorChord
```

`.txt`, `.pdf`, and `.docx` are **not** in `indexableExtensions` (lines 218–274), so they are silently skipped at the first step. PDFs additionally contain null bytes and would also fail `isBinary()`. Neither `.txt` nor any document format currently passes through to chunking.

### 1. Add `.txt` to `indexableExtensions`

One-liner: add `".txt": true` to the map. `.txt` files are already plain text so no converter is needed — they flow straight into `NewTextChunks()`.

### 2. File-Type Converters for Binary Document Formats

For PDF, DOCX, and PPTX the intercept point is between `read bytes` and `isBinary()`. When the file extension is a known document format, run a converter to produce plain text, then pass that text directly to `NewTextChunks()`, skipping the `isBinary()` check entirely:

```
isIndexable(path)              → must add: .pdf, .docx, .pptx, .txt, .html
  ↓ pass
read bytes                     → FileContent()
  ↓
isConvertible(ext)?            → NEW: .pdf / .docx / .pptx branch
  → convert(bytes) → text      → PDF parser, DOCX parser, etc.
  → NewTextChunks(text)
isConvertible = false?
  → isBinary(content)          → existing null-byte check
  → NewTextChunks(string(content))
```

| Extension | Converter approach |
|-----------|-------------------|
| `.txt` | None — add to whitelist and pass through as-is |
| `.html`, `.htm` | Already in whitelist; already text (tag stripping optional) |
| `.md`, `.rst`, `.adoc` | Already in whitelist; already text |
| `.pdf` | Add to whitelist + Go PDF parser (extract text per page) |
| `.docx` | Add to whitelist + Go DOCX parser (paragraphs, headings) |
| `.pptx` | Add to whitelist + Go PPTX parser (slide text), optional |
| images | Deferred (vision pipeline) |

The developer has confirmed they will build these converters in kodit.

### 3. Configuration of SimpleChunking for Documents

The default kodit setup in helix already uses `WithSimpleChunking()` (kodit_init.go line 44). SimpleChunking parameters (size=1500 runes, overlap=200, min=50) are reasonable for documents but may need tuning separately from code. The `ChunkParams` struct supports this.

### 4. Metadata Pass-Through

Knowledge entities have per-file `.metadata.yaml` files in the filestore with custom `map[string]string` metadata. These can be committed to the git repo alongside the data files (as `{filename}.metadata.yaml`). Kodit must read these sidecar files and store the metadata in VectorChord JSONB for filtering.

Alternatively, metadata can be encoded into the git repo structure (e.g., a single `_metadata.json` at the root). Simpler.

### 5. Repo-Scoped Search (already supported)

Kodit's existing `SearchCodeWithScores` / `SearchKeywordsWithScores` accept a `koditRepoID` filter. This naturally maps to `data_entity_id` — one kodit repo per knowledge entity. No new filtering API needed.

### 6. Async Indexing and Status Polling

Kodit indexes in the background after `RegisterRepository()`/`SyncRepository()`. Helix's knowledge indexer needs to poll `GetRepositoryStatus()` (already exists in `KoditService`) until the repo reaches a terminal state (ready or error). The KoditRAG adapter can encapsulate this polling loop.

---

## Helix-Side Changes

### KoditRAG Adapter (`api/pkg/rag/rag_kodit.go`)

```go
type KoditRAG struct {
    koditSvc    KoditServicer          // existing service interface
    gitSvc      GitRepositoryServicer  // to create/manage synthetic git repos
    db          Store                  // to persist DataEntityID → git repo ID mapping
}

func (r *KoditRAG) Index(ctx context.Context, files ...*types.SessionRAGIndexChunk) error
// For each file: write to git repo (keyed by DataEntityID), commit
// Then: kodit.RegisterRepository or SyncRepository
// Then: poll GetRepositoryStatus until ready

func (r *KoditRAG) Query(ctx context.Context, q *types.SessionRAGQuery) ([]*types.SessionRAGResult, error)
// Look up kodit repo ID for q.DataEntityID
// Call koditSvc.SemanticSearch + KeywordSearch, merge results (RRF)
// Map KoditFileResult → SessionRAGResult

func (r *KoditRAG) Delete(ctx context.Context, req *types.DeleteIndexRequest) error
// Look up kodit repo ID for req.DataEntityID
// Call koditSvc.DeleteRepository
// Delete the synthetic git repo
```

The `rag.RAG` interface is unchanged. All three types (`SessionRAGIndexChunk`, `SessionRAGQuery`, `SessionRAGResult`) and `DeleteIndexRequest` are unchanged.

### DataEntityID → Git Repo Mapping

A mapping table (or stored in `KnowledgeVersion.EmbeddingsModel` field repurposed, or a new `KnowledgeVersion` field) is needed to associate a `DataEntityID` with a kodit repo ID. Options:
- Add `KoditRepoID int64` to the `KnowledgeVersion` table (cleanest)
- Store a deterministic name (`knowledge-{DataEntityID}`) and look up by name

### Knowledge Indexer Adjustment

When kodit is the RAG provider, set `DisableChunking=true` so the indexer sends whole files rather than pre-chunked text. This can be done by the `KoditRAG` adapter exposing a method, or by convention (knowledge bases default to no-chunk when provider=kodit).

---

## File Storage Integration

Helix filestore (local disk, S3, GCS) remains the source of truth for raw uploaded files. The git repo is a derived view of those files, committed by the knowledge indexer.

The synthetic git repos live in helix's existing git server (the same one code intelligence uses). They are private and only accessed by kodit via the authenticated clone URL.

### Re-indexing Strategy

Existing haystack vector data does **not** need to be migrated:
- On cutover, mark all existing knowledge versions as "Pending"
- The knowledge reconciler re-indexes everything: writes files to git repos, registers with kodit
- Haystack VectorChord tables can be dropped after re-indexing completes

---

## Interfaces to Maintain (no upstream changes)

These types in `api/pkg/types/types.go` remain unchanged:

| Type | Fields |
|------|--------|
| `SessionRAGIndexChunk` | DataEntityID, Source, Filename, DocumentID, DocumentGroupID, ContentOffset, Content, Metadata, Pipeline |
| `SessionRAGQuery` | Prompt, DataEntityID, DistanceThreshold, DistanceFunction, MaxResults, ExhaustiveSearch, DocumentIDList, Pipeline |
| `SessionRAGResult` | ID, SessionID, InteractionID, DocumentID, DocumentGroupID, Filename, Source, ContentOffset, Content, Distance, Metadata |
| `DeleteIndexRequest` | DataEntityID |

The `rag.RAG` interface (`Index`, `Query`, `Delete`) is the sole coupling between the knowledge indexer and any RAG provider. The external HTTP API for knowledge bases (`/api/v1/knowledge/...`) is unchanged.

---

## Remaining Migration Challenges

1. **Async indexing latency**: Kodit indexes asynchronously after repo registration. The polling loop in KoditRAG must handle long-running indexing jobs (large PDF collections) without timing out. May need a configurable timeout.

2. **Kodit file-type filtering**: Without reading kodit v1.1.8 source, it's unknown whether `.txt` and `.pdf` files are currently skipped by language detection. This must be validated early — if kodit silently ignores non-code file types, this approach fails at the first step.

3. **Result mapping — ContentOffset**: Kodit returns file paths and line ranges; `SessionRAGResult` expects `ContentOffset` (byte offset). Line-to-byte offset mapping is needed. Can be computed from the stored file content if needed.

4. **Vision pipeline**: Image embeddings are not supported by kodit. Deferred — the haystack vision pipeline could theoretically be kept in parallel for vision-enabled knowledge bases, or this feature is dropped initially.

5. **Hybrid search merge in helix**: Kodit exposes `SemanticSearch` and `KeywordSearch` as separate calls. The KoditRAG adapter must merge results using Reciprocal Rank Fusion (same as haystack's implementation, k=60). This logic lives in the adapter.

6. **Metadata sidecar format**: `.metadata.yaml` files from filestore need to be committed into the synthetic git repo and read by kodit. A simple convention (e.g., commit `{filename}.meta.json` alongside each file) must be agreed between helix and kodit.

---

## Migration Order

**Phase 1 — Validate kodit file type support** (fast check, blocks everything)
- Inspect kodit v1.1.8 source to confirm `.txt` files are indexed (not filtered by language detection)
- Create a test git repo with a `.txt` file, register it with kodit, verify it appears in search results
- If `.txt` files are filtered: add a kodit change to allow arbitrary text files before proceeding

**Phase 2 — Kodit file-type indexers** (kodit repo work)
- Add PDF indexer to kodit (text extraction per page)
- Add DOCX indexer to kodit (paragraph extraction)
- Add HTML indexer to kodit (tag stripping)
- Add metadata sidecar reading (`.meta.json` alongside files)
- Verify chunking and embedding pipeline works end-to-end for each type

**Phase 3 — KoditRAG adapter in helix**
- Implement `api/pkg/rag/rag_kodit.go`
- Implement DataEntityID → git repo mapping (add `KoditRepoID` to `KnowledgeVersion`)
- Implement RRF merge for hybrid search results
- Wire `RAGProviderKodit` into config and serve.go
- Unit + integration tests

**Phase 4 — Knowledge indexer adjustments**
- Set DisableChunking=true automatically for kodit provider
- Handle async indexing status polling
- Test full upload → index → query roundtrip

**Phase 5 — Validation**
- Run haystack and kodit in parallel (shadow mode)
- Compare search quality on real-world PDF, DOCX, TXT documents
- Load test: 10k+ documents, measure indexing time and query latency

**Phase 6 — Cutover and haystack removal**
- Switch `RAG_DEFAULT_PROVIDER=kodit`
- Mark all KnowledgeVersions as Pending to trigger re-indexing
- Remove `haystack_service/` and `api/pkg/rag/rag_haystack.go`
- Update docker-compose and Helm charts

---

## Configuration Changes

No new top-level config needed. The existing `Kodit` struct in `config.go` covers the database URL and git URL. Add:
```
RAG_DEFAULT_PROVIDER=kodit   # new valid value
```

The kodit VectorChord instance (`KODIT_DB_URL`) serves both code intelligence and document RAG. No separate database needed.

---

## Patterns Found in This Codebase

- RAG providers are selected by `RAG_DEFAULT_PROVIDER` env var, switch-cased in `serve.go`
- All RAG implementations satisfy `rag.RAG` interface (`api/pkg/rag/rag.go`)
- Knowledge indexer at `api/pkg/controller/knowledge/knowledge_indexer.go` is RAG-provider-agnostic
- Kodit uses build tags (`//go:build !nokodit`) — KoditRAG adapter should follow the same pattern
- Helix git HTTP server at `/git/{repo_id}` already serves authenticated repos for code intelligence
- `BuildAuthenticatedCloneURL(repoID, apiKey)` in `git_repository_service.go` produces URLs kodit can clone
- `KoditService.GetRepositoryStatus()` already exists for polling indexing progress
- File metadata is loaded from `{filename}.metadata.yaml` in filestore — needs to be committed to git repo alongside data files
