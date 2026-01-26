package server

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
)

// SubdomainProxyConfig configures the subdomain proxy middleware
type SubdomainProxyConfig struct {
	// DevSubdomain is the subdomain prefix for dev container proxying
	// e.g., "dev" means *.dev.helix.example.com
	DevSubdomain string

	// BaseDomain is the main domain without the dev subdomain
	// e.g., "helix.example.com"
	BaseDomain string

	// Enabled controls whether subdomain proxying is active
	Enabled bool
}

// SubdomainProxyMiddleware routes requests based on subdomain patterns
// Supports two URL formats:
//   - p{port}-{session_id}.{dev_subdomain}.{base_domain} → proxy to session:port
//   - {name}-{session_id}.{dev_subdomain}.{base_domain} → proxy to session's named port
//
// Examples:
//   - p8080-ses_abc123.dev.helix.example.com → session ses_abc123, port 8080
//   - api-ses_abc123.dev.helix.example.com → session ses_abc123, port named "api"
type SubdomainProxyMiddleware struct {
	config  *SubdomainProxyConfig
	handler http.Handler // The proxy handler
	next    http.Handler // Next handler in chain (regular API routes)

	// Regex patterns for subdomain parsing
	// Format: p{port}-{session_id}.dev.domain.com
	portPattern *regexp.Regexp
	// Format: {name}-{session_id}.dev.domain.com
	namePattern *regexp.Regexp
}

// NewSubdomainProxyMiddleware creates a new subdomain proxy middleware
func NewSubdomainProxyMiddleware(config *SubdomainProxyConfig, proxyHandler, next http.Handler) *SubdomainProxyMiddleware {
	m := &SubdomainProxyMiddleware{
		config:  config,
		handler: proxyHandler,
		next:    next,
	}

	// Pattern for port-based subdomain: p{port}-{session_id}
	// Matches: p8080-ses_abc123, p3000-ses_xyz789
	m.portPattern = regexp.MustCompile(`^p(\d+)-(.+)$`)

	// Pattern for name-based subdomain: {name}-{session_id}
	// Matches: api-ses_abc123, frontend-ses_xyz789
	m.namePattern = regexp.MustCompile(`^([a-z][a-z0-9]*)-(.+)$`)

	return m
}

// ServeHTTP implements http.Handler
func (m *SubdomainProxyMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !m.config.Enabled {
		m.next.ServeHTTP(w, r)
		return
	}

	// Parse the host to extract subdomain
	host := r.Host
	// Remove port if present
	if colonIdx := strings.LastIndex(host, ":"); colonIdx != -1 {
		// Check if it's not an IPv6 address
		if !strings.Contains(host, "]") || colonIdx > strings.LastIndex(host, "]") {
			host = host[:colonIdx]
		}
	}

	// Check if this is a dev subdomain request
	devSuffix := fmt.Sprintf(".%s.%s", m.config.DevSubdomain, m.config.BaseDomain)
	if !strings.HasSuffix(host, devSuffix) {
		// Not a dev subdomain, pass through
		m.next.ServeHTTP(w, r)
		return
	}

	// Extract the subdomain prefix (everything before .dev.domain.com)
	subdomain := strings.TrimSuffix(host, devSuffix)

	// Try to parse as port-based subdomain: p{port}-{session_id}
	if matches := m.portPattern.FindStringSubmatch(subdomain); len(matches) == 3 {
		portStr := matches[1]
		sessionID := matches[2]

		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			http.Error(w, fmt.Sprintf("invalid port in subdomain: %s", portStr), http.StatusBadRequest)
			return
		}

		log.Debug().
			Str("host", r.Host).
			Str("session_id", sessionID).
			Int("port", port).
			Str("path", r.URL.Path).
			Msg("Subdomain proxy: routing to session port")

		// Rewrite the request to go through the proxy endpoint
		// The proxy handler expects /api/v1/sessions/{id}/proxy/{port}/...
		proxyPath := fmt.Sprintf("/api/v1/sessions/%s/proxy/%d%s", sessionID, port, r.URL.Path)
		r.URL.Path = proxyPath
		r.URL.RawPath = "" // Clear encoded path

		m.handler.ServeHTTP(w, r)
		return
	}

	// Try to parse as name-based subdomain: {name}-{session_id}
	// This requires looking up the named port from the exposed ports registry
	if matches := m.namePattern.FindStringSubmatch(subdomain); len(matches) == 3 {
		name := matches[1]
		sessionID := matches[2]

		log.Debug().
			Str("host", r.Host).
			Str("session_id", sessionID).
			Str("name", name).
			Str("path", r.URL.Path).
			Msg("Subdomain proxy: routing to named port")

		// For named ports, we need to look up the port number
		// This is handled by the subdomain proxy handler which has access to the exposed port manager
		// Rewrite with a special marker
		proxyPath := fmt.Sprintf("/api/v1/sessions/%s/proxy-named/%s%s", sessionID, name, r.URL.Path)
		r.URL.Path = proxyPath
		r.URL.RawPath = ""

		m.handler.ServeHTTP(w, r)
		return
	}

	// Invalid subdomain format
	http.Error(w, fmt.Sprintf("invalid subdomain format: %s (expected p{port}-{session_id} or {name}-{session_id})", subdomain), http.StatusBadRequest)
}

// parseDevSubdomainConfig parses the DEV_SUBDOMAIN environment variable
// Format: "dev.helix.example.com" or just "dev" (uses SERVER_URL domain)
func parseDevSubdomainConfig(devSubdomainEnv, serverURL string) *SubdomainProxyConfig {
	config := &SubdomainProxyConfig{
		Enabled: false,
	}

	if devSubdomainEnv == "" {
		return config
	}

	// If it contains a dot, treat as full domain specification
	if strings.Contains(devSubdomainEnv, ".") {
		// Format: dev.helix.example.com
		parts := strings.SplitN(devSubdomainEnv, ".", 2)
		if len(parts) == 2 {
			config.DevSubdomain = parts[0]
			config.BaseDomain = parts[1]
			config.Enabled = true
		}
	} else {
		// Just the subdomain prefix, extract domain from SERVER_URL
		config.DevSubdomain = devSubdomainEnv

		// Parse SERVER_URL to get domain
		// e.g., https://helix.example.com → helix.example.com
		domain := serverURL
		domain = strings.TrimPrefix(domain, "https://")
		domain = strings.TrimPrefix(domain, "http://")
		// Remove port if present
		if colonIdx := strings.Index(domain, ":"); colonIdx != -1 {
			domain = domain[:colonIdx]
		}
		// Remove path if present
		if slashIdx := strings.Index(domain, "/"); slashIdx != -1 {
			domain = domain[:slashIdx]
		}

		if domain != "" && domain != "localhost" {
			config.BaseDomain = domain
			config.Enabled = true
		}
	}

	return config
}
