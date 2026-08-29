package hydra

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSandboxAPIProxyForwardsRequest(t *testing.T) {
	var expectedHost string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/base/v1/test", r.URL.Path)
		require.Equal(t, "stream=true", r.URL.RawQuery)
		require.Equal(t, "Bearer test", r.Header.Get("Authorization"))
		require.Equal(t, expectedHost, r.Host)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "proxied")
	}))
	defer upstream.Close()
	expectedHost = strings.TrimPrefix(upstream.URL, "http://")

	handler, err := newSandboxAPIProxyHandler(upstream.URL + "/base")
	require.NoError(t, err)
	server := httptest.NewServer(handler)
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/test?stream=true", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer test")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.Equal(t, "proxied", string(body))
}

func TestSandboxAPIProxyRejectsInvalidUpstream(t *testing.T) {
	for _, upstream := range []string{"", "api:8080", "ftp://api.example.com"} {
		_, err := newSandboxAPIProxyHandler(upstream)
		require.Error(t, err, upstream)
	}
}
