package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	giteagit "code.gitea.io/gitea/modules/git"
	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/kodit/domain/enrichment"
	"github.com/helixml/kodit/domain/repository"
	"github.com/helixml/kodit/domain/tracking"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fakes — unavoidable: real KoditService needs FTS5, real Store needs Postgres.

type fakeKoditService struct {
	enabled     bool
	enrichments []enrichment.Enrichment
	enrichment  enrichment.Enrichment
	commits     []repository.Commit
	status      tracking.RepositoryStatusSummary
	repoID      int64
	isNew       bool
	err         error
}

func (f *fakeKoditService) IsEnabled() bool { return f.enabled }
func (f *fakeKoditService) RegisterRepository(_ context.Context, _ string) (int64, bool, error) {
	return f.repoID, f.isNew, f.err
}
func (f *fakeKoditService) GetRepositoryEnrichments(_ context.Context, _ int64, _, _ string) ([]enrichment.Enrichment, error) {
	return f.enrichments, f.err
}
func (f *fakeKoditService) GetEnrichment(_ context.Context, _ string) (enrichment.Enrichment, error) {
	return f.enrichment, f.err
}
func (f *fakeKoditService) GetRepositoryCommits(_ context.Context, _ int64, _ int) ([]repository.Commit, error) {
	return f.commits, f.err
}
func (f *fakeKoditService) SearchSnippets(_ context.Context, _ int64, _ string, _ int) ([]enrichment.Enrichment, error) {
	return f.enrichments, f.err
}
func (f *fakeKoditService) GetRepositoryStatus(_ context.Context, _ int64) (tracking.RepositoryStatusSummary, error) {
	return f.status, f.err
}
func (f *fakeKoditService) RescanCommit(_ context.Context, _ int64, _ string) error { return f.err }

type fakeGitRepositoryStore struct {
	store.Store
	repository *types.GitRepository
}

func (f *fakeGitRepositoryStore) GetGitRepository(_ context.Context, _ string) (*types.GitRepository, error) {
	return f.repository, nil
}
func (f *fakeGitRepositoryStore) UpdateGitRepository(_ context.Context, repo *types.GitRepository) error {
	f.repository = repo
	return nil
}

// Helpers

func newTestAPIServer(t *testing.T, koditSvc services.KoditServicer, s *fakeGitRepositoryStore) *HelixAPIServer {
	t.Helper()
	return &HelixAPIServer{
		koditService: koditSvc,
		gitRepositoryService: services.NewGitRepositoryService(
			s, t.TempDir(), "http://localhost:8080", "test", "test@example.com",
		),
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	path := t.TempDir()
	require.NoError(t, giteagit.InitRepository(context.Background(), path, false, "sha1"))
	return path
}

func makeRequest(vars map[string]string, query string) *http.Request {
	req := httptest.NewRequest("GET", "/test", nil)
	if query != "" {
		req.URL.RawQuery = query
	}
	return mux.SetURLVars(req, vars)
}

// Tests

func TestKoditHandlerValidation(t *testing.T) {
	srv := newTestAPIServer(t, &fakeKoditService{enabled: true}, &fakeGitRepositoryStore{})
	tests := []struct {
		name    string
		handler http.HandlerFunc
		vars    map[string]string
		query   string
	}{
		{"enrichments missing id", srv.getRepositoryEnrichments, map[string]string{"id": ""}, ""},
		{"commits missing id", srv.getRepositoryKoditCommits, map[string]string{"id": ""}, ""},
		{"search missing id", srv.searchRepositorySnippets, map[string]string{"id": ""}, "query=test"},
		{"search missing query", srv.searchRepositorySnippets, map[string]string{"id": "repo-123"}, ""},
		{"status missing id", srv.getRepositoryIndexingStatus, map[string]string{"id": ""}, ""},
		{"enrichment missing repo id", srv.getEnrichment, map[string]string{"id": "", "enrichmentId": "1"}, ""},
		{"enrichment missing enrichment id", srv.getEnrichment, map[string]string{"id": "repo-123", "enrichmentId": ""}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tt.handler(rec, makeRequest(tt.vars, tt.query))
			assert.Equal(t, 400, rec.Code)
		})
	}
}

