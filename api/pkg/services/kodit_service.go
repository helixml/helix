//go:build !nokodit

package services

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/helixml/kodit"
	"github.com/helixml/kodit/application/service"
	"github.com/helixml/kodit/domain/enrichment"
	"github.com/helixml/kodit/domain/repository"
	"github.com/helixml/kodit/domain/search"
	"github.com/helixml/kodit/domain/tracking"
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

// MCPDocumentation returns a markdown section describing the kodit MCP server
// tools and their usage instructions, suitable for embedding in agent prompts.
// Returns empty string when the service is disabled.
func (s *KoditService) MCPDocumentation() string {
	if !s.IsEnabled() {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Code Intelligence — Kodit MCP Server\n\n")
	b.WriteString(s.client.MCPServer.Instructions())
	b.WriteString("\n\n### Tool Reference\n\n")
	b.WriteString("| Tool | Description | Required Params | Optional Params |\n")
	b.WriteString("|------|-------------|-----------------|------------------|\n")

	for _, tool := range s.client.MCPServer.Tools() {
		var required, optional []string
		for _, p := range tool.Parameters() {
			entry := fmt.Sprintf("`%s` (%s)", p.Name(), p.Type())
			if p.Required() {
				required = append(required, entry)
			} else {
				optional = append(optional, entry)
			}
		}
		reqStr := "—"
		if len(required) > 0 {
			reqStr = strings.Join(required, ", ")
		}
		optStr := "—"
		if len(optional) > 0 {
			optStr = strings.Join(optional, ", ")
		}
		b.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s |\n",
			tool.Name(), tool.Description(), reqStr, optStr))
	}

	return b.String()
}

// wrapNotFound converts kodit.ErrNotFound to ErrKoditNotFound so callers
// outside this package don't need to import the root kodit package.
func wrapNotFound(err error) error {
	if errors.Is(err, kodit.ErrNotFound) {
		return fmt.Errorf("%w: %v", ErrKoditNotFound, err)
	}
	return err
}

// RegisterRepository registers a repository with Kodit for indexing.
// Returns the source ID (int64), whether it was newly created, and any error.
func (s *KoditService) RegisterRepository(ctx context.Context, cloneURL string) (int64, bool, error) {
	if !s.enabled {
		return 0, false, fmt.Errorf("kodit service not enabled")
	}

	source, isNew, err := s.client.Repositories.Add(ctx, &service.RepositoryAddParams{
		URL: cloneURL,
	})
	if err != nil {
		return 0, false, fmt.Errorf("failed to register repository: %w", err)
	}

	if source.ID() == 0 {
		log.Error().
			Str("clone_url", cloneURL).
			Bool("is_new", isNew).
			Str("remote_url", source.RemoteURL()).
			Str("status", source.Status().String()).
			Str("last_error", source.LastError()).
			Bool("is_cloned", source.IsCloned()).
			Str("cloned_path", source.ClonedPath()).
			Msg("Kodit Repositories.Add returned zero ID — this indicates a persistence bug in kodit")
		return 0, false, fmt.Errorf("kodit returned zero source ID for clone URL (is_new=%v, status=%s)", isNew, source.Status())
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
		return enrichment.Enrichment{}, wrapNotFound(err)
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
		return tracking.RepositoryStatusSummary{}, wrapNotFound(err)
	}

	return summary, nil
}

// ListRepositories returns all Kodit repositories with pagination.
func (s *KoditService) ListRepositories(ctx context.Context, limit, offset int) ([]repository.Repository, int64, error) {
	if !s.enabled {
		return nil, 0, fmt.Errorf("kodit service not enabled")
	}

	opts := []repository.Option{
		repository.WithLimit(limit),
		repository.WithOffset(offset),
		repository.WithOrderDesc("created_at"),
	}

	repos, err := s.client.Repositories.Find(ctx, opts...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list repositories: %w", err)
	}

	total, err := s.client.Repositories.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count repositories: %w", err)
	}

	return repos, total, nil
}

