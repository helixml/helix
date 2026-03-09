package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
)

const (
	defaultBrowserPoolSize = 3
	defaultPagePoolSize    = 50
)

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

// EnsureNoProxyForBrowserHosts adds the Chrome and launcher service hostnames
// to the NO_PROXY environment variable so that internal service connections
// bypass any configured HTTP proxy. This must be called before any HTTP requests
// are made (including by third-party libraries like go-rod) to ensure Go's
// cached proxy configuration includes these hosts.
func EnsureNoProxyForBrowserHosts(cfg *config.ServerConfig) {
	// Only relevant if a proxy is configured
	if os.Getenv("HTTP_PROXY") == "" && os.Getenv("http_proxy") == "" &&
		os.Getenv("HTTPS_PROXY") == "" && os.Getenv("https_proxy") == "" {
		return
	}

	seen := make(map[string]bool)
	var bypassHosts []string
	for _, rawURL := range []string{cfg.RAG.Crawler.ChromeURL, cfg.RAG.Crawler.LauncherURL} {
		if rawURL == "" {
			continue
		}
		parsed, err := url.Parse(rawURL)
		if err != nil {
			log.Warn().Err(err).Str("url", rawURL).Msg("Failed to parse browser service URL for NO_PROXY configuration")
			continue
		}
		host := strings.ToLower(parsed.Hostname())
		if host != "" && !seen[host] {
			seen[host] = true
			bypassHosts = append(bypassHosts, host)
		}
	}

	if len(bypassHosts) == 0 {
		return
	}

	// Read existing NO_PROXY value (check both cases)
	noProxy := os.Getenv("NO_PROXY")
	if noProxy == "" {
		noProxy = os.Getenv("no_proxy")
	}

	var toAdd []string
	for _, host := range bypassHosts {
		found := false
		for _, existing := range strings.Split(strings.ToLower(noProxy), ",") {
			if strings.TrimSpace(existing) == host {
				found = true
				break
			}
		}
		if !found {
			toAdd = append(toAdd, host)
		}
	}

	if len(toAdd) == 0 {
		return
	}

	newNoProxy := noProxy
	if newNoProxy != "" {
		newNoProxy += ","
	}
	newNoProxy += strings.Join(toAdd, ",")

	// Set both cases so Go's httpproxy.FromEnvironment() picks it up
	os.Setenv("NO_PROXY", newNoProxy)
	os.Setenv("no_proxy", newNoProxy)

	log.Info().
		Strs("hosts", toAdd).
		Str("NO_PROXY", newNoProxy).
		Msg("Added browser service hosts to NO_PROXY to bypass HTTP proxy")
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

func (b *Browser) GetBrowser() (*rod.Browser, error) {
	if b.cfg.RAG.Crawler.LauncherEnabled {
		return b.getFromPool()
	}

	return b.browser, nil
}

func (b *Browser) getFromPool() (*rod.Browser, error) {
	log.Info().Msg("Getting browser from pool")
	browser, err := b.browserPool.Get(b.getBrowser)
	if err != nil {
		return nil, err
	}

	log.Info().Msg("Browser from pool")

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

// noProxyHTTPClient returns an HTTP client that bypasses any configured proxy.
// This is used for internal service communication (e.g., Chrome browser) where
// requests should never go through an external HTTP proxy.
func noProxyHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: nil, // explicitly bypass proxy
		},
	}
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
	req, err := http.NewRequest("GET", resolvedURL+"/json/version", nil)
	if err != nil {
		return "", fmt.Errorf("error creating request for Chrome URL (%s): %w", resolvedURL, err)
	}
	req.Header.Set("Host", parsedURL.Hostname()) // Set the original hostname in the Host header

	client := noProxyHTTPClient()
	resp, err := client.Do(req)
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
