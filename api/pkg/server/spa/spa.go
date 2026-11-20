package spa

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

type FileServer struct {
	fileSystem http.FileSystem
	fileServer http.Handler
	policy     ShellPolicy
}

// ShellPolicy is the browser policy sent with the HTML shell (index.html and
// every client-side route that falls back to it).
type ShellPolicy struct {
	// FrameAncestors is a CSP frame-ancestors source list. Empty leaves
	// framing unrestricted. /embed/* routes are never restricted: they exist
	// to be framed by other sites.
	FrameAncestors string
	// HeadTags returns per-page <head> tags (link previews) for a request, or
	// "" to keep the shell's own. Optional.
	HeadTags func(*http.Request) string
}

// The shell's default link-preview tags sit between these markers in
// index.html; per-page tags replace them so crawlers see one set.
var (
	headTagsStart = []byte("<!-- helix:og -->")
	headTagsEnd   = []byte("<!-- /helix:og -->")
)

// injectHeadTags swaps the marked default tags for tags, or inserts tags
// before </head> when the shell has no markers.
func injectHeadTags(shell []byte, tags string) []byte {
	if i := bytes.Index(shell, headTagsStart); i >= 0 {
		if j := bytes.Index(shell[i:], headTagsEnd); j >= 0 {
			j += i + len(headTagsEnd)
			return bytes.Join([][]byte{shell[:i], []byte(tags), shell[j:]}, nil)
		}
	}
	if i := bytes.Index(shell, []byte("</head>")); i >= 0 {
		return bytes.Join([][]byte{shell[:i], []byte(tags), shell[i:]}, nil)
	}
	return shell
}

// shellPermissionsPolicy turns off browser features the web UI never uses,
// for the shell and anything it frames.
const shellPermissionsPolicy = "geolocation=(), payment=(), usb=(), serial=(), hid=(), bluetooth=(), browsing-topics=()"

func NewSPAFileServer(fileSystem http.FileSystem, policy ShellPolicy) *FileServer {
	return &FileServer{
		fileSystem: fileSystem,
		fileServer: http.FileServer(fileSystem),
		policy:     policy,
	}
}

// contentSecurityPolicy is the shell's CSP for a request path. It is scoped
// so it cannot break the SPA, its third-party scripts or embedding: no
// plugins, no <base> retargeting, and frame-ancestors when configured.
func (p ShellPolicy) contentSecurityPolicy(urlPath string) string {
	csp := "object-src 'none'; base-uri 'self'"
	if p.FrameAncestors != "" && !isEmbedPath(urlPath) {
		csp += "; frame-ancestors " + p.FrameAncestors
	}
	return csp
}

func isEmbedPath(urlPath string) bool {
	return urlPath == "/embed" || strings.HasPrefix(urlPath, "/embed/")
}

// isMissingFilePath reports whether a request for a file that does not exist
// should be a 404 rather than the SPA shell: API paths (an unknown API route
// is not a page), hashed bundle assets (a stale chunk must fail as a missing
// script, not parse HTML as JavaScript), and dot-files such as /.env or
// /.git/HEAD, which are never client-side routes.
func isMissingFilePath(urlPath string) bool {
	if urlPath == "/api" || strings.HasPrefix(urlPath, "/api/") || strings.HasPrefix(urlPath, "/assets/") {
		return true
	}
	for _, segment := range strings.Split(urlPath, "/") {
		if strings.HasPrefix(segment, ".") {
			return true
		}
	}
	return false
}

