package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authpkg "github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// GET /v1/models sits behind authMiddleware.auth. Without a caller the
// handler must refuse before it asks any provider (the provider manager is
// nil here, so reaching it panics).
func TestListModels_RefusesAnonymousCaller(t *testing.T) {
	st, _ := newVictimStore(t)
	s := newRouteTestServer(t, st)

	for _, target := range []string{"/v1/models", "/v1/models?provider=openai"} {
		rec := httptest.NewRecorder()
		s.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://localhost:8080"+target, nil))
		require.Equal(t, http.StatusUnauthorized, rec.Code, target)
	}
}

const (
	bffSessionID = "sess-model-picker"
	bffUserID    = "usr_model_picker"
)

// modelsServer answers /v1/models?provider= for bffUserID from a mocked
// provider, with a BFF session manager wired like production.
func modelsServer(t *testing.T) (*HelixAPIServer, *store.MockStore) {
	t.Helper()
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	pm := manager.NewMockProviderManager(ctrl)
	client := openai.NewMockClient(ctrl)

	st.EXPECT().GetUserSession(gomock.Any(), bffSessionID).Return(&types.UserSession{
		ID: bffSessionID, UserID: bffUserID, ExpiresAt: time.Now().Add(time.Hour), LastUsedAt: time.Now(),
	}, nil).AnyTimes()
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: bffUserID}).Return(&types.User{ID: bffUserID}, nil).AnyTimes()
	st.EXPECT().GetProviderEndpoint(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).AnyTimes()
	pm.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{Provider: "openai", Owner: bffUserID}).Return(client, nil).AnyTimes()
	client.EXPECT().ListModels(gomock.Any()).Return([]types.OpenAIModel{{ID: "gpt-test"}}, nil).AnyTimes()

	cfg := &config.ServerConfig{}
	s := &HelixAPIServer{Cfg: cfg, Store: st, providerManager: pm}
	s.authMiddleware = newAuthMiddleware(nil, nil, st, authMiddlewareConfig{}, cfg, authpkg.NewSessionManager(st, nil, nil))
	s.authMiddleware.lastSeenCache.Store(bffUserID, time.Now().Add(time.Hour))
	return s, st
}

// The browser model picker (useHelixModelsForProvider) calls the root
// /v1/models with only the helix_session cookie. It must be served.
func TestListModels_ServesBFFSessionCookie(t *testing.T) {
	s, _ := modelsServer(t)
	handler := s.authMiddleware.auth(s.listModels) // as mounted in registerRoutes

	req := httptest.NewRequest(http.MethodGet, "/v1/models?provider=openai", nil)
	req.AddCookie(&http.Cookie{Name: authpkg.SessionCookieName, Value: bffSessionID})
	rec := httptest.NewRecorder()
	handler(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var list types.OpenAIModelsList
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&list))
	require.Len(t, list.Models, 1)
	require.Equal(t, "gpt-test", list.Models[0].ID)
}

// The root /v1 routes have no CSRF middleware, so the session cookie
// authenticates GET/HEAD only. A cookie-only POST runs as nobody, even with
// a CSRF token: the handler sees no user.
func TestAuthWrapper_SessionCookieOnlyForSafeMethods(t *testing.T) {
	s, _ := modelsServer(t)
	var seen *types.User
	handler := s.authMiddleware.auth(func(w http.ResponseWriter, r *http.Request) {
		seen = getRequestUser(r)
		w.WriteHeader(http.StatusNoContent)
	})

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		seen = nil
		req := httptest.NewRequest(method, "/v1/models", nil)
		req.AddCookie(&http.Cookie{Name: authpkg.SessionCookieName, Value: bffSessionID})
		handler(httptest.NewRecorder(), req)
		require.NotNil(t, seen, method)
		require.Equal(t, bffUserID, seen.ID, method)
	}

	seen = nil
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.AddCookie(&http.Cookie{Name: authpkg.SessionCookieName, Value: bffSessionID})
	req.AddCookie(&http.Cookie{Name: authpkg.CSRFCookieName, Value: "csrf-1"})
	req.Header.Set(authpkg.CSRFHeaderName, "csrf-1")
	handler(httptest.NewRecorder(), req)
	require.False(t, hasUser(seen), "a cookie-only POST must not authenticate as the session user")
}

// The embeddings unix socket has no credentials. With no
// HELIX_EMBEDDINGS_SOCKET_USER_ID it runs as the socket user, which
// listModels must serve.
func TestEmbeddingsSocket_ModelsServedWithoutConfiguredUser(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	pm := manager.NewMockProviderManager(ctrl)
	client := openai.NewMockClient(ctrl)
	pm.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{Provider: "openai", Owner: socketUserID}).Return(client, nil)
	client.EXPECT().ListModels(gomock.Any()).Return([]types.OpenAIModel{{ID: "embed-test"}}, nil)
	st.EXPECT().GetProviderEndpoint(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)

	s := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: st, providerManager: pm}
	router, err := s.embeddingsSocketRouter(context.Background())
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/models?provider=openai", nil))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}
