// Package server exposes the HTTP API. Reads are CRUD-ish; mutations flow
// through MCP at /workers/{id}/mcp — every Worker is its own MCP server,
// scoped to the tools that worker holds grants for.
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
	envsDir     string
}

// New returns a Server bound to the given store, registry, broadcaster
// and envs directory. envsDir is where the bootstrap handler creates
// the owner's Environment subdirectory; pass "" if bootstrap is not
// needed (e.g. tests that seed state directly). If logger is nil, a
// discard logger is used. If broadcaster is nil, the feed endpoint
// silently falls back to plain polling — ?wait= is honoured but fires
// only on timeout.
func New(s *store.Store, registry *tools.Registry, broadcaster *broadcast.Broadcaster, logger *slog.Logger, envsDir string) *Server {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(discardWriter{}, nil))
	}
	return &Server{store: s, registry: registry, broadcaster: broadcaster, logger: logger, envsDir: envsDir}
}

// Handler returns an http.Handler with all routes registered and the
// request-logging middleware applied.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /roles", s.listRoles)
	mux.HandleFunc("GET /roles/{id}", s.getRole)

	mux.HandleFunc("GET /positions", s.listPositions)
	mux.HandleFunc("GET /positions/{id}", s.getPosition)
	mux.HandleFunc("GET /positions/{id}/children", s.listPositionChildren)

	mux.HandleFunc("GET /workers", s.listWorkers)
	mux.HandleFunc("GET /workers/{id}", s.getWorker)
	mux.HandleFunc("GET /workers/{id}/grants", s.listWorkerGrants)
	mux.HandleFunc("GET /workers/{id}/feed", s.listFeed)
	mux.HandleFunc("GET /workers/{id}/environment", s.getEnvironment)
	mux.Handle("/workers/{id}/mcp", s.mcpHandler())

	mux.HandleFunc("GET /channels", s.listChannels)
	mux.HandleFunc("GET /channels/{id}", s.getChannel)
	mux.HandleFunc("GET /channels/{id}/events", s.listChannelEvents)

	mux.HandleFunc("GET /grants/{id}", s.getGrant)

	mux.HandleFunc("POST /bootstrap", s.bootstrap)

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
