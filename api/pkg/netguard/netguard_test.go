package netguard

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCheckIP(t *testing.T) {
	strict := Policy{}
	lax := Policy{AllowPrivate: true}

	cases := []struct {
		ip          string
		strictOK    bool
		allowPrivOK bool
	}{
		{"8.8.8.8", true, true},
		{"2606:4700:4700::1111", true, true},
		{"127.0.0.1", false, true},
		{"::1", false, true},
		{"10.1.2.3", false, true},
		{"172.20.0.5", false, true},
		{"192.168.1.10", false, true},
		{"100.64.0.1", false, true},
		{"fd12:3456::1", false, true},
		{"169.254.169.254", false, false},
		{"::ffff:169.254.169.254", false, false},
		{"64:ff9b::a9fe:a9fe", false, false}, // NAT64 → 169.254.169.254
		{"64:ff9b::7f00:1", false, true},     // NAT64 → 127.0.0.1
		{"fd00:ec2::254", false, false},
		{"fe80::1", false, false},
		{"0.0.0.0", false, false},
		{"::", false, false},
		{"224.0.0.1", false, false},
		{"255.255.255.255", false, false},
	}
	for _, tc := range cases {
		ip := net.ParseIP(tc.ip)
		if got := strict.CheckIP(ip) == nil; got != tc.strictOK {
			t.Errorf("strict CheckIP(%s) allowed=%v, want %v", tc.ip, got, tc.strictOK)
		}
		if got := lax.CheckIP(ip) == nil; got != tc.allowPrivOK {
			t.Errorf("AllowPrivate CheckIP(%s) allowed=%v, want %v", tc.ip, got, tc.allowPrivOK)
		}
	}
}

func TestValidateURL(t *testing.T) {
	ctx := context.Background()
	strict := Policy{}

	for _, raw := range []string{
		"file:///etc/passwd",
		"gopher://8.8.8.8/",
		"ftp://8.8.8.8/",
		"http://",
		"http://127.0.0.1:8080/v1",
		"http://localhost/v1",
		"http://api.localhost/v1",
		"http://169.254.169.254/latest/meta-data/",
		"http://[::ffff:a9fe:a9fe]/",
		"http://10.0.0.5:11434/v1",
	} {
		if err := strict.ValidateURL(ctx, raw); err == nil {
			t.Errorf("ValidateURL(%q) = nil, want error", raw)
		}
	}

	for _, raw := range []string{"https://8.8.8.8/v1", "http://[2606:4700:4700::1111]/v1"} {
		if err := strict.ValidateURL(ctx, raw); err != nil {
			t.Errorf("ValidateURL(%q) = %v, want nil", raw, err)
		}
	}

	lax := Policy{AllowPrivate: true}
	if err := lax.ValidateURL(ctx, "http://10.0.0.5:11434/v1"); err != nil {
		t.Errorf("AllowPrivate ValidateURL(private) = %v, want nil", err)
	}
	if err := lax.ValidateURL(ctx, "http://169.254.169.254/"); !errors.Is(err, ErrBlockedDestination) {
		t.Errorf("AllowPrivate ValidateURL(metadata) = %v, want ErrBlockedDestination", err)
	}
	if err := lax.ValidateURL(ctx, "file:///etc/passwd"); !errors.Is(err, ErrBlockedDestination) {
		t.Errorf("AllowPrivate ValidateURL(file) = %v, want ErrBlockedDestination", err)
	}
}

// The guard must block at dial time, so a URL that slipped past validation
// (stored before the fix, DNS rebinding, redirect) still never connects.
func TestGuardedClientBlocksLoopbackAtDial(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit = true
	}))
	defer srv.Close()

	_, err := Policy{}.NewClient(5 * time.Second).Get(srv.URL)
	if !errors.Is(err, ErrBlockedDestination) {
		t.Fatalf("Get(loopback) err = %v, want ErrBlockedDestination", err)
	}
	if hit {
		t.Fatal("loopback server received a request through the guarded client")
	}

	resp, err := Policy{AllowPrivate: true}.NewClient(5 * time.Second).Get(srv.URL)
	if err != nil {
		t.Fatalf("AllowPrivate Get(loopback) err = %v", err)
	}
	resp.Body.Close()
	if !hit {
		t.Fatal("AllowPrivate client did not reach the loopback server")
	}
}

