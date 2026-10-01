package hydra

import (
	"bufio"
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

// A hosted web service may take more than 30s to answer. The proxy used to cap
// every request at 30s end to end, which turned a 32s CV parse on we-find.ai
// into the branded "Starting up" page. Takes 31s, so it is skipped with -short.
func TestForwardOutlastsThirtySeconds(t *testing.T) {
	if testing.Short() {
		t.Skip("takes 31s")
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(31 * time.Second)
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
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("a 31s response came back %d; the proxy is still capping requests at 30s", resp.StatusCode)
	}
}

// A streamed response must reach the client as it is produced, not when it ends.
func TestForwardStreamsBeforeTheResponseEnds(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: first\n\n"))
		w.(http.Flusher).Flush()
		time.Sleep(2 * time.Second)
		_, _ = w.Write([]byte("data: second\n\n"))
	}))
	defer upstream.Close()
	proxy := forwardServer()
	defer proxy.Close()

	req, _ := http.NewRequest(http.MethodGet, proxy.URL, nil)
	req.Header.Set("X-Test-Target", upstream.URL)
	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	line, err := bufio.NewReader(resp.Body).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, "first") {
		t.Fatalf("first line = %q", line)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("the first event arrived after %v; the proxy buffered the stream", elapsed)
	}
}

// Removing the flat cap must not let a hung upstream hold a connection forever:
// an upstream that never starts answering still gets the upstream-unavailable
// signal the API proxy turns into its holding page.
func TestForwardGivesUpOnAnUpstreamThatNeverAnswers(t *testing.T) {
	orig := devContainerProxyTransport
	t.Cleanup(func() { devContainerProxyTransport = orig })
	short := orig.Clone()
	short.ResponseHeaderTimeout = 200 * time.Millisecond
	devContainerProxyTransport = short

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
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