// RepositorySummary returns a detailed summary for a repository.
func (s *KoditService) RepositorySummary(ctx context.Context, koditRepoID int64) (repository.RepositorySummary, error) {
	if !s.enabled {
		return repository.RepositorySummary{}, fmt.Errorf("kodit service not enabled")
	}

	summary, err := s.client.Repositories.SummaryByID(ctx, koditRepoID)
	if err != nil {
		return repository.RepositorySummary{}, wrapNotFound(err)
	}

	return summary, nil
}

// SyncRepository triggers a full sync (git fetch + branch scan + re-index).
func (s *KoditService) SyncRepository(ctx context.Context, koditRepoID int64) error {
	if !s.enabled {
		return fmt.Errorf("kodit service not enabled")
	}

	if err := s.client.Repositories.Sync(ctx, koditRepoID); err != nil {
		return fmt.Errorf("failed to sync repository: %w", wrapNotFound(err))
	}

	log.Info().Int64("kodit_repo_id", koditRepoID).Msg("Triggered repository sync in Kodit")
	return nil
}

// DeleteRepository queues a repository for deletion.
func (s *KoditService) DeleteRepository(ctx context.Context, koditRepoID int64) error {
	if !s.enabled {
		return fmt.Errorf("kodit service not enabled")
	}

	if err := s.client.Repositories.Delete(ctx, koditRepoID); err != nil {
		return fmt.Errorf("failed to delete repository from kodit: %w", wrapNotFound(err))
	}

	log.Info().Int64("kodit_repo_id", koditRepoID).Msg("Deleted repository from Kodit")
	return nil
}

