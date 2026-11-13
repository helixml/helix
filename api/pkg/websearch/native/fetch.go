package native

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/html"

	"github.com/helixml/helix/api/pkg/controller/knowledge/browser"
	"github.com/helixml/helix/api/pkg/netguard"
	"github.com/helixml/helix/api/pkg/websearch"
)

const (
	maxRedirects     = 10
	maxBodyBytes     = 20 << 20
	defaultUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

	// After window.onload, wait for this long without new network requests
	// so client-side rendering can finish, but never longer than settleMax.
	settleQuiet = 500 * time.Millisecond
	settleMax   = 3 * time.Second

	// cleanupTimeout bounds closing a page and disposing its browser
	// context, which run even after the fetch's ctx is done.
	cleanupTimeout = 5 * time.Second
)

var (
	errTooManyRedirects = fmt.Errorf("stopped after %d redirects", maxRedirects)
	errBodyTooLarge     = fmt.Errorf("response body exceeds %d bytes", maxBodyBytes)
)

// FetcherOptions configures HTTPChromeFetcher.
type FetcherOptions struct {
	// Policy is enforced on every connection the HTTP client makes. Chrome is
	// confined to it by the egress proxy, which must enforce the same policy.
	Policy netguard.Policy
	// Browser renders pages that need JavaScript. Nil disables rendering:
	// such pages are returned as served.
	Browser *browser.Browser
	// ProxyURL is the netguard.Proxy every Chrome browser context uses, as
	// reachable from the Chrome process (e.g. http://api:9223). Required
	// with Browser.
	ProxyURL      string
	ProxyUsername string
	ProxyPassword string
	// UserAgent defaults to a desktop Chrome string.
	UserAgent string
}

// HTTPChromeFetcher fetches with a guarded HTTP client and re-renders HTML
// that needs JavaScript in headless Chrome, isolated in a fresh browser
// context whose traffic all goes through the egress proxy.
type HTTPChromeFetcher struct {
	client        *http.Client
	browser       *browser.Browser
	proxyURL      string
	proxyUsername string
	proxyPassword string
	userAgent     string
}

var _ Fetcher = (*HTTPChromeFetcher)(nil)

func NewFetcher(opts FetcherOptions) (*HTTPChromeFetcher, error) {
	if opts.Browser != nil && (opts.ProxyURL == "" || opts.ProxyUsername == "" || opts.ProxyPassword == "") {
		return nil, errors.New("fetcher: a browser requires the egress proxy URL and credentials")
	}
	userAgent := opts.UserAgent
	if userAgent == "" {
		userAgent = defaultUserAgent
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	opts.Policy.GuardTransport(transport)
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errTooManyRedirects
			}
			_, err := netguard.ValidateHTTPURL(req.URL.String())
			return err
		},
	}
	return &HTTPChromeFetcher{
		client:        client,
		browser:       opts.Browser,
		proxyURL:      opts.ProxyURL,
		proxyUsername: opts.ProxyUsername,
		proxyPassword: opts.ProxyPassword,
		userAgent:     userAgent,
	}, nil
}

// Fetch GETs rawURL, following redirects, within timeout (timeout <= 0
// leaves ctx as the only bound). HTML that NeedsRendering is then loaded in
// Chrome and its rendered DOM returned instead.
func (f *HTTPChromeFetcher) Fetch(ctx context.Context, rawURL string, timeout time.Duration) (*RawPage, error) {
	start := time.Now()
	page, err := f.fetch(ctx, rawURL, timeout)
	var fe *FetchError
	if !errors.As(err, &fe) {
		return page, err
	}
	if fe.Tag == websearch.TagBlockedDestination {
		caller := websearch.CallerFrom(ctx)
		log.Warn().Str("user_id", caller.UserID).Str("org_id", caller.OrgID).Str("url", rawURL).
			Err(err).Msg("websearch: fetch blocked by egress policy")
	} else {
		log.Debug().Str("url", rawURL).Str("tag", fe.Tag).Dur("elapsed", time.Since(start)).
			Err(err).Msg("websearch: page fetch failed")
	}
	return page, err
}

