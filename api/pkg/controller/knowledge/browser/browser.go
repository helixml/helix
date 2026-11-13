package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
)

const (
	defaultBrowserPoolSize = 3
	defaultPagePoolSize    = 50
)

const webRTCIPHandlingPolicy flags.Flag = "force-webrtc-ip-handling-policy"

type Browser struct {
	ctx context.Context
	cfg *config.ServerConfig

	// Used when launcher is setup
	browserPool rod.Pool[rod.Browser]
	launcher    *launcher.Launcher

	// Used when launcher is not setup
	browser *rod.Browser

	pagePool rod.Pool[rod.Page]
}

func New(cfg *config.ServerConfig) (*Browser, error) {
	browserPoolSize := cfg.RAG.Crawler.BrowserPoolSize
	pagePoolSize := cfg.RAG.Crawler.PagePoolSize

	if browserPoolSize <= 0 {
		browserPoolSize = defaultBrowserPoolSize
	}
	if pagePoolSize <= 0 {
		pagePoolSize = defaultPagePoolSize
	}

	browserPool := rod.NewBrowserPool(browserPoolSize)
	pagePool := rod.NewPagePool(pagePoolSize)

	b := &Browser{
		ctx:         context.Background(),
		cfg:         cfg,
		browserPool: browserPool,
		pagePool:    pagePool,
	}

	// If launcher is not enabled (using chrome directly, connect to existing browser)
	if cfg.RAG.Crawler.LauncherEnabled {
		// Using a rod manager (https://github.com/go-rod/rod/blob/main/lib/examples/launch-managed/main.go)
		l, err := launcher.NewManaged(cfg.RAG.Crawler.LauncherURL)
		if err != nil {
			return nil, fmt.Errorf("error initializing launcher: %w", err)
		}
		// WebRTC can send UDP around the egress proxy; keep it on the proxy.
		l.Set(webRTCIPHandlingPolicy, "disable_non_proxied_udp")
		b.launcher = l

	} else {
		browser, err := b.getBrowser()
		if err != nil {
			return nil, fmt.Errorf("error initializing browser: %w", err)
		}
		b.browser = browser
	}

	return b, nil
}

// Close closes the browser and all associated pages
func (b *Browser) Close() {
	// Clean up page pool first
	b.pagePool.Cleanup(func(page *rod.Page) { page.MustClose() })
	// Then clean up browser pool
	b.browserPool.Cleanup(func(browser *rod.Browser) { browser.MustClose() })
	// If we have a standalone browser (not from pool), close it
	if b.browser != nil {
		b.browser.MustClose()
	}
}

// GetBrowser returns a browser, waiting for a free pool slot until ctx is
// done. Pass it back with PutBrowser.
func (b *Browser) GetBrowser(ctx context.Context) (*rod.Browser, error) {
	if b.cfg.RAG.Crawler.LauncherEnabled {
		return b.getFromPool(ctx)
	}

	return b.browser, nil
}

