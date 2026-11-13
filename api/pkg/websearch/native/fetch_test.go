package native

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller/knowledge/browser"
	"github.com/helixml/helix/api/pkg/netguard"
	"github.com/helixml/helix/api/pkg/websearch"
)

// httptest servers are on loopback, so these fetchers allow private targets.
func newTestFetcher(t *testing.T) *HTTPChromeFetcher {
	t.Helper()
	f, err := NewFetcher(FetcherOptions{Policy: netguard.Policy{AllowPrivate: true}})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func fetchErr(t *testing.T, err error, tag string) *FetchError {
	t.Helper()
	var fe *FetchError
	if !errors.As(err, &fe) {
		t.Fatalf("err = %v, want *FetchError", err)
	}
	if fe.Tag != tag {
		t.Fatalf("tag = %s (%v), want %s", fe.Tag, fe.Err, tag)
	}
	return fe
}

const articleHTML = `<html><head><title>Hi</title><script src="/a.js"></script></head><body><article><p>` +
	`This page is rendered on the server and has plenty of readable text, so it does not need a browser. ` +
	`It goes on for a while to clear the visible-text threshold used by the rendering heuristic in the fetcher.` +
	`</p></article></body></html>`

func TestFetchFollowsRedirects(t *testing.T) {
	var ua string
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/middle", http.StatusFound) })
	mux.HandleFunc("/middle", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
		ua = r.UserAgent()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(articleHTML))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	page, err := newTestFetcher(t).Fetch(context.Background(), srv.URL+"/start#frag", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if page.URL != srv.URL+"/start" {
		t.Errorf("URL = %s, want normalised request URL", page.URL)
	}
	if page.FinalURL != srv.URL+"/final" {
		t.Errorf("FinalURL = %s", page.FinalURL)
	}
	if page.StatusCode != http.StatusOK || !strings.HasPrefix(page.ContentType, "text/html") || string(page.Body) != articleHTML {
		t.Errorf("page = %d %s %q", page.StatusCode, page.ContentType, page.Body)
	}
	if page.FetchedAt.IsZero() {
		t.Error("FetchedAt not set")
	}
	if !strings.Contains(ua, "Chrome/") {
		t.Errorf("User-Agent = %q, want a browser UA", ua)
	}
}

func TestFetchRedirectLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Path+"x", http.StatusFound)
	}))
	defer srv.Close()
	_, err := newTestFetcher(t).Fetch(context.Background(), srv.URL+"/", 5*time.Second)
	fetchErr(t, err, websearch.TagCrawlHTTPError)
}

func TestFetchReturnsNonHTMLRaw(t *testing.T) {
	pdf := []byte("%PDF-1.7\n...")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdf)
	}))
	defer srv.Close()
	page, err := newTestFetcher(t).Fetch(context.Background(), srv.URL+"/doc.pdf", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if page.ContentType != "application/pdf" || string(page.Body) != string(pdf) {
		t.Fatalf("got %s %q", page.ContentType, page.Body)
	}
}

func TestFetchStatusTags(t *testing.T) {
	for code, tag := range map[int]string{
		http.StatusNotFound:            websearch.TagCrawlNotFound,
		http.StatusGone:                websearch.TagCrawlNotFound,
		http.StatusForbidden:           websearch.TagCrawlHTTPError,
		http.StatusInternalServerError: websearch.TagCrawlHTTPError,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }))
		_, err := newTestFetcher(t).Fetch(context.Background(), srv.URL, 5*time.Second)
		srv.Close()
		if fe := fetchErr(t, err, tag); fe.HTTPStatusCode != code {
			t.Errorf("%d: HTTPStatusCode = %d", code, fe.HTTPStatusCode)
		}
	}
}

func TestFetchTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer srv.Close()
	defer close(release)
	_, err := newTestFetcher(t).Fetch(context.Background(), srv.URL, 100*time.Millisecond)
	fetchErr(t, err, websearch.TagCrawlTimeout)
}

func TestFetchSourceNotAvailable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	addr := srv.URL
	srv.Close() // nothing listens any more
	_, err := newTestFetcher(t).Fetch(context.Background(), addr, 5*time.Second)
	fetchErr(t, err, websearch.TagSourceNotAvailable)
}

