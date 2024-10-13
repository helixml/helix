package browser

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
)

type Browser struct {
	ctx context.Context
	cfg *config.ServerConfig

	// Used when launcher is setup
	pool rod.Pool[rod.Browser]
	// Used when launcher is not setup
	browser *rod.Browser

	pagePool rod.Pool[rod.Page]
}

func New(cfg *config.ServerConfig) (*Browser, error) {
	pool := rod.NewBrowserPool(1) // TODO: move to rod launcher
	pagePool := rod.NewPagePool(50)

	b := &Browser{
		ctx:      context.Background(),
		cfg:      cfg,
		pool:     pool,
		pagePool: pagePool,
	}

	browser, err := b.getBrowser()
	if err != nil {
		return nil, fmt.Errorf("error initializing browser: %w", err)
	}
	b.browser = browser

	return b, nil
}

func (b *Browser) GetBrowser() (*rod.Browser, error) {
	if b.cfg.RAG.Crawler.LauncherEnabled {
		return b.getFromPool()
	}

	return b.browser, nil
}

func (b *Browser) GetPage(opts proto.TargetCreateTarget) (*rod.Page, error) {

	create := func() (*rod.Page, error) {
		// page, err = m.Browser.Timeout(time.Duration(m.config.timeout) * time.Second).Page(opts)
		// incognito, err := b.browser.Incognito()
		// if err != nil {
		// 	return nil, err
		// }

		page, err := b.browser.Timeout(5 * time.Second).Page(proto.TargetCreateTarget{
			URL: "about:blank",
		})
		if err != nil {
			return nil, fmt.Errorf("error creating page: %w", err)
		}
		return page, nil
	}

	fmt.Println("Creating page")

	page, err := b.pagePool.Get(create)
	if err != nil {
		return nil, err
	}

	fmt.Println("Navigating to", opts.URL)

	err = page.Timeout(5 * time.Second).Navigate(opts.URL)
	if err != nil {
		return nil, fmt.Errorf("error navigating to %s: %w", opts.URL, err)
	}

	return page, nil
}

func (b *Browser) PutPage(page *rod.Page) {
	page = page.CancelTimeout()
	b.pagePool.Put(page)
}

func (b *Browser) getFromPool() (*rod.Browser, error) {
	browser, err := b.pool.Get(b.getBrowser)
	if err != nil {
		return nil, err
	}

	return browser, nil
}
func (b *Browser) getBrowser() (*rod.Browser, error) {
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

	// Use the resolved URL for the request
	req, err := http.NewRequest("GET", resolvedURL+"/json/version", nil)
	if err != nil {
		return "", fmt.Errorf("error creating request for Chrome URL (%s): %w", resolvedURL, err)
	}
	req.Header.Set("Host", parsedURL.Hostname()) // Set the original hostname in the Host header

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
