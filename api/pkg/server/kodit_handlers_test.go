package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	giteagit "code.gitea.io/gitea/modules/git"
	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/kodit"
	"github.com/stretchr/testify/suite"
)

// KoditHandlersSuite tests the kodit handlers
type KoditHandlersSuite struct {
	suite.Suite
	ctx              context.Context
	apiServer        *HelixAPIServer
	koditClient      *kodit.Client
	fakeGitRepoStore *fakeGitRepositoryStore
}

func TestKoditHandlersSuite(t *testing.T) {
	suite.Run(t, new(KoditHandlersSuite))
}

func (suite *KoditHandlersSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.fakeGitRepoStore = &fakeGitRepositoryStore{}

	// Create an in-process kodit client with SQLite for testing
	dataDir := suite.T().TempDir()
	dbPath := filepath.Join(dataDir, "test.db")

	client, err := kodit.New(
		kodit.WithSQLite(dbPath),
		kodit.WithDataDir(dataDir),
		kodit.WithWorkerCount(0), // No background workers in tests
		kodit.WithEmbeddingProvider(&noopEmbedder{}),
		kodit.WithSkipProviderValidation(),
	)
	suite.Require().NoError(err)
	suite.koditClient = client

	suite.apiServer = &HelixAPIServer{
		koditService: services.NewKoditService(client),
		gitRepositoryService: services.NewGitRepositoryService(
			suite.fakeGitRepoStore, suite.T().TempDir(), "http://localhost:8080", "test", "test@example.com",
		),
	}
}

func (suite *KoditHandlersSuite) TearDownTest() {
	if suite.koditClient != nil {
		suite.koditClient.Close()
	}
}

// =============================================================================
// Repository Factories
// =============================================================================

// repoWithKoditEnabled creates a repository with kodit enabled and a valid kodit_repo_id
func (suite *KoditHandlersSuite) repoWithKoditEnabled(koditRepoID int64) *types.GitRepository {
	return &types.GitRepository{
		ID: "repo-123", KoditIndexing: true, LocalPath: suite.initGitRepo(),
		Metadata: map[string]any{"kodit_repo_id": koditRepoID},
	}
}

func (suite *KoditHandlersSuite) repoWithKoditDisabled() *types.GitRepository {
	return &types.GitRepository{ID: "repo-123", KoditIndexing: false, LocalPath: suite.initGitRepo()}
}

func (suite *KoditHandlersSuite) repoWithKoditEnabledNoID() *types.GitRepository {
	return &types.GitRepository{ID: "repo-123", KoditIndexing: true, LocalPath: suite.initGitRepo(), Metadata: nil}
}

// initGitRepo creates a minimal git repo and returns its path
func (suite *KoditHandlersSuite) initGitRepo() string {
	path := suite.T().TempDir()
	err := giteagit.InitRepository(context.Background(), path, false, "sha1")
	suite.Require().NoError(err)
	return path
}

// =============================================================================
// Fake Store
// =============================================================================

type fakeGitRepositoryStore struct {
	store.Store
	repository *types.GitRepository
	err        error
}

func (f *fakeGitRepositoryStore) GetGitRepository(_ context.Context, _ string) (*types.GitRepository, error) {
	return f.repository, f.err
}

func (f *fakeGitRepositoryStore) UpdateGitRepository(_ context.Context, repo *types.GitRepository) error {
	f.repository = repo
	return nil
}

// =============================================================================
// Tests for getRepositoryEnrichments
// =============================================================================

func (suite *KoditHandlersSuite) TestGetRepositoryEnrichments_MissingRepoID() {
	req := suite.makeRequest("/enrichments", map[string]string{"id": ""}, "")
	rec := httptest.NewRecorder()
	suite.apiServer.getRepositoryEnrichments(rec, req)
	suite.Equal(400, rec.Code)
}

func (suite *KoditHandlersSuite) TestGetRepositoryEnrichments_KoditDisabled() {
	suite.fakeGitRepoStore.repository = suite.repoWithKoditDisabled()
	req := suite.makeRequest("/enrichments", map[string]string{"id": "repo-123"}, "")
	rec := httptest.NewRecorder()
	suite.apiServer.getRepositoryEnrichments(rec, req)
	suite.Equal(404, rec.Code)
}

func (suite *KoditHandlersSuite) TestGetRepositoryEnrichments_NoKoditRepoID() {
	suite.fakeGitRepoStore.repository = suite.repoWithKoditEnabledNoID()
	req := suite.makeRequest("/enrichments", map[string]string{"id": "repo-123"}, "")
	rec := httptest.NewRecorder()
	suite.apiServer.getRepositoryEnrichments(rec, req)
	// Re-registration will fail (no user in context), so expect 500
	suite.Equal(500, rec.Code)
}

