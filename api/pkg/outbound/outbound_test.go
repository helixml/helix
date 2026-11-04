package outbound

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

type staticResolver map[string][]string

func (r staticResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	ips, ok := r[host]
	if !ok {
		return nil, errors.New("no such host")
	}
	out := make([]net.IPAddr, 0, len(ips))
	for _, ip := range ips {
		out = append(out, net.IPAddr{IP: net.ParseIP(ip)})
	}
	return out, nil
}

func mustPolicy(t *testing.T, cidrs ...string) *Policy {
	t.Helper()
	p, err := NewPolicy(cidrs)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestIPAllowed(t *testing.T) {
	p := mustPolicy(t)
	blocked := []string{
		"127.0.0.1", "10.0.0.1", "172.16.5.4", "192.168.1.1", "169.254.169.254",
		"100.64.0.1", "0.0.0.0", "224.0.0.1", "::1", "fd00:ec2::254", "fe80::1",
		"::ffff:127.0.0.1", "::ffff:169.254.169.254", "64:ff9b::a00:1", "::",
	}
	for _, ip := range blocked {
		if p.IPAllowed(net.ParseIP(ip)) {
			t.Errorf("%s should be blocked", ip)
		}
	}
	for _, ip := range []string{"8.8.8.8", "140.82.112.3", "2606:4700::1111"} {
		if !p.IPAllowed(net.ParseIP(ip)) {
			t.Errorf("%s should be allowed", ip)
		}
	}

	withNet := mustPolicy(t, "10.20.0.0/16")
	if !withNet.IPAllowed(net.ParseIP("10.20.3.4")) {
		t.Error("explicitly allowed CIDR should be allowed")
	}
	if withNet.IPAllowed(net.ParseIP("10.21.0.1")) {
		t.Error("private address outside allowed CIDR should stay blocked")
	}
}

func TestNewPolicyRejectsBadCIDR(t *testing.T) {
	if _, err := NewPolicy([]string{"not-a-cidr"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateURL(t *testing.T) {
	p := mustPolicy(t)
	p.Resolver = staticResolver{
		"gitlab.example.com":   {"203.0.113.10"},
		"internal.example.com": {"10.0.0.5"},
		"mixed.example.com":    {"203.0.113.10", "127.0.0.1"},
	}
	ok := []string{"https://gitlab.example.com", "http://gitlab.example.com/api/v4", "https://8.8.8.8"}
	for _, u := range ok {
		if err := p.ValidateURL(context.Background(), u); err != nil {
			t.Errorf("%s: unexpected error %v", u, err)
		}
	}
	bad := []string{
		"ftp://gitlab.example.com", "file:///etc/passwd", "gopher://x", "https://",
		"https://user:pass@gitlab.example.com", "http://127.0.0.1:8080", "http://[::1]/",
		"http://169.254.169.254/latest/meta-data", "https://internal.example.com",
		"https://mixed.example.com", "https://unresolvable.example.com", "://bad",
	}
	for _, u := range bad {
		err := p.ValidateURL(context.Background(), u)
		if !errors.Is(err, ErrDestinationNotAllowed) {
			t.Errorf("%s: expected ErrDestinationNotAllowed, got %v", u, err)
		}
	}
}

func countingServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestHTTPClientBlocksAtDialTime(t *testing.T) {
	srv, hits := countingServer(t)
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())

	// A name that passes no pre-flight and resolves to loopback at dial
	// time (DNS rebinding) must still be refused.
	p := mustPolicy(t)
	p.Resolver = staticResolver{"rebind.example.com": {"127.0.0.1"}}
	client := p.NewHTTPClient(5 * time.Second)

	for _, u := range []string{srv.URL, "http://rebind.example.com:" + port} {
		resp, err := client.Get(u)
		if err == nil {
			resp.Body.Close()
			t.Fatalf("%s: expected dial to be blocked", u)
		}
		if !errors.Is(err, ErrDestinationNotAllowed) {
			t.Fatalf("%s: expected ErrDestinationNotAllowed, got %v", u, err)
		}
	}
	if hits.Load() != 0 {
		t.Fatalf("blocked destination received %d requests", hits.Load())
	}
}

func TestHTTPClientAllowsConfiguredCIDR(t *testing.T) {
	srv, hits := countingServer(t)
	client := mustPolicy(t, "127.0.0.0/8").NewHTTPClient(5 * time.Second)
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if hits.Load() != 1 {
		t.Fatalf("expected 1 hit, got %d", hits.Load())
	}
}

func TestHTTPClientRejectsCrossOriginRedirect(t *testing.T) {
	target, targetHits := countingServer(t)
	targetURL, _ := url.Parse(target.URL)
	_, port, _ := net.SplitHostPort(targetURL.Host)

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/same" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/samehost" {
			http.Redirect(w, r, "/same", http.StatusFound)
			return
		}
		// Different origin: same IP, different host name and port.
		http.Redirect(w, r, "http://other.example.com:"+port+"/", http.StatusFound)
	}))
	defer redirector.Close()

	p := mustPolicy(t, "127.0.0.0/8")
	p.Resolver = staticResolver{"other.example.com": {"127.0.0.1"}}
	client := p.NewHTTPClient(5 * time.Second)

	resp, err := client.Get(redirector.URL + "/samehost")
	if err != nil {
		t.Fatalf("same-origin redirect should be followed: %v", err)
	}
	resp.Body.Close()

	resp, err = client.Get(redirector.URL + "/away")
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected cross-origin redirect to be rejected")
	}
	if !errors.Is(err, ErrDestinationNotAllowed) {
		t.Fatalf("expected ErrDestinationNotAllowed, got %v", err)
	}
	if targetHits.Load() != 0 {
		t.Fatalf("redirect target received %d requests", targetHits.Load())
	}
}

func TestHTTPClientIgnoresProxyEnv(t *testing.T) {
	proxy, proxyHits := countingServer(t)
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)

	srv, _ := countingServer(t)
	client := mustPolicy(t).NewHTTPClient(5 * time.Second)
	if resp, err := client.Get(srv.URL); err == nil {
		resp.Body.Close()
		t.Fatal("expected loopback target to be blocked even with a proxy configured")
	}
	if proxyHits.Load() != 0 {
		t.Fatal("request was sent via proxy")
	}
}
