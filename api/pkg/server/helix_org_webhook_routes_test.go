package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

// captureHandler records the mux vars the router hands the handler, so a
// test can assert the id actually reached it rather than only that the
// path matched.
type captureHandler struct {
	called    bool
	triggerID string
	org       string
}

func (c *captureHandler) ServeHTTP(_ http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	c.called = true
	c.org = vars["org"]
	c.triggerID = vars["trigger_id"]
}

// TestPublicTriggerWebhookRoutes pins the wiring that #3087 broke: the
// route patterns must declare the same variable name the handlers read,
// on every path shape a live webhook may still be pointing at.
//
// The pre-fix failure modes this locks out:
//   - /topics/… matched but yielded an empty trigger_id -> 400
//   - /triggers/… was not mounted at all -> fell through to authRouter -> 401
func TestPublicTriggerWebhookRoutes(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
	}{
		{"github current", "/api/v1/orgs/acme/triggers/s-github-pr/github/webhook"},
		{"github legacy", "/api/v1/orgs/acme/topics/s-github-pr/github/webhook"},
		{"gitlab current", "/api/v1/orgs/acme/triggers/s-github-pr/gitlab/webhook"},
		{"gitlab legacy", "/api/v1/orgs/acme/topics/s-github-pr/gitlab/webhook"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			github, gitlab := &captureHandler{}, &captureHandler{}
			router := mux.NewRouter()
			insecure := router.PathPrefix(APIPrefix).Subrouter()
			registerPublicTriggerWebhookRoutes(insecure, github, gitlab)
			// Stand in for authRouter's catch-all. Anything the
			// webhook routes fail to claim lands here, which is a
			// 401 in production.
			router.PathPrefix("/orgs/{org}/").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			})

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, tc.path, nil))

			got := github
			if gitlab.called {
				got = gitlab
			}
			require.True(t, got.called, "no webhook handler ran; got status %d", rec.Code)
			require.Equal(t, "acme", got.org)
			require.Equal(t, "s-github-pr", got.triggerID,
				"handler reads vars[\"trigger_id\"]; the route pattern must declare that name")
		})
	}
}

// TestPublicTriggerWebhookRoutesSkipNilHandlers covers the deployments
// where a transport is unwired: no route is mounted, so the request falls
// through rather than panicking on a nil handler.
func TestPublicTriggerWebhookRoutesSkipNilHandlers(t *testing.T) {
	router := mux.NewRouter()
	registerPublicTriggerWebhookRoutes(router.PathPrefix(APIPrefix).Subrouter(), nil, nil)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost,
		"/api/v1/orgs/acme/triggers/s-github-pr/github/webhook", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
}