// EnrichmentCount returns the total number of enrichments for a repository.
func (s *KoditService) EnrichmentCount(ctx context.Context, koditRepoID int64) (int64, error) {
	if !s.enabled {
		return 0, fmt.Errorf("kodit service not enabled")
	}

	commits, err := s.client.Commits.Find(ctx, repository.WithRepoID(koditRepoID))
	if err != nil {
		return 0, fmt.Errorf("failed to list commits for repo: %w", err)
	}
	if len(commits) == 0 {
		return 0, nil
	}

	shas := make([]string, len(commits))
	for i, c := range commits {
		shas[i] = c.SHA()
	}

	count, err := s.client.Enrichments.Count(ctx, &service.EnrichmentListParams{
		CommitSHAs: shas,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to count enrichments: %w", err)
	}

	return count, nil
}

// RepositoryTasks returns tracking statuses and pending queue tasks for a repository.
func (s *KoditService) RepositoryTasks(ctx context.Context, koditRepoID int64) (KoditRepositoryTasks, error) {
	if !s.enabled {
		return KoditRepositoryTasks{}, fmt.Errorf("kodit service not enabled")
	}

	// Get tracking statuses (operation-level status for this repo)
	trackingStatuses, err := s.client.Tracking.Statuses(ctx, koditRepoID)
	if err != nil {
		return KoditRepositoryTasks{}, fmt.Errorf("failed to get tracking statuses: %w", err)
	}

	statuses := make([]KoditTaskStatus, 0, len(trackingStatuses))
	for _, ts := range trackingStatuses {
		statuses = append(statuses, KoditTaskStatus{
			Operation: string(ts.Operation()),
			State:     string(ts.State()),
			Message:   ts.Message(),
			Error:     ts.Error(),
			Current:   ts.Current(),
			Total:     ts.Total(),
			UpdatedAt: ts.UpdatedAt(),
		})
	}

	// Get pending queue tasks and filter by this repo
	allPending, err := s.client.Tasks.List(ctx, &service.TaskListParams{Limit: 500})
	if err != nil {
		return KoditRepositoryTasks{}, fmt.Errorf("failed to list pending tasks: %w", err)
	}

	pending := make([]KoditPendingTask, 0)
	for _, t := range allPending {
		payload := t.Payload()
		if payload == nil {
			continue
		}
		// repository_id in payload can be int64, int, or float64 (JSON round-trip)
		var taskRepoID int64
		switch v := payload["repository_id"].(type) {
		case int64:
			taskRepoID = v
		case int:
			taskRepoID = int64(v)
		case float64:
			taskRepoID = int64(v)
		}
		if taskRepoID == koditRepoID {
			pending = append(pending, KoditPendingTask{
				ID:           t.ID(),
				Operation:    string(t.Operation()),
				Priority:     t.Priority(),
				CreatedAt:    t.CreatedAt(),
				RepositoryID: taskRepoID,
			})
		}
	}

	return KoditRepositoryTasks{
		Statuses:     statuses,
		PendingTasks: pending,
	}, nil
}

// SystemStats returns aggregate counts for the Kodit system.
func (s *KoditService) SystemStats(ctx context.Context) (KoditSystemStats, error) {
	if !s.enabled {
		return KoditSystemStats{}, fmt.Errorf("kodit service not enabled")
	}

	repos, err := s.client.Repositories.Count(ctx)
	if err != nil {
		return KoditSystemStats{}, fmt.Errorf("failed to count repositories: %w", err)
	}

	enrichments, err := s.client.Enrichments.Count(ctx, &service.EnrichmentListParams{})
	if err != nil {
		return KoditSystemStats{}, fmt.Errorf("failed to count enrichments: %w", err)
	}

	commits, err := s.client.Commits.Count(ctx)
	if err != nil {
		return KoditSystemStats{}, fmt.Errorf("failed to count commits: %w", err)
	}

	pendingTasks, err := s.client.Tasks.Count(ctx)
	if err != nil {
		return KoditSystemStats{}, fmt.Errorf("failed to count pending tasks: %w", err)
	}

	return KoditSystemStats{
		Repositories: repos,
		Enrichments:  enrichments,
		Commits:      commits,
		PendingTasks: pendingTasks,
	}, nil
}

// ListAllTasks returns a paginated list of all pending tasks across all repositories.
func (s *KoditService) ListAllTasks(ctx context.Context, limit, offset int) ([]KoditPendingTask, int64, error) {
	if !s.enabled {
		return nil, 0, fmt.Errorf("kodit service not enabled")
	}

	tasks, err := s.client.Tasks.List(ctx, &service.TaskListParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list tasks: %w", err)
	}

	total, err := s.client.Tasks.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count tasks: %w", err)
	}

	result := make([]KoditPendingTask, 0, len(tasks))
	for _, t := range tasks {
		var repoID int64
		if payload := t.Payload(); payload != nil {
			switch v := payload["repository_id"].(type) {
			case int64:
				repoID = v
			case int:
				repoID = int64(v)
			case float64:
				repoID = int64(v)
			}
		}
		result = append(result, KoditPendingTask{
			ID:           t.ID(),
			Operation:    string(t.Operation()),
			Priority:     t.Priority(),
			CreatedAt:    t.CreatedAt(),
			RepositoryID: repoID,
		})
	}

	return result, total, nil
}

// ActiveTasks returns all tasks currently being worked on (started or in_progress).
func (s *KoditService) ActiveTasks(ctx context.Context) ([]KoditActiveTask, error) {
	if !s.enabled {
		return nil, fmt.Errorf("kodit service not enabled")
	}

	statuses, err := s.client.Tracking.ActiveStatuses(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get active statuses: %w", err)
	}

	result := make([]KoditActiveTask, 0, len(statuses))
	for _, st := range statuses {
		result = append(result, KoditActiveTask{
			Operation:    string(st.Operation()),
			State:        string(st.State()),
			Message:      st.Message(),
			Current:      st.Current(),
			Total:        st.Total(),
			RepositoryID: st.TrackableID(),
			UpdatedAt:    st.UpdatedAt(),
		})
	}
	return result, nil
}

// DeleteTask removes a specific task from the queue by ID.
func (s *KoditService) DeleteTask(ctx context.Context, taskID int64) error {
	if !s.enabled {
		return fmt.Errorf("kodit service not enabled")
	}
	return s.client.Tasks.Remove(ctx, taskID)
}

// UpdateTaskPriority updates the priority of a specific task by ID.
func (s *KoditService) UpdateTaskPriority(ctx context.Context, taskID int64, priority int) error {
	if !s.enabled {
		return fmt.Errorf("kodit service not enabled")
	}
	return s.client.Tasks.Reprioritize(ctx, taskID, priority)
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
