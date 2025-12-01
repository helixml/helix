package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-git/go-git/v6"
	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
)

// KoditHandlersSuite tests the kodit handlers
type KoditHandlersSuite struct {
	suite.Suite
	ctx              context.Context
	apiServer        *HelixAPIServer
	fakeKoditServer  *httptest.Server
	koditHandler     *fakeKoditHandler
	fakeGitRepoStore *fakeGitRepositoryStore
}

func TestKoditHandlersSuite(t *testing.T) {
	suite.Run(t, new(KoditHandlersSuite))
}

func (suite *KoditHandlersSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.koditHandler = &fakeKoditHandler{}
	suite.fakeKoditServer = httptest.NewServer(suite.koditHandler)
	suite.fakeGitRepoStore = &fakeGitRepositoryStore{}

	suite.apiServer = &HelixAPIServer{
		koditService: services.NewKoditService(suite.fakeKoditServer.URL, "test-api-key"),
		gitRepositoryService: services.NewGitRepositoryService(
			suite.fakeGitRepoStore, suite.T().TempDir(), "http://localhost:8080", "test", "test@example.com",
		),
	}
}

func (suite *KoditHandlersSuite) TearDownTest() {
	suite.fakeKoditServer.Close()
}

// =============================================================================
// Repository Factories
// =============================================================================

