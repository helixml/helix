package helixorg

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The manifest callback is a public GET redirected from github.com. The
// created app's private key must only be stored for the org whose member
// started the flow, from that same member's browser session.
func manifestCallbackRequest(state string) *http.Request {
	q := url.Values{"code": {"gh-code"}, "state": {state}}
	return httptest.NewRequest(http.MethodGet, "/api/v1/orgs/org-1/github/app-manifest/callback?"+q.Encode(), nil)
}

func sealedManifestState(t *testing.T, userID string) string {
	t.Helper()
	state, err := encodeGitHubManifestState(githubManifestState{
		OrgID: "org-1", GitHubOrg: "acme", UserID: userID,
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}, testEncKey)
	require.NoError(t, err)
	return state
}

func TestGitHubManifestCallback_RefusesAnonymousBrowser(t *testing.T) {
	h := NewGitHubManifestCallbackHandler(testKeyGetter, nil, nil, "https://github.com", "",
		func(*http.Request) string { return "" })
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, manifestCallbackRequest(sealedManifestState(t, "usr_admin")))
	require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
}

func TestGitHubManifestCallback_RefusesAnotherUsersSession(t *testing.T) {
	h := NewGitHubManifestCallbackHandler(testKeyGetter, nil, nil, "https://github.com", "",
		func(*http.Request) string { return "usr_victim" })
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, manifestCallbackRequest(sealedManifestState(t, "usr_attacker")))
	require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
}

func TestGitHubManifestCallback_RefusesStateWithoutUser(t *testing.T) {
	state, err := encodeGitHubManifestState(githubManifestState{
		OrgID: "org-1", GitHubOrg: "acme", ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}, testEncKey)
	require.NoError(t, err)
	h := NewGitHubManifestCallbackHandler(testKeyGetter, nil, nil, "https://github.com", "",
		func(*http.Request) string { return "usr_admin" })
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, manifestCallbackRequest(state))
	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
}

func TestGitHubManifestCallback_ProceedsForFlowStarter(t *testing.T) {
	// A GitHub API stand-in that rejects the conversion: reaching it proves the
	// identity gate let the starter through, without storing anything.
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer gh.Close()

	h := NewGitHubManifestCallbackHandler(testKeyGetter, nil, nil, "https://github.com", gh.URL,
		func(*http.Request) string { return "usr_admin" })
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, manifestCallbackRequest(sealedManifestState(t, "usr_admin")))
	require.Equal(t, http.StatusBadGateway, rr.Code, rr.Body.String())
}