func TestFetchBlocksForbiddenDestinations(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	defer srv.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer redirector.Close()

	strict, err := NewFetcher(FetcherOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{srv.URL, "http://localhost/", "http://169.254.169.254/", "file:///etc/passwd"} {
		_, err := strict.Fetch(context.Background(), raw, 5*time.Second)
		fetchErr(t, err, websearch.TagBlockedDestination)
	}
	if hit {
		t.Fatal("loopback server was reached under the default policy")
	}
	// Even where private targets are allowed, a redirect to metadata is refused.
	_, err = newTestFetcher(t).Fetch(context.Background(), redirector.URL, 5*time.Second)
	fetchErr(t, err, websearch.TagBlockedDestination)
}

// A page that needs rendering while Chrome is unreachable fails the request:
// it is not this page's fault, so it must not become a per-URL status.
func TestFetchBrowserUnavailableFailsRequest(t *testing.T) {
	spa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><div id="root"></div><script src="/app.js"></script></body></html>`))
	}))
	defer spa.Close()
	// Answers the launcher's handshake, then refuses the CDP websocket.
	manager := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"flags":{}}`))
	}))
	defer manager.Close()

	cfg := &config.ServerConfig{}
	cfg.RAG.Crawler.LauncherEnabled = true
	cfg.RAG.Crawler.LauncherURL = manager.URL
	cfg.RAG.Crawler.BrowserPoolSize = 1
	b, err := browser.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	f, err := NewFetcher(FetcherOptions{
		Policy:  netguard.Policy{AllowPrivate: true},
		Browser: b, ProxyURL: "http://proxy:1", ProxyUsername: "u", ProxyPassword: "p",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Repeated failures must not drain the browser pool: each attempt fails
	// on connect, never by waiting for a slot until the fetch times out.
	for i := range 3 {
		_, err = f.Fetch(context.Background(), spa.URL, 2*time.Second)
		var fe *FetchError
		if err == nil || errors.As(err, &fe) || errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("attempt %d: err = %v, want a browser connect failure", i, err)
		}
	}
}

func TestFetchCallerCancellationFailsRequest(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer srv.Close()
	defer close(release)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	_, err := newTestFetcher(t).Fetch(ctx, srv.URL, 5*time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	var fe *FetchError
	if errors.As(err, &fe) {
		t.Fatalf("err = %v is a *FetchError; a cancelled request is not a page failure", err)
	}
}

func TestNewFetcherRequiresProxyForBrowser(t *testing.T) {
	// A non-nil Browser is all that matters; it is never dialled here.
	_, err := NewFetcher(FetcherOptions{Browser: &browser.Browser{}})
	if err == nil {
		t.Fatal("NewFetcher with a browser but no proxy succeeded")
	}
}

func TestNeedsRendering(t *testing.T) {
	long := strings.Repeat("Readable server-rendered text. ", 20)
	cases := []struct {
		name string
		html string
		want bool
	}{
		{"server rendered article", articleHTML, false},
		{"empty react root", `<html><head><script type="module" src="/main.js"></script></head><body><div id="root"></div></body></html>`, true},
		{"noscript only", `<html><body><noscript>You need to enable JavaScript to run this app. ` + long + `</noscript><script src="/app.js"></script></body></html>`, true},
		{"shell around empty app", `<html><body><header>` + long + `</header><div id="app"></div><script src="/app.js"></script></body></html>`, true},
		{"filled next root", `<html><body><div id="__next"><main>` + long + `</main></div><script src="/_next/x.js"></script></body></html>`, false},
		{"tiny page without scripts", `<html><body><p>Hello</p></body></html>`, false},
		{"script text not counted", `<html><body><script>` + long + `</script></body></html>`, true},
	}
	for _, tc := range cases {
		if got := NeedsRendering([]byte(tc.html)); got != tc.want {
			t.Errorf("%s: NeedsRendering = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestProxyBlocked(t *testing.T) {
	var blockedHeader proto.NetworkHeaders
	if err := json.Unmarshal([]byte(`{"`+netguard.BlockedHeader+`":"1"}`), &blockedHeader); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		resp proto.NetworkResponse
		want bool
	}{
		{"proxy 403 over http", proto.NetworkResponse{URL: "http://10.0.0.1/", Status: 403, Headers: blockedHeader}, true},
		{"https origin cannot claim a block", proto.NetworkResponse{URL: "https://evil.example/", Status: 403, Headers: blockedHeader}, false},
		{"403 without the header", proto.NetworkResponse{URL: "http://a.com/", Status: 403}, false},
		{"header on a 200", proto.NetworkResponse{URL: "http://a.com/", Status: 200, Headers: blockedHeader}, false},
	}
	for _, tc := range cases {
		if got := proxyBlocked(&tc.resp); got != tc.want {
			t.Errorf("%s: proxyBlocked = %v, want %v", tc.name, got, tc.want)
		}
	}
}
