# Implementation Tasks

## Module Update
- [x] Run `go get github.com/helixml/kodit@417f16b7dfce928b0e9d1a888454cfc6cbe98892` in `api/` and commit updated `go.mod` / `go.sum`

## Database
- [x] Add `KoditRepositoryID *int64` field (with `gorm:"column:kodit_repository_id"`) to `DataEntity` struct in `api/pkg/types/types.go`

## Config
- [x] Add `RAGProviderKodit RAGProvider = "kodit"` constant to `api/pkg/config/config.go`

## RAG Interface
- [x] Add `KoditIndexer` interface to `api/pkg/rag/rag.go`; also export `InitKodit`/`KoditResult` from server pkg and update `NewServer` to accept pre-initialized result

## KoditRAG Implementation
- [x] Create `api/pkg/rag/rag_kodit.go` (build tag `//go:build !nokodit`) implementing `rag.RAG` and `rag.KoditIndexer`:
  - `NewKoditRAG(kodit services.KoditServicer, store store.Store, fsCfg config.FileStore) *KoditRAG`
  - `Index()` returns nil (no-op)
  - `RegisterDirectory()` calls `koditService.RegisterRepository` with `file://localPath`, creates/updates DataEntity with `KoditRepositoryID`
  - `Query()` fetches DataEntity, asserts `KoditRepositoryID != nil`, calls `SemanticSearch`, maps results to `[]*types.SessionRAGResult`
  - `Delete()` fetches DataEntity, calls `DeleteRepository` if `KoditRepositoryID != nil`
- [x] Create `api/pkg/rag/rag_kodit_nokodit.go` (build tag `//go:build nokodit`) with stub returning "kodit support not compiled in" error

## Knowledge Reconciler
- [x] In `api/pkg/controller/knowledge/knowledge_indexer.go`, add a check before `getIndexingData()`: if RAG client implements `rag.KoditIndexer` and source is filestore, call `RegisterDirectory` and skip the normal extraction pipeline
- [x] Resolve local filesystem path from `cfg.FileStore.LocalFSPath` + app-scoped filestore path (reuse the path logic from `getFilestoreFiles()`)
- [x] Return error if `FileStore.Type != "fs"` when kodit provider is active

## RAG Factory
- [x] Add `case config.RAGProviderKodit:` to the provider switch in `api/cmd/helix/serve.go`, constructing `rag.NewKoditRAG(koditService, store, cfg.FileStore)` and returning error if `!cfg.Kodit.Enabled`

## Unit Tests
- [x] Create `api/pkg/rag/rag_kodit_test.go` using gomock suite pattern:
  - `Index()` is a no-op
  - `RegisterDirectory()` calls `RegisterRepository` with correct `file://` URI
  - `RegisterDirectory()` updates `DataEntity.KoditRepositoryID` in store
  - `Query()` calls `SemanticSearch` with correct repo ID and maps results
  - `Query()` returns error when `KoditRepositoryID` is nil
  - `Delete()` calls `DeleteRepository` with correct repo ID
  - `Delete()` is no-op when `KoditRepositoryID` is nil

## E2E Tests
- [x] Create `api/pkg/controller/knowledge/knowledge_kodit_e2e_test.go`:
  - Use in-memory/mock store and mock `KoditServicer`
  - Index a knowledge entry (filestore source) with kodit RAG provider
  - Assert `RegisterRepository` called with `file://` URI
  - Assert DataEntity's `KoditRepositoryID` is set after indexing
  - Assert subsequent `Query()` calls `SemanticSearch` with the stored repo ID

## Verification
- [x] `go build ./...` passes with kodit enabled
- [x] `go build -tags nokodit ./...` passes with kodit disabled
- [x] Unit tests pass: `go test ./pkg/rag/... -run TestKoditRAG`
- [x] E2E tests pass: `go test ./pkg/controller/knowledge/... -run TestKoditE2E`
