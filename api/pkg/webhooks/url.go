package webhooks

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/netguard"
)

// endpointPolicy is the outbound policy for webhook endpoints. allowPrivate
// (WEBHOOKS_ALLOW_PRIVATE_ENDPOINTS) admits loopback, RFC1918, unique-local
// and CGNAT addresses; link-local, cloud metadata and other reserved ranges
// stay blocked either way.
func endpointPolicy(allowPrivate bool) netguard.Policy {
	return netguard.Policy{AllowPrivate: allowPrivate}
}

// ValidateEndpointURL checks a webhook endpoint URL when it is saved. Hosts
// that are IP literals or localhost names are checked against the outbound
// policy here; every address a hostname resolves to is checked again when
// a delivery dials it.
func ValidateEndpointURL(rawURL string, allowPrivate bool) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse webhook URL: %w", err)
	}
	if u.User != nil {
		return errors.New("webhook URL must not contain credentials")
	}
	if u.Hostname() == "" {
		return errors.New("webhook URL must include a host")
	}
	if u.Scheme != "https" && !(allowPrivate && u.Scheme == "http") {
		return errors.New("webhook URL must use HTTPS")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if !allowPrivate && (host == "localhost" || strings.HasSuffix(host, ".localhost")) {
		return errors.New("webhook URL must not target localhost")
	}
	if ip := net.ParseIP(host); ip != nil {
		if err := endpointPolicy(allowPrivate).CheckIP(ip); err != nil {
			return errors.New("webhook URL must target a public IP address")
		}
	}
	return nil
}

// NewHTTPClient returns the client used for webhook deliveries. Every
// connection goes through the outbound policy and redirects are not followed.
// Deliveries never use an environment proxy.
func NewHTTPClient(timeout time.Duration, allowPrivate bool) *http.Client {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	transport := &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          50,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: timeout,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	}
	endpointPolicy(allowPrivate).GuardTransport(transport)
	return &http.Client{
		Transport:     transport,
		Timeout:       timeout,
		CheckRedirect: netguard.NoRedirects,
	}
}
