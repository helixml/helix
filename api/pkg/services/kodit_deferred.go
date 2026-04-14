package services

import (
	"context"
	"sync/atomic"

	"github.com/helixml/kodit/domain/enrichment"
	"github.com/helixml/kodit/domain/repository"
	"github.com/helixml/kodit/domain/tracking"
)

// DeferredKoditService wraps a KoditServicer with an atomically-swappable inner.
// Used at startup to allow the HTTP server to start listening before kodit's
// embedding dimension probe runs (kodit itself may need to call back into
// Helix's /v1/embeddings endpoint). Starts as disabled and is swapped to the
// real service once kodit.New() completes in a background goroutine.
type DeferredKoditService struct {
	inner atomic.Pointer[KoditServicer]
}

// NewDeferredKoditService returns a DeferredKoditService initialised with a
// disabled service. Callers should call Set() once kodit is ready.
func NewDeferredKoditService() *DeferredKoditService {
	d := &DeferredKoditService{}
	disabled := NewDisabledKoditService()
	d.inner.Store(&disabled)
	return d
}

// Set atomically swaps the inner service. Typically called once after
// kodit.New() completes successfully.
func (d *DeferredKoditService) Set(s KoditServicer) {
	d.inner.Store(&s)
}

func (d *DeferredKoditService) current() KoditServicer { return *d.inner.Load() }

func (d *DeferredKoditService) IsEnabled() bool          { return d.current().IsEnabled() }
func (d *DeferredKoditService) MCPDocumentation() string { return d.current().MCPDocumentation() }

func (d *DeferredKoditService) RegisterRepository(ctx context.Context, p *RegisterRepositoryParams) (int64, bool, error) {
	return d.current().RegisterRepository(ctx, p)
}
func (d *DeferredKoditService) GetRepositoryEnrichments(ctx context.Context, id int64, t, c string) ([]enrichment.Enrichment, error) {
	return d.current().GetRepositoryEnrichments(ctx, id, t, c)
}
func (d *DeferredKoditService) GetEnrichment(ctx context.Context, id string) (enrichment.Enrichment, error) {
	return d.current().GetEnrichment(ctx, id)
}
func (d *DeferredKoditService) GetRepositoryCommits(ctx context.Context, id int64, limit int) ([]repository.Commit, error) {
	return d.current().GetRepositoryCommits(ctx, id, limit)
}
func (d *DeferredKoditService) SearchSnippets(ctx context.Context, id int64, q string, limit int) ([]enrichment.Enrichment, error) {
	return d.current().SearchSnippets(ctx, id, q, limit)
}
func (d *DeferredKoditService) GetRepositoryStatus(ctx context.Context, id int64) (tracking.RepositoryStatusSummary, error) {
	return d.current().GetRepositoryStatus(ctx, id)
}
func (d *DeferredKoditService) RescanCommit(ctx context.Context, id int64, sha string) error {
	return d.current().RescanCommit(ctx, id, sha)
}
func (d *DeferredKoditService) DeleteRepository(ctx context.Context, id int64) error {
	return d.current().DeleteRepository(ctx, id)
}
func (d *DeferredKoditService) ListRepositories(ctx context.Context, limit, offset int) ([]repository.Repository, int64, error) {
	return d.current().ListRepositories(ctx, limit, offset)
}
func (d *DeferredKoditService) RepositorySummary(ctx context.Context, id int64) (repository.RepositorySummary, error) {
	return d.current().RepositorySummary(ctx, id)
}
func (d *DeferredKoditService) SyncRepository(ctx context.Context, id int64) error {
	return d.current().SyncRepository(ctx, id)
}
func (d *DeferredKoditService) EnrichmentCount(ctx context.Context, id int64) (int64, error) {
	return d.current().EnrichmentCount(ctx, id)
}
func (d *DeferredKoditService) SystemStats(ctx context.Context) (KoditSystemStats, error) {
	return d.current().SystemStats(ctx)
}
func (d *DeferredKoditService) RepositoryTasks(ctx context.Context, id int64) (KoditRepositoryTasks, error) {
	return d.current().RepositoryTasks(ctx, id)
}
func (d *DeferredKoditService) GetWikiTree(ctx context.Context, id int64) ([]KoditWikiTreeNode, error) {
	return d.current().GetWikiTree(ctx, id)
}
func (d *DeferredKoditService) GetWikiPage(ctx context.Context, id int64, p string) (*KoditWikiPage, error) {
	return d.current().GetWikiPage(ctx, id, p)
}
func (d *DeferredKoditService) SemanticSearch(ctx context.Context, id int64, q string, limit int, lang string) ([]KoditFileResult, error) {
	return d.current().SemanticSearch(ctx, id, q, limit, lang)
}
func (d *DeferredKoditService) VisualSearch(ctx context.Context, id int64, q string, limit int) ([]KoditFileResult, error) {
	return d.current().VisualSearch(ctx, id, q, limit)
}
func (d *DeferredKoditService) KeywordSearch(ctx context.Context, id int64, q string, limit int, lang string) ([]KoditFileResult, error) {
	return d.current().KeywordSearch(ctx, id, q, limit, lang)
}
func (d *DeferredKoditService) GrepSearch(ctx context.Context, id int64, pat, glob string, limit int) ([]KoditGrepResult, error) {
	return d.current().GrepSearch(ctx, id, pat, glob, limit)
}
func (d *DeferredKoditService) ListFiles(ctx context.Context, id int64, pat string) ([]KoditFileEntry, error) {
	return d.current().ListFiles(ctx, id, pat)
}
func (d *DeferredKoditService) ReadFile(ctx context.Context, id int64, path string, s, e int) (*KoditFileContent, error) {
	return d.current().ReadFile(ctx, id, path, s, e)
}
func (d *DeferredKoditService) RenderPageImage(ctx context.Context, id int64, path string, page int) ([]byte, error) {
	return d.current().RenderPageImage(ctx, id, path, page)
}
func (d *DeferredKoditService) UpdateChunkingConfig(ctx context.Context, id int64, cs, co, mcs int) error {
	return d.current().UpdateChunkingConfig(ctx, id, cs, co, mcs)
}
func (d *DeferredKoditService) ListAllTasks(ctx context.Context, limit, offset int) ([]KoditPendingTask, int64, error) {
	return d.current().ListAllTasks(ctx, limit, offset)
}
func (d *DeferredKoditService) ActiveTasks(ctx context.Context) ([]KoditActiveTask, error) {
	return d.current().ActiveTasks(ctx)
}
func (d *DeferredKoditService) DeleteTask(ctx context.Context, id int64) error {
	return d.current().DeleteTask(ctx, id)
}
func (d *DeferredKoditService) UpdateTaskPriority(ctx context.Context, id int64, p int) error {
	return d.current().UpdateTaskPriority(ctx, id, p)
}