func (b *Browser) getFromPool(ctx context.Context) (*rod.Browser, error) {
	var browser *rod.Browser
	select {
	case browser = <-b.browserPool:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if browser != nil {
		return browser, nil
	}
	browser, err := b.getBrowser()
	if err != nil {
		// Return the empty slot, or every failed connect shrinks the pool.
		b.browserPool.Put(nil)
		return nil, err
	}
	return browser, nil
}
func (b *Browser) getBrowser() (*rod.Browser, error) {
	if b.launcher != nil {
		log.Info().Msg("Getting client from launcher")
		client, err := b.launcher.Client()
		if err != nil {
			return nil, fmt.Errorf("error getting launcher client: %w", err)
		}

		log.Info().Msg("Creating new browser")

		// Setup browser with the client
		browser := rod.New().Trace(true).Client(client)

		log.Info().Msg("Connecting to browser")
		// Connect to the browser
		err = browser.Connect()
		if err != nil {
			return nil, fmt.Errorf("error connecting to browser: %w", err)
		}
		log.Info().Msg("Browser connected")
		return browser, nil
	}

	chromeURL, err := b.getChromeURL()
	if err != nil {
		return nil, err
	}
	log.Info().Str("chromeURL", chromeURL).Msg("Creating browser")
	browser := rod.New().ControlURL(chromeURL)
	err = browser.Connect()
	if err != nil {
		return nil, err
	}

	return browser, nil
}

func (b *Browser) GetPage(browser *rod.Browser, opts proto.TargetCreateTarget) (*rod.Page, error) {
	create := func() (*rod.Page, error) {
		// Open a blank page on first create
		page, err := browser.Page(proto.TargetCreateTarget{
			URL: "about:blank",
		})
		if err != nil {
			return nil, fmt.Errorf("error creating page: %w", err)
		}
		return page, nil
	}

	page, err := b.pagePool.Get(create)
	if err != nil {
		return nil, fmt.Errorf("error getting page: %w", err)
	}

	err = page.Navigate(opts.URL)
	if err != nil {
		return nil, fmt.Errorf("error navigating to %s: %w", opts.URL, err)
	}

	return page, nil
}

func (b *Browser) PutPage(page *rod.Page) {
	if page == nil {
		return
	}

	b.pagePool.Put(page)
}

func (b *Browser) PutBrowser(browser *rod.Browser) error {
	// If launcher is disabled, it's no-op
	if !b.cfg.RAG.Crawler.LauncherEnabled {
		return nil
	}

	b.browserPool.Put(browser)
	// b.browserPool.Cleanup(func(browser *rod.Browser) { browser.MustClose() })
	return nil
}

// DiscardBrowser releases a pooled browser whose connection has failed, so
// the pool reconnects instead of handing it out again. The single browser
// used without the launcher is shared and is left alone.
func (b *Browser) DiscardBrowser(browser *rod.Browser) {
	if !b.cfg.RAG.Crawler.LauncherEnabled {
		return
	}
	_ = browser.Timeout(5 * time.Second).Close()
	b.browserPool.Put(nil)
}

// noProxyClient is a shared HTTP client that bypasses any configured proxy.
// Used for internal service communication (e.g., Chrome browser) where
// requests should never go through an external HTTP proxy.
var noProxyClient = &http.Client{
	Transport: &http.Transport{
		Proxy: nil, // explicitly bypass proxy
	},
}

// chromeVersionResponse is the JSON structure returned by Chrome's /json/version endpoint.
type chromeVersionResponse struct {
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

func (b *Browser) getChromeURL() (string, error) {
	chromeURL := b.cfg.RAG.Crawler.ChromeURL

	// Parse the URL to extract the hostname
	parsedURL, err := url.Parse(chromeURL)
	if err != nil {
		return "", fmt.Errorf("error parsing Chrome URL (%s): %w", chromeURL, err)
	}

	// Resolve the hostname to an IP address. This is required for the browser to connect,
	// as if you try to connect with hostname/domain then chrome will reject the connection
	ips, err := net.LookupIP(parsedURL.Hostname())
	if err != nil {
		return "", fmt.Errorf("error resolving Chrome URL (%s) to IP: %w", chromeURL, err)
	}

	if len(ips) == 0 {
		return "", fmt.Errorf("no IP addresses found for Chrome URL (%s)", chromeURL)
	}

	// Use the first IP address
	ip := ips[0].String()

	// Replace the hostname with the IP address in the original URL
	resolvedURL := strings.Replace(chromeURL, parsedURL.Hostname(), ip, 1)

	// Use a no-proxy HTTP client for internal Chrome service communication.
	// This also replaces the previous launcher.ResolveURL() call which used
	// http.Get() (affected by HTTP_PROXY) and made a redundant second request
	// to the same /json/version endpoint.
	versionURL, err := url.JoinPath(resolvedURL, "/json/version")
	if err != nil {
		return "", fmt.Errorf("error building version URL for Chrome (%s): %w", resolvedURL, err)
	}
	req, err := http.NewRequest("GET", versionURL, nil)
	if err != nil {
		return "", fmt.Errorf("error creating request for Chrome URL (%s): %w", resolvedURL, err)
	}
	req.Header.Set("Host", parsedURL.Hostname()) // Set the original hostname in the Host header

	resp, err := noProxyClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error checking Chrome URL (%s): %w", resolvedURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading Chrome URL (%s) response: %w", resolvedURL, err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error checking Chrome URL (%s): %s", resolvedURL, string(body))
	}

	// Extract the WebSocket debugger URL from the /json/version response
	var versionInfo chromeVersionResponse
	if err := json.Unmarshal(body, &versionInfo); err != nil {
		return "", fmt.Errorf("error parsing Chrome version response from %s: %w", resolvedURL, err)
	}

	if versionInfo.WebSocketDebuggerURL == "" {
		return "", fmt.Errorf("Chrome at %s returned empty webSocketDebuggerUrl", resolvedURL)
	}

	// Replace the host in the WebSocket URL with our resolved host
	wsURL, err := url.Parse(versionInfo.WebSocketDebuggerURL)
	if err != nil {
		return "", fmt.Errorf("error parsing webSocketDebuggerUrl (%s): %w", versionInfo.WebSocketDebuggerURL, err)
	}

	resolvedParsed, err := url.Parse(resolvedURL)
	if err != nil {
		return "", fmt.Errorf("error parsing resolved URL (%s): %w", resolvedURL, err)
	}
	wsURL.Host = resolvedParsed.Host

	return wsURL.String(), nil
}

// GuardPage makes client carry every HTTP(S) request page issues —
// navigations, redirects, script-initiated navigations, subresources,
// fetch/XHR, and requests from its dedicated workers and out-of-process
// iframes — so the client's transport (an outbound policy) decides what the
// browser can reach and its own DNS resolution is never used. Requests the
// client refuses fail in the page. WebSocket connections, which request
// interception does not see, are blocked outright, and service workers are
// held paused. Not covered: connection-only hints (<link rel=preconnect>,
// dns-prefetch), which can open a connection or resolve a name but return
// nothing to the page. Call the returned function before returning the page
// to the pool.
func GuardPage(page *rod.Page, client *http.Client) (func(), error) {
	if err := (proto.NetworkEnable{}).Call(page); err != nil {
		return nil, fmt.Errorf("enable network domain: %w", err)
	}
	if err := page.SetBlockedURLs([]string{"ws://*", "wss://*"}); err != nil {
		return nil, fmt.Errorf("block websocket URLs: %w", err)
	}
	// Service and shared workers run as their own targets, outside this page's request
	// interception (dedicated workers and out-of-process iframes are
	// covered by it). Pause every service worker the browser starts and
	// never resume it, and keep the page's requests from being answered by
	// one. The browsers used here only render guarded pages.
	if err := (proto.TargetSetAutoAttach{
		AutoAttach:             true,
		WaitForDebuggerOnStart: true,
		Flatten:                true,
		Filter:                 proto.TargetTargetFilter{{Type: "service_worker"}, {Type: "shared_worker"}, {Exclude: true}},
	}).Call(page.Browser()); err != nil {
		return nil, fmt.Errorf("hold service workers: %w", err)
	}
	if err := (proto.NetworkSetBypassServiceWorker{Bypass: true}).Call(page); err != nil {
		return nil, fmt.Errorf("bypass service workers: %w", err)
	}
	router := page.HijackRequests()
	err := router.Add("*", "", func(h *rod.Hijack) {
		if err := h.LoadResponse(client, true); err != nil {
			log.Debug().Err(err).Str("url", h.Request.URL().String()).Msg("browser request refused")
			h.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
		}
	})
	if err != nil {
		_ = router.Stop()
		return nil, fmt.Errorf("intercept page requests: %w", err)
	}
	go router.Run()
	return func() {
		_ = page.Navigate("about:blank") // leave nothing running unguarded
		_ = router.Stop()
	}, nil
}
