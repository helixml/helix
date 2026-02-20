package services

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/helixml/kodit"
	"github.com/helixml/kodit/application/service"
	"github.com/helixml/kodit/domain/enrichment"
	"github.com/helixml/kodit/domain/repository"
	"github.com/helixml/kodit/domain/search"
	"github.com/rs/zerolog/log"
)

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

// Client returns the underlying kodit.Client for direct access (e.g. MCP handler)
func (s *KoditService) Client() *kodit.Client {
	if s == nil {
		return nil
	}
	return s.client
}

// Enrichment type constants
const (
	EnrichmentTypeUsage               = "usage"
	EnrichmentTypeDeveloper           = "developer"
	EnrichmentTypeLivingDocumentation = "living_documentation"
)

// Enrichment subtype constants
const (
	EnrichmentSubtypeSnippet           = "snippet"
	EnrichmentSubtypeExample           = "example"
	EnrichmentSubtypeCookbook          = "cookbook"
	EnrichmentSubtypeArchitecture      = "architecture"
	EnrichmentSubtypeAPIDocs           = "api_docs"
	EnrichmentSubtypeDatabaseSchema    = "database_schema"
	EnrichmentSubtypeCommitDescription = "commit_description"
)

// Response types for API compatibility with frontend
type (
	KoditEnrichmentListResponse struct {
		Data []KoditEnrichmentData `json:"data"`
	}
	KoditEnrichmentData struct {
		Type       string                    `json:"type"`
		ID         string                    `json:"id"`
		Attributes KoditEnrichmentAttributes `json:"attributes"`
		CommitSHA  string                    `json:"commit_sha,omitempty"`
	}
	KoditEnrichmentAttributes struct {
		Type      string    `json:"type"`
		Subtype   *string   `json:"subtype,omitempty"`
		Content   string    `json:"content"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
)

// KoditIndexingStatus represents the indexing status response for a repository
type KoditIndexingStatus struct {
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	UpdatedAt time.Time `json:"updated_at"`
}

// KoditSearchResult represents a search result for frontend consumption
type KoditSearchResult struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Language string `json:"language"`
	Content  string `json:"content"`
	FilePath string `json:"file_path"`
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

// GetRepositoryEnrichments fetches enrichments for a repository from Kodit.
// enrichmentType can be: "usage", "developer", "living_documentation" (or empty for all).
// commitSHA can filter enrichments for a specific commit.
func (s *KoditService) GetRepositoryEnrichments(ctx context.Context, koditRepoID int64, enrichmentType, commitSHA string) (*KoditEnrichmentListResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("kodit service not enabled")
	}

	params := &service.EnrichmentListParams{}

	if enrichmentType != "" {
		t := enrichment.Type(enrichmentType)
		params.Type = &t
	}

	if commitSHA != "" {
		params.CommitSHA = commitSHA
	} else {
		// If no commit SHA provided, get enrichments for latest commits of this repo
		commits, err := s.client.Commits.Find(ctx, repository.WithRepoID(koditRepoID), repository.WithLimit(50))
		if err != nil {
			return nil, fmt.Errorf("failed to list commits for repo: %w", err)
		}
		if len(commits) > 0 {
			shas := make([]string, len(commits))
			for i, c := range commits {
				shas[i] = c.SHA()
			}
			params.CommitSHAs = shas
		}
	}

	enrichments, err := s.client.Enrichments.List(ctx, params)
	if err != nil {
		if errors.Is(err, kodit.ErrNotFound) {
			return &KoditEnrichmentListResponse{Data: []KoditEnrichmentData{}}, nil
		}
		return nil, fmt.Errorf("failed to list enrichments: %w", err)
	}

	return filterAndConvertEnrichments(enrichments, commitSHA), nil
}

// GetEnrichment fetches a single enrichment by ID from Kodit
func (s *KoditService) GetEnrichment(ctx context.Context, enrichmentID string) (*KoditEnrichmentData, error) {
	if !s.enabled {
		return nil, fmt.Errorf("kodit service not enabled")
	}

	id, err := strconv.ParseInt(enrichmentID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid enrichment ID %q: %w", enrichmentID, err)
	}

	e, err := s.client.Enrichments.Get(ctx, repository.WithID(id))
	if err != nil {
		if errors.Is(err, kodit.ErrNotFound) {
			return nil, fmt.Errorf("enrichment not found: %w", kodit.ErrNotFound)
		}
		return nil, fmt.Errorf("failed to get enrichment: %w", err)
	}

	subtype := string(e.Subtype())
	var subtypePtr *string
	if subtype != "" {
		subtypePtr = &subtype
	}

	return &KoditEnrichmentData{
		Type: string(e.Type()),
		ID:   strconv.FormatInt(e.ID(), 10),
		Attributes: KoditEnrichmentAttributes{
			Type:      string(e.Type()),
			Subtype:   subtypePtr,
			Content:   e.Content(),
			CreatedAt: e.CreatedAt(),
			UpdatedAt: e.UpdatedAt(),
		},
	}, nil
}

// GetRepositoryCommits fetches commits for a repository from Kodit
func (s *KoditService) GetRepositoryCommits(ctx context.Context, koditRepoID int64, limit int) ([]map[string]any, error) {
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
			return []map[string]any{}, nil
		}
		return nil, fmt.Errorf("failed to list commits: %w", err)
	}

	result := make([]map[string]any, 0, len(commits))
	for _, c := range commits {
		result = append(result, map[string]any{
			"id":   strconv.FormatInt(c.ID(), 10),
			"type": "commit",
			"attributes": map[string]any{
				"sha":          c.SHA(),
				"message":      c.Message(),
				"authored_at":  c.AuthoredAt(),
				"committed_at": c.CommittedAt(),
			},
		})
	}

	return result, nil
}

// SearchSnippets searches for code snippets in a repository from Kodit
func (s *KoditService) SearchSnippets(ctx context.Context, koditRepoID int64, query string, limit int, commitSHA string) ([]KoditSearchResult, error) {
	if !s.enabled {
		return nil, fmt.Errorf("kodit service not enabled")
	}

	if query == "" {
		return []KoditSearchResult{}, nil
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

	enrichments := result.Enrichments()
	results := make([]KoditSearchResult, 0, len(enrichments))
	for _, e := range enrichments {
		results = append(results, KoditSearchResult{
			ID:       strconv.FormatInt(e.ID(), 10),
			Type:     string(e.Type()),
			Language: e.Language(),
			Content:  e.Content(),
		})
	}

	return results, nil
}

// GetRepositoryStatus fetches indexing status for a repository from Kodit
func (s *KoditService) GetRepositoryStatus(ctx context.Context, koditRepoID int64) (*KoditIndexingStatus, error) {
	if !s.enabled {
		return nil, fmt.Errorf("kodit service not enabled")
	}

	summary, err := s.client.Tracking.Summary(ctx, koditRepoID)
	if err != nil {
		if errors.Is(err, kodit.ErrNotFound) {
			return nil, fmt.Errorf("repository not found: %w", kodit.ErrNotFound)
		}
		return nil, fmt.Errorf("failed to get repository status: %w", err)
	}

	return &KoditIndexingStatus{
		Status:    string(summary.Status()),
		Message:   summary.Message(),
		UpdatedAt: summary.UpdatedAt(),
	}, nil
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

// filterAndConvertEnrichments filters out internal summaries and converts domain types to API response types
func filterAndConvertEnrichments(enrichments []enrichment.Enrichment, commitSHA string) *KoditEnrichmentListResponse {
	const maxContentLength = 500
	result := &KoditEnrichmentListResponse{Data: make([]KoditEnrichmentData, 0, len(enrichments))}

	for _, e := range enrichments {
		subtype := string(e.Subtype())

		// Skip internal summary types
		if subtype == "snippet_summary" || subtype == "example_summary" {
			continue
		}

		content := e.Content()
		if len(content) > maxContentLength {
			content = content[:maxContentLength] + "..."
		}

		var subtypePtr *string
		if subtype != "" {
			subtypePtr = &subtype
		}

		result.Data = append(result.Data, KoditEnrichmentData{
			Type:      string(e.Type()),
			ID:        strconv.FormatInt(e.ID(), 10),
			CommitSHA: commitSHA,
			Attributes: KoditEnrichmentAttributes{
				Type:      string(e.Type()),
				Subtype:   subtypePtr,
				Content:   content,
				CreatedAt: e.CreatedAt(),
				UpdatedAt: e.UpdatedAt(),
			},
		})
	}

	return result
}
