// Package netguard restricts where server-side outbound HTTP requests built
// from user-supplied URLs may connect. It is the shared defence against
// server-side request forgery (SSRF): validate the URL up front for a fast,
// clear error, and enforce the same policy at dial time so DNS rebinding,
// redirects and exotic IP literals cannot bypass it.
package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	dialTimeout   = 30 * time.Second
	dialKeepAlive = 30 * time.Second
)

// ErrBlockedDestination is returned (wrapped) whenever a URL or dial targets
// an address the policy forbids.
var ErrBlockedDestination = errors.New("destination not allowed")

// ErrPrivateNetwork additionally marks a rejection that Policy.AllowPrivate
// would have permitted (loopback, RFC1918, unique-local, CGNAT).
var ErrPrivateNetwork = errors.New("private-network destination")

// Policy decides which destinations an outbound request may reach.
type Policy struct {
	// AllowPrivate permits loopback, RFC1918, IPv6 unique-local and CGNAT
	// destinations, for self-hosted deployments whose model servers run on
	// the host or internal network. Link-local (including cloud metadata),
	// unspecified, multicast and broadcast addresses stay blocked regardless.
	AllowPrivate bool
}

var (
	alwaysBlocked = mustCIDRs(
		"0.0.0.0/8",         // "this network"; 0.0.0.0 reaches loopback on Linux
		"169.254.0.0/16",    // IPv4 link-local, cloud instance metadata
		"192.0.0.0/24",      // IETF protocol assignments
		"224.0.0.0/4",       // multicast
		"240.0.0.0/4",       // reserved, includes 255.255.255.255
		"::/128",            // unspecified
		"fe80::/10",         // IPv6 link-local
		"fec0::/10",         // deprecated site-local
		"ff00::/8",          // IPv6 multicast
		"fd00:ec2::254/128", // AWS IMDS over IPv6
		"100::/64",          // discard-only
		"2001:db8::/32",     // documentation
		"198.18.0.0/15",     // benchmarking
		"192.0.2.0/24",      // TEST-NET-1
		"198.51.100.0/24",   // TEST-NET-2
		"203.0.113.0/24",    // TEST-NET-3
	)
	privateRanges = mustCIDRs(
		"127.0.0.0/8",    // loopback
		"10.0.0.0/8",     // RFC1918
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"100.64.0.0/10",  // CGNAT, used by some cloud VPCs and tailnets
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 unique-local
	)
	// Prefixes that embed an IPv4 address in the low 32 bits. The embedded
	// address is checked too, so ::ffff:169.254.169.254 or a NAT64-mapped
	// metadata address is treated like the IPv4 address it reaches.
	nat64 = mustCIDRs("64:ff9b::/96", "64:ff9b:1::/48")
)

func mustCIDRs(cidrs ...string) []*net.IPNet {
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

func inAny(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// CheckIP returns an error wrapping ErrBlockedDestination if ip may not be
// dialled under this policy.
func (p Policy) CheckIP(ip net.IP) error {
	if err := p.classify(ip); err != nil {
		return fmt.Errorf("%w %s", err, ip)
	}
	return nil
}

// blockedError is a rejection that matches ErrBlockedDestination and, for
// private-network rejections, ErrPrivateNetwork.
type blockedError struct {
	class   string
	private bool
}

func (e *blockedError) Error() string { return ErrBlockedDestination.Error() + ": " + e.class }

func (e *blockedError) Is(target error) bool {
	return target == ErrBlockedDestination || (e.private && target == ErrPrivateNetwork)
}

var (
	errReserved = &blockedError{class: "reserved or link-local address"}
	errPrivate  = &blockedError{class: "loopback or private-network address", private: true}
)

// classify returns errReserved, errPrivate or nil. The errors deliberately
// carry no address, so callers resolving a hostname can report the class
// without revealing what an internal name resolves to.
func (p Policy) classify(ip net.IP) error {
	if ip == nil {
		return errReserved
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	} else if inAny(ip, nat64) {
		if err := p.classify(net.IP(ip[12:16])); err != nil {
			return err
		}
	}
	if inAny(ip, alwaysBlocked) {
		return errReserved
	}
	if !p.AllowPrivate && inAny(ip, privateRanges) {
		return errPrivate
	}
	return nil
}

// ValidateHTTPURL checks that raw is an absolute http or https URL with a
// host. It rejects file://, gopher:// and similar schemes regardless of policy.
func ValidateHTTPURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("%w: URL scheme %q (only http and https are allowed)", ErrBlockedDestination, u.Scheme)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("invalid URL: missing host")
	}
	return u, nil
}

