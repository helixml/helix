// Package server exposes the HTTP surface. There is exactly one
// endpoint: /workers/{id}/mcp — every Worker is its own MCP server,
// scoped to the tools that Worker holds grants for, and used for both
// reads and mutations of the org graph. The CLI bootstraps by opening
// the store directly; there is no other HTTP write path.
package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/helixml/helix-org/broadcast"
	"github.com/helixml/helix-org/store"
	"github.com/helixml/helix-org/tools"
)

// Server wires handlers over a store and the tool registry.
type Server struct {
	store       *store.Store
	registry    *tools.Registry
	broadcaster *broadcast.Broadcaster
	logger      *slog.Logger
}

// New returns a Server bound to the given store, registry, broadcaster
// and logger. If logger is nil, a discard logger is used. The
// broadcaster wakes long-poll readers (e.g. read_events with wait>0);
// it may be nil in tests that don't exercise long-poll paths.
func New(s *store.Store, registry *tools.Registry, broadcaster *broadcast.Broadcaster, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(discardWriter{}, nil))
	}
	return &Server{store: s, registry: registry, broadcaster: broadcaster, logger: logger}
}

// Handler returns an http.Handler with all routes registered and the
// request-logging middleware applied.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/workers/{id}/mcp", s.mcpHandler())
	return s.requestLogger(mux)
}

// requestLogger logs one line per HTTP request at info level with method,
// path, status, and elapsed time.
func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusCapture{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		s.logger.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"elapsed", time.Since(start).Round(time.Millisecond),
		)
	})
}

// statusCapture wraps http.ResponseWriter to record the status code that
// was written so the logging middleware can report it.
type statusCapture struct {
	http.ResponseWriter
	status int
}

func (s *statusCapture) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// discardWriter is an io.Writer that throws away everything.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
