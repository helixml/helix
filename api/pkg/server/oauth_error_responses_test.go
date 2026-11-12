package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func newOAuthTestServer(t *testing.T) (*HelixAPIServer, *store.MockStore) {
	ctrl := gomock.NewController(t)
	m := store.NewMockStore(ctrl)
	return &HelixAPIServer{
		Store:        m,
		Cfg:          &config.ServerConfig{},
		oauthManager: oauth.NewManager(m, false),
	}, m
}

func oauthRequest(method, target, body string, vars map[string]string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if vars != nil {
		req = mux.SetURLVars(req, vars)
	}
	return req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_owner"}))
}

// Bodies the route must refuse with a 400 before it looks up any OAuth
// connection (the mock store fails the test on an unexpected call).
func TestResolveSharePointSite_RejectsBadInputWith400(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"number for site_url", `{"site_url": 1, "provider_id": "p"}`},
		{"object for site_url", `{"site_url": {}, "provider_id": "p"}`},
		{"malformed JSON", `{"site_url": `},
		{"missing site_url", `{"provider_id": "p"}`},
		{"missing provider_id", `{"site_url": "https://contoso.sharepoint.com/sites/x"}`},
		{"look-alike domain", `{"site_url": "https://evilsharepoint.com/sites/x", "provider_id": "p"}`},
		{"sharepoint.com as a subdomain of another host", `{"site_url": "https://contoso.sharepoint.com.evil.example/sites/x", "provider_id": "p"}`},
		{"sharepoint.com in the path", `{"site_url": "https://evil.example/contoso.sharepoint.com", "provider_id": "p"}`},
		{"bare sharepoint.com", `{"site_url": "https://sharepoint.com/sites/x", "provider_id": "p"}`},
		{"plain http", `{"site_url": "http://contoso.sharepoint.com/sites/x", "provider_id": "p"}`},
		{"embedded credentials", `{"site_url": "https://user:pw@contoso.sharepoint.com/sites/x", "provider_id": "p"}`},
		{"trailing dot", `{"site_url": "https://contoso.sharepoint.com./sites/x", "provider_id": "p"}`},
		{"non-default port", `{"site_url": "https://contoso.sharepoint.com:8443/sites/x", "provider_id": "p"}`},
		{"dot segments", `{"site_url": "https://contoso.sharepoint.com/../../me", "provider_id": "p"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newOAuthTestServer(t)
			rr := httptest.NewRecorder()
			system.DefaultWrapper(srv.handleResolveSharePointSite)(rr,
				oauthRequest(http.MethodPost, "/api/v1/oauth/sharepoint/resolve-site", tc.body, nil))

			require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
			require.NotContains(t, rr.Body.String(), "Go struct")
			require.NotContains(t, rr.Body.String(), "SharePointSiteResolveRequest")
			require.NotContains(t, rr.Body.String(), "json:")
		})
	}
}

// A valid tenant URL passes validation; an unconnected or unknown provider id
// (including one that is not a uuid) is a 404 with no database text.
func TestResolveSharePointSite_UnconnectedProviderGets404(t *testing.T) {
	srv, m := newOAuthTestServer(t)
	m.EXPECT().GetOAuthConnectionByUserAndProvider(gomock.Any(), "usr_owner", "not-a-uuid").Return(nil, store.ErrNotFound)

	rr := httptest.NewRecorder()
	system.DefaultWrapper(srv.handleResolveSharePointSite)(rr, oauthRequest(http.MethodPost, "/api/v1/oauth/sharepoint/resolve-site",
		`{"site_url": "https://contoso.sharepoint.com/sites/x", "provider_id": "not-a-uuid"}`, nil))

	require.Equal(t, http.StatusNotFound, rr.Code)
	require.Equal(t, "no connection to this OAuth provider; connect it first\n", rr.Body.String())
}

// GET /oauth/connections/{id}: missing, malformed and another user's ids all
// answer the same 404; a store failure is a 500 without the database text.
func TestGetOAuthConnection_ErrorResponses(t *testing.T) {
	cases := []struct {
		name  string
		setup func(m *store.MockStore)
		code  int
		body  string
	}{
		{"missing", func(m *store.MockStore) {
			m.EXPECT().GetOAuthConnection(gomock.Any(), "c1").Return(nil, store.ErrNotFound)
		}, http.StatusNotFound, "OAuth connection not found\n"},
		{"another user's connection", func(m *store.MockStore) {
			m.EXPECT().GetOAuthConnection(gomock.Any(), "c1").Return(&types.OAuthConnection{ID: "c1", UserID: "usr_other"}, nil)
		}, http.StatusNotFound, "OAuth connection not found\n"},
		{"store failure", func(m *store.MockStore) {
			m.EXPECT().GetOAuthConnection(gomock.Any(), "c1").
				Return(nil, errors.New(`failed to get OAuth connection: ERROR: invalid input syntax for type uuid: "c1" (SQLSTATE 22P02)`))
		}, http.StatusInternalServerError, "error getting OAuth connection\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, m := newOAuthTestServer(t)
			tc.setup(m)
			for _, h := range []struct {
				method string
				fn     http.HandlerFunc
			}{
				{http.MethodGet, system.DefaultWrapper(srv.handleGetOAuthConnection)},
				{http.MethodDelete, system.DefaultWrapper(srv.handleDeleteOAuthConnection)},
			} {
				if h.method == http.MethodDelete {
					tc.setup(m)
				}
				rr := httptest.NewRecorder()
				h.fn(rr, oauthRequest(h.method, "/api/v1/oauth/connections/c1", "", map[string]string{"id": "c1"}))
				require.Equal(t, tc.code, rr.Code, h.method)
				require.Equal(t, tc.body, rr.Body.String(), h.method)
			}
		})
	}
}

// Starting a flow for a provider that does not exist is a 404, not a 500.
func TestStartOAuthFlow_UnknownProviderGets404(t *testing.T) {
	srv, m := newOAuthTestServer(t)
	m.EXPECT().GetOAuthProvider(gomock.Any(), "github").Return(nil, store.ErrNotFound)

	rr := httptest.NewRecorder()
	system.DefaultWrapper(srv.handleStartOAuthFlow)(rr,
		oauthRequest(http.MethodGet, "/api/v1/oauth/flow/start/github", "", map[string]string{"provider_id": "github"}))

	require.Equal(t, http.StatusNotFound, rr.Code)
	require.Equal(t, "OAuth provider not found\n", rr.Body.String())
}

// Creating a provider with a mistyped body is a 400 that names the field, not
// a 500 that names the Go type.
func TestCreateOAuthProvider_TypeErrorGets400(t *testing.T) {
	srv, _ := newOAuthTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/providers", strings.NewReader(`{"name": 5}`))
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_admin", Admin: true}))

	rr := httptest.NewRecorder()
	system.DefaultWrapper(srv.handleCreateOAuthProvider)(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Equal(t, "Invalid request body: field \"name\" has the wrong type\n", rr.Body.String())
}

// Deleting a provider that does not exist (including a malformed id the store
// treats as not found) is a 404, not a silent 200.
func TestDeleteOAuthProvider_MissingGets404(t *testing.T) {
	srv, m := newOAuthTestServer(t)
	m.EXPECT().GetOAuthProvider(gomock.Any(), "not-a-uuid").Return(nil, store.ErrNotFound)
	req := mux.SetURLVars(httptest.NewRequest(http.MethodDelete, "/api/v1/oauth/providers/not-a-uuid", nil), map[string]string{"id": "not-a-uuid"})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_admin", Admin: true}))

	rr := httptest.NewRecorder()
	system.DefaultWrapper(srv.handleDeleteOAuthProvider)(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
	require.Equal(t, "OAuth provider not found\n", rr.Body.String())
}

// The body form of POST /api_keys names the missing field instead of calling
// a well-formed body invalid JSON.
func TestCreateAPIKey_MissingFieldNamed(t *testing.T) {
	srv := &HelixAPIServer{Cfg: &config.ServerConfig{}}
	for body, want := range map[string]string{
		`{"name":"k","type":"api"}`:           "app_id is required\n",
		`{"name":"k","app_id":""}`:            "type is required\n",
		`{"type":"api","app_id":""}`:          "name is required\n",
		`{"name":1,"type":"api","app_id":""}`: "Invalid request body: field \"name\" must be a string\n",
		`not json`:                            "Invalid request body: must be a valid JSON object\n",
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/api_keys", strings.NewReader(body))
		req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_a"}))
		rr := httptest.NewRecorder()
		system.DefaultWrapper(srv.createAPIKey)(rr, req)
		require.Equal(t, http.StatusBadRequest, rr.Code, body)
		require.Equal(t, want, rr.Body.String(), body)
	}
}
