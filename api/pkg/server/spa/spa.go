package spa

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"

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

	if f, err := s.fileSystem.Open(path); err == nil {
		if err = f.Close(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		s.fileServer.ServeHTTP(w, r)
	} else if os.IsNotExist(err) {
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

	return &ReverseProxyServer{
		reverseProxy: reverseProxy,
	}
}

func (s *ReverseProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.reverseProxy.ServeHTTP(w, r)
}
