// Package server exposes the HTTP surface: the per-Worker MCP endpoint
// (/orgs/{org}/workers/{id}/mcp — every Worker is its own MCP server,
// scoped to the tools in its Role) and the inbound webhook endpoint
// (/webhooks/{org}/{streamID}). Neither handler touches a store
// repository: caller/role resolution and stream reads go through the
// queries read facade, and inbound events go through the publishing
// service (append → notify → dispatch). The store stays in the
// composition root, behind those services.
package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/helixml/helix/api/pkg/org/application/prompts"
	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/application/queries"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/infrastructure/wakebus"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
)

// Server wires the MCP + webhook handlers over the application services.
type Server struct {
	queries    *queries.Queries
	publishing *publishing.Publishing
	registry   *mcptools.Registry
	prompts    *prompts.Registry
	logger     *slog.Logger
}

// New returns a Server bound to the read facade, the publishing service,
// the tool registry and a logger. If logger is nil, a discard logger is
// used. publishing may be nil in tests that don't exercise the inbound
// webhook route.
func New(q *queries.Queries, pub *publishing.Publishing, registry *mcptools.Registry, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(discardWriter{}, nil))
	}
	return &Server{queries: q, publishing: pub, registry: registry, logger: logger}
}

// NewFromStore is the composition-time convenience constructor: it
// builds the read facade and the publishing service from a store (+
// broadcaster + dispatcher) and returns a Server over them. The Server
// itself never holds the store — this just keeps the wiring (and tests)
// terse. broadcaster/dispatcher may be nil (the publish trio then skips
// the corresponding step).
func NewFromStore(s *store.Store, registry *mcptools.Registry, broadcaster *wakebus.Bus, dispatcher publishing.Dispatcher, logger *slog.Logger) *Server {
	q := queries.New(queries.Deps{
		Roles:          s.Roles,
		Workers:        s.Workers,
		ReportingLines: s.ReportingLines,
		Streams:        s.Streams,
		Subscriptions:  s.Subscriptions,
		Events:         s.Events,
		Environments:   s.Environments,
		Activations:    s.Activations,
	})
	pd := publishing.Deps{
		Streams: s.Streams,
		Events:  s.Events,
		Now:     func() time.Time { return time.Now().UTC() },
		NewID:   uuid.NewString,
	}
	if broadcaster != nil {
		pd.Hub = broadcaster
	}
	if dispatcher != nil {
		pd.Dispatcher = dispatcher
	}
	return New(q, publishing.New(pd), registry, logger)
}

// WithPrompts attaches a prompts.Registry so the per-worker MCP server
// will surface MCP prompts (slash commands) alongside tools. Returns
// the same Server so the call can be chained off New. Passing nil is
// equivalent to no prompts registered — the MCP server just answers
// prompts/list with an empty list.
func (s *Server) WithPrompts(reg *prompts.Registry) *Server {
	s.prompts = reg
	return s
}

// Route is a (pattern, handler) pair callers pass to Handler so
// transports can mount their own inbound endpoints (e.g. the email
// transport's /email/postmark) without server.go importing them.
type Route struct {
	Pattern string
	Handler http.Handler
}

// Handler returns an http.Handler with all built-in routes registered
// (MCP per-worker, /webhooks/{streamID}) plus any extras passed in by
// the wiring layer. The request-logging middleware wraps the lot.
func (s *Server) Handler(extras ...Route) http.Handler {
	mux := http.NewServeMux()
	// Per-org MCP per Worker. The {org} segment is required: composite
	// (id, org_id) PKs mean the worker handle ("w-owner") repeats
	// across tenants. The MCP handler reads orgID from
	// OrgIDFromContext, so this route wraps the inner handler in a
	// middleware that lifts {org} into the request context.
	mux.Handle("/orgs/{org}/workers/{id}/mcp", withMCPOrgScope(s.mcpHandler()))
	mux.Handle("POST /webhooks/{org}/{streamID}", s.webhookHandler())
	for _, r := range extras {
		mux.Handle(r.Pattern, r.Handler)
	}
	return s.requestLogger(mux)
}

// withMCPOrgScope lifts the {org} URL segment into the context via
// WithOrgID so the per-Worker MCP handler can scope its store lookups
// to the right helix tenant. Used by the standalone helix-org server
// only — the helix-embedded MCP backend (mcp_backend_helix_org.go in
// the helix package) does its own resolution because it needs to
// check org membership too.
func withMCPOrgScope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID := r.PathValue("org")
		if orgID == "" {
			http.Error(w, "missing org", http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithOrgID(r.Context(), orgID)))
	})
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

// statusCapture wraps http.ResponseWriter to record the status code
// that was written so the logging middleware can report it. Flush is
// passed through so streaming handlers (SSE, MCP streamable HTTP) keep
// working when the middleware is in the chain — without it,
// w.(http.Flusher) fails the type assertion and the handler errors
// out.
type statusCapture struct {
	http.ResponseWriter
	status int
}

func (s *statusCapture) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusCapture) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// discardWriter is an io.Writer that throws away everything.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
