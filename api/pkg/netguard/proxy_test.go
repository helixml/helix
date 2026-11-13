package netguard

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func startProxy(t *testing.T, policy Policy) *url.URL {
	t.Helper()
	p, err := NewProxy(policy, "user", "secret")
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = p.Serve(ctx, ln) }()
	return &url.URL{Scheme: "http", Host: ln.Addr().String()}
}

// clientVia returns a client that sends everything through proxyURL. The
// client itself is unguarded: the proxy is what is under test.
func clientVia(proxyURL *url.URL) *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // httptest certificate
		},
	}
}

func withCreds(u *url.URL, user, pass string) *url.URL {
	c := *u
	c.User = url.UserPassword(user, pass)
	return &c
}

func TestNewProxyRequiresCredentials(t *testing.T) {
	if _, err := NewProxy(Policy{}, "", "x"); err == nil {
		t.Fatal("NewProxy with empty username succeeded")
	}
	if _, err := NewProxy(Policy{}, "x", ""); err == nil {
		t.Fatal("NewProxy with empty password succeeded")
	}
}

func TestProxyRequiresCredentials(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("origin reached without proxy credentials")
	}))
	defer origin.Close()
	proxyURL := startProxy(t, Policy{AllowPrivate: true})

	for name, u := range map[string]*url.URL{
		"missing": proxyURL,
		"wrong":   withCreds(proxyURL, "user", "nope"),
	} {
		resp, err := clientVia(u).Get(origin.URL)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusProxyAuthRequired {
			t.Fatalf("%s creds: status %d, want 407", name, resp.StatusCode)
		}
		if got := resp.Header.Get("Proxy-Authenticate"); !strings.HasPrefix(got, "Basic realm=") {
			t.Fatalf("%s creds: Proxy-Authenticate = %q", name, got)
		}
	}

	// CONNECT without credentials is refused before any dial.
	tlsOrigin := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("TLS origin reached without proxy credentials")
	}))
	defer tlsOrigin.Close()
	if resp, err := clientVia(proxyURL).Get(tlsOrigin.URL); err == nil {
		resp.Body.Close()
		t.Fatal("CONNECT without credentials succeeded")
	}
}

func TestProxyForwardsAllowedHTTP(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != "" {
			t.Error("Proxy-Authorization leaked to the origin")
		}
		_, _ = io.WriteString(w, "hello "+r.URL.Path)
	}))
	defer origin.Close()
	proxyURL := withCreds(startProxy(t, Policy{AllowPrivate: true}), "user", "secret")

	resp, err := clientVia(proxyURL).Get(origin.URL + "/page")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "hello /page" {
		t.Fatalf("got %d %q", resp.StatusCode, body)
	}
}

func TestProxyConnectTunnel(t *testing.T) {
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "tunnelled")
	}))
	defer origin.Close()
	proxyURL := withCreds(startProxy(t, Policy{AllowPrivate: true}), "user", "secret")

	resp, err := clientVia(proxyURL).Get(origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "tunnelled" {
		t.Fatalf("body = %q", body)
	}
}

func TestProxyBlocksForbiddenDestinations(t *testing.T) {
	hit := false
	origin := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	defer origin.Close()
	tlsOrigin := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	defer tlsOrigin.Close()

	strict := clientVia(withCreds(startProxy(t, Policy{}), "user", "secret"))
	lax := clientVia(withCreds(startProxy(t, Policy{AllowPrivate: true}), "user", "secret"))

	cases := []struct {
		name   string
		client *http.Client
		url    string
	}{
		{"loopback http", strict, origin.URL},
		{"localhost name", strict, "http://localhost:1/"},
		{"metadata http", strict, "http://169.254.169.254/latest/meta-data/"},
		{"metadata http allow-private", lax, "http://169.254.169.254/latest/meta-data/"},
		{"mapped metadata", lax, "http://[::ffff:a9fe:a9fe]/"},
	}
	for _, tc := range cases {
		resp, err := tc.client.Get(tc.url)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden || resp.Header.Get(BlockedHeader) != "1" {
			t.Fatalf("%s: status %d blocked=%q, want 403 blocked", tc.name, resp.StatusCode, resp.Header.Get(BlockedHeader))
		}
	}

	// A refused CONNECT surfaces as a tunnel error in the client.
	for _, raw := range []string{tlsOrigin.URL, "https://169.254.169.254/"} {
		if resp, err := strict.Get(raw); err == nil {
			resp.Body.Close()
			t.Fatalf("CONNECT %s succeeded, want refusal", raw)
		} else if !strings.Contains(err.Error(), "Forbidden") {
			t.Fatalf("CONNECT %s: err = %v, want Forbidden", raw, err)
		}
	}
	if hit {
		t.Fatal("a blocked origin received a request")
	}
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
