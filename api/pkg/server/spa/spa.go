package spa

import (
	"fmt"
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
}

func NewSPAFileServer(fileSystem http.FileSystem) *FileServer {
	return &FileServer{
		fileSystem: fileSystem,
		fileServer: http.FileServer(fileSystem),
	}
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

	if f, err := s.fileSystem.Open(path); err == nil {
		if err = f.Close(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Serving actual file - check if it's index.html
		isIndex := r.URL.Path == "/" || r.URL.Path == "/index.html"
		setCacheHeaders(isIndex)
		s.fileServer.ServeHTTP(w, r)
	} else if os.IsNotExist(err) {
		// SPA fallback - serving index.html for client-side routing
		setCacheHeaders(true)
		r.URL.Path = ""
		s.fileServer.ServeHTTP(w, r)
		return
	} else {
		log.Error().Err(err).Msg("file system open")
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// ReverseProxyServer is used for local development or in theory it could be used
// if the frontend is running on a different container
type ReverseProxyServer struct {
	reverseProxy *httputil.ReverseProxy
}

func NewSPAReverseProxyServer(frontend string) *ReverseProxyServer {
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
		return nil
	}

	return &ReverseProxyServer{
		reverseProxy: reverseProxy,
	}
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
