package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type GitProviderConnectionSuite struct {
	suite.Suite
	store      *store.MockStore
	server     *HelixAPIServer
	connection *types.GitProviderConnection
	key        []byte
	requests   []string
	requestsMu sync.Mutex
}

func TestGitProviderConnectionSuite(t *testing.T) {
	suite.Run(t, new(GitProviderConnectionSuite))
}

func (s *GitProviderConnectionSuite) SetupTest() {
	s.T().Setenv("HELIX_ENCRYPTION_KEY", "git-provider-connection-test-key")
	var err error
	s.key, err = crypto.GetEncryptionKey()
	s.Require().NoError(err)
	oldToken, err := crypto.EncryptAES256GCM([]byte("old-revoked-token"), s.key)
	s.Require().NoError(err)
	s.requests = nil
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requestsMu.Lock()
		s.requests = append(s.requests, r.URL.Path)
		s.requestsMu.Unlock()
		token := r.Header.Get("Private-Token")
		if token != "new-valid-token" && token != "no-repository-access" {
			http.Error(w, `{"message":"401 Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v4/user":
			_, _ = w.Write([]byte(`{"id":1,"username":"gitlab-user","email":"user@example.com","avatar_url":"https://example.com/avatar.png"}`))
		case "/api/v4/projects":
			if token == "no-repository-access" {
				http.Error(w, `{"message":"403 Forbidden"}`, http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte(`[{"id":2,"name":"repo","path_with_namespace":"gitlab-user/repo","http_url_to_repo":"https://example.com/repo.git","visibility":"private"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	s.T().Cleanup(provider.Close)
	s.connection = &types.GitProviderConnection{
		ID:           "saved-connection",
		UserID:       "owner",
		ProviderType: types.ExternalRepositoryTypeGitLab,
		BaseURL:      provider.URL,
		Name:         "My GitLab",
		Token:        oldToken,
		CreatedAt:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	s.store = store.NewMockStore(gomock.NewController(s.T()))
	s.server = &HelixAPIServer{Store: s.store}
}

func (s *GitProviderConnectionSuite) requestedPaths() []string {
	s.requestsMu.Lock()
	defer s.requestsMu.Unlock()
	return append([]string(nil), s.requests...)
}

func (s *GitProviderConnectionSuite) request(method, body, owner string) *http.Request {
	r := httptest.NewRequest(method, "/api/v1/git-provider-connections/saved-connection", strings.NewReader(body))
	r = mux.SetURLVars(r, map[string]string{"id": s.connection.ID})
	if owner != "" {
		r = r.WithContext(setRequestUser(r.Context(), types.User{ID: owner}))
	}
	return r
}

func (s *GitProviderConnectionSuite) TestReplaceThenBrowseSameConnectionWithoutExposingTokens() {
	original := *s.connection
	var saved *types.GitProviderConnection
	s.store.EXPECT().GetGitProviderConnection(gomock.Any(), original.ID).Return(s.connection, nil)
	s.store.EXPECT().UpdateGitProviderConnection(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, updated *types.GitProviderConnection) error {
			token, err := crypto.DecryptAES256GCM(updated.Token, s.key)
			s.Require().NoError(err)
			s.Equal("new-valid-token", string(token))
			s.Equal(original.ID, updated.ID)
			s.Equal(original.UserID, updated.UserID)
			s.Equal(original.BaseURL, updated.BaseURL)
			s.Equal(original.Name, updated.Name)
			s.Equal(original.CreatedAt, updated.CreatedAt)
			s.NotNil(updated.LastTestedAt)
			saved = updated
			return nil
		})

	response := httptest.NewRecorder()
	s.server.updateGitProviderConnection(response, s.request(http.MethodPut, `{"token":"new-valid-token"}`, "owner"))
	s.Require().Equal(http.StatusOK, response.Code, response.Body.String())
	for _, secret := range []string{"old-revoked-token", original.Token, "new-valid-token", saved.Token} {
		s.NotContains(response.Body.String(), secret)
	}
	s.NotContains(response.Body.String(), `"token"`)
	s.Equal(original.Token, s.connection.Token)

	s.store.EXPECT().GetGitProviderConnection(gomock.Any(), original.ID).Return(saved, nil)
	browse := httptest.NewRecorder()
	s.server.browseGitProviderConnectionRepositories(browse, s.request(http.MethodGet, "", "owner"))
	s.Equal(http.StatusOK, browse.Code, browse.Body.String())
	s.Contains(browse.Body.String(), "gitlab-user/repo")
	s.Equal([]string{"/api/v4/user", "/api/v4/projects", "/api/v4/projects"}, s.requestedPaths())
}

func (s *GitProviderConnectionSuite) TestInvalidReplacementKeepsExistingToken() {
	s.store.EXPECT().GetGitProviderConnection(gomock.Any(), s.connection.ID).Return(s.connection, nil)
	oldToken := s.connection.Token
	response := httptest.NewRecorder()
	s.server.updateGitProviderConnection(response, s.request(http.MethodPut, `{"token":"invalid-new-token"}`, "owner"))
	s.Equal(http.StatusBadRequest, response.Code)
	s.Equal(oldToken, s.connection.Token)
	s.NotContains(response.Body.String(), oldToken)
	s.Equal([]string{"/api/v4/user"}, s.requestedPaths())
}

func (s *GitProviderConnectionSuite) TestMissingRepositoryAccessKeepsExistingToken() {
	s.store.EXPECT().GetGitProviderConnection(gomock.Any(), s.connection.ID).Return(s.connection, nil)
	oldToken := s.connection.Token
	response := httptest.NewRecorder()
	s.server.updateGitProviderConnection(response, s.request(http.MethodPut, `{"token":"no-repository-access"}`, "owner"))
	s.Equal(http.StatusBadRequest, response.Code)
	s.Contains(response.Body.String(), "Failed to browse repositories with new token")
	s.Equal(oldToken, s.connection.Token)
	s.Equal([]string{"/api/v4/user", "/api/v4/projects"}, s.requestedPaths())
}

func (s *GitProviderConnectionSuite) TestRejectMissingAndMalformedTokens() {
	for _, body := range []string{`{}`, `{"token":"  "}`, `{"token":123}`, `not-json`} {
		s.store.EXPECT().GetGitProviderConnection(gomock.Any(), s.connection.ID).Return(s.connection, nil)
		response := httptest.NewRecorder()
		s.server.updateGitProviderConnection(response, s.request(http.MethodPut, body, "owner"))
		s.Equal(http.StatusBadRequest, response.Code, body)
	}
	s.Empty(s.requestedPaths())
}

func (s *GitProviderConnectionSuite) TestRejectUnauthenticatedAndOtherOwners() {
	response := httptest.NewRecorder()
	s.server.updateGitProviderConnection(response, s.request(http.MethodPut, `{"token":"new-valid-token"}`, ""))
	s.Equal(http.StatusUnauthorized, response.Code)

	s.store.EXPECT().GetGitProviderConnection(gomock.Any(), s.connection.ID).Return(s.connection, nil)
	response = httptest.NewRecorder()
	s.server.updateGitProviderConnection(response, s.request(http.MethodPut, `{"token":"new-valid-token"}`, "other-owner"))
	s.Equal(http.StatusNotFound, response.Code)
	s.Empty(s.requestedPaths())
}

func (s *GitProviderConnectionSuite) TestMissingConnectionAndStoreFailures() {
	for _, failure := range []struct {
		err    error
		status int
	}{{store.ErrNotFound, http.StatusNotFound}, {errors.New("database unavailable"), http.StatusInternalServerError}} {
		s.store.EXPECT().GetGitProviderConnection(gomock.Any(), s.connection.ID).Return(nil, failure.err)
		response := httptest.NewRecorder()
		s.server.updateGitProviderConnection(response, s.request(http.MethodPut, `{"token":"new-valid-token"}`, "owner"))
		s.Equal(failure.status, response.Code)
	}
	s.Empty(s.requestedPaths())
}

func (s *GitProviderConnectionSuite) TestPersistenceFailureDoesNotExposeOrMutateToken() {
	s.store.EXPECT().GetGitProviderConnection(gomock.Any(), s.connection.ID).Return(s.connection, nil)
	s.store.EXPECT().UpdateGitProviderConnection(gomock.Any(), gomock.Any()).Return(errors.New("database unavailable"))
	oldToken := s.connection.Token
	response := httptest.NewRecorder()
	s.server.updateGitProviderConnection(response, s.request(http.MethodPut, `{"token":"new-valid-token"}`, "owner"))
	s.Equal(http.StatusInternalServerError, response.Code)
	s.NotContains(response.Body.String(), "new-valid-token")
	s.Equal(oldToken, s.connection.Token)
}

func (s *GitProviderConnectionSuite) TestReplacementPreservesEncryptedBitbucketUsername() {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, token, ok := r.BasicAuth()
		if !ok || username != "bitbucket-login" || token != "new-valid-token" {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/rest/api/1.0/repos" {
			_, _ = w.Write([]byte(`{"values":[],"isLastPage":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":1,"name":"bitbucket-login","displayName":"Bitbucket user"}`))
	}))
	s.T().Cleanup(provider.Close)
	s.connection.ProviderType = types.ExternalRepositoryTypeBitbucket
	s.connection.BaseURL = provider.URL
	var err error
	s.connection.AuthUsername, err = crypto.EncryptAES256GCM([]byte("bitbucket-login"), s.key)
	s.Require().NoError(err)
	s.store.EXPECT().GetGitProviderConnection(gomock.Any(), s.connection.ID).Return(s.connection, nil)
	s.store.EXPECT().UpdateGitProviderConnection(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, updated *types.GitProviderConnection) error {
			s.Equal(s.connection.AuthUsername, updated.AuthUsername)
			token, err := crypto.DecryptAES256GCM(updated.Token, s.key)
			s.Require().NoError(err)
			s.Equal("new-valid-token", string(token))
			return nil
		})
	response := httptest.NewRecorder()
	s.server.updateGitProviderConnection(response, s.request(http.MethodPut, `{"token":"new-valid-token"}`, "owner"))
	s.Equal(http.StatusOK, response.Code, response.Body.String())
	s.NotContains(response.Body.String(), s.connection.AuthUsername)
	s.NotContains(response.Body.String(), `"auth_username"`)
}

func (s *GitProviderConnectionSuite) TestRemoveRevokedConnectionDoesNotContactProvider() {
	s.store.EXPECT().GetGitProviderConnection(gomock.Any(), s.connection.ID).Return(s.connection, nil)
	s.store.EXPECT().DeleteGitProviderConnection(gomock.Any(), s.connection.ID).Return(nil)
	response := httptest.NewRecorder()
	s.server.deleteGitProviderConnection(response, s.request(http.MethodDelete, "", "owner"))
	s.Equal(http.StatusNoContent, response.Code)
	s.Empty(s.requestedPaths())
}
