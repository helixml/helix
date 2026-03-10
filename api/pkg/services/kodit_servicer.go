package services

import (
	"context"
	"errors"
	"time"

	"github.com/helixml/kodit/domain/enrichment"
	"github.com/helixml/kodit/domain/repository"
	"github.com/helixml/kodit/domain/tracking"
)

// KoditWikiTreeNode represents a wiki page in the navigation tree (no content).
type KoditWikiTreeNode struct {
	Slug     string              `json:"slug"`
	Title    string              `json:"title"`
	Path     string              `json:"path"`
	Children []KoditWikiTreeNode `json:"children,omitempty"`
}

// KoditWikiPage represents a single wiki page with its rendered content.
type KoditWikiPage struct {
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// KoditFileResult represents a search result (semantic or keyword) with file context.
type KoditFileResult struct {
	Path     string  `json:"path"`
	Language string  `json:"language"`
	Lines    string  `json:"lines,omitempty"`
	Score    float64 `json:"score"`
	Preview  string  `json:"preview"`
}

// KoditGrepResult represents a grep match grouped by file.
type KoditGrepResult struct {
	Path     string            `json:"path"`
	Language string            `json:"language"`
	Matches  []KoditGrepMatch  `json:"matches"`
}

// KoditGrepMatch represents a single grep match line.
type KoditGrepMatch struct {
	Line    int    `json:"line"`
	Content string `json:"content"`
}

// KoditFileEntry represents a file in a repository listing.
type KoditFileEntry struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// KoditFileContent represents the content of a file.
type KoditFileContent struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	CommitSHA string `json:"commit_sha"`
}

// ErrKoditNotFound is a sentinel error for "not found" responses from Kodit.
// Handlers use this instead of importing the root kodit package.
var ErrKoditNotFound = errors.New("kodit: not found")

// KoditSystemStats holds aggregate counts for the Kodit system.
type KoditSystemStats struct {
	Repositories int64
	Enrichments  int64
	Commits      int64
	PendingTasks int64
}

// KoditTaskStatus represents the status of a tracked operation for a repository.
type KoditTaskStatus struct {
	Operation string
	State     string // started, in_progress, completed, failed, skipped
	Message   string
	Error     string
	Current   int
	Total     int
	UpdatedAt time.Time
}

// KoditPendingTask represents a queued task waiting to be processed.
type KoditPendingTask struct {
	ID           int64
	Operation    string
	Priority     int
	CreatedAt    time.Time
	RepositoryID int64
}

// KoditActiveTask represents a task currently being worked on.
type KoditActiveTask struct {
	Operation    string
	State        string // started or in_progress
	Message      string
	Current      int
	Total        int
	RepositoryID int64
	UpdatedAt    time.Time
}

// KoditRepositoryTasks combines tracking statuses and pending queue tasks for a repository.
type KoditRepositoryTasks struct {
	Statuses     []KoditTaskStatus
	PendingTasks []KoditPendingTask
}

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

	// MCP documentation for agent prompts
	MCPDocumentation() string

	// Admin operations
	ListRepositories(ctx context.Context, limit, offset int) ([]repository.Repository, int64, error)
	RepositorySummary(ctx context.Context, koditRepoID int64) (repository.RepositorySummary, error)
	SyncRepository(ctx context.Context, koditRepoID int64) error
	EnrichmentCount(ctx context.Context, koditRepoID int64) (int64, error)
	SystemStats(ctx context.Context) (KoditSystemStats, error)
	RepositoryTasks(ctx context.Context, koditRepoID int64) (KoditRepositoryTasks, error)

	// Wiki
	GetWikiTree(ctx context.Context, koditRepoID int64) ([]KoditWikiTreeNode, error)
	GetWikiPage(ctx context.Context, koditRepoID int64, pagePath string) (*KoditWikiPage, error)

	// Search tools (mirrors MCP server capabilities)
	SemanticSearch(ctx context.Context, koditRepoID int64, query string, limit int, language string) ([]KoditFileResult, error)
	KeywordSearch(ctx context.Context, koditRepoID int64, keywords string, limit int, language string) ([]KoditFileResult, error)
	GrepSearch(ctx context.Context, koditRepoID int64, pattern string, glob string, limit int) ([]KoditGrepResult, error)
	ListFiles(ctx context.Context, koditRepoID int64, pattern string) ([]KoditFileEntry, error)
	ReadFile(ctx context.Context, koditRepoID int64, filePath string, startLine, endLine int) (*KoditFileContent, error)

	// Global task queue management
	ListAllTasks(ctx context.Context, limit, offset int) ([]KoditPendingTask, int64, error)
	ActiveTasks(ctx context.Context) ([]KoditActiveTask, error)
	DeleteTask(ctx context.Context, taskID int64) error
	UpdateTaskPriority(ctx context.Context, taskID int64, priority int) error
}

// disabledKoditService is a KoditServicer that is always disabled.
type disabledKoditService struct{}

func (d *disabledKoditService) IsEnabled() bool          { return false }
func (d *disabledKoditService) MCPDocumentation() string { return "" }
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
func (d *disabledKoditService) SystemStats(context.Context) (KoditSystemStats, error) {
	return KoditSystemStats{}, errors.New("kodit service not enabled")
}
func (d *disabledKoditService) RepositoryTasks(context.Context, int64) (KoditRepositoryTasks, error) {
	return KoditRepositoryTasks{}, errors.New("kodit service not enabled")
}
func (d *disabledKoditService) GetWikiTree(context.Context, int64) ([]KoditWikiTreeNode, error) {
	return nil, errors.New("kodit service not enabled")
}
func (d *disabledKoditService) GetWikiPage(context.Context, int64, string) (*KoditWikiPage, error) {
	return nil, errors.New("kodit service not enabled")
}
func (d *disabledKoditService) SemanticSearch(context.Context, int64, string, int, string) ([]KoditFileResult, error) {
	return nil, errors.New("kodit service not enabled")
}
func (d *disabledKoditService) KeywordSearch(context.Context, int64, string, int, string) ([]KoditFileResult, error) {
	return nil, errors.New("kodit service not enabled")
}
func (d *disabledKoditService) GrepSearch(context.Context, int64, string, string, int) ([]KoditGrepResult, error) {
	return nil, errors.New("kodit service not enabled")
}
func (d *disabledKoditService) ListFiles(context.Context, int64, string) ([]KoditFileEntry, error) {
	return nil, errors.New("kodit service not enabled")
}
func (d *disabledKoditService) ReadFile(context.Context, int64, string, int, int) (*KoditFileContent, error) {
	return nil, errors.New("kodit service not enabled")
}
func (d *disabledKoditService) ListAllTasks(context.Context, int, int) ([]KoditPendingTask, int64, error) {
	return nil, 0, errors.New("kodit service not enabled")
}
func (d *disabledKoditService) ActiveTasks(context.Context) ([]KoditActiveTask, error) {
	return nil, errors.New("kodit service not enabled")
}
func (d *disabledKoditService) DeleteTask(context.Context, int64) error {
	return errors.New("kodit service not enabled")
}
func (d *disabledKoditService) UpdateTaskPriority(context.Context, int64, int) error {
	return errors.New("kodit service not enabled")
}

// NewDisabledKoditService returns a KoditServicer that reports as disabled.
func NewDisabledKoditService() KoditServicer {
	return &disabledKoditService{}
}
