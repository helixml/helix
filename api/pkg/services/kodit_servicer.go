package services

import (
	"context"
	"errors"

	"github.com/helixml/kodit/domain/enrichment"
	"github.com/helixml/kodit/domain/repository"
	"github.com/helixml/kodit/domain/tracking"
)

// ErrKoditNotFound is a sentinel error for "not found" responses from Kodit.
// Handlers use this instead of importing the root kodit package.
var ErrKoditNotFound = errors.New("kodit: not found")

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
	DeleteRepository(ctx context.Context, koditRepoID int64) error

	// Admin operations
	ListRepositories(ctx context.Context, limit, offset int) ([]repository.Repository, int64, error)
	RepositorySummary(ctx context.Context, koditRepoID int64) (repository.RepositorySummary, error)
	SyncRepository(ctx context.Context, koditRepoID int64) error
	EnrichmentCount(ctx context.Context, koditRepoID int64) (int64, error)
}

// disabledKoditService is a KoditServicer that is always disabled.
type disabledKoditService struct{}

func (d *disabledKoditService) IsEnabled() bool { return false }
func (d *disabledKoditService) RegisterRepository(context.Context, string) (int64, bool, error) {
	return 0, false, errors.New("kodit service not enabled")
}
func (d *disabledKoditService) GetRepositoryEnrichments(context.Context, int64, string, string) ([]enrichment.Enrichment, error) {
	return nil, errors.New("kodit service not enabled")
}
func (d *disabledKoditService) GetEnrichment(context.Context, string) (enrichment.Enrichment, error) {
	return enrichment.Enrichment{}, errors.New("kodit service not enabled")
}
func (d *disabledKoditService) GetRepositoryCommits(context.Context, int64, int) ([]repository.Commit, error) {
	return nil, errors.New("kodit service not enabled")
}
func (d *disabledKoditService) SearchSnippets(context.Context, int64, string, int) ([]enrichment.Enrichment, error) {
	return nil, errors.New("kodit service not enabled")
}
func (d *disabledKoditService) GetRepositoryStatus(context.Context, int64) (tracking.RepositoryStatusSummary, error) {
	return tracking.RepositoryStatusSummary{}, errors.New("kodit service not enabled")
}
func (d *disabledKoditService) RescanCommit(context.Context, int64, string) error {
	return errors.New("kodit service not enabled")
}
func (d *disabledKoditService) DeleteRepository(context.Context, int64) error {
	return errors.New("kodit service not enabled")
}
func (d *disabledKoditService) ListRepositories(context.Context, int, int) ([]repository.Repository, int64, error) {
	return nil, 0, errors.New("kodit service not enabled")
}
func (d *disabledKoditService) RepositorySummary(context.Context, int64) (repository.RepositorySummary, error) {
	return repository.RepositorySummary{}, errors.New("kodit service not enabled")
}
func (d *disabledKoditService) SyncRepository(context.Context, int64) error {
	return errors.New("kodit service not enabled")
}
func (d *disabledKoditService) EnrichmentCount(context.Context, int64) (int64, error) {
	return 0, errors.New("kodit service not enabled")
}

// NewDisabledKoditService returns a KoditServicer that reports as disabled.
func NewDisabledKoditService() KoditServicer {
	return &disabledKoditService{}
}
