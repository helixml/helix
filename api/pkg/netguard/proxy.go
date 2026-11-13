package netguard

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// BlockedHeader is set on the 403 a Proxy returns for a forbidden
// destination, so a client can tell a policy block from an origin's 403.
const BlockedHeader = "X-Helix-Egress-Blocked"

const proxyRealm = "helix-egress"

// Proxy is an authenticated HTTP forward proxy (CONNECT for https,
// absolute-URI requests for http) whose every upstream connection is made by
// Policy.DialContext. It confines software that resolves and connects on its
// own, such as headless Chrome, to the policy.
type Proxy struct {
	policy    Policy
	username  string
	password  string
	forwarder *httputil.ReverseProxy
}

// NewProxy returns a Proxy that admits only requests carrying
// Proxy-Authorization: Basic username:password.
func NewProxy(policy Policy, username, password string) (*Proxy, error) {
	if username == "" || password == "" {
		return nil, errors.New("egress proxy requires a username and password")
	}
	transport := &http.Transport{
		DialContext:           policy.DialContext,
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	}
	p := &Proxy{policy: policy, username: username, password: password}
	p.forwarder = &httputil.ReverseProxy{
		Rewrite:      func(*httputil.ProxyRequest) {}, // Out.URL is already the absolute target
		Transport:    transport,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) { writeDialError(w, err) },
	}
	return p, nil
}

// RandomCredential returns a random token for use as a Proxy username or
// password.
func RandomCredential() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate proxy credential: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !p.authorized(r.Header.Get("Proxy-Authorization")) {
		w.Header().Set("Proxy-Authenticate", `Basic realm="`+proxyRealm+`"`)
		http.Error(w, "proxy authentication required", http.StatusProxyAuthRequired)
		return
	}
	if r.Method == http.MethodConnect {
		p.tunnel(w, r)
		return
	}
	if r.URL.Scheme != "http" || r.URL.Host == "" {
		http.Error(w, "only absolute http:// requests and CONNECT are proxied", http.StatusBadRequest)
		return
	}
	p.forwarder.ServeHTTP(w, r)
}

func (p *Proxy) authorized(header string) bool {
	encoded, ok := strings.CutPrefix(header, "Basic ")
	if !ok {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return false
	}
	user, pass, ok := strings.Cut(string(raw), ":")
	if !ok {
		return false
	}
	userOK := subtle.ConstantTimeCompare([]byte(user), []byte(p.username))
	passOK := subtle.ConstantTimeCompare([]byte(pass), []byte(p.password))
	return userOK&passOK == 1
}

func (p *Proxy) tunnel(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), dialTimeout)
	upstream, err := p.policy.DialContext(ctx, "tcp", r.Host)
	cancel()
	if err != nil {
		writeDialError(w, err)
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		upstream.Close()
		http.Error(w, "connection cannot be hijacked", http.StatusInternalServerError)
		return
	}
	client, buffered, err := hijacker.Hijack()
	if err != nil {
		upstream.Close()
		log.Warn().Err(err).Msg("egress proxy: hijack failed")
		return
	}
	if _, err := client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		client.Close()
		upstream.Close()
		return
	}
	// Bytes the client sent after the CONNECT head are already buffered.
	if n := buffered.Reader.Buffered(); n > 0 {
		head, _ := buffered.Reader.Peek(n)
		if _, err := upstream.Write(head); err != nil {
			client.Close()
			upstream.Close()
			return
		}
	}
	splice(client, upstream)
}

// splice copies both ways until either side ends, then closes both.
func splice(a, b net.Conn) {
	var once sync.Once
	closeBoth := func() { a.Close(); b.Close() }
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(a, b); once.Do(closeBoth); done <- struct{}{} }()
	go func() { _, _ = io.Copy(b, a); once.Do(closeBoth); done <- struct{}{} }()
	<-done
	<-done
}

func writeDialError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrBlockedDestination) {
		w.Header().Set(BlockedHeader, "1")
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	http.Error(w, "upstream unavailable", http.StatusBadGateway)
}

// Serve runs the proxy on ln until ctx is cancelled.
func (p *Proxy) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{Handler: p, ReadHeaderTimeout: 30 * time.Second}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("egress proxy: %w", err)
	}
	return nil
}