func (f *HTTPChromeFetcher) fetch(ctx context.Context, rawURL string, timeout time.Duration) (*RawPage, error) {
	if _, err := netguard.ValidateHTTPURL(rawURL); err != nil {
		return nil, classifyFetchError(ctx, err)
	}
	normalized, err := websearch.NormalizeURL(rawURL)
	if err != nil {
		return nil, classifyFetchError(ctx, err)
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	page, err := f.get(ctx, normalized)
	if err != nil {
		return nil, err
	}
	if f.browser == nil || !isHTML(page.ContentType) || !NeedsRendering(page.Body) {
		return page, nil
	}
	rendered, err := f.render(ctx, page.FinalURL)
	if err != nil {
		return nil, err
	}
	rendered.URL = normalized
	return rendered, nil
}

func (f *HTTPChromeFetcher) get(ctx context.Context, normalized string) (*RawPage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, normalized, nil)
	if err != nil {
		return nil, classifyFetchError(ctx, err)
	}
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,application/pdf,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, classifyFetchError(ctx, err)
	}
	defer resp.Body.Close()
	if err := statusError(resp.StatusCode); err != nil {
		return nil, err
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return nil, classifyFetchError(ctx, err)
	}
	if len(body) > maxBodyBytes {
		return nil, &FetchError{Tag: websearch.TagUnsupportedDocument, Err: errBodyTooLarge}
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(body)
	}
	return &RawPage{
		URL:         normalized,
		FinalURL:    resp.Request.URL.String(),
		StatusCode:  resp.StatusCode,
		ContentType: contentType,
		Body:        body,
		FetchedAt:   time.Now(),
	}, nil
}

func statusError(code int) error {
	switch {
	case code == http.StatusNotFound || code == http.StatusGone:
		return &FetchError{Tag: websearch.TagCrawlNotFound, HTTPStatusCode: code, Err: fmt.Errorf("HTTP %d", code)}
	case code >= 400:
		return &FetchError{Tag: websearch.TagCrawlHTTPError, HTTPStatusCode: code, Err: fmt.Errorf("HTTP %d", code)}
	}
	return nil
}

// classifyFetchError tags a failure to reach or load a page. A request the
// caller cancelled is not a page failure and is returned as ctx.Err().
func classifyFetchError(ctx context.Context, err error) error {
	var netErr net.Error
	switch {
	case errors.Is(ctx.Err(), context.Canceled):
		return ctx.Err()
	case errors.Is(err, netguard.ErrBlockedDestination):
		return &FetchError{Tag: websearch.TagBlockedDestination, Err: err}
	case errors.Is(err, errTooManyRedirects):
		return &FetchError{Tag: websearch.TagCrawlHTTPError, Err: err}
	case errors.Is(err, context.DeadlineExceeded), errors.Is(ctx.Err(), context.DeadlineExceeded),
		errors.As(err, &netErr) && netErr.Timeout():
		return &FetchError{Tag: websearch.TagCrawlTimeout, Err: err}
	}
	return &FetchError{Tag: websearch.TagSourceNotAvailable, Err: err}
}

func isHTML(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && isHTMLMediaType(mediaType)
}