func (s *FileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path, err := filepath.Abs(r.URL.Path)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Set cache headers based on path:
	// - index.html and SPA fallback: never cache (ensures fresh deploys work)
	// - /assets/* with hashed filenames: cache forever (content-addressed)
	setCacheHeaders := func(isIndexHTML bool) {
		if isIndexHTML {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		} else if strings.HasPrefix(r.URL.Path, "/assets/") {
			// Vite hashes asset filenames, so they can be cached indefinitely
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
	}
	setShellHeaders := func() {
		w.Header().Set("Content-Security-Policy", s.policy.contentSecurityPolicy(r.URL.Path))
		w.Header().Set("Permissions-Policy", shellPermissionsPolicy)
	}

	if f, err := s.fileSystem.Open(path); err == nil {
		if err = f.Close(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Serving actual file - check if it's index.html
		isIndex := r.URL.Path == "/" || r.URL.Path == "/index.html"
		setCacheHeaders(isIndex)
		if isIndex {
			setShellHeaders()
		}
		s.fileServer.ServeHTTP(w, r)
	} else if os.IsNotExist(err) {
		if isMissingFilePath(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		// SPA fallback - serving index.html for client-side routing
		setCacheHeaders(true)
		setShellHeaders()
		if s.policy.HeadTags != nil {
			if tags := s.policy.HeadTags(r); tags != "" {
				s.serveShellWithTags(w, tags)
				return
			}
		}
		r.URL.Path = ""
		s.fileServer.ServeHTTP(w, r)
		return
	} else {
		log.Error().Err(err).Msg("file system open")
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *FileServer) serveShellWithTags(w http.ResponseWriter, tags string) {
	f, err := s.fileSystem.Open("/index.html")
	if err != nil {
		log.Error().Err(err).Msg("open index.html")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer f.Close()
	shell, err := io.ReadAll(f)
	if err != nil {
		log.Error().Err(err).Msg("read index.html")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(injectHeadTags(shell, tags))
}

// ReverseProxyServer is used for local development or in theory it could be used
// if the frontend is running on a different container
type ReverseProxyServer struct {
	reverseProxy *httputil.ReverseProxy
}

// NewSPAReverseProxyServer proxies to a frontend dev server. headTags is
// optional; see ShellPolicy.HeadTags.
func NewSPAReverseProxyServer(frontend string, headTags func(*http.Request) string) *ReverseProxyServer {
	u, err := url.Parse(frontend)
	if err != nil {
		log.Error().Err(err).Msg("failed to parse frontend URL")
	}

	reverseProxy := httputil.NewSingleHostReverseProxy(u)

	// Customize the Director to handle WebSocket headers
	originalDirector := reverseProxy.Director
	reverseProxy.Director = func(req *http.Request) {
		originalDirector(req)
		if req.Header.Get("Upgrade") == "websocket" {
			// For WebSocket connections, the proxy should use the target host.
			req.Host = u.Host
			req.URL.Host = u.Host
			req.URL.Scheme = u.Scheme
		}
	}

	// Log upstream errors (connection failures, upstream error responses)
	reverseProxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
		log.Error().
			Err(err).
			Str("path", req.URL.Path).
			Str("method", req.Method).
			Msg("spa reverse proxy: upstream error")
		w.WriteHeader(http.StatusBadGateway)
	}

	// Log every response from the upstream, capturing status code and duration
	reverseProxy.ModifyResponse = func(resp *http.Response) error {
		if resp.StatusCode >= 400 {
			log.Warn().
				Int("status", resp.StatusCode).
				Str("path", resp.Request.URL.Path).
				Str("upstream", fmt.Sprintf("%s://%s", u.Scheme, u.Host)).
				Msg("spa reverse proxy: upstream returned error status")
		}
		return injectProxiedHeadTags(resp, headTags)
	}

	return &ReverseProxyServer{
		reverseProxy: reverseProxy,
	}
}

func injectProxiedHeadTags(resp *http.Response, headTags func(*http.Request) string) error {
	if headTags == nil || resp.StatusCode != http.StatusOK ||
		resp.Header.Get("Content-Encoding") != "" ||
		!strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		return nil
	}
	tags := headTags(resp.Request)
	if tags == "" {
		return nil
	}
	shell, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read proxied shell: %w", err)
	}
	_ = resp.Body.Close()
	shell = injectHeadTags(shell, tags)
	resp.Body = io.NopCloser(bytes.NewReader(shell))
	resp.ContentLength = int64(len(shell))
	resp.Header.Set("Content-Length", fmt.Sprint(len(shell)))
	return nil
}

func (s *ReverseProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	s.reverseProxy.ServeHTTP(rw, r)
	if rw.status >= 400 {
		log.Warn().
			Int("status", rw.status).
			Str("path", r.URL.Path).
			Dur("duration", time.Since(start)).
			Msg("spa reverse proxy: non-2xx response to client")
	}
}

// statusRecorder wraps ResponseWriter to capture the written status code
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer %T does not support hijacking", r.ResponseWriter)
	}
	return hijacker.Hijack()
}
