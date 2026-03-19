# Design: Kodit-Based RAG Implementation

## Architecture Overview

Kodit performs document ingestion (conversion) and embedding in one operation when given a `file://` URI pointing to a local directory. This replaces helix's normal extract → chunk → embed → `rag.Index()` pipeline. The kodit RAG implementation wires into two distinct points of the knowledge indexing pipeline:

1. **Registration** (replaces conversion): registers the filestore directory with kodit, stores the returned repository ID
2. **Query**: uses `KoditServicer.SemanticSearch()` with the stored repo ID
3. **Index** (`rag.Index()`): no-op — kodit already indexed during registration
4. **Delete**: calls `KoditServicer.DeleteRepository()` with the stored repo ID

```
Normal pipeline:  extractFiles → chunkText → embed → rag.Index(chunks)
Kodit pipeline:   RegisterDirectory(file://path) → store repoID → rag.Index() [no-op]
                  Query → KoditServicer.SemanticSearch(repoID, query)
```

## Kodit Module Update

Update `go.mod` to use the commit that adds `file://` URI support:

```
go get github.com/helixml/kodit@417f16b7dfce928b0e9d1a888454cfc6cbe98892
```

The commit adds `isFileURI()`, `localPathFromFileURI()` helpers in `cloner.go` and modifies `Clone()`/`Update()` to skip git operations for `file://` sources.

## Database: New Field on DataEntity

**File**: `api/pkg/types/types.go`

Add a nullable `int64` column to `DataEntity`:

```go
type DataEntity struct {
    // ... existing fields ...
    KoditRepositoryID *int64 `json:"kodit_repository_id,omitempty" gorm:"column:kodit_repository_id"`
}
```

GORM AutoMigrate (already called at startup with `DATABASE_AUTO_MIGRATE=true`) will add the nullable column automatically. No manual migration needed.

The `DataEntity.Config.FilestorePath` already holds the logical filestore path; `KoditRepositoryID` maps that path to a kodit repository.

## New KoditRAG Implementation

**File**: `api/pkg/rag/rag_kodit.go` (build tag `//go:build !nokodit`)

```go
type KoditRAG struct {
    kodit  services.KoditServicer
    store  store.Store
    fsCfg  config.FileStore  // to resolve local FS path
}

func NewKoditRAG(kodit services.KoditServicer, store store.Store, fsCfg config.FileStore) *KoditRAG

// Index is a no-op: kodit already indexed during directory registration
func (k *KoditRAG) Index(ctx, chunks...) error { return nil }

// Query uses SemanticSearch with the repo ID stored on the DataEntity
func (k *KoditRAG) Query(ctx, q *types.SessionRAGQuery) ([]*types.SessionRAGResult, error)

// Delete removes the kodit repository (no-op if no repo ID stored)
func (k *KoditRAG) Delete(ctx, req *types.DeleteIndexRequest) error
```

The `KoditRAG` also implements an additional interface checked by the reconciler:

```go
// KoditIndexer is implemented by KoditRAG to register a filestore directory.
// The knowledge reconciler checks for this interface and calls it instead of
// the normal extraction pipeline.
type KoditIndexer interface {
    RegisterDirectory(ctx context.Context, dataEntityID string, localPath string) error
}
```

`RegisterDirectory`:
1. Calls `koditService.RegisterRepository(ctx, "file://"+localPath, "")` → returns `koditRepoID`
2. Fetches the `DataEntity` from store by `dataEntityID`
3. Sets `DataEntity.KoditRepositoryID = &koditRepoID`
4. Calls `store.UpdateDataEntity(ctx, dataEntity)`

## Knowledge Reconciler Changes

**File**: `api/pkg/controller/knowledge/knowledge_indexer.go`

In `indexKnowledge()`, before calling `getIndexingData()`, check if the RAG client implements `rag.KoditIndexer`:

```go
if ki, ok := r.rag.(rag.KoditIndexer); ok && k.Source.Filestore != nil {
    localPath := filepath.Join(r.config.FileStore.LocalFSPath, filestorePath)
    dataEntityID := GetDataEntityID(k.ID, version)
    if err := ki.RegisterDirectory(ctx, dataEntityID, localPath); err != nil {
        return fmt.Errorf("kodit directory registration failed: %w", err)
    }
    // Skip normal extraction pipeline — kodit handles ingestion
    return r.finishIndexing(ctx, k, version, nil)
}
// ... normal pipeline continues
```

The `localPath` must be computed from `cfg.FileStore.LocalFSPath` + the app-scoped filestore path (same logic as `getFilestoreFiles()`). If `FileStore.Type != "fs"`, return an error.

