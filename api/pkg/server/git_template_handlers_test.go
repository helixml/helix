package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

type GitTemplateSuite struct {
	suite.Suite

	ctrl    *gomock.Controller
	store   *store.MockStore
	server  *HelixAPIServer
	authCtx context.Context
	userID  string

	github *http.ServeMux
	gitlab *http.ServeMux
}

func TestGitTemplateSuite(t *testing.T) {
	suite.Run(t, new(GitTemplateSuite))
}

func (s *GitTemplateSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.userID = "user_template_test"
	s.authCtx = setRequestUser(context.Background(), types.User{ID: s.userID})

	s.github = http.NewServeMux()
	githubSrv := httptest.NewServer(s.github)
	s.T().Cleanup(githubSrv.Close)
	s.gitlab = http.NewServeMux()
	gitlabSrv := httptest.NewServer(s.gitlab)
	s.T().Cleanup(gitlabSrv.Close)

	s.server = &HelixAPIServer{
		Cfg:   &config.ServerConfig{WebServer: config.WebServer{URL: "https://helix.example/"}},
		Store: s.store,
		gitTemplate: gitTemplateConfig{
			GitHubAPIURL: githubSrv.URL + "/api/v3/",
			GitLabURL:    gitlabSrv.URL,
			PollInterval: 5 * time.Millisecond,
			ReadyTimeout: 5 * time.Second,
		},
	}
}

func (s *GitTemplateSuite) expectOAuth(conns ...*types.OAuthConnection) {
	s.store.EXPECT().ListOAuthConnections(gomock.Any(), &store.ListOAuthConnectionsQuery{UserID: s.userID}).Return(conns, nil)
}

func (s *GitTemplateSuite) expectPATs(conns ...*types.GitProviderConnection) {
	s.store.EXPECT().ListGitProviderConnections(gomock.Any(), s.userID).Return(conns, nil)
}

func (s *GitTemplateSuite) githubOAuth(scopes ...string) *types.OAuthConnection {
	return &types.OAuthConnection{
		UserID:      s.userID,
		AccessToken: "gh-oauth-token",
		Scopes:      scopes,
		Provider:    types.OAuthProvider{Type: types.OAuthProviderTypeGitHub},
	}
}

func (s *GitTemplateSuite) gitlabPAT(token string) *types.GitProviderConnection {
	key, err := crypto.GetEncryptionKey()
	s.Require().NoError(err)
	encrypted, err := crypto.EncryptAES256GCM([]byte(token), key)
	s.Require().NoError(err)
	return &types.GitProviderConnection{UserID: s.userID, ProviderType: types.ExternalRepositoryTypeGitLab, Token: encrypted}
}

func (s *GitTemplateSuite) listOwners(provider string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git/owners?provider="+provider, nil).WithContext(s.authCtx)
	rec := httptest.NewRecorder()
	s.server.listGitOwners(rec, req)
	return rec
}

