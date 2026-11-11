package browser_test

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"
	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/controller/knowledge/browser"
	"github.com/helixml/helix/api/pkg/controller/knowledge/browser/browsertest"
)

const guardSecret = "INTERNAL-ONLY-VALUE"

// refuseHost stands in for the outbound policy: it refuses one host:port.
type refuseHost struct{ host string }

func (r refuseHost) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host == r.host {
		return nil, errors.New("destination not allowed")
	}
	return http.DefaultTransport.RoundTrip(req)
}

// A guarded page must not reach a refused destination through anything it
// can start: service workers, dedicated workers (fetch and WebSocket) or
// cross-site iframes, which run outside the page's own request interception.
func TestGuardPage_CoversWorkersAndFrames(t *testing.T) {
	b := browsertest.New(t)

	var internalHits atomic.Int32
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		internalHits.Add(1)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		_, _ = w.Write([]byte(guardSecret))
	}))
	defer internal.Close()
	internalURL, _ := url.Parse(internal.URL)
	wsURL := "ws://" + internalURL.Host + "/ws"

	report := `function report(t) { const p = document.createElement("p"); p.textContent = t; document.body.appendChild(p) }`
	var publicPort string
	public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		js := func(body string) {
			w.Header().Set("Content-Type", "text/javascript")
			_, _ = w.Write([]byte(body))
		}
		page := func(body string) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<!doctype html><html><body><script>" + report + body + "</script></body></html>"))
		}
		switch r.URL.Path {
		case "/sw":
			page(`navigator.serviceWorker.addEventListener("message", e => report(e.data));
navigator.serviceWorker.register("/sw.js").then(() => navigator.serviceWorker.ready).then(r => r.active.postMessage("go"))`)
		case "/sw.js":
			js(`self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", e => e.waitUntil(self.clients.claim()));
fetch("` + internal.URL + `/sw-install").catch(() => {});
self.addEventListener("message", e => fetch("` + internal.URL + `/sw").then(r => r.text()).then(t => e.source.postMessage(t)));`)
		case "/worker":
			page(`const w = new Worker("/w.js"); w.onmessage = e => report(e.data)`)
		case "/w.js":
			js(`fetch("` + internal.URL + `/worker").then(r => r.text()).then(t => postMessage(t)).catch(() => {});
try { const s = new WebSocket("` + wsURL + `"); s.onmessage = e => postMessage(e.data) } catch (e) {}`)
		case "/frame":
			// localhost and 127.0.0.1 are different sites: an out-of-process iframe.
			page(`window.addEventListener("message", e => report(e.data));
const f = document.createElement("iframe"); f.src = "http://localhost:` + publicPort + `/framecontent"; document.body.appendChild(f)`)
		case "/framecontent":
			page(`fetch("` + internal.URL + `/frame").then(r => r.text()).then(t => parent.postMessage(t, "*")).catch(() => {})`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer public.Close()
	_, publicPort, _ = net.SplitHostPort(strings.TrimPrefix(public.URL, "http://"))

	client := &http.Client{Transport: refuseHost{host: internalURL.Host}}
	for _, path := range []string{"/sw", "/worker", "/frame"} {
		t.Run(path, func(t *testing.T) {
			before := internalHits.Load()
			rb, err := b.GetBrowser()
			require.NoError(t, err)
			page, err := b.GetPage(rb, proto.TargetCreateTarget{})
			require.NoError(t, err)
			defer b.PutPage(page)
			stop, err := browser.GuardPage(page, client)
			require.NoError(t, err)
			defer stop()

			require.NoError(t, page.Navigate(public.URL+path))
			_ = page.Timeout(10 * time.Second).WaitLoad()
			time.Sleep(3 * time.Second)
			html, err := page.HTML()
			require.NoError(t, err)
			require.NotContains(t, html, guardSecret)
			require.Equal(t, before, internalHits.Load(), "the internal server was contacted")
		})
	}
}
