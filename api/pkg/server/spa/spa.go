package spa

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
)

type SPAFileServer struct {
	fileSystem http.FileSystem
	fileServer http.Handler
}

func NewSPAFileServer(fileSystem http.FileSystem) *SPAFileServer {
	return &SPAFileServer{
		fileSystem: fileSystem,
		fileServer: http.FileServer(fileSystem),
	}
}

func (s *SPAFileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

// SPAReverseProxyServer is used for local development or in theory it could be used
// if the frontend is running on a different container
type SPAReverseProxyServer struct {
	reverseProxy *httputil.ReverseProxy
}

func NewSPAReverseProxyServer(frontend string) *SPAReverseProxyServer {
	u, err := url.Parse(frontend)
	if err != nil {
		log.Error().Err(err).Msg("failed to parse frontend URL")
	}

	return &SPAReverseProxyServer{
		reverseProxy: httputil.NewSingleHostReverseProxy(u),
	}
}

func (s *SPAReverseProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.reverseProxy.ServeHTTP(w, r)
}