// repoWithKoditEnabled creates a repository with kodit enabled and a valid local path
func (suite *KoditHandlersSuite) repoWithKoditEnabled(koditRepoID string) *types.GitRepository {
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
	_, err := git.PlainInit(path, false)
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

// =============================================================================
// Fake Kodit Handler
// =============================================================================

type fakeKoditHandler struct {
	response   any
	statusCode int
	// Captured for search assertions
	lastQuery, lastSearchCommitSHA string
}

func (h *fakeKoditHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	code := h.statusCode
	if code == 0 {
		code = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")

	// Capture search params for assertions
	if r.URL.Path == "/api/v1/search" && r.Method == "POST" {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if data, ok := req["data"].(map[string]any); ok {
			if attrs, ok := data["attributes"].(map[string]any); ok {
				h.lastQuery, _ = attrs["text"].(string)
				if filters, ok := attrs["filters"].(map[string]any); ok {
					if sha, ok := filters["commit_sha"].([]any); ok && len(sha) > 0 {
						h.lastSearchCommitSHA, _ = sha[0].(string)
					}
				}
			}
		}
	}
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(h.response)
}

// =============================================================================
// Test Case Type
// =============================================================================

type handlerTestCase struct {
	name        string
	repo        *types.GitRepository
	storeErr    error
	koditStatus int
	koditResp   any
	wantStatus  int
}

// standardTestCases returns common test cases for handlers
func (suite *KoditHandlersSuite) standardTestCases(koditRepoID string) []handlerTestCase {
	return []handlerTestCase{
		{"repo service error returns 500", nil, errors.New("db error"), 0, nil, 500},
		{"kodit indexing false returns 404", suite.repoWithKoditDisabled(), nil, 0, nil, 404},
		{"kodit repo ID not set returns 500", suite.repoWithKoditEnabledNoID(), nil, 0, nil, 500},
		{"kodit 404 returns 404", suite.repoWithKoditEnabled(koditRepoID), nil, 404, map[string]any{"error": "not found"}, 404},
		{"kodit error returns 502", suite.repoWithKoditEnabled(koditRepoID), nil, 500, map[string]any{"error": "internal"}, 502},
		{"success returns 200", suite.repoWithKoditEnabled(koditRepoID), nil, 200, map[string]any{"data": []any{}}, 200},
	}
}

func (suite *KoditHandlersSuite) runHandlerTest(tc handlerTestCase, handler func(http.ResponseWriter, *http.Request), req *http.Request) {
	suite.fakeGitRepoStore.repository = tc.repo
	suite.fakeGitRepoStore.err = tc.storeErr
	suite.koditHandler.statusCode = tc.koditStatus
	suite.koditHandler.response = tc.koditResp

	rec := httptest.NewRecorder()
	handler(rec, req)
	suite.Equal(tc.wantStatus, rec.Code)
}

func (suite *KoditHandlersSuite) makeRequest(path string, vars map[string]string, query string) *http.Request {
	req := httptest.NewRequest("GET", path, nil)
	req = req.WithContext(suite.ctx)
	if query != "" {
		req.URL.RawQuery = query
	}
	return mux.SetURLVars(req, vars)
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

func (suite *KoditHandlersSuite) TestGetRepositoryEnrichments() {
	for _, tc := range suite.standardTestCases("kodit-123") {
		suite.Run(tc.name, func() {
			req := suite.makeRequest("/enrichments", map[string]string{"id": "repo-123"}, "")
			suite.runHandlerTest(tc, suite.apiServer.getRepositoryEnrichments, req)
		})
	}
}

// =============================================================================
// Tests for getEnrichment
// =============================================================================

func (suite *KoditHandlersSuite) TestGetEnrichment_MissingIDs() {
	tests := []struct {
		name string
		vars map[string]string
	}{
		{"missing repo ID", map[string]string{"id": "", "enrichmentId": "enr-1"}},
		{"missing enrichment ID", map[string]string{"id": "repo-123", "enrichmentId": ""}},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			req := suite.makeRequest("/enrichments/enr-1", tt.vars, "")
			rec := httptest.NewRecorder()
			suite.apiServer.getEnrichment(rec, req)
			suite.Equal(400, rec.Code)
		})
	}
}

func (suite *KoditHandlersSuite) TestGetEnrichment() {
	for _, tc := range suite.standardTestCases("kodit-123") {
		suite.Run(tc.name, func() {
			// Override success response for single enrichment format
			if tc.wantStatus == 200 {
				tc.koditResp = map[string]any{"data": map[string]any{"id": "enr-1"}}
			}
			req := suite.makeRequest("/enrichments/enr-1", map[string]string{"id": "repo-123", "enrichmentId": "enr-1"}, "")
			suite.runHandlerTest(tc, suite.apiServer.getEnrichment, req)
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

func (suite *KoditHandlersSuite) TestGetRepositoryKoditCommits() {
	for _, tc := range suite.standardTestCases("kodit-123") {
		suite.Run(tc.name, func() {
			req := suite.makeRequest("/kodit-commits", map[string]string{"id": "repo-123"}, "")
			suite.runHandlerTest(tc, suite.apiServer.getRepositoryKoditCommits, req)
		})
	}
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

func (suite *KoditHandlersSuite) TestSearchRepositorySnippets() {
	for _, tc := range suite.standardTestCases("kodit-123") {
		suite.Run(tc.name, func() {
			req := suite.makeRequest("/search-snippets", map[string]string{"id": "repo-123"}, "query=test")
			suite.runHandlerTest(tc, suite.apiServer.searchRepositorySnippets, req)
		})
	}
}

func (suite *KoditHandlersSuite) TestSearchRepositorySnippets_ParamsPassedToService() {
	suite.fakeGitRepoStore.repository = suite.repoWithKoditEnabled("kodit-123")
	suite.koditHandler.response = map[string]any{"data": []any{}}

	req := suite.makeRequest("/search-snippets", map[string]string{"id": "repo-123"}, "query=findMe&commit_sha=abc123")
	rec := httptest.NewRecorder()
	suite.apiServer.searchRepositorySnippets(rec, req)

	suite.Equal(200, rec.Code)
	suite.Equal("findMe", suite.koditHandler.lastQuery)
	suite.Equal("abc123", suite.koditHandler.lastSearchCommitSHA)
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

func (suite *KoditHandlersSuite) TestGetRepositoryIndexingStatus() {
	// Status endpoint requires numeric kodit_repo_id
	for _, tc := range suite.standardTestCases("123") {
		suite.Run(tc.name, func() {
			// Override success response for status summary format
			if tc.wantStatus == 200 {
				tc.koditResp = map[string]any{
					"data": map[string]any{
						"id":   "123",
						"type": "repository_status_summary",
						"attributes": map[string]any{
							"status":     "completed",
							"message":    "All indexing tasks completed",
							"updated_at": "2024-01-01T00:00:00Z",
						},
					},
				}
			}
			req := suite.makeRequest("/kodit-status", map[string]string{"id": "repo-123"}, "")
			suite.runHandlerTest(tc, suite.apiServer.getRepositoryIndexingStatus, req)
		})
	}
}
