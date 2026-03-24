//go:build !nokodit

package rag

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// KoditRAG implements rag.RAG and rag.KoditIndexer using kodit as the backend.
//
// Indexing lifecycle:
//  - RegisterDirectory: registers the local filestore directory with kodit via a
//    file:// URI, then persists the kodit repository ID on the DataEntity record.
//    Kodit handles document conversion (PDF, DOCX, etc.) and embedding natively.
//  - Index: no-op — kodit has already indexed everything during RegisterDirectory.
//  - Query: looks up the kodit repository ID from the DataEntity and delegates to
//    kodit's semantic search.
//  - Delete: removes the kodit repository when the knowledge base is deleted.
type KoditRAG struct {
	kodit  services.KoditServicer
	store  store.Store
	fsCfg  config.FileStore
}

// NewKoditRAG creates a KoditRAG. The kodit service must be enabled.
func NewKoditRAG(koditSvc services.KoditServicer, s store.Store, fsCfg config.FileStore) *KoditRAG {
	return &KoditRAG{
		kodit: koditSvc,
		store: s,
		fsCfg: fsCfg,
	}
}

// Index is a no-op for the kodit provider: kodit converts and indexes files
// during RegisterDirectory, so there is nothing left to do here.
func (k *KoditRAG) Index(_ context.Context, _ ...*types.SessionRAGIndexChunk) error {
	return nil
}

// RegisterDirectory implements rag.KoditIndexer.
// It registers the given local directory with kodit (using a file:// URI), then
// creates or updates the DataEntity identified by dataEntityID to store the
// returned kodit repository ID. When the repo already exists in Kodit (e.g.
// a second file was added), it triggers a sync so Kodit rescans and picks up
// new files.
func (k *KoditRAG) RegisterDirectory(ctx context.Context, dataEntityID, localPath, owner, ownerType string) error {
	fileURI := "file://" + filepath.ToSlash(localPath)

	log.Info().
		Str("data_entity_id", dataEntityID).
		Str("local_path", localPath).
		Str("file_uri", fileURI).
		Msg("registering directory with kodit")

	repoID, isNew, err := k.kodit.RegisterRepository(ctx, fileURI, "")
	if err != nil {
		return fmt.Errorf("kodit RegisterRepository failed for %s: %w", fileURI, err)
	}

	// If the repo already existed, trigger a rescan of the latest commit.
	// SyncRepository only does a git fetch which is a no-op for local
	// file:// directories. This mirrors what the kodit admin UI does.
	if !isNew {
		commits, err := k.kodit.GetRepositoryCommits(ctx, repoID, 1)
		if err != nil {
			return fmt.Errorf("failed to get latest commit for rescan: %w", err)
		}
		if len(commits) == 0 {
			log.Warn().Int64("kodit_repo_id", repoID).Msg("no commits found for kodit repo, skipping rescan")
		} else {
			commitSHA := commits[0].SHA()
			log.Info().
				Int64("kodit_repo_id", repoID).
				Str("commit_sha", commitSHA).
				Str("file_uri", fileURI).
				Msg("kodit repo already exists, triggering rescan of latest commit")
			if err := k.kodit.RescanCommit(ctx, repoID, commitSHA); err != nil {
				return fmt.Errorf("kodit rescan failed for repo %d: %w", repoID, err)
			}
		}
	}

	log.Info().
		Str("data_entity_id", dataEntityID).
		Int64("kodit_repo_id", repoID).
		Bool("is_new", isNew).
		Msg("kodit repository registered, storing repo ID")

	// Upsert the DataEntity with the kodit repository ID.
	entity, err := k.store.GetDataEntity(ctx, dataEntityID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("failed to get data entity %s: %w", dataEntityID, err)
	}

	if errors.Is(err, store.ErrNotFound) {
		_, err = k.store.CreateDataEntity(ctx, &types.DataEntity{
			ID:                dataEntityID,
			Owner:             owner,
			OwnerType:         types.OwnerType(ownerType),
			KoditRepositoryID: &repoID,
			Config: types.DataEntityConfig{
				FilestorePath: localPath,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to create data entity %s: %w", dataEntityID, err)
		}
	} else {
		entity.KoditRepositoryID = &repoID
		entity.Config.FilestorePath = localPath
		_, err = k.store.UpdateDataEntity(ctx, entity)
		if err != nil {
			return fmt.Errorf("failed to update data entity %s: %w", dataEntityID, err)
		}
	}

	return nil
}

// Query delegates to kodit's semantic search using the kodit repository ID
// stored on the DataEntity.
func (k *KoditRAG) Query(ctx context.Context, q *types.SessionRAGQuery) ([]*types.SessionRAGResult, error) {
	entity, err := k.store.GetDataEntity(ctx, q.DataEntityID)
	if err != nil {
		return nil, fmt.Errorf("failed to get data entity %s: %w", q.DataEntityID, err)
	}

	if entity.KoditRepositoryID == nil {
		return nil, fmt.Errorf("data entity %s has no kodit repository ID: was it indexed with the kodit provider?", q.DataEntityID)
	}

	maxResults := q.MaxResults
	if maxResults <= 0 {
		maxResults = 10
	}

	results, err := k.kodit.SemanticSearch(ctx, *entity.KoditRepositoryID, q.Prompt, maxResults, "")
	if err != nil {
		return nil, fmt.Errorf("kodit semantic search failed: %w", err)
	}

	ragResults := make([]*types.SessionRAGResult, 0, len(results))
	for _, r := range results {
		result := &types.SessionRAGResult{
			Content:  r.Preview,
			Source:   r.Path,
			Filename: r.Path,
			Distance: 1.0 - r.Score, // kodit uses similarity scores (0-1); RAG uses distance
		}

		// Compute DocumentID from the actual file content so the frontend
		// can resolve citations back to filestore files.
		if entity.Config.FilestorePath != "" {
			fullPath := filepath.Join(entity.Config.FilestorePath, r.Path)
			if fileBytes, err := os.ReadFile(fullPath); err == nil {
				result.DocumentID = data.ContentHash(fileBytes)
			} else {
				log.Warn().Err(err).
					Str("path", fullPath).
					Msg("could not read file for document ID hash")
			}
		}

		ragResults = append(ragResults, result)
	}

	return ragResults, nil
}

// Delete removes the kodit repository associated with the given data entity.
// If no kodit repository ID is stored, the call is a no-op.
func (k *KoditRAG) Delete(ctx context.Context, req *types.DeleteIndexRequest) error {
	entity, err := k.store.GetDataEntity(ctx, req.DataEntityID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil // nothing to clean up
		}
		return fmt.Errorf("failed to get data entity %s: %w", req.DataEntityID, err)
	}

	if entity.KoditRepositoryID == nil {
		return nil // no kodit repo registered
	}

	if err := k.kodit.DeleteRepository(ctx, *entity.KoditRepositoryID); err != nil {
		return fmt.Errorf("failed to delete kodit repository %d: %w", *entity.KoditRepositoryID, err)
	}

	return nil
}
