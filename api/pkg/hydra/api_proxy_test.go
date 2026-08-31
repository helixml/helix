package hydra

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestSandboxAPIProxyBlocksDebugEndpoints(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "SHOULD-NOT-REACH-UPSTREAM")
	}))
	defer upstream.Close()

	handler, err := newSandboxAPIProxyHandler(upstream.URL)
	require.NoError(t, err)
	server := httptest.NewServer(handler)
	defer server.Close()

	// /debug/pprof must never reach the upstream API through the isolated route.
	for _, path := range []string{"/debug/pprof/", "/debug/pprof/goroutine", "/debug"} {
		resp, err := http.Get(server.URL + path)
		require.NoError(t, err, path)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.Equal(t, http.StatusNotFound, resp.StatusCode, path)
		require.NotContains(t, string(body), "SHOULD-NOT-REACH-UPSTREAM", path)
	}

	// A normal API path still forwards.
	resp, err := http.Get(server.URL + "/api/v1/config")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestSandboxAPIProxyStreamsServerSentEvents(t *testing.T) {
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: ready\n\n")
		w.(http.Flusher).Flush()
		<-release
	}))
	defer upstream.Close()

	handler, err := newSandboxAPIProxyHandler(upstream.URL)
	require.NoError(t, err)
	proxy := httptest.NewServer(handler)
	defer proxy.Close()

	resp, err := http.Get(proxy.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	line := make(chan string, 1)
	go func() {
		read, _ := bufio.NewReader(resp.Body).ReadString('\n')
		line <- read
	}()
	select {
	case got := <-line:
		require.Equal(t, "data: ready\n", got)
	case <-time.After(time.Second):
		t.Fatal("first event was buffered by the sandbox API proxy")
	}
	close(release)
}

func TestSandboxAPIProxyRetriesListenerUntilAddressIsAvailable(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := occupied.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	server := NewServer(NewManager(t.TempDir(), t.TempDir()), "")
	require.NoError(t, server.StartSandboxAPIProxyWithRetry(ctx, address, upstream.URL, 10*time.Millisecond))
	require.NoError(t, occupied.Close())

	require.Eventually(t, func() bool {
		resp, requestErr := http.Get("http://" + address)
		if requestErr != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusNoContent
	}, time.Second, 10*time.Millisecond)

	cancel()
	require.NoError(t, server.Stop(context.Background()))
}

func TestSandboxAPIProxyRejectsInvalidUpstream(t *testing.T) {
	for _, upstream := range []string{"", "ftp://api.example.com"} {
		_, err := newSandboxAPIProxyHandler(upstream)
		require.Error(t, err, upstream)
	}
}

// A scheme-less HELIX_API_URL (host:port) must be tolerated by coercing it to
// http, not rejected — rejecting it previously log.Fatal'd hydra into a boot
// crash-loop that took the whole sandbox host offline.
func TestSandboxAPIProxyCoercesSchemelessUpstream(t *testing.T) {
	handler, err := newSandboxAPIProxyHandler("api:8080")
	require.NoError(t, err)
	require.NotNil(t, handler)
}
