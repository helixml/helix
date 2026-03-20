package config

import (
	"net/url"
	"os"
	"strings"

	"github.com/rs/zerolog/log"
)

// InternalServiceURLs returns URLs for services that Helix deploys itself
// (via Helm chart / docker-compose) and should always bypass an HTTP proxy.
// Customer-configured external services (OIDC, VLLM, dynamic providers) are
// intentionally excluded — those may be external and need the proxy.
// Customers should add those to NO_PROXY manually if they are internal.
func (c *ServerConfig) InternalServiceURLs() []string {
	return []string{
		// Browser / crawler
		c.RAG.Crawler.ChromeURL,
		c.RAG.Crawler.LauncherURL,
		// RAG backends
		c.RAG.Typesense.URL,
		c.RAG.Llamaindex.RAGIndexingURL,
		c.RAG.Llamaindex.RAGQueryURL,
		c.RAG.Llamaindex.RAGDeleteURL,
		c.RAG.Haystack.URL,
		// Text extraction
		c.TextExtractor.Unstructured.URL,
		c.TextExtractor.Tika.URL,
		// Search
		c.Search.SearXNGBaseURL,
		// Frontend
		c.WebServer.FrontendURL,
		// Kodit (git server is always the local API)
		c.Kodit.GitURL,
	}
}

// EnsureNoProxyForInternalHosts adds hostnames from all configured internal
// service URLs to the NO_PROXY environment variable so that service-to-service
// connections bypass any configured HTTP proxy. This must be called early in
// startup, before any HTTP requests are made, so that Go's proxy configuration
// includes these hosts.
func (c *ServerConfig) EnsureNoProxyForInternalHosts() {
	// Only relevant if a proxy is configured
	if os.Getenv("HTTP_PROXY") == "" && os.Getenv("http_proxy") == "" &&
		os.Getenv("HTTPS_PROXY") == "" && os.Getenv("https_proxy") == "" {
		return
	}

	seen := make(map[string]bool)
	var bypassHosts []string
	for _, rawURL := range c.InternalServiceURLs() {
		if rawURL == "" {
			continue
		}
		// Skip filesystem paths (e.g. FRONTEND_URL=/www in production)
		if !strings.Contains(rawURL, "://") {
			continue
		}
		parsed, err := url.Parse(rawURL)
		if err != nil {
			log.Warn().Err(err).Str("url", rawURL).Msg("Failed to parse internal service URL for NO_PROXY configuration")
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
		Msg("Added internal service hosts to NO_PROXY to bypass HTTP proxy")
}
