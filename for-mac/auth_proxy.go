package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
)

// AuthProxy runs a local reverse proxy that injects the helix_session cookie
// into every request. This works around WKWebView's cross-origin iframe
// cookie blocking: instead of relying on the browser to store and send
// cookies, we inject them server-side.
type AuthProxy struct {
	mu            sync.Mutex
	sessionCookie string // helix_session value
	csrfCookie    string // helix_csrf value
	listener      net.Listener
	server        *http.Server
	port          int
	targetPort    int
}

// NewAuthProxy creates a new auth proxy targeting the given API port.
func NewAuthProxy(targetPort int) *AuthProxy {
	return &AuthProxy{targetPort: targetPort}
}

// Start starts the reverse proxy on a random available port.
func (p *AuthProxy) Start() error {
	target, err := url.Parse(fmt.Sprintf("http://localhost:%d", p.targetPort))
	if err != nil {
		return fmt.Errorf("invalid target URL: %w", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	// Modify requests to inject the session cookie
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		p.mu.Lock()
		session := p.sessionCookie
		csrf := p.csrfCookie
		p.mu.Unlock()

		if session != "" {
			// Replace or add the helix_session cookie
			req.Header.Del("Cookie")
			cookies := []string{fmt.Sprintf("helix_session=%s", session)}
			if csrf != "" {
				cookies = append(cookies, fmt.Sprintf("helix_csrf=%s", csrf))
			}
			req.Header.Set("Cookie", strings.Join(cookies, "; "))
		}
	}

	// Remove Set-Cookie headers from responses — the browser doesn't need
	// to store cookies since we inject them server-side.
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Del("Set-Cookie")
		return nil
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	p.listener = listener
	p.port = listener.Addr().(*net.TCPAddr).Port
	p.server = &http.Server{Handler: proxy}

	go func() {
		if err := p.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("Auth proxy error: %v", err)
		}
	}()

	log.Printf("Auth proxy started on port %d → localhost:%d", p.port, p.targetPort)
	return nil
}

// Authenticate calls the desktop-callback endpoint, extracts the session
// cookie from the response, and stores it for injection into proxied requests.
func (p *AuthProxy) Authenticate(secret string) error {
	callbackURL := fmt.Sprintf("http://localhost:%d/api/v1/auth/desktop-callback?token=%s",
		p.targetPort, secret)

	// Use a client that doesn't follow redirects — we just want the Set-Cookie headers
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(callbackURL)
	if err != nil {
		return fmt.Errorf("callback request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("callback returned status %d", resp.StatusCode)
	}

	// Extract cookies from the response
	for _, cookie := range resp.Cookies() {
		switch cookie.Name {
		case "helix_session":
			p.mu.Lock()
			p.sessionCookie = cookie.Value
			p.mu.Unlock()
			log.Printf("Auth proxy: got session cookie (expires %s)", cookie.Expires.Format("2006-01-02"))
		case "helix_csrf":
			p.mu.Lock()
			p.csrfCookie = cookie.Value
			p.mu.Unlock()
		}
	}

	if p.sessionCookie == "" {
		return fmt.Errorf("no helix_session cookie in callback response")
	}

	return nil
}

// GetURL returns the proxy base URL.
func (p *AuthProxy) GetURL() string {
	if p.port == 0 {
		return ""
	}
	return fmt.Sprintf("http://localhost:%d", p.port)
}

// Stop shuts down the proxy server.
func (p *AuthProxy) Stop() {
	if p.server != nil {
		p.server.Close()
	}
}