// Non-canonical IPv4 literals either fail to resolve or resolve to an address
// that CheckIP then rejects; either way no connection is made.
func TestGuardedClientRejectsObscureIPv4Literals(t *testing.T) {
	client := Policy{}.NewClient(2 * time.Second)
	for _, raw := range []string{"http://2852039166/", "http://0x7f.0.0.1/", "http://017700000001/"} {
		if resp, err := client.Get(raw); err == nil {
			resp.Body.Close()
			t.Errorf("Get(%q) succeeded, want error", raw)
		}
	}
}

func TestGuardedClientDoesNotFollowRedirects(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("redirect target was fetched")
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirector.Close()

	resp, err := Policy{AllowPrivate: true}.NewClient(5 * time.Second).Get(redirector.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302 returned unfollowed", resp.StatusCode)
	}
}

// An operator-configured proxy is dialled even on a private address, but the
// request destination is still checked before it is handed to the proxy.
func TestGuardTransportProxy(t *testing.T) {
	var proxied []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied = append(proxied, r.URL.Host)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)

	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = http.ProxyURL(proxyURL)
	Policy{}.GuardTransport(tr)
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}

	resp, err := client.Get("http://8.8.8.8/v1/models")
	if err != nil {
		t.Fatalf("public destination via private proxy: %v", err)
	}
	resp.Body.Close()

	if _, err := client.Get("http://169.254.169.254/latest/meta-data/"); !errors.Is(err, ErrBlockedDestination) {
		t.Fatalf("metadata via proxy err = %v, want ErrBlockedDestination", err)
	}
	if len(proxied) != 1 || proxied[0] != "8.8.8.8" {
		t.Fatalf("proxy saw %v, want only [8.8.8.8]", proxied)
	}
}

func TestErrorClasses(t *testing.T) {
	ctx := context.Background()
	err := Policy{}.ValidateURL(ctx, "http://localhost:8080/")
	if !errors.Is(err, ErrBlockedDestination) || !errors.Is(err, ErrPrivateNetwork) {
		t.Fatalf("localhost: %v should be blocked and private", err)
	}
	for _, raw := range []string{"http://169.254.169.254/", "file:///etc/passwd"} {
		err := Policy{}.ValidateURL(ctx, raw)
		if !errors.Is(err, ErrBlockedDestination) || errors.Is(err, ErrPrivateNetwork) {
			t.Fatalf("%s: %v should be blocked but not private", raw, err)
		}
	}
}

// GuardContext covers clients whose transport cannot be set: a dial to a
// blocked address cancels the request before anything is sent.
func TestGuardContext(t *testing.T) {
	var got atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		got.Add(1)
	}))
	defer srv.Close()

	ctx := Policy{}.GuardContext(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (&http.Client{}).Do(req)
	if err == nil || !errors.Is(BlockedCause(ctx), ErrBlockedDestination) {
		t.Fatalf("err = %v, cause = %v; want a policy rejection", err, context.Cause(ctx))
	}
	if got.Load() != 0 {
		t.Fatal("request reached a loopback server")
	}

	ctx = Policy{AllowPrivate: true}.GuardContext(context.Background())
	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got.Load() != 1 {
		t.Fatal("allowed request did not arrive")
	}

	// A connection already in the shared pool is checked too.
	shared := &http.Client{}
	req, _ = http.NewRequest(http.MethodGet, srv.URL, nil)
	resp, err = shared.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	before := got.Load()
	req, _ = http.NewRequestWithContext(Policy{}.GuardContext(context.Background()), http.MethodGet, srv.URL, nil)
	if _, err := shared.Do(req); err == nil {
		t.Fatal("pooled connection to a blocked address was used")
	}
	if got.Load() != before {
		t.Fatal("request over a pooled connection reached a loopback server")
	}
}