## RAG Factory

**File**: `api/cmd/helix/serve.go`

Add a `kodit` case to the provider switch (lines ~423-445):

```go
case config.RAGProviderKodit:
    if !cfg.Kodit.Enabled {
        return fmt.Errorf("RAG provider 'kodit' requires KODIT_ENABLED=true")
    }
    ragClient = rag.NewKoditRAG(koditService, store, cfg.FileStore)
```

**File**: `api/pkg/config/config.go`

```go
const (
    RAGProviderTypesense  RAGProvider = "typesense"
    RAGProviderLlamaindex RAGProvider = "llamaindex"
    RAGProviderHaystack   RAGProvider = "haystack"
    RAGProviderKodit      RAGProvider = "kodit"   // new
)
```

## Query Path Detail

`rag.Query()` receives `SessionRAGQuery.DataEntityID`. The KoditRAG:

1. Fetches `DataEntity` from store using `DataEntityID`
2. Checks `DataEntity.KoditRepositoryID != nil` (errors if nil)
3. Calls `koditService.SemanticSearch(ctx, *koditRepoID, query.Prompt, query.MaxResults)`
4. Maps `[]services.KoditFileResult` → `[]*types.SessionRAGResult`:
   - `Content` = result content
   - `Source` = result file path
   - `DocumentID` = result ID (if available)

## Build Tags

`rag_kodit.go` uses `//go:build !nokodit` to match the existing kodit build tag pattern. A stub `rag_kodit_nokodit.go` returns `fmt.Errorf("kodit support not compiled in")` when built with `-tags nokodit`.

## Testing Strategy

**Unit tests** (`api/pkg/rag/rag_kodit_test.go`):
- `Index()` returns nil (no-op)
- `RegisterDirectory()` calls `RegisterRepository` with correct `file://` URI and stores repo ID on DataEntity
- `Query()` calls `SemanticSearch` with correct repo ID and maps results
- `Query()` returns error when `KoditRepositoryID` is nil
- `Delete()` calls `DeleteRepository` with correct repo ID
- `Delete()` is a no-op when `KoditRepositoryID` is nil

Use `gomock` to mock `KoditServicer` and `store.Store`.

**E2E test** (`api/pkg/controller/knowledge/knowledge_kodit_e2e_test.go`):
- Stand up an in-memory store and a mock `KoditServicer`
- Create a knowledge entry with a filestore source and kodit provider
- Run the reconciler's `indexKnowledge()`
- Assert `RegisterRepository` was called with a `file://` URI
- Assert the DataEntity's `KoditRepositoryID` was set
- Assert a subsequent `rag.Query()` calls `SemanticSearch` with the correct repo ID

## Key Files to Change

| File | Change |
|------|--------|
| `go.mod` / `go.sum` | Update kodit to commit 417f16b |
| `api/pkg/types/types.go` | Add `KoditRepositoryID *int64` to `DataEntity` |
| `api/pkg/config/config.go` | Add `RAGProviderKodit` constant |
| `api/pkg/rag/rag.go` | Add `KoditIndexer` interface |
| `api/pkg/rag/rag_kodit.go` | New file: KoditRAG implementation |
| `api/pkg/rag/rag_kodit_nokodit.go` | New file: stub for no-kodit builds |
| `api/pkg/rag/rag_kodit_test.go` | New file: unit tests |
| `api/pkg/controller/knowledge/knowledge_indexer.go` | Add `KoditIndexer` check before extraction |
| `api/cmd/helix/serve.go` | Add `kodit` case to RAG provider switch |

## Codebase Patterns Discovered

- GORM AutoMigrate is used — adding a nullable field to a struct is sufficient for migration; no manual SQL needed
- Build tag `//go:build !nokodit` is the convention for kodit-conditional code; always provide a `_nokodit` stub
- RAG implementations are created in `api/cmd/helix/serve.go` in a switch on `cfg.RAG.DefaultRagProvider`
- `KoditServicer.SemanticSearch` and `RegisterRepository` already exist on the interface
- DataEntity is the primary struct mapping filestore paths to indexed data; `DataEntity.Config.FilestorePath` holds the logical path
- The knowledge reconciler in `controller/knowledge/knowledge_indexer.go` is the right place to hook in provider-specific pre-indexing behavior
- `gomock` is the test mock framework (not testify/mock)
- Test suite pattern: embed `suite.Suite`, use `SetupTest()`, run with `suite.Run(t, new(MySuite))`