// render loads pageURL in a new Chrome browser context that sends all its
// traffic (navigation, redirects, subresources, scripts' requests) through
// the egress proxy, and returns the rendered DOM. The context is always
// disposed, closing its pages and discarding its cookies and cache.
//
// Failures of Chrome itself are returned as plain errors so they fail the
// request; only failures to load this page are *FetchError.
func (f *HTTPChromeFetcher) render(ctx context.Context, pageURL string) (*RawPage, error) {
	pooled, err := f.browser.GetBrowser(ctx)
	if err != nil {
		return nil, fmt.Errorf("get browser: %w", err)
	}
	broken := false
	defer func() {
		if broken {
			f.browser.DiscardBrowser(pooled)
			return
		}
		_ = f.browser.PutBrowser(pooled)
	}()
	b := pooled.Context(ctx)

	bctx, err := proto.TargetCreateBrowserContext{
		ProxyServer: f.proxyURL,
		// Chrome sends loopback hosts direct by default; "<-loopback>"
		// removes that bypass so localhost inside Chrome's container is
		// proxied (and refused) too.
		ProxyBypassList: "<-loopback>",
		DisposeOnDetach: true,
	}.Call(b)
	if err != nil {
		// Within the deadline, this failing means the connection is dead.
		broken = ctx.Err() == nil
		return nil, fmt.Errorf("create browser context: %w", err)
	}
	defer func() {
		// Dispose even when ctx is done: the context holds the pages' memory.
		_ = proto.TargetDisposeBrowserContext{BrowserContextID: bctx.BrowserContextID}.
			Call(pooled.Context(context.WithoutCancel(ctx)).Timeout(cleanupTimeout))
	}()

	target, err := proto.TargetCreateTarget{URL: "about:blank", BrowserContextID: bctx.BrowserContextID}.Call(b)
	if err != nil {
		return nil, fmt.Errorf("create page: %w", err)
	}
	page, err := b.PageFromTarget(target.TargetID)
	if err != nil {
		return nil, fmt.Errorf("attach page: %w", err)
	}
	defer func() { _ = page.Context(context.WithoutCancel(ctx)).Timeout(cleanupTimeout).Close() }()

	pageCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	page = page.Context(pageCtx)

	mainDoc, err := f.navigate(page, pageURL)
	if err != nil {
		return nil, classifyFetchError(ctx, err)
	}
	if mainDoc.blocked {
		return nil, &FetchError{Tag: websearch.TagBlockedDestination, Err: fmt.Errorf("%w: %s", netguard.ErrBlockedDestination, mainDoc.url)}
	}
	if err := statusError(mainDoc.status); err != nil {
		return nil, err
	}
	// Measure in Chrome first so an oversized DOM is never sent to us.
	size, err := page.Eval(`() => document.documentElement ? document.documentElement.outerHTML.length : 0`)
	if err != nil {
		return nil, classifyFetchError(ctx, fmt.Errorf("measure rendered HTML: %w", err))
	}
	if size.Value.Int() > maxBodyBytes {
		return nil, &FetchError{Tag: websearch.TagUnsupportedDocument, Err: errBodyTooLarge}
	}
	content, err := page.HTML()
	if err != nil {
		return nil, classifyFetchError(ctx, fmt.Errorf("read rendered HTML: %w", err))
	}
	if len(content) > maxBodyBytes { // the length above counts UTF-16 units, not bytes
		return nil, &FetchError{Tag: websearch.TagUnsupportedDocument, Err: errBodyTooLarge}
	}
	return &RawPage{
		FinalURL:    mainDoc.url,
		StatusCode:  mainDoc.status,
		ContentType: "text/html; charset=utf-8",
		Body:        []byte(content),
		FetchedAt:   time.Now(),
	}, nil
}

// mainFrameResponse is the main frame's last document response.
type mainFrameResponse struct {
	url     string
	status  int
	blocked bool
}

