package services

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/helixml/kodit"
	"github.com/helixml/kodit/application/service"
	"github.com/helixml/kodit/domain/enrichment"
	"github.com/helixml/kodit/domain/repository"
	"github.com/helixml/kodit/domain/search"
	"github.com/helixml/kodit/domain/tracking"
	"github.com/rs/zerolog/log"
)

// KoditServicer is the interface for Kodit code intelligence operations.
// Used by handlers and other services; allows faking in tests.
type KoditServicer interface {
	IsEnabled() bool
	RegisterRepository(ctx context.Context, cloneURL string) (int64, bool, error)
	GetRepositoryEnrichments(ctx context.Context, koditRepoID int64, enrichmentType, commitSHA string) ([]enrichment.Enrichment, error)
	GetEnrichment(ctx context.Context, enrichmentID string) (enrichment.Enrichment, error)
	GetRepositoryCommits(ctx context.Context, koditRepoID int64, limit int) ([]repository.Commit, error)
	SearchSnippets(ctx context.Context, koditRepoID int64, query string, limit int) ([]enrichment.Enrichment, error)
	GetRepositoryStatus(ctx context.Context, koditRepoID int64) (tracking.RepositoryStatusSummary, error)
	RescanCommit(ctx context.Context, koditRepoID int64, commitSHA string) error
}

// KoditService handles communication with Kodit code intelligence library
type KoditService struct {
	enabled bool
	client  *kodit.Client
}

// NewKoditService creates a new Kodit service wrapping a kodit.Client
func NewKoditService(client *kodit.Client) *KoditService {
	if client == nil {
		log.Info().Msg("Kodit service not configured (no client)")
		return &KoditService{enabled: false}
	}
	return &KoditService{enabled: true, client: client}
}

// IsEnabled returns whether the Kodit service is enabled
func (s *KoditService) IsEnabled() bool {
	return s != nil && s.enabled
}

// RegisterRepository registers a repository with Kodit for indexing.
// Returns the source ID (int64), whether it was newly created, and any error.
func (s *KoditService) RegisterRepository(ctx context.Context, cloneURL string) (int64, bool, error) {
	if !s.enabled {
		log.Debug().Msg("Kodit service not enabled, skipping repository registration")
		return 0, false, nil
	}

	source, isNew, err := s.client.Repositories.Add(ctx, &service.RepositoryAddParams{
		URL: cloneURL,
	})
	if err != nil {
		return 0, false, fmt.Errorf("failed to register repository: %w", err)
	}

	log.Info().Str("clone_url", cloneURL).Int64("kodit_repo_id", source.ID()).Bool("is_new", isNew).Msg("Registered repository with Kodit")
	return source.ID(), isNew, nil
}