// ValidateURL is the up-front check for a user-supplied URL: it must pass
// ValidateHTTPURL, and if the host is an IP literal, localhost, or resolves
// locally, every address must pass CheckIP. A host that does not resolve
// locally is not rejected here (deployments may resolve only via an egress
// proxy); GuardTransport still enforces the policy when it is dialled.
func (p Policy) ValidateURL(ctx context.Context, raw string) error {
	u, err := ValidateHTTPURL(raw)
	if err != nil {
		return err
	}
	if _, err := p.resolve(ctx, u.Hostname()); errors.Is(err, ErrBlockedDestination) {
		return err
	}
	return nil
}

// resolve looks host up and checks every address. A hostname with any
// blocked address is rejected outright rather than picking a "good" one.
func (p Policy) resolve(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if err := p.CheckIP(ip); err != nil {
			return nil, err
		}
		return []net.IP{ip}, nil
	}
	if !p.AllowPrivate && isLocalhostName(host) {
		return nil, fmt.Errorf("%w (host %s)", errPrivate, host)
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", host, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("resolve %s: no addresses", host)
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		if err := p.classify(a.IP); err != nil {
			return nil, fmt.Errorf("%w (host %s)", err, host)
		}
		ips = append(ips, a.IP)
	}
	return ips, nil
}

func isLocalhostName(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	return h == "localhost" || strings.HasSuffix(h, ".localhost")
}

// GuardTransport enforces the policy on every connection t makes.
//
// Direct connections: the target host is resolved once, every address is
// checked, and the connection is made only to those checked addresses so the
// HTTP stack cannot re-resolve to something else (DNS rebinding).
//
// Proxied connections (HTTP(S)_PROXY from the operator's environment): the
// proxy address itself comes from operator configuration and is dialled
// as-is, but the request destination is still checked before the proxy is
// used. If the destination does not resolve locally (proxy-only DNS), the
// request is passed to the proxy, which is then the egress control point.
func (p Policy) GuardTransport(t *http.Transport) {
	var proxyAddrs sync.Map // host:port of operator-configured proxies
	if origProxy := t.Proxy; origProxy != nil {
		t.Proxy = func(req *http.Request) (*url.URL, error) {
			proxyURL, err := origProxy(req)
			if err != nil || proxyURL == nil {
				return proxyURL, err
			}
			if err := p.checkProxiedDestination(req.Context(), req.URL.Hostname()); err != nil {
				return nil, err
			}
			proxyAddrs.Store(canonicalProxyAddr(proxyURL), struct{}{})
			return proxyURL, nil
		}
	}

	dialer := &net.Dialer{Timeout: dialTimeout, KeepAlive: dialKeepAlive}
	baseDial := t.DialContext
	if baseDial == nil {
		baseDial = dialer.DialContext
	}
	t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if _, ok := proxyAddrs.Load(addr); ok {
			return baseDial(ctx, network, addr)
		}
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := p.resolve(ctx, host)
		if err != nil {
			return nil, err
		}
		var lastErr error
		for _, ip := range ips {
			conn, err := baseDial(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, lastErr
	}
}

func (p Policy) checkProxiedDestination(ctx context.Context, host string) error {
	if _, err := p.resolve(ctx, host); errors.Is(err, ErrBlockedDestination) {
		return err
	}
	return nil
}

func canonicalProxyAddr(u *url.URL) string {
	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "https":
			port = "443"
		case "socks5", "socks5h":
			port = "1080"
		default:
			port = "80"
		}
	}
	return net.JoinHostPort(u.Hostname(), port)
}

// NoRedirects is an http.Client.CheckRedirect that returns the redirect
// response instead of following it.
func NoRedirects(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

// NewClient returns an http.Client whose transport is a clone of
// http.DefaultTransport (keeping proxy and TLS defaults) guarded by p, with
// redirect following disabled.
func (p Policy) NewClient(timeout time.Duration) *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	p.GuardTransport(t)
	return &http.Client{Timeout: timeout, Transport: t, CheckRedirect: NoRedirects}
}

// ResolveHost resolves host (a name or IP literal) and checks every address
// against the policy, for callers that hand the vetted addresses to another
// program (e.g. git's http.curloptResolve) so it cannot re-resolve. A host
// with any blocked address is rejected; a lookup failure is returned as is.
func (p Policy) ResolveHost(ctx context.Context, host string) ([]net.IP, error) {
	return p.resolve(ctx, host)
}
