# Requirements: Update Kodit and Implement Kodit-Based RAG

## Overview

Update the kodit Go module to the version that supports local directory indexing via `file://` URIs, then implement kodit as a RAG provider that indexes filestore directories directly through kodit rather than the existing extract-chunk-embed pipeline.

## User Stories

### US-1: Kodit RAG Provider Option
**As a** Helix operator,
**I want to** set `RAG_DEFAULT_PROVIDER=kodit`,
**So that** knowledge bases are indexed and searched using kodit's semantic engine.

**Acceptance Criteria:**
- `RAG_DEFAULT_PROVIDER=kodit` starts the server without error
- Unknown provider values still produce an error on startup
- Kodit RAG requires kodit to be enabled (`KODIT_ENABLED=true`)

### US-2: Directory-Based Kodit Indexing
**As a** Helix operator,
**I want** knowledge filestore directories to be registered with kodit as `file://` repositories,
**So that** kodit handles document conversion (PDF, DOCX, etc.) and embedding natively without helix's extract-chunk-embed pipeline.

**Acceptance Criteria:**
- When a knowledge source of type `filestore` is indexed with the kodit provider, the local filesystem path is registered with kodit as a `file://` URI
- The kodit repository ID returned by kodit is persisted in the database against the corresponding data entity
- The normal helix extraction/chunking/embedding pipeline is skipped (kodit handles it)
- `rag.Index()` is a no-op (returns nil immediately) — kodit already indexed during registration
- If the filestore type is not `fs` (e.g. GCS), the kodit provider returns an error — local path is required

### US-3: Kodit Semantic Search
**As a** Helix user,
**I want** knowledge queries to use kodit's semantic search,
**So that** search results benefit from kodit's enrichment and ranking.

**Acceptance Criteria:**
- `rag.Query()` calls `KoditServicer.SemanticSearch()` with the stored kodit repository ID
- Results are mapped to `[]*types.SessionRAGResult` with content, source, and distance fields
- If no kodit repository ID is stored for the data entity, an error is returned

### US-4: Kodit Repository Cleanup
**As a** Helix operator,
**I want** deleting a knowledge base to also clean up the corresponding kodit repository,
**So that** kodit does not accumulate stale indexed data.

**Acceptance Criteria:**
- `rag.Delete()` calls `KoditServicer.DeleteRepository()` with the stored kodit repository ID
- If no kodit repo ID exists for the data entity, the call is a no-op (not an error)

### US-5: Kodit Repo ID Persistence
**As a** developer,
**I want** a nullable `kodit_repository_id` field on the data entity database record,
**So that** a filestore can be unambiguously mapped to a kodit repository.

**Acceptance Criteria:**
- `DataEntity` struct has a new `KoditRepositoryID *int64` GORM column
- NULL means no kodit repository is associated
- GORM AutoMigrate adds the column without data loss
- Store read/write methods are updated to handle the new field

## Non-Goals

- Kodit indexing for web or SharePoint sources (filestore only for now)
- Streaming search results
- Multi-repository search across multiple knowledge bases in one query
