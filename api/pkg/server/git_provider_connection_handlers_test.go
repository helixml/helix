package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// closedLoopbackPort returns a loopback port with nothing listening on it, so a
// dial gets "connection refused" (the PoC used 127.0.0.1:9).
func closedLoopbackPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	return port
}

func postGitProviderConnection(t *testing.T, srv *HelixAPIServer, body map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/git-provider-connections", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "user-1"}))
	rec := httptest.NewRecorder()
	srv.createGitProviderConnection(rec, req)
	return rec
}

// TST-01-271: upstream/transport error text must not be reflected by
// POST /api/v1/git-provider-connections, otherwise it acts as an internal
// connection-state oracle and discloses the upstream request path.
func TestCreateGitProviderConnection_DoesNotLeakUpstreamErrors(t *testing.T) {
	htmlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>not gitlab</body></html>"))
	}))
	defer htmlServer.Close()

	closedPort := closedLoopbackPort(t)

	cases := []struct {
		name      string
		baseURL   string
		forbidden []string
	}{
		{
			name:    "closed loopback port (PoC)",
			baseURL: fmt.Sprintf("http://127.0.0.1:%d", closedPort),
			forbidden: []string{
				"127.0.0.1", fmt.Sprintf("%d", closedPort), "dial tcp", "connection refused",
				"/api/v4/user", "failed to get current user", "failed to validate",
			},
		},
		{
			name:    "listener returning HTML",
			baseURL: htmlServer.URL,
			forbidden: []string{
				"invalid character", "looking for beginning of value", "127.0.0.1",
				"/api/v4/user", "failed to get current user", "failed to validate",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			// No store expectations: a failed validation must persist nothing.
			srv := &HelixAPIServer{Store: store.NewMockStore(ctrl)}

			rec := postGitProviderConnection(t, srv, map[string]string{
				"name":          "probe",
				"provider_type": string(types.ExternalRepositoryTypeGitLab),
				"base_url":      tc.baseURL,
				"token":         "dummy-token",
			})

			require.Equal(t, http.StatusBadRequest, rec.Code)
			body := strings.TrimSpace(rec.Body.String())
			require.Equal(t, errInvalidProviderConnectionMsg, body)
			for _, s := range tc.forbidden {
				require.NotContains(t, body, s)
			}
		})
	}
}

// The closed-port and responding-host cases must be indistinguishable by
// response body, so the endpoint cannot be used to tell open from closed ports.
func TestCreateGitProviderConnection_UniformErrorAcrossConnectionStates(t *testing.T) {
	htmlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html></html>"))
	}))
	defer htmlServer.Close()
	jsonErrServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"401 Unauthorized"}`))
	}))
	defer jsonErrServer.Close()

	var bodies []string
	for _, baseURL := range []string{
		fmt.Sprintf("http://127.0.0.1:%d", closedLoopbackPort(t)),
		htmlServer.URL,
		jsonErrServer.URL,
	} {
		ctrl := gomock.NewController(t)
		srv := &HelixAPIServer{Store: store.NewMockStore(ctrl)}
		rec := postGitProviderConnection(t, srv, map[string]string{
			"provider_type": string(types.ExternalRepositoryTypeGitLab),
			"base_url":      baseURL,
			"token":         "dummy-token",
		})
		require.Equal(t, http.StatusBadRequest, rec.Code)
		bodies = append(bodies, rec.Body.String())
	}
	for _, b := range bodies[1:] {
		require.Equal(t, bodies[0], b)
	}
}

func TestGitProviderClientErrorMessage(t *testing.T) {
	const fallback = "generic"

	// Messages we wrote ourselves remain visible to the user, even when wrapped.
	userErr := newGitProviderUserError("GitHub token is missing required scopes: %s", "repo")
	require.Equal(t, "GitHub token is missing required scopes: repo", gitProviderClientErrorMessage(userErr, fallback))
	require.Equal(t, "GitHub token is missing required scopes: repo",
		gitProviderClientErrorMessage(fmt.Errorf("outer: %w", userErr), fallback))

	// Anything else (transport errors etc.) is replaced by the fallback.
	transportErr := fmt.Errorf("failed to validate GitLab token: %w",
		&net.OpError{Op: "dial", Net: "tcp", Err: fmt.Errorf("connect: connection refused")})
	require.Equal(t, fallback, gitProviderClientErrorMessage(transportErr, fallback))
}

func TestGitHubPATScopeErrorIsUserFacing(t *testing.T) {
	err := githubPATScopeError([]string{"repo"}, true)
	require.Error(t, err)
	require.Equal(t, err.Error(), gitProviderClientErrorMessage(err, "generic"))
}