func (s *GitTemplateSuite) createFromTemplate(body types.CreateRepoFromTemplateRequest) *httptest.ResponseRecorder {
	payload, err := json.Marshal(body)
	s.Require().NoError(err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/git/repositories/from-template", bytes.NewReader(payload)).WithContext(s.authCtx)
	rec := httptest.NewRecorder()
	s.server.createGitRepositoryFromTemplate(rec, req)
	return rec
}

func writeJSONBody(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func (s *GitTemplateSuite) TestGitHubOwners() {
	s.expectOAuth(s.githubOAuth("repo"))
	s.github.HandleFunc("GET /api/v3/user", func(w http.ResponseWriter, r *http.Request) {
		s.Equal("Bearer gh-oauth-token", r.Header.Get("Authorization"))
		writeJSONBody(w, http.StatusOK, `{"login":"alice","id":11,"avatar_url":"https://avatars/alice"}`)
	})
	s.github.HandleFunc("GET /api/v3/user/orgs", func(w http.ResponseWriter, _ *http.Request) {
		writeJSONBody(w, http.StatusOK, `[{"login":"acme","id":22,"avatar_url":"https://avatars/acme"}]`)
	})

	rec := s.listOwners("github")

	s.Require().Equal(http.StatusOK, rec.Code, rec.Body.String())
	var owners []types.GitOwner
	s.Require().NoError(json.Unmarshal(rec.Body.Bytes(), &owners))
	s.Equal([]types.GitOwner{
		{Login: "alice", Kind: types.GitOwnerKindUser, AvatarURL: "https://avatars/alice", ID: 11},
		{Login: "acme", Kind: types.GitOwnerKindOrg, AvatarURL: "https://avatars/acme", ID: 22},
	}, owners)
}

func (s *GitTemplateSuite) TestGitLabOwnersWithPATPaginatesGroups() {
	s.expectOAuth()
	s.expectPATs(s.gitlabPAT("gl-pat"))
	s.gitlab.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, r *http.Request) {
		s.Equal("gl-pat", r.Header.Get("PRIVATE-TOKEN"))
		writeJSONBody(w, http.StatusOK, `{"id":7,"username":"bob","avatar_url":"https://avatars/bob"}`)
	})
	s.gitlab.HandleFunc("GET /api/v4/namespaces/bob", func(w http.ResponseWriter, _ *http.Request) {
		writeJSONBody(w, http.StatusOK, `{"id":70,"path":"bob","full_path":"bob","kind":"user"}`)
	})
	s.gitlab.HandleFunc("GET /api/v4/groups", func(w http.ResponseWriter, r *http.Request) {
		s.Equal("30", r.URL.Query().Get("min_access_level"))
		if r.URL.Query().Get("page") == "2" {
			writeJSONBody(w, http.StatusOK, `[{"id":81,"full_path":"team/sub","avatar_url":""}]`)
			return
		}
		w.Header().Set("X-Next-Page", "2")
		writeJSONBody(w, http.StatusOK, `[{"id":80,"full_path":"team","avatar_url":"https://avatars/team"}]`)
	})

	rec := s.listOwners("gitlab")

	s.Require().Equal(http.StatusOK, rec.Code, rec.Body.String())
	var owners []types.GitOwner
	s.Require().NoError(json.Unmarshal(rec.Body.Bytes(), &owners))
	s.Equal([]types.GitOwner{
		{Login: "bob", Kind: types.GitOwnerKindUser, AvatarURL: "https://avatars/bob", ID: 70},
		{Login: "team", Kind: types.GitOwnerKindGroup, AvatarURL: "https://avatars/team", ID: 80},
		{Login: "team/sub", Kind: types.GitOwnerKindGroup, AvatarURL: "", ID: 81},
	}, owners)
}

func (s *GitTemplateSuite) TestGitHubGenerateFromTemplate() {
	s.expectOAuth(s.githubOAuth("repo", "read:user"))
	var generateBody map[string]any
	s.github.HandleFunc("POST /api/v3/repos/helixml/helixos-module-template/generate", func(w http.ResponseWriter, r *http.Request) {
		s.Equal("Bearer gh-oauth-token", r.Header.Get("Authorization"))
		s.Require().NoError(json.NewDecoder(r.Body).Decode(&generateBody))
		writeJSONBody(w, http.StatusCreated, `{"name":"my-module","full_name":"alice/my-module","owner":{"login":"alice"},
			"default_branch":"main","clone_url":"https://github.com/alice/my-module.git","html_url":"https://github.com/alice/my-module"}`)
	})
	var branchChecks atomic.Int32
	s.github.HandleFunc("GET /api/v3/repos/alice/my-module/branches/main", func(w http.ResponseWriter, _ *http.Request) {
		if branchChecks.Add(1) == 1 {
			writeJSONBody(w, http.StatusNotFound, `{"message":"Branch not found"}`)
			return
		}
		writeJSONBody(w, http.StatusOK, `{"name":"main"}`)
	})

	rec := s.createFromTemplate(types.CreateRepoFromTemplateRequest{
		Provider:    types.ExternalRepositoryTypeGitHub,
		Owner:       "alice",
		Name:        "my-module",
		Description: "My module",
		Template:    "helixml/helixos-module-template",
	})

	s.Require().Equal(http.StatusCreated, rec.Code, rec.Body.String())
	s.Equal(map[string]any{
		"owner":                "alice",
		"name":                 "my-module",
		"description":          "My module",
		"private":              true,
		"include_all_branches": false,
	}, generateBody)
	s.Equal(int32(2), branchChecks.Load(), "should wait until the default branch exists")
	var resp types.CreateRepoFromTemplateResponse
	s.Require().NoError(json.Unmarshal(rec.Body.Bytes(), &resp))
	s.Equal(types.CreateRepoFromTemplateResponse{
		CloneURL:      "https://github.com/alice/my-module.git",
		HTMLURL:       "https://github.com/alice/my-module",
		DefaultBranch: "main",
		Provider:      types.ExternalRepositoryTypeGitHub,
		FullName:      "alice/my-module",
	}, resp)
}