func TestKoditHandlerSuccess(t *testing.T) {
	repo := &types.GitRepository{
		ID: "repo-123", KoditIndexing: true, LocalPath: initGitRepo(t),
		Metadata: map[string]any{"kodit_repo_id": int64(999)},
	}
	koditSvc := &fakeKoditService{enabled: true, enrichment: enrichment.NewCookbook("test content")}
	srv := newTestAPIServer(t, koditSvc, &fakeGitRepositoryStore{repository: repo})

	tests := []struct {
		name    string
		handler http.HandlerFunc
		vars    map[string]string
		query   string
	}{
		{"enrichments", srv.getRepositoryEnrichments, map[string]string{"id": "repo-123"}, ""},
		{"enrichment", srv.getEnrichment, map[string]string{"id": "repo-123", "enrichmentId": "42"}, ""},
		{"commits", srv.getRepositoryKoditCommits, map[string]string{"id": "repo-123"}, ""},
		{"search", srv.searchRepositorySnippets, map[string]string{"id": "repo-123"}, "query=test"},
		{"status", srv.getRepositoryIndexingStatus, map[string]string{"id": "repo-123"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tt.handler(rec, makeRequest(tt.vars, tt.query))
			assert.Equal(t, 200, rec.Code)
		})
	}

	// Verify enrichment response body content (DTO conversion).
	rec := httptest.NewRecorder()
	srv.getEnrichment(rec, makeRequest(map[string]string{"id": "repo-123", "enrichmentId": "42"}, ""))
	var dto KoditEnrichmentDTO
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&dto))
	assert.Equal(t, "test content", dto.Attributes.Content)
}

func TestEnsureKoditRepoID_EdgeCases(t *testing.T) {
	t.Run("kodit disabled", func(t *testing.T) {
		repo := &types.GitRepository{ID: "repo-123", KoditIndexing: false, LocalPath: initGitRepo(t)}
		srv := newTestAPIServer(t, &fakeKoditService{enabled: true}, &fakeGitRepositoryStore{repository: repo})
		rec := httptest.NewRecorder()
		srv.getRepositoryEnrichments(rec, makeRequest(map[string]string{"id": "repo-123"}, ""))
		assert.Equal(t, 404, rec.Code)
	})
	t.Run("missing kodit repo id triggers re-registration", func(t *testing.T) {
		repo := &types.GitRepository{ID: "repo-123", KoditIndexing: true, LocalPath: initGitRepo(t)}
		srv := newTestAPIServer(t, &fakeKoditService{enabled: true}, &fakeGitRepositoryStore{repository: repo})
		rec := httptest.NewRecorder()
		srv.getRepositoryEnrichments(rec, makeRequest(map[string]string{"id": "repo-123"}, ""))
		assert.Equal(t, 500, rec.Code) // re-registration fails (no user in context)
	})
}

func TestExtractKoditRepoID(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]interface{}
		want     int64
	}{
		{"int64", map[string]interface{}{"kodit_repo_id": int64(42)}, 42},
		{"float64", map[string]interface{}{"kodit_repo_id": float64(42)}, 42},
		{"int", map[string]interface{}{"kodit_repo_id": int(42)}, 42},
		{"string", map[string]interface{}{"kodit_repo_id": "42"}, 42},
		{"json.Number", map[string]interface{}{"kodit_repo_id": json.Number("42")}, 42},
		{"missing key", map[string]interface{}{}, 0},
		{"invalid string", map[string]interface{}{"kodit_repo_id": "not-a-number"}, 0},
		{"nil map", nil, 0},
		{"bool value", map[string]interface{}{"kodit_repo_id": true}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractKoditRepoID(tt.metadata); got != tt.want {
				t.Errorf("extractKoditRepoID() = %d, want %d", got, tt.want)
			}
		})
	}
}
