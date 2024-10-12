package browser

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
)

type Browser struct {
	cfg  *config.ServerConfig
	pool rod.Pool[rod.Browser]
}

func New(cfg *config.ServerConfig) *Browser {
	pool := rod.NewBrowserPool(1) // TODO: move to rod launcher

	pool.Cleanup(func(p *rod.Browser) {
		p.MustClose()
	})

	return &Browser{
		cfg:  cfg,
		pool: pool,
	}
}

func (b *Browser) Get(k *types.Knowledge) (*rod.Browser, error) {
	chromeURL, err := b.getChromeURL(k)
	if err != nil {
		return nil, err
	}

	browser, err := b.pool.Get(func() (*rod.Browser, error) {
		return rod.New().ControlURL(chromeURL), nil
	})
	if err != nil {
		return nil, err
	}

	return browser, nil
}

func (b *Browser) Put(browser *rod.Browser) error {
	b.pool.Put(browser)
	return nil
}

func (b *Browser) getChromeURL(k *types.Knowledge) (string, error) {
	chromeURL := k.Source.Web.Crawler.ChromeURL

	if chromeURL == "" {
		chromeURL = b.cfg.RAG.Crawler.ChromeURL
	}

	// Parse the URL to extract the hostname
	parsedURL, err := url.Parse(chromeURL)
	if err != nil {
		return "", fmt.Errorf("error parsing Chrome URL (%s): %w", chromeURL, err)
	}

	switch parsedURL.Hostname() {
	case "localhost", "127.0.0.1":
		return chromeURL, nil
	default:
		// Resolve
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