func (suite *KoditHandlersSuite) TestGetRepositoryEnrichments_Success() {
	suite.fakeGitRepoStore.repository = suite.repoWithKoditEnabled(999)
	req := suite.makeRequest("/enrichments", map[string]string{"id": "repo-123"}, "")
	rec := httptest.NewRecorder()
	suite.apiServer.getRepositoryEnrichments(rec, req)
	suite.Equal(200, rec.Code)

	var response services.KoditEnrichmentListResponse
	suite.NoError(json.NewDecoder(rec.Body).Decode(&response))
	suite.NotNil(response.Data)
}

// =============================================================================
// Tests for getEnrichment
// =============================================================================

func (suite *KoditHandlersSuite) TestGetEnrichment_MissingIDs() {
	tests := []struct {
		name string
		vars map[string]string
	}{
		{"missing repo ID", map[string]string{"id": "", "enrichmentId": "1"}},
		{"missing enrichment ID", map[string]string{"id": "repo-123", "enrichmentId": ""}},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			req := suite.makeRequest("/enrichments/1", tt.vars, "")
			rec := httptest.NewRecorder()
			suite.apiServer.getEnrichment(rec, req)
			suite.Equal(400, rec.Code)
		})
	}
}

// =============================================================================
// Tests for getRepositoryKoditCommits
// =============================================================================

func (suite *KoditHandlersSuite) TestGetRepositoryKoditCommits_MissingRepoID() {
	req := suite.makeRequest("/kodit-commits", map[string]string{"id": ""}, "")
	rec := httptest.NewRecorder()
	suite.apiServer.getRepositoryKoditCommits(rec, req)
	suite.Equal(400, rec.Code)
}

func (suite *KoditHandlersSuite) TestGetRepositoryKoditCommits_Success() {
	suite.fakeGitRepoStore.repository = suite.repoWithKoditEnabled(999)
	req := suite.makeRequest("/kodit-commits", map[string]string{"id": "repo-123"}, "")
	rec := httptest.NewRecorder()
	suite.apiServer.getRepositoryKoditCommits(rec, req)
	suite.Equal(200, rec.Code)
}

// =============================================================================
// Tests for searchRepositorySnippets
// =============================================================================

func (suite *KoditHandlersSuite) TestSearchRepositorySnippets_MissingParams() {
	tests := []struct {
		name  string
		vars  map[string]string
		query string
	}{
		{"missing repo ID", map[string]string{"id": ""}, "query=test"},
		{"missing query", map[string]string{"id": "repo-123"}, ""},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			req := suite.makeRequest("/search-snippets", tt.vars, tt.query)
			rec := httptest.NewRecorder()
			suite.apiServer.searchRepositorySnippets(rec, req)
			suite.Equal(400, rec.Code)
		})
	}
}

func (suite *KoditHandlersSuite) TestSearchRepositorySnippets_Success() {
	suite.fakeGitRepoStore.repository = suite.repoWithKoditEnabled(999)
	req := suite.makeRequest("/search-snippets", map[string]string{"id": "repo-123"}, "query=test")
	rec := httptest.NewRecorder()
	suite.apiServer.searchRepositorySnippets(rec, req)
	suite.Equal(200, rec.Code)
}

// =============================================================================
// Tests for getRepositoryIndexingStatus
// =============================================================================

func (suite *KoditHandlersSuite) TestGetRepositoryIndexingStatus_MissingRepoID() {
	req := suite.makeRequest("/kodit-status", map[string]string{"id": ""}, "")
	rec := httptest.NewRecorder()
	suite.apiServer.getRepositoryIndexingStatus(rec, req)
	suite.Equal(400, rec.Code)
}

func (suite *KoditHandlersSuite) TestGetRepositoryIndexingStatus_Success() {
	suite.fakeGitRepoStore.repository = suite.repoWithKoditEnabled(999)
	req := suite.makeRequest("/kodit-status", map[string]string{"id": "repo-123"}, "")
	rec := httptest.NewRecorder()
	suite.apiServer.getRepositoryIndexingStatus(rec, req)
	suite.Equal(200, rec.Code)
}

// =============================================================================
// Helpers
// =============================================================================

func (suite *KoditHandlersSuite) makeRequest(path string, vars map[string]string, query string) *http.Request {
	req := httptest.NewRequest("GET", path, nil)
	req = req.WithContext(suite.ctx)
	if query != "" {
		req.URL.RawQuery = query
	}
	return mux.SetURLVars(req, vars)
}
