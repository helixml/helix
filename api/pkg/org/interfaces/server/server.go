// Package server exposes the HTTP surface: the per-Bot MCP endpoint
// (every Bot is its own MCP server, scoped to the tools in its Bot.Tools;
// served by BotMCPHandler, and at /orgs/{org}/workers/{id}/mcp only by the
// standalone StandaloneHandler) and the inbound webhook endpoint
// (/webhooks/{org}/{triggerID}). The {id} is a Bot ID. Neither handler touches a store
// repository: caller resolution and topic reads go through the queries
// read facade, and inbound events go through the publishing service
// (append → notify → dispatch). The store stays in the composition
// root, behind those services.
package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/helixml/helix/api/pkg/org/application/prompts"
	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/application/queries"
	orgaudit "github.com/helixml/helix/api/pkg/org/domain/audit"
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
	audit      orgaudit.Recorder
	projects   orgaudit.ProjectResolver
	assets     store.Assets
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
// broadcaster + router) and returns a Server over them. The Server
// itself never holds the store — this just keeps the wiring (and tests)
// terse. broadcaster/router may be nil (the publish trio then skips
// the corresponding step).
func NewFromStore(s *store.Store, registry *mcptools.Registry, broadcaster *wakebus.Bus, router publishing.Router, logger *slog.Logger) *Server {
	q := queries.New(queries.Deps{
		Nodes:          s.Nodes,
		ReportingLines: s.ReportingLines,
		Triggers:       s.Triggers,
		Attachments:    s.WorkerAttachments,
		Processors:     s.Processors,
		Events:         s.Events,
		Activations:    s.Activations,
	})
	pd := publishing.Deps{
		Triggers: s.Triggers,
		Events:   s.Events,
		Now:      func() time.Time { return time.Now().UTC() },
		NewID:    uuid.NewString,
	}
	if broadcaster != nil {
		pd.Hub = broadcaster
	}
	if router != nil {
		pd.Router = router
	}
	server := New(q, publishing.New(pd), registry, logger)
	server.assets = s.Assets
	return server
}

func (s *Server) WithAudit(recorder orgaudit.Recorder, projects orgaudit.ProjectResolver) *Server {
	s.audit = recorder
	s.projects = projects
	return s
}

// WithPrompts attaches a prompts.Registry so the per-bot MCP server
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

// Handler returns the http.Handler an embedding host mounts behind its own
// authenticated, org-scoped middleware: /webhooks/{triggerID} plus any extras
// passed in by the wiring layer. The request-logging middleware wraps the lot.
//
// It deliberately does NOT register the per-bot MCP route. That route takes
// the org from the URL, and an embedding host forwards caller-controlled path
// suffixes into this mux (e.g. /api/v1/orgs/{A}/orgs/{B}/workers/{bot}/mcp
// would otherwise reach org B's bot after only org A was authorised). Hosts
// serve per-bot MCP through BotMCPHandler, binding the org and bot from their
// own authenticated state.
func (s *Server) Handler(extras ...Route) http.Handler {
	return s.handler(false, extras)
}

// StandaloneHandler is Handler plus the per-bot MCP route
// /orgs/{org}/workers/{id}/mcp, which trusts the {org} and {id} URL segments.
// Only for a standalone helix-org server with no tenant authentication in
// front of it (and tests); never mount it behind an embedding host.
func (s *Server) StandaloneHandler(extras ...Route) http.Handler {
	return s.handler(true, extras)
}

func (s *Server) handler(standaloneMCP bool, extras []Route) http.Handler {
	mux := http.NewServeMux()
	if standaloneMCP {
		// Per-org MCP per Bot. The {org} segment is required: composite
		// (id, org_id) PKs mean the bot handle repeats across tenants.
		// {id} is a Bot ID.
		mux.Handle("/orgs/{org}/workers/{id}/mcp", withMCPOrgScope(s.BotMCPHandler()))
	}
	mux.Handle("POST /webhooks/{org}/{triggerID}", s.webhookHandler())
	// Embedded in helix the org is already resolved into the request
	// context by the org-scope middleware and the mount prefix supplies
	// the {org} segment, so the public URL carries only the Trigger id.
	mux.Handle("POST /webhooks/{triggerID}", s.webhookHandler())
	for _, r := range extras {
		mux.Handle(r.Pattern, r.Handler)
	}
	return s.requestLogger(mux)
}

// withMCPOrgScope lifts the {org} and {id} URL segments into the context so
// BotMCPHandler can scope its store lookups to the right tenant. Standalone
// helix-org server only. If an org is already on the context the request came
// through an embedding host that authorised that org; a URL segment must
// never override it, so such requests are refused outright.
func withMCPOrgScope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if OrgIDFromContext(r.Context()) != "" {
			http.NotFound(w, r)
			return
		}
		orgID := r.PathValue("org")
		if orgID == "" {
			http.Error(w, "missing org", http.StatusBadRequest)
			return
		}
		ctx := WithOrgID(r.Context(), orgID)
		ctx = WithBotID(ctx, r.PathValue("id"))
		next.ServeHTTP(w, r.WithContext(ctx))
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
