//go:build !nokodit

package rag

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/kodit/domain/repository"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
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

	repoID, isNew, err := k.kodit.RegisterRepository(ctx, &services.RegisterRepositoryParams{
		CloneURL: fileURI,
		Pipeline: repository.PipelineNameRAG,
	})
	if err != nil {
		return fmt.Errorf("kodit RegisterRepository failed for %s: %w", fileURI, err)
	}

	// If the repo already existed, trigger a rescan so kodit picks up the
	// latest filesystem state. Preferred path: fetch the latest commit and
	// rescan it in place. Fallback path (empty repo — e.g. one whose commits
	// were wiped by an earlier cleanup): SyncRepository, which runs Kodit's
	// full clone-update + branch scan and will pick up whatever's on disk
	// now as a fresh commit history.
	if !isNew {
		commits, err := k.kodit.GetRepositoryCommits(ctx, repoID, 1)
		if err != nil {
			return fmt.Errorf("failed to get latest commit for rescan: %w", err)
		}
		if len(commits) == 0 {
			log.Info().
				Int64("kodit_repo_id", repoID).
				Str("file_uri", fileURI).
				Msg("kodit repo has no commits; triggering full sync to scan current filesystem state")
			if err := k.kodit.SyncRepository(ctx, repoID); err != nil {
				return fmt.Errorf("kodit sync failed for empty repo %d: %w", repoID, err)
			}
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

	repoID := *entity.KoditRepositoryID

	// Run semantic, visual, and keyword searches in parallel.
	var semanticResults, visualResults, keywordResults []services.KoditFileResult
	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		semanticResults, err = k.kodit.SemanticSearch(gctx, repoID, q.Prompt, maxResults, "")
		if err != nil {
			return fmt.Errorf("kodit semantic search failed: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		visualResults, err = k.kodit.VisualSearch(gctx, repoID, q.Prompt, maxResults)
		if err != nil {
			log.Warn().Err(err).Msg("kodit visual search failed, using other results only")
		}
		return nil
	})

	g.Go(func() error {
		var err error
		keywordResults, err = k.kodit.KeywordSearch(gctx, repoID, q.Prompt, maxResults, "")
		if err != nil {
			log.Warn().Err(err).Msg("kodit keyword search failed, using other results only")
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Merge all results, keeping best score per path+page.
	ragResults := k.mergeAndConvert(entity, maxResults, semanticResults, visualResults, keywordResults)
	return ragResults, nil
}

// mergeAndConvert merges multiple search result sets, deduplicates by
// path+page, sorts by score (best first), and converts to SessionRAGResult.
func (k *KoditRAG) mergeAndConvert(entity *types.DataEntity, maxResults int, resultSets ...[]services.KoditFileResult) []*types.SessionRAGResult {
	type dedupKey struct {
		path string
		page int
	}

	// Deduplicate by path+page, keeping the best score.
	best := make(map[dedupKey]services.KoditFileResult)
	for _, results := range resultSets {
		for _, r := range results {
			dk := dedupKey{r.Path, r.Page}
			if existing, ok := best[dk]; !ok || r.Score > existing.Score {
				best[dk] = r
			}
		}
	}

	// Flatten and sort by score descending.
	merged := make([]services.KoditFileResult, 0, len(best))
	for _, r := range best {
		merged = append(merged, r)
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Score > merged[j].Score
	})
	if len(merged) > maxResults {
		merged = merged[:maxResults]
	}

	// Path returned by kodit is relative to the directory that was registered
	// (entity.Config.FilestorePath, an absolute path on disk). The session
	// controller and frontend expect Source to be the path relative to the
	// filestore root so the viewer URL resolves correctly.
	relDir := strings.TrimPrefix(entity.Config.FilestorePath, k.fsCfg.LocalFSPath)
	relDir = strings.TrimPrefix(relDir, "/")

	// Convert to RAG results.
	ragResults := make([]*types.SessionRAGResult, 0, len(merged))
	for _, r := range merged {
		result := &types.SessionRAGResult{
			Content:  r.Content,
			Source:   filepath.Join(relDir, r.Path),
			Filename: filepath.Base(r.Path),
			Distance: 1.0 - r.Score,
		}

		if r.Page > 0 {
			if result.Metadata == nil {
				result.Metadata = make(map[string]string)
			}
			result.Metadata["page_number"] = strconv.Itoa(r.Page)
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

	return ragResults
}

// RenderPageImage implements rag.PageImageRenderer. It renders a document page
// as a PNG and returns the raw bytes.
func (k *KoditRAG) RenderPageImage(ctx context.Context, dataEntityID string, filePath string, page int) ([]byte, error) {
	entity, err := k.store.GetDataEntity(ctx, dataEntityID)
	if err != nil {
		return nil, fmt.Errorf("failed to get data entity %s: %w", dataEntityID, err)
	}
	if entity.KoditRepositoryID == nil {
		return nil, fmt.Errorf("data entity %s has no kodit repository ID", dataEntityID)
	}
	return k.kodit.RenderPageImage(ctx, *entity.KoditRepositoryID, filePath, page)
}

// Delete cleans up the data entity for a knowledge version. Each version of a
// knowledge source has its own data_entity row, but they all share the same
// kodit repository (kodit indexes the live filestore directory, not a
// per-version snapshot). So pruning an old version must NOT delete the kodit
// repo while newer versions still reference it — doing so previously orphaned
// the live versions and caused kodit to wipe the user's filestore (see
// helixml/kodit#TODO: Delete handler RemoveAll's the working copy path, which
// for file:// registrations is the user's data dir).
//
// Strategy: always delete this version's data_entity row. Then check whether
// any other data_entity still references the same kodit_repository_id. Only
// delete the kodit repo when this was the last reference (i.e. the whole
// knowledge source is being torn down).
func (k *KoditRAG) Delete(ctx context.Context, req *types.DeleteIndexRequest) error {
	entity, err := k.store.GetDataEntity(ctx, req.DataEntityID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil // nothing to clean up
		}
		return fmt.Errorf("failed to get data entity %s: %w", req.DataEntityID, err)
	}

	repoID := entity.KoditRepositoryID

	// Drop the data_entity row first so the sibling check below sees the
	// post-delete state.
	if err := k.store.DeleteDataEntity(ctx, req.DataEntityID); err != nil {
		return fmt.Errorf("failed to delete data entity %s: %w", req.DataEntityID, err)
	}

	if repoID == nil {
		return nil // no kodit repo registered for this version
	}

	siblings, err := k.store.ListDataEntitiesByKoditRepositoryID(ctx, *repoID)
	if err != nil {
		return fmt.Errorf("failed to count sibling references to kodit repository %d: %w", *repoID, err)
	}
	if len(siblings) > 0 {
		log.Info().
			Int64("kodit_repo_id", *repoID).
			Int("siblings", len(siblings)).
			Str("data_entity_id", req.DataEntityID).
			Msg("kodit repository still referenced by other knowledge versions; skipping kodit delete")
		return nil
	}

	if err := k.kodit.DeleteRepository(ctx, *repoID); err != nil {
		return fmt.Errorf("failed to delete kodit repository %d: %w", *repoID, err)
	}

	return nil
}