func (s *GitTemplateSuite) TestGitHubNameTakenIs422WithProviderMessage() {
	s.expectOAuth(s.githubOAuth("repo"))
	s.github.HandleFunc("POST /api/v3/repos/helixml/helixos-module-template/generate", func(w http.ResponseWriter, _ *http.Request) {
		writeJSONBody(w, http.StatusUnprocessableEntity, `{"message":"Repository creation failed.",
			"errors":[{"resource":"Repository","code":"custom","field":"name","message":"name already exists on this account"}]}`)
	})

	rec := s.createFromTemplate(types.CreateRepoFromTemplateRequest{
		Provider: types.ExternalRepositoryTypeGitHub,
		Owner:    "alice",
		Name:     "taken",
		Template: "helixml/helixos-module-template",
	})

	s.Require().Equal(http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	var httpErr system.HTTPError
	s.Require().NoError(json.Unmarshal(rec.Body.Bytes(), &httpErr))
	s.Equal("github: Repository creation failed.: name already exists on this account", httpErr.Message)
}

func (s *GitTemplateSuite) TestGitLabImportPollsUntilFinished() {
	s.expectOAuth(&types.OAuthConnection{
		UserID:      s.userID,
		AccessToken: "gl-oauth-token",
		Scopes:      []string{"api", "read_repository"},
		Provider:    types.OAuthProvider{Type: types.OAuthProviderTypeGitLab},
	})
	s.gitlab.HandleFunc("GET /api/v4/namespaces/{path}", func(w http.ResponseWriter, r *http.Request) {
		s.Equal("Bearer gl-oauth-token", r.Header.Get("Authorization"))
		s.Equal("team/sub", r.PathValue("path"))
		writeJSONBody(w, http.StatusOK, `{"id":81,"full_path":"team/sub","kind":"group"}`)
	})
	var createBody map[string]any
	s.gitlab.HandleFunc("POST /api/v4/projects", func(w http.ResponseWriter, r *http.Request) {
		s.Require().NoError(json.NewDecoder(r.Body).Decode(&createBody))
		writeJSONBody(w, http.StatusCreated, `{"id":99,"path_with_namespace":"team/sub/my-module","import_status":"scheduled"}`)
	})
	var importChecks atomic.Int32
	s.gitlab.HandleFunc("GET /api/v4/projects/99/import", func(w http.ResponseWriter, _ *http.Request) {
		switch importChecks.Add(1) {
		case 1:
			writeJSONBody(w, http.StatusOK, `{"id":99,"import_status":"scheduled"}`)
		case 2:
			writeJSONBody(w, http.StatusOK, `{"id":99,"import_status":"started"}`)
		default:
			writeJSONBody(w, http.StatusOK, `{"id":99,"import_status":"finished"}`)
		}
	})
	s.gitlab.HandleFunc("GET /api/v4/projects/99", func(w http.ResponseWriter, _ *http.Request) {
		writeJSONBody(w, http.StatusOK, `{"id":99,"path_with_namespace":"team/sub/my-module","default_branch":"main",
			"http_url_to_repo":"https://gitlab.example/team/sub/my-module.git","web_url":"https://gitlab.example/team/sub/my-module"}`)
	})

	private := false
	rec := s.createFromTemplate(types.CreateRepoFromTemplateRequest{
		Provider: types.ExternalRepositoryTypeGitLab,
		Owner:    "team/sub",
		Name:     "my-module",
		Private:  &private,
		Template: "helixml/helixos-module-template",
	})

	s.Require().Equal(http.StatusCreated, rec.Code, rec.Body.String())
	s.Equal(map[string]any{
		"name":         "my-module",
		"path":         "my-module",
		"namespace_id": float64(81),
		"visibility":   "public",
		"description":  "",
		"import_url":   "https://github.com/helixml/helixos-module-template.git",
	}, createBody)
	s.Equal(int32(3), importChecks.Load())
	var resp types.CreateRepoFromTemplateResponse
	s.Require().NoError(json.Unmarshal(rec.Body.Bytes(), &resp))
	s.Equal(types.CreateRepoFromTemplateResponse{
		CloneURL:      "https://gitlab.example/team/sub/my-module.git",
		HTMLURL:       "https://gitlab.example/team/sub/my-module",
		DefaultBranch: "main",
		Provider:      types.ExternalRepositoryTypeGitLab,
		FullName:      "team/sub/my-module",
	}, resp)
}

func (s *GitTemplateSuite) TestGitLabImportFailedIs422() {
	s.expectOAuth()
	s.expectPATs(s.gitlabPAT("gl-pat"))
	s.gitlab.HandleFunc("GET /api/v4/namespaces/bob", func(w http.ResponseWriter, _ *http.Request) {
		writeJSONBody(w, http.StatusOK, `{"id":70,"full_path":"bob","kind":"user"}`)
	})
	s.gitlab.HandleFunc("POST /api/v4/projects", func(w http.ResponseWriter, _ *http.Request) {
		writeJSONBody(w, http.StatusCreated, `{"id":99,"path_with_namespace":"bob/my-module"}`)
	})
	s.gitlab.HandleFunc("GET /api/v4/projects/99/import", func(w http.ResponseWriter, _ *http.Request) {
		writeJSONBody(w, http.StatusOK, `{"id":99,"import_status":"failed","import_error":"repository not found"}`)
	})

	rec := s.createFromTemplate(types.CreateRepoFromTemplateRequest{
		Provider: types.ExternalRepositoryTypeGitLab,
		Owner:    "bob",
		Name:     "my-module",
		Template: "helixml/missing",
	})

	s.Require().Equal(http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	s.Contains(rec.Body.String(), "repository not found")
}

func (s *GitTemplateSuite) TestGitLabNameTakenIs422WithProviderMessage() {
	s.expectOAuth()
	s.expectPATs(s.gitlabPAT("gl-pat"))
	s.gitlab.HandleFunc("GET /api/v4/namespaces/bob", func(w http.ResponseWriter, _ *http.Request) {
		writeJSONBody(w, http.StatusOK, `{"id":70,"full_path":"bob","kind":"user"}`)
	})
	s.gitlab.HandleFunc("POST /api/v4/projects", func(w http.ResponseWriter, _ *http.Request) {
		writeJSONBody(w, http.StatusBadRequest, `{"message":{"name":["has already been taken"],"path":["has already been taken"]}}`)
	})

	rec := s.createFromTemplate(types.CreateRepoFromTemplateRequest{
		Provider: types.ExternalRepositoryTypeGitLab,
		Owner:    "bob",
		Name:     "taken",
		Template: "helixml/helixos-module-template",
	})

	s.Require().Equal(http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	s.Contains(rec.Body.String(), "has already been taken")
}

func (s *GitTemplateSuite) TestConnectRequiredWithoutCredentials() {
	// A GitHub OAuth connection without the repo scope is not usable, and a
	// GitHub Enterprise PAT cannot generate from a github.com template.
	s.expectOAuth(s.githubOAuth("read:user"))
	s.expectPATs(&types.GitProviderConnection{
		UserID:       s.userID,
		ProviderType: types.ExternalRepositoryTypeGitHub,
		BaseURL:      "https://github.corp.example",
	})

	rec := s.createFromTemplate(types.CreateRepoFromTemplateRequest{
		Provider: types.ExternalRepositoryTypeGitHub,
		Owner:    "alice",
		Name:     "my-module",
		Template: "helixml/helixos-module-template",
	})

	s.Require().Equal(http.StatusConflict, rec.Code, rec.Body.String())
	var body types.ConnectRequiredError
	s.Require().NoError(json.Unmarshal(rec.Body.Bytes(), &body))
	s.Equal(types.ConnectRequiredError{
		Code:       "connect_required",
		Provider:   types.ExternalRepositoryTypeGitHub,
		ConnectURL: "https://helix.example/orgs?dialog=connected-services&provider=github",
	}, body)
}

func (s *GitTemplateSuite) TestRevokedTokenIsConnectRequired() {
	s.expectOAuth()
	s.expectPATs(s.gitlabPAT("revoked"))
	s.gitlab.HandleFunc("GET /api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		writeJSONBody(w, http.StatusUnauthorized, `{"message":"401 Unauthorized"}`)
	})

	rec := s.listOwners("gitlab")

	s.Require().Equal(http.StatusConflict, rec.Code, rec.Body.String())
	s.Contains(rec.Body.String(), `"code":"connect_required"`)
}

func (s *GitTemplateSuite) TestInvalidRequestIs400() {
	rec := s.createFromTemplate(types.CreateRepoFromTemplateRequest{
		Provider: types.ExternalRepositoryTypeGitHub,
		Owner:    "alice",
		Name:     "my-module",
		Template: "https://github.com/helixml/helixos-module-template",
	})
	s.Equal(http.StatusBadRequest, rec.Code, rec.Body.String())

	rec = s.listOwners("bitbucket")
	s.Equal(http.StatusBadRequest, rec.Code, rec.Body.String())
}
