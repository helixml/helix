package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// runnerTokenAdminRouter serves the runner-token admin routes behind
// requireAdmin, with the caller identity fixed as the auth middleware would.
func runnerTokenAdminRouter(st store.Store, caller types.User) http.Handler {
	apiServer := &HelixAPIServer{Store: st}
	router := mux.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(setRequestUser(r.Context(), caller)))
		})
	})
	adminRouter := router.MatcherFunc(matchAllRoutes).Subrouter()
	adminRouter.Use(requireAdmin)
	apiServer.registerRunnerTokenRoutes(adminRouter)
	return router
}

func serveJSON(h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != nil {
		bts, _ := json.Marshal(body)
		reader = bytes.NewReader(bts)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(method, path, reader))
	return rr
}

var testAdmin = types.User{ID: "usr_admin", Admin: true, TokenType: types.TokenTypeAPIKey, Token: "hl-admin"}

// Minting stores only a hash; the secret appears once, in the response.
func TestCreateRunnerToken_ReturnsSecretOnceAndStoresHash(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	var stored *types.RunnerToken
	st.EXPECT().CreateRunnerToken(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ any, tok *types.RunnerToken) (*types.RunnerToken, error) {
			stored = tok
			tok.CreatedAt = time.Now()
			return tok, nil
		})

	rr := serveJSON(runnerTokenAdminRouter(st, testAdmin), http.MethodPost, "/admin/runner-tokens",
		types.CreateRunnerTokenRequest{RunnerID: "host1", Name: "rack 3"})
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

	var resp types.CreateRunnerTokenResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.True(t, strings.HasPrefix(resp.Token, types.RunnerTokenPrefix))
	require.Equal(t, "host1", resp.RunnerID)
	require.NotContains(t, rr.Body.String(), "token_hash")

	require.NotNil(t, stored)
	require.Equal(t, "host1", stored.RunnerID)
	require.Equal(t, "usr_admin", stored.CreatedBy)
	require.Equal(t, types.HashRunnerToken(resp.Token), stored.TokenHash)
	require.NotContains(t, stored.TokenHash, resp.Token)
	require.True(t, strings.HasPrefix(resp.Token, stored.TokenPrefix))
	require.Less(t, len(stored.TokenPrefix), len(resp.Token)/2)
}

func TestCreateRunnerToken_RejectsBadRunnerIDs(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl) // no CreateRunnerToken expected
	h := runnerTokenAdminRouter(st, testAdmin)
	for _, id := range []string{"", " ", "desktop-ses_1", "a/b", "../x", strings.Repeat("a", 129)} {
		rr := serveJSON(h, http.MethodPost, "/admin/runner-tokens", types.CreateRunnerTokenRequest{RunnerID: id})
		require.Equal(t, http.StatusBadRequest, rr.Code, "runner id %q", id)
	}
}

// Only global admins manage runner tokens: not users, not org-scoped keys
// (their Admin flag is cleared by the auth middleware), not runner tokens.
func TestRunnerTokenRoutes_NonAdminsRefused(t *testing.T) {
	for _, caller := range []types.User{
		{ID: "usr_member", TokenType: types.TokenTypeAPIKey, Token: "hl-member"},
		{ID: "usr_admin", TokenType: types.TokenTypeAPIKey, Token: "hl-org-key", OrganizationID: "org_1"},
		{ID: "runner-system", TokenType: types.TokenTypeRunner, Token: testSharedRunnerToken},
		{ID: "runner-system", TokenType: types.TokenTypeRunner, Token: testHost1Token, RunnerID: "host1"},
	} {
		ctrl := gomock.NewController(t)
		st := store.NewMockStore(ctrl) // any store call fails the test
		h := runnerTokenAdminRouter(st, caller)

		require.Equal(t, http.StatusUnauthorized,
			serveJSON(h, http.MethodPost, "/admin/runner-tokens", types.CreateRunnerTokenRequest{RunnerID: "host1"}).Code)
		require.Equal(t, http.StatusUnauthorized, serveJSON(h, http.MethodGet, "/admin/runner-tokens", nil).Code)
		require.Equal(t, http.StatusUnauthorized, serveJSON(h, http.MethodDelete, "/admin/runner-tokens/rtok_1", nil).Code)
		ctrl.Finish()
	}
}

func TestListRunnerTokens_NoSecrets(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	st.EXPECT().ListRunnerTokens(gomock.Any(), "host1").Return([]*types.RunnerToken{
		{ID: "rtok_1", RunnerID: "host1", TokenHash: "stored-hash-value", TokenPrefix: "hrt-abcd"},
	}, nil)

	rr := serveJSON(runnerTokenAdminRouter(st, testAdmin), http.MethodGet, "/admin/runner-tokens?runner_id=host1", nil)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "rtok_1")
	require.NotContains(t, rr.Body.String(), "stored-hash-value")
}

func TestRevokeRunnerToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	now := time.Now()
	st.EXPECT().RevokeRunnerToken(gomock.Any(), "rtok_1").Return(&types.RunnerToken{ID: "rtok_1", RunnerID: "host1", RevokedAt: &now}, nil)
	st.EXPECT().RevokeRunnerToken(gomock.Any(), "rtok_missing").Return(nil, store.ErrNotFound)
	h := runnerTokenAdminRouter(st, testAdmin)

	rr := serveJSON(h, http.MethodDelete, "/admin/runner-tokens/rtok_1", nil)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "revoked_at")

	require.Equal(t, http.StatusNotFound, serveJSON(h, http.MethodDelete, "/admin/runner-tokens/rtok_missing", nil).Code)
}
