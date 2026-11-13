package netguard

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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
		{"::a9fe:a9fe", false, false},        // IPv4-compatible → 169.254.169.254
		{"::a00:1", false, true},             // IPv4-compatible → 10.0.0.1
		{"2002:a9fe:a9fe::1", false, false},  // 6to4 → 169.254.169.254
		{"2002:808:808::1", true, true},      // 6to4 → 8.8.8.8
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
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())

	client := Policy{}.NewClient(2 * time.Second)
	// Each spells 127.0.0.1, where srv listens.
	for _, host := range []string{"2130706433", "0x7f.0.0.1", "017700000001", "127.1"} {
		raw := "http://" + net.JoinHostPort(host, port) + "/"
		if resp, err := client.Get(raw); err == nil {
			resp.Body.Close()
			t.Errorf("Get(%q) succeeded, want error", raw)
		}
	}
	if hit {
		t.Fatal("loopback server was reached through a non-canonical IPv4 literal")
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

// The proxy's own address is exempt from the policy only as a proxy: a
// request sent directly to it (e.g. NO_PROXY covers it) is still checked.
func TestGuardTransportDirectRequestToProxyAddress(t *testing.T) {
	var direct int
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !r.URL.IsAbs() {
			direct++
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)

	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = func(req *http.Request) (*url.URL, error) {
		if req.URL.Host == proxyURL.Host { // NO_PROXY covers the proxy's own address
			return nil, nil
		}
		return proxyURL, nil
	}
	Policy{}.GuardTransport(tr)
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}

	resp, err := client.Get("http://8.8.8.8/")
	if err != nil {
		t.Fatalf("public destination via proxy: %v", err)
	}
	resp.Body.Close()

	if _, err := client.Get(proxy.URL + "/admin"); !errors.Is(err, ErrBlockedDestination) {
		t.Fatalf("direct request to the proxy address err = %v, want ErrBlockedDestination", err)
	}
	if direct != 0 {
		t.Fatalf("proxy served %d direct requests, want 0", direct)
	}
}

func TestGuardTransportRejectsUnguardableDialers(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("GuardTransport accepted a transport with DialTLSContext set")
		}
	}()
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialTLSContext = func(context.Context, string, string) (net.Conn, error) { return nil, errors.New("unused") }
	Policy{}.GuardTransport(tr)
}

func TestDialContext(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	ctx := context.Background()

	if _, err := (Policy{}).DialContext(ctx, "tcp", ln.Addr().String()); !errors.Is(err, ErrBlockedDestination) {
		t.Fatalf("strict dial loopback err = %v, want ErrBlockedDestination", err)
	}
	if _, err := (Policy{AllowPrivate: true}).DialContext(ctx, "tcp", "169.254.169.254:80"); !errors.Is(err, ErrBlockedDestination) {
		t.Fatalf("dial metadata err = %v, want ErrBlockedDestination", err)
	}
	conn, err := Policy{AllowPrivate: true}.DialContext(ctx, "tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("AllowPrivate dial loopback: %v", err)
	}
	conn.Close()
}

// Deployments that resolve names only through their egress proxy must still
// pass up-front validation; the dial-time guard enforces the policy later.
func TestValidateURLAllowsUnresolvableHost(t *testing.T) {
	if err := (Policy{}).ValidateURL(context.Background(), "https://proxy-only-dns.invalid/"); err != nil {
		t.Fatalf("ValidateURL(unresolvable host) = %v, want nil", err)
	}
}
