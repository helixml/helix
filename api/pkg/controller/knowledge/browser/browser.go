package browser

import (
	"context"
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
	defaultBrowserPoolSize = 5
	defaultPagePoolSize    = 50
)

type Browser struct {
	ctx context.Context
	cfg *config.ServerConfig

	// Used when launcher is setup
	pool     rod.Pool[rod.Browser]
	launcher *launcher.Launcher

	// Used when launcher is not setup
	browser *rod.Browser

	pagePool rod.Pool[rod.Page]
}

func New(cfg *config.ServerConfig) (*Browser, error) {
	// Skip initialization entirely if features are disabled
	ragCrawlerEnabled := os.Getenv("RAG_CRAWLER_ENABLED") == "true"
	browserPoolEnabled := os.Getenv("BROWSER_POOL_ENABLED") == "true"

	if !ragCrawlerEnabled || !browserPoolEnabled {
		return nil, fmt.Errorf("Browser features are disabled, skipping browser pool initialization")
	}

	// Early check to avoid initializing if Chrome URL is empty or it's our placeholder
	if !cfg.RAG.Crawler.LauncherEnabled {
		// Check if it's empty, invalid, or our placeholder
		if cfg.RAG.Crawler.ChromeURL == "" ||
			cfg.RAG.Crawler.ChromeURL == "localhost" ||
			cfg.RAG.Crawler.ChromeURL == "127.0.0.1" ||
			cfg.RAG.Crawler.ChromeURL == "http://chrome-placeholder-not-used" {
			return nil, fmt.Errorf("Chrome URL is empty, invalid, or features are disabled")
		}
	}

	browserPoolSize := cfg.RAG.Crawler.BrowserPoolSize
	pagePoolSize := cfg.RAG.Crawler.PagePoolSize

	if browserPoolSize <= 0 {
		browserPoolSize = defaultBrowserPoolSize
	}
	if pagePoolSize <= 0 {
		pagePoolSize = defaultPagePoolSize
	}

	pool := rod.NewBrowserPool(browserPoolSize)
	pagePool := rod.NewPagePool(pagePoolSize)

	b := &Browser{
		ctx:      context.Background(),
		cfg:      cfg,
		pool:     pool,
		pagePool: pagePool,
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

func (b *Browser) GetBrowser() (*rod.Browser, error) {
	if b.cfg.RAG.Crawler.LauncherEnabled {
		return b.getFromPool()
	}

	return b.browser, nil
}

func (b *Browser) getFromPool() (*rod.Browser, error) {
	browser, err := b.pool.Get(b.getBrowser)
	if err != nil {
		return nil, err
	}

	return browser, nil
}
func (b *Browser) getBrowser() (*rod.Browser, error) {
	if b.launcher != nil {
		client, err := b.launcher.Client()
		if err != nil {
			return nil, fmt.Errorf("error getting launcher client: %w", err)
		}

		// Setup browser with the client
		browser := rod.New().Client(client)

		// Connect to the browser
		err = browser.Connect()
		if err != nil {
			return nil, fmt.Errorf("error connecting to browser: %w", err)
		}
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

	b.pool.Put(browser)
	b.pool.Cleanup(func(browser *rod.Browser) { browser.MustClose() })
	return nil
}

func (b *Browser) getChromeURL() (string, error) {
	chromeURL := b.cfg.RAG.Crawler.ChromeURL

	if chromeURL == "" {
		return "", fmt.Errorf("Chrome URL is empty, please set RAG_CRAWLER_CHROME_URL environment variable")
	}

	// Check for localhost or 127.0.0.1 before trying to resolve
	parsedURL, err := url.Parse(chromeURL)
	if err != nil {
		return "", fmt.Errorf("error parsing Chrome URL (%s): %w", chromeURL, err)
	}

	hostname := parsedURL.Hostname()
	if hostname == "localhost" || hostname == "127.0.0.1" {
		return chromeURL, nil // No need to resolve localhost
	}

	// Resolve the hostname to an IP address. This is required for the browser to connect,
	// as if you try to connect with hostname/domain then chrome will reject the connection
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return "", fmt.Errorf("error resolving Chrome URL (%s) to IP: %w", chromeURL, err)
	}

	if len(ips) == 0 {
		return "", fmt.Errorf("no IP addresses found for Chrome URL (%s)", chromeURL)
	}

	// Use the first IP address
	ip := ips[0].String()

	// Replace the hostname with the IP address in the original URL
	resolvedURL := strings.Replace(chromeURL, hostname, ip, 1)

	// Use the resolved URL for the request
	req, err := http.NewRequest("GET", resolvedURL+"/json/version", nil)
	if err != nil {
		return "", fmt.Errorf("error creating request for Chrome URL (%s): %w", resolvedURL, err)
	}
	req.Header.Set("Host", hostname) // Set the original hostname in the Host header

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error checking Chrome URL (%s): %w", resolvedURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bts, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("error reading Chrome URL (%s) response: %w", resolvedURL, err)
		}
		return "", fmt.Errorf("error checking Chrome URL (%s): %s", resolvedURL, string(bts))
	}

	u, err := launcher.ResolveURL(resolvedURL)
	if err != nil {
		return "", fmt.Errorf("error resolving Chrome URL (%s): %w", resolvedURL, err)
	}

	return u, nil
}