// navigate loads pageURL, answering the proxy's auth challenges, and waits
// for the page to load and briefly settle.
func (f *HTTPChromeFetcher) navigate(page *rod.Page, pageURL string) (mainFrameResponse, error) {
	if err := page.SetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: f.userAgent}); err != nil {
		return mainFrameResponse{}, fmt.Errorf("set user agent: %w", err)
	}
	if err := (proto.NetworkEnable{}).Call(page); err != nil {
		return mainFrameResponse{}, fmt.Errorf("enable network events: %w", err)
	}
	// With the Fetch domain enabled every request pauses and must be
	// continued; that is the price of answering proxy auth challenges.
	if err := (proto.FetchEnable{HandleAuthRequests: true}).Call(page); err != nil {
		return mainFrameResponse{}, fmt.Errorf("enable proxy auth: %w", err)
	}

	var (
		mu   sync.Mutex
		last mainFrameResponse
	)
	wait := page.EachEvent(
		func(e *proto.FetchRequestPaused) {
			_ = proto.FetchContinueRequest{RequestID: e.RequestID}.Call(page)
		},
		func(e *proto.FetchAuthRequired) {
			resp := &proto.FetchAuthChallengeResponse{Response: proto.FetchAuthChallengeResponseResponseCancelAuth}
			if e.AuthChallenge != nil && e.AuthChallenge.Source == proto.FetchAuthChallengeSourceProxy {
				resp = &proto.FetchAuthChallengeResponse{
					Response: proto.FetchAuthChallengeResponseResponseProvideCredentials,
					Username: f.proxyUsername,
					Password: f.proxyPassword,
				}
			}
			_ = proto.FetchContinueWithAuth{RequestID: e.RequestID, AuthChallengeResponse: resp}.Call(page)
		},
		func(e *proto.NetworkResponseReceived) {
			if e.Type != proto.NetworkResourceTypeDocument || e.FrameID != page.FrameID || e.Response == nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			last = mainFrameResponse{
				url:     e.Response.URL,
				status:  e.Response.Status,
				blocked: proxyBlocked(e.Response),
			}
		},
	)
	go wait()

	if err := page.Navigate(pageURL); err != nil {
		return mainFrameResponse{}, fmt.Errorf("navigate: %w", err)
	}
	if err := page.WaitLoad(); err != nil {
		return mainFrameResponse{}, fmt.Errorf("wait for load: %w", err)
	}
	page.Timeout(settleMax).WaitRequestIdle(settleQuiet, nil, nil, nil)()

	mu.Lock()
	defer mu.Unlock()
	if last.url == "" {
		return mainFrameResponse{}, errors.New("no document response received")
	}
	return last, nil
}

// proxyBlocked reports whether the egress proxy refused this response's
// destination. Only a plain-http 403 can come from the proxy (a refused
// CONNECT never yields a response), and the proxy strips the header from
// origin responses, so an origin cannot fake it.
func proxyBlocked(r *proto.NetworkResponse) bool {
	return r.Status == http.StatusForbidden && strings.HasPrefix(r.URL, "http:") &&
		hasHeader(r.Headers, netguard.BlockedHeader)
}

func hasHeader(headers proto.NetworkHeaders, name string) bool {
	for k := range headers {
		if strings.EqualFold(k, name) {
			return true
		}
	}
	return false
}

// NeedsRendering reports whether served HTML must run JavaScript to show its
// content. It does when the page has scripts and either
//   - under minVisibleText characters of visible body text, or
//   - an empty SPA mount node (id root, app, __next, __nuxt, ___gatsby or
//     svelte) and under maxShellText characters, i.e. a server-rendered
//     header/footer shell around a client-rendered app.
//
// Visible text excludes head, script, style, noscript and template content.
func NeedsRendering(body []byte) bool {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return false
	}
	var s htmlStats
	s.walk(doc, false)
	if s.scripts == 0 {
		return false
	}
	return s.textLen < minVisibleText || (s.emptyMount && s.textLen < maxShellText)
}

const (
	minVisibleText = 200
	maxShellText   = 1000
)

var spaMountIDs = map[string]bool{"root": true, "app": true, "__next": true, "__nuxt": true, "___gatsby": true, "svelte": true}

type htmlStats struct {
	textLen    int
	scripts    int
	emptyMount bool
}

func (s *htmlStats) walk(n *html.Node, hidden bool) {
	if n.Type == html.ElementNode {
		switch n.Data {
		case "script":
			s.scripts++
			return
		case "head", "style", "noscript", "template":
			hidden = true
		}
		if spaMountIDs[nodeAttr(n, "id")] && isEmpty(n) {
			s.emptyMount = true
		}
	}
	if n.Type == html.TextNode && !hidden {
		s.textLen += utf8.RuneCountInString(strings.Join(strings.Fields(n.Data), " "))
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		s.walk(c, hidden)
	}
}

func isEmpty(n *html.Node) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode || (c.Type == html.TextNode && strings.TrimSpace(c.Data) != "") {
			return false
		}
	}
	return true
}
