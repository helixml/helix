package hydra

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func forwardServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The target URL is carried in a header only for the test; the real
		// handler builds it from the container's IP.
		forwardToDevContainer(w, r, r.Header.Get("X-Test-Target"), "ses_test", 8080)
	}))
}

func TestForwardMarksResponseThatReachedUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer upstream.Close()
	proxy := forwardServer()
	defer proxy.Close()

	req, _ := http.NewRequest(http.MethodHead, proxy.URL, nil)
	req.Header.Set("X-Test-Target", upstream.URL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound || resp.Header.Get(devContainerUpstreamReachedHeader) != "1" {
		t.Fatalf("got %d (upstream reached=%q), want upstream 404 with marker",
			resp.StatusCode, resp.Header.Get(devContainerUpstreamReachedHeader))
	}
}

// withResponseHeaderTimeout shrinks the proxy's header timeout for one test.
func withResponseHeaderTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	orig := devContainerProxyClient.Transport
	t.Cleanup(func() { devContainerProxyClient.Transport = orig })
	short := devContainerProxyTransport.Clone()
	short.ResponseHeaderTimeout = d
	devContainerProxyClient.Transport = short
}

// A hosted web service may take more than 30s to answer. The proxy used to cap
// every request at 30s end to end (http.Client.Timeout), which turned a 32s CV
// parse on we-find.ai into the branded "Starting up" page. The only upstream
// bound is the transport's header timeout, and it must stay generous.
func TestForwardHasNoEndToEndCap(t *testing.T) {
	if devContainerProxyClient.Timeout != 0 {
		t.Fatalf("proxy client Timeout = %v; it caps whole responses, body included", devContainerProxyClient.Timeout)
	}
	if devContainerProxyResponseHeaderTimeout < 2*time.Minute {
		t.Fatalf("header timeout %v is too short for an API route waiting on a model", devContainerProxyResponseHeaderTimeout)
	}
}

// Once headers arrive the body streams for as long as it takes: the header
// timeout must not also cap the body.
func TestForwardBodyOutlastsTheHeaderTimeout(t *testing.T) {
	withResponseHeaderTimeout(t, 200*time.Millisecond)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		time.Sleep(600 * time.Millisecond)
		_, _ = w.Write([]byte(`{"parsed":true}`))
	}))
	defer upstream.Close()
	proxy := forwardServer()
	defer proxy.Close()

	req, _ := http.NewRequest(http.MethodPost, proxy.URL, nil)
	req.Header.Set("X-Test-Target", upstream.URL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || string(body) != `{"parsed":true}` {
		t.Fatalf("got %d %q; the body was cut off by the header timeout", resp.StatusCode, body)
	}
}

// A streamed response must reach the client as it is produced, not when it ends.
// The upstream holds the stream open until the client has read the first
// event, so a buffering proxy never delivers it and the read hits the deadline.
func TestForwardStreamsBeforeTheResponseEnds(t *testing.T) {
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: first\n\n"))
		w.(http.Flusher).Flush()
		<-release
		_, _ = w.Write([]byte("data: second\n\n"))
	}))
	defer upstream.Close()
	defer close(release)
	proxy := forwardServer()
	defer proxy.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, proxy.URL, nil)
	req.Header.Set("X-Test-Target", upstream.URL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("no response while the upstream stream was still open; the proxy buffered it: %v", err)
	}
	defer resp.Body.Close()
	line, err := bufio.NewReader(resp.Body).ReadString('\n')
	if err != nil {
		t.Fatalf("the first event never arrived while the stream was open; the proxy buffered it: %v", err)
	}
	if !strings.Contains(line, "first") {
		t.Fatalf("first line = %q", line)
	}
}

// Removing the flat cap must not let a hung upstream hold a connection forever:
// an upstream that never starts answering still gets the upstream-unavailable
// signal the API proxy turns into its holding page.
func TestForwardGivesUpOnAnUpstreamThatNeverAnswers(t *testing.T) {
	withResponseHeaderTimeout(t, 200*time.Millisecond)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // never answers; released when the proxy gives up
	}))
	defer upstream.Close()
	proxy := forwardServer()
	defer proxy.Close()

	req, _ := http.NewRequest(http.MethodGet, proxy.URL, nil)
	req.Header.Set("X-Test-Target", upstream.URL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway || resp.Header.Get("X-Helix-Upstream-Unavailable") != "1" {
		t.Errorf("got %d (unavailable=%q), want 502 with the upstream-unavailable signal",
			resp.StatusCode, resp.Header.Get("X-Helix-Upstream-Unavailable"))
	}
}
