// Package outbound validates and performs server-side HTTP requests to
// caller-supplied URLs (self-hosted git providers, provider endpoints, …).
//
// A Policy blocks loopback, private (RFC1918/ULA), CGNAT, link-local
// (including cloud metadata), multicast and unspecified destinations unless
// the operator explicitly allows a CIDR. Enforcement happens at dial time on
// the resolved IP, so it also covers DNS rebinding, numeric IP encodings and
// redirects.
package outbound

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrDestinationNotAllowed is returned (wrapped) whenever a URL or resolved
// address is rejected by the policy.
var ErrDestinationNotAllowed = errors.New("outbound destination not allowed")

const maxRedirects = 5

var blockedNets = mustParseCIDRs(
	"0.0.0.0/8",     // "this network"
	"100.64.0.0/10", // CGNAT / shared address space
	"192.0.0.0/24",  // IETF protocol assignments
	"198.18.0.0/15", // benchmarking
	"240.0.0.0/4",   // reserved
	"64:ff9b::/96",  // NAT64 can map to internal IPv4
)

// Resolver resolves host names. *net.Resolver satisfies it.
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type Policy struct {
	// AllowedNets are operator-approved ranges that are otherwise blocked
	// (e.g. the subnet of an on-prem GitLab).
	AllowedNets []*net.IPNet
	Resolver    Resolver
}

// NewPolicy builds a policy from operator-configured CIDRs.
func NewPolicy(allowedCIDRs []string) (*Policy, error) {
	p := &Policy{Resolver: net.DefaultResolver}
	for _, raw := range allowedCIDRs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		_, n, err := net.ParseCIDR(raw)
		if err != nil {
			return nil, fmt.Errorf("parse allowed outbound CIDR %q: %w", raw, err)
		}
		p.AllowedNets = append(p.AllowedNets, n)
	}
	return p, nil
}

// IPAllowed reports whether a connection to ip is permitted.
func (p *Policy) IPAllowed(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	for _, n := range p.AllowedNets {
		if n.Contains(ip) {
			return true
		}
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	for _, n := range blockedNets {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

// ValidateURL checks the URL shape and that every address the host resolves
// to is allowed. It is a fail-fast pre-flight; NewHTTPClient re-checks at dial
// time so a rebinding resolver cannot bypass it.
func (p *Policy) ValidateURL(ctx context.Context, rawURL string) error {
	u, err := parseURL(rawURL)
	if err != nil {
		return err
	}
	_, err = p.resolveAllowed(ctx, u.Hostname())
	return err
}

// NewHTTPClient returns a client that only dials allowed addresses, ignores
// proxy environment variables (a proxy would dial on our behalf, bypassing
// the check), and only follows redirects that stay on the original host.
func (p *Policy) NewHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: timeout,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := p.resolveAllowed(ctx, host)
			if err != nil {
				return nil, err
			}
			// Dial the IP we checked so the stack cannot re-resolve to a different one.
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("%w: too many redirects", ErrDestinationNotAllowed)
			}
			orig := via[0].URL
			if req.URL.Scheme != orig.Scheme || !strings.EqualFold(req.URL.Host, orig.Host) {
				return fmt.Errorf("%w: redirect to a different origin", ErrDestinationNotAllowed)
			}
			return nil
		},
	}
}

func (p *Policy) resolveAllowed(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if !p.IPAllowed(ip) {
			return nil, fmt.Errorf("%w: address %s is not allowed", ErrDestinationNotAllowed, ip)
		}
		return []net.IP{ip}, nil
	}
	addrs, err := p.Resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve %s: %v", ErrDestinationNotAllowed, host, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("%w: %s did not resolve", ErrDestinationNotAllowed, host)
	}
	ips := make([]net.IP, 0, len(addrs))
	// Reject if ANY address is blocked rather than picking a safe one.
	for _, a := range addrs {
		if !p.IPAllowed(a.IP) {
			return nil, fmt.Errorf("%w: %s resolves to %s", ErrDestinationNotAllowed, host, a.IP)
		}
		ips = append(ips, a.IP)
	}
	return ips, nil
}

func parseURL(rawURL string) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid URL", ErrDestinationNotAllowed)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("%w: scheme %q not allowed", ErrDestinationNotAllowed, u.Scheme)
	}
	if u.User != nil {
		return nil, fmt.Errorf("%w: URL must not contain credentials", ErrDestinationNotAllowed)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("%w: URL must include a host", ErrDestinationNotAllowed)
	}
	return u, nil
}

func mustParseCIDRs(cidrs ...string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(err)
		}
		out = append(out, n)
	}
	return out
}
