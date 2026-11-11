package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
)

// recordingGitRepositoryService fails the test if a repository would be
// created or updated (i.e. cloned or fetched) from the external URL.
type recordingGitRepositoryService struct {
	gitRepositoryServicer
	t *testing.T
}

func (r *recordingGitRepositoryService) CreateRepository(context.Context, *types.GitRepositoryCreateRequest) (*types.GitRepository, error) {
	r.t.Fatal("CreateRepository reached with a disallowed external_url")
	return nil, nil
}

func (r *recordingGitRepositoryService) GetRepositoryMetadata(_ context.Context, repoID string) (*types.GitRepository, error) {
	return &types.GitRepository{ID: repoID, OwnerID: "user_id"}, nil
}

func (r *recordingGitRepositoryService) UpdateRepository(context.Context, string, *types.GitRepositoryUpdateRequest, string) (*types.GitRepository, error) {
	r.t.Fatal("UpdateRepository reached with a disallowed external_url")
	return nil, nil
}

var disallowedExternalURLs = []string{
	"file:///filestore/git-repos/another-tenant",
	"/filestore/git-repos/another-tenant",
	"ext::sh -c id",
	"http://127.0.0.1:8080/repo.git",
	"http://169.254.169.254/latest/meta-data/",
	"http://localhost/repo.git",
}

func TestCreateGitRepositoryRejectsDisallowedExternalURL(t *testing.T) {
	srv := &HelixAPIServer{Cfg: &config.ServerConfig{}}
	srv.gitRepositoryService = &recordingGitRepositoryService{t: t}
	ctx := setRequestUser(context.Background(), types.User{ID: "user_id"})

	for _, externalURL := range disallowedExternalURLs {
		body, _ := json.Marshal(types.GitRepositoryCreateRequest{Name: "r", OwnerID: "user_id", ExternalURL: externalURL})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/git/repositories", bytes.NewReader(body)).WithContext(ctx)
		rec := httptest.NewRecorder()
		srv.createGitRepository(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%q: expected 400, got %d: %s", externalURL, rec.Code, rec.Body.String())
		}
	}
}

func TestUpdateGitRepositoryRejectsDisallowedExternalURL(t *testing.T) {
	srv := &HelixAPIServer{Cfg: &config.ServerConfig{}}
	srv.gitRepositoryService = &recordingGitRepositoryService{t: t}
	ctx := setRequestUser(context.Background(), types.User{ID: "user_id"})

	for _, externalURL := range disallowedExternalURLs {
		body, _ := json.Marshal(types.GitRepositoryUpdateRequest{ExternalURL: externalURL})
		req := httptest.NewRequest(http.MethodPut, "/api/v1/git/repositories/repo1", bytes.NewReader(body)).WithContext(ctx)
		req = mux.SetURLVars(req, map[string]string{"id": "repo1"})
		rec := httptest.NewRecorder()
		srv.updateGitRepository(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%q: expected 400, got %d: %s", externalURL, rec.Code, rec.Body.String())
		}
	}
}

var disallowedOrganizationURLs = []string{
	"http://127.0.0.1:8080/tfs",
	"http://169.254.169.254/latest/meta-data/",
	"https://100.100.100.200/",
	"file:///etc",
}

// The Azure DevOps organization URL is called by the API server (SDK, PR
// updates, code review) and must meet the same outbound policy.
func TestGitRepositoryRejectsDisallowedAzureDevOpsOrganizationURL(t *testing.T) {
	srv := &HelixAPIServer{Cfg: &config.ServerConfig{}}
	srv.gitRepositoryService = &recordingGitRepositoryService{t: t}
	ctx := setRequestUser(context.Background(), types.User{ID: "user_id"})

	for _, orgURL := range disallowedOrganizationURLs {
		ado := &types.AzureDevOps{OrganizationURL: orgURL, PersonalAccessToken: "pat"}

		body, _ := json.Marshal(types.GitRepositoryCreateRequest{Name: "r", OwnerID: "user_id", ExternalType: types.ExternalRepositoryTypeADO, AzureDevOps: ado})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/git/repositories", bytes.NewReader(body)).WithContext(ctx)
		rec := httptest.NewRecorder()
		srv.createGitRepository(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("create %q: expected 400, got %d: %s", orgURL, rec.Code, rec.Body.String())
		}

		body, _ = json.Marshal(types.GitRepositoryUpdateRequest{AzureDevOps: ado})
		req = httptest.NewRequest(http.MethodPut, "/api/v1/git/repositories/repo1", bytes.NewReader(body)).WithContext(ctx)
		req = mux.SetURLVars(req, map[string]string{"id": "repo1"})
		rec = httptest.NewRecorder()
		srv.updateGitRepository(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("update %q: expected 400, got %d: %s", orgURL, rec.Code, rec.Body.String())
		}
	}
}