// GetRepositoryEnrichments fetches enrichments for a repository from Kodit,
// filtering out internal summary types (snippet_summary, example_summary).
func (s *KoditService) GetRepositoryEnrichments(ctx context.Context, koditRepoID int64, enrichmentType, commitSHA string) ([]enrichment.Enrichment, error) {
	if !s.enabled {
		return nil, fmt.Errorf("kodit service not enabled")
	}

	params := &service.EnrichmentListParams{
		Limit: 500, // Cap results to prevent unbounded queries
	}

	if enrichmentType != "" {
		t := enrichment.Type(enrichmentType)
		params.Type = &t
	}

	if commitSHA != "" {
		params.CommitSHA = commitSHA
	} else {
		// If no commit SHA provided, get enrichments for latest commits of this repo.
		// We MUST scope by commit SHAs because EnrichmentListParams has no repo ID
		// field — without commit scoping the query returns enrichments from all repos.
		commits, err := s.client.Commits.Find(ctx, repository.WithRepoID(koditRepoID), repository.WithLimit(50))
		if err != nil {
			return nil, fmt.Errorf("failed to list commits for repo: %w", err)
		}
		if len(commits) == 0 {
			return nil, nil
		}
		shas := make([]string, len(commits))
		for i, c := range commits {
			shas[i] = c.SHA()
		}
		params.CommitSHAs = shas
	}

	enrichments, err := s.client.Enrichments.List(ctx, params)
	if err != nil {
		if errors.Is(err, kodit.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list enrichments: %w", err)
	}

	// Filter out internal summary types
	filtered := make([]enrichment.Enrichment, 0, len(enrichments))
	for _, e := range enrichments {
		if e.Subtype() == enrichment.SubtypeSnippetSummary || e.Subtype() == enrichment.SubtypeExampleSummary {
			continue
		}
		filtered = append(filtered, e)
	}

	return filtered, nil
}

// GetEnrichment fetches a single enrichment by ID from Kodit
func (s *KoditService) GetEnrichment(ctx context.Context, enrichmentID string) (enrichment.Enrichment, error) {
	if !s.enabled {
		return enrichment.Enrichment{}, fmt.Errorf("kodit service not enabled")
	}

	id, err := strconv.ParseInt(enrichmentID, 10, 64)
	if err != nil {
		return enrichment.Enrichment{}, fmt.Errorf("invalid enrichment ID %q: %w", enrichmentID, err)
	}

	e, err := s.client.Enrichments.Get(ctx, repository.WithID(id))
	if err != nil {
		if errors.Is(err, kodit.ErrNotFound) {
			return enrichment.Enrichment{}, fmt.Errorf("enrichment not found: %w", kodit.ErrNotFound)
		}
		return enrichment.Enrichment{}, fmt.Errorf("failed to get enrichment: %w", err)
	}

	return e, nil
}

// GetRepositoryCommits fetches commits for a repository from Kodit
func (s *KoditService) GetRepositoryCommits(ctx context.Context, koditRepoID int64, limit int) ([]repository.Commit, error) {
	if !s.enabled {
		return nil, fmt.Errorf("kodit service not enabled")
	}

	opts := []repository.Option{repository.WithRepoID(koditRepoID)}
	if limit > 0 {
		opts = append(opts, repository.WithLimit(limit))
	}
	opts = append(opts, repository.WithOrderDesc("date"))

	commits, err := s.client.Commits.Find(ctx, opts...)
	if err != nil {
		if errors.Is(err, kodit.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list commits: %w", err)
	}

	return commits, nil
}

// SearchSnippets searches for code snippets in a repository from Kodit
func (s *KoditService) SearchSnippets(ctx context.Context, koditRepoID int64, query string, limit int) ([]enrichment.Enrichment, error) {
	if !s.enabled {
		return nil, fmt.Errorf("kodit service not enabled")
	}

	if query == "" {
		return nil, nil
	}

	if limit <= 0 {
		limit = 20
	}

	log.Debug().Str("query", query).Int("limit", limit).Int64("kodit_repo_id", koditRepoID).Msg("Searching snippets in Kodit")

	// Call Search.Search directly instead of Search.Query because Query has a
	// bug: it accepts WithRepositories but never passes repo IDs to the
	// underlying search filters.
	filters := search.NewFilters(
		search.WithSourceRepos([]int64{koditRepoID}),
	)
	request := search.NewMultiRequest(limit, query, query, nil, filters)

	result, err := s.client.Search.Search(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to search snippets: %w", err)
	}

	return result.Enrichments(), nil
}

// GetRepositoryStatus fetches indexing status for a repository from Kodit
func (s *KoditService) GetRepositoryStatus(ctx context.Context, koditRepoID int64) (tracking.RepositoryStatusSummary, error) {
	if !s.enabled {
		return tracking.RepositoryStatusSummary{}, fmt.Errorf("kodit service not enabled")
	}

	summary, err := s.client.Tracking.Summary(ctx, koditRepoID)
	if err != nil {
		if errors.Is(err, kodit.ErrNotFound) {
			return tracking.RepositoryStatusSummary{}, fmt.Errorf("repository not found: %w", kodit.ErrNotFound)
		}
		return tracking.RepositoryStatusSummary{}, fmt.Errorf("failed to get repository status: %w", err)
	}

	return summary, nil
}

// RescanCommit triggers a rescan of a specific commit in Kodit
func (s *KoditService) RescanCommit(ctx context.Context, koditRepoID int64, commitSHA string) error {
	if !s.enabled {
		return fmt.Errorf("kodit service not enabled")
	}

	if commitSHA == "" {
		return fmt.Errorf("commit SHA is required")
	}

	err := s.client.Repositories.Rescan(ctx, &service.RescanParams{
		RepositoryID: koditRepoID,
		CommitSHA:    commitSHA,
	})
	if err != nil {
		return fmt.Errorf("failed to rescan commit: %w", err)
	}

	log.Info().Int64("kodit_repo_id", koditRepoID).Str("commit_sha", commitSHA).Msg("Triggered commit rescan in Kodit")
	return nil
}