func TestRedirectingClientFollowsAllowedHops(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusMovedPermanently)
	}))
	defer redirector.Close()

	resp, err := Policy{AllowPrivate: true}.NewRedirectingClient(5 * time.Second).Get(redirector.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 after following the redirect", resp.StatusCode)
	}
}

func TestRedirectingClientRechecksEachHop(t *testing.T) {
	for _, location := range []string{
		"http://169.254.169.254/latest/meta-data/", // always blocked
		"file:///etc/passwd",                       // bad scheme
		"http://127.0.0.1:1/",                      // private, not allowed by this policy
	} {
		var hops atomic.Int32
		policy := Policy{}
		client := policy.NewRedirectingClient(5 * time.Second)
		// Stand the redirector up on loopback but reach it through a
		// transport that skips the guard, so only the redirect hop is checked.
		redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hops.Add(1)
			http.Redirect(w, r, location, http.StatusFound)
		}))
		client.Transport = http.DefaultTransport
		_, err := client.Get(redirector.URL)
		redirector.Close()
		if !errors.Is(err, ErrBlockedDestination) {
			t.Errorf("redirect to %s: err = %v, want ErrBlockedDestination", location, err)
		}
		if hops.Load() != 1 {
			t.Errorf("redirect to %s: %d requests, want 1", location, hops.Load())
		}
	}
}

func TestRedirectingClientCapsHops(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/again", http.StatusFound)
	}))
	defer srv.Close()

	_, err := Policy{AllowPrivate: true}.NewRedirectingClient(5 * time.Second).Get(srv.URL)
	if err == nil || !strings.Contains(err.Error(), "stopped after 10 redirects") {
		t.Fatalf("err = %v, want redirect cap", err)
	}
}

// Legacy numeric spellings of an address are refused up front, under either
// policy, with ErrBlockedDestination rather than a resolver error.
func TestValidateURLRejectsNonCanonicalIPv4Literals(t *testing.T) {
	ctx := context.Background()
	for _, policy := range []Policy{{}, {AllowPrivate: true}} {
		for _, raw := range []string{
			"http://2130706433/",
			"http://0x7f000001/",
			"http://0X7F.0.0.1/",
			"http://0177.0.0.1/",
			"http://127.1/",
			"http://10.1/",
			"http://2852039166:8080/",
			"http://017700000001/",
			"http://127.0.0.1./",
			"http://0x/",
		} {
			if err := policy.ValidateURL(ctx, raw); !errors.Is(err, ErrBlockedDestination) {
				t.Errorf("ValidateURL(%q, AllowPrivate=%v) = %v, want ErrBlockedDestination", raw, policy.AllowPrivate, err)
			}
			if _, err := policy.NewClient(2 * time.Second).Get(raw); !errors.Is(err, ErrBlockedDestination) {
				t.Errorf("Get(%q, AllowPrivate=%v) = %v, want ErrBlockedDestination at dial", raw, policy.AllowPrivate, err)
			}
		}
	}
}

func TestIsNonCanonicalIPv4Literal(t *testing.T) {
	for host, want := range map[string]bool{
		"2130706433":   true,
		"0x7f000001":   true,
		"0177.0.0.1":   true,
		"127.1":        true,
		"1.2.3.4.5":    false,
		"github.com":   false,
		"0xdead.com":   false,
		"git.example":  false,
		"123.example":  false,
		"abc":          false,
		"":             false,
		"deadbeef":     false,
		"1a2b.0.0.1":   false,
		"192.168.0.1.": true,
	} {
		if got := isNonCanonicalIPv4Literal(host); got != want {
			t.Errorf("isNonCanonicalIPv4Literal(%q) = %v, want %v", host, got, want)
		}
	}
}
