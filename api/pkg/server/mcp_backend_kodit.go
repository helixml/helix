//go:build !nokodit

package server

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/kodit"
)

// KoditMCPBackend implements MCPBackend for Kodit code intelligence.
// It creates a per-session HTTP handler scoped to the repositories
// the session's project has access to.
type KoditMCPBackend struct {
	koditClient *kodit.Client
	store       store.Store
	enabled     bool

	handlers   map[string]*sessionHandler
	handlersMu sync.RWMutex

	cleanupCtx    context.Context
	cleanupCancel context.CancelFunc
}

type sessionHandler struct {
	handler   http.Handler
	lastUsed  time.Time
	mu        sync.Mutex
}

func (h *sessionHandler) touch() {
	h.mu.Lock()
	h.lastUsed = time.Now()
	h.mu.Unlock()
}

func (h *sessionHandler) isExpired(ttl time.Duration) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return time.Since(h.lastUsed) > ttl
}

// NewKoditMCPBackend creates a new Kodit MCP backend with session-aware scoping.
func NewKoditMCPBackend(koditClient *kodit.Client, enabled bool, store store.Store) *KoditMCPBackend {
	if !enabled {
		return &KoditMCPBackend{enabled: false}
	}

	ctx, cancel := context.WithCancel(context.Background())
	b := &KoditMCPBackend{
		koditClient:   koditClient,
		store:         store,
		enabled:       koditClient != nil,
		handlers:      make(map[string]*sessionHandler),
		cleanupCtx:    ctx,
		cleanupCancel: cancel,
	}
	go b.cleanupLoop()
	return b
}

// setClient installs a kodit client after construction. Used because kodit.New
// runs asynchronously (its embedding-dimension probe may require the Helix
// listener to be up first).
func (b *KoditMCPBackend) setClient(koditClient *kodit.Client) {
	if koditClient == nil {
		return
	}
	b.koditClient = koditClient
	b.enabled = true
	if b.handlers == nil {
		b.handlers = make(map[string]*sessionHandler)
	}
	if b.cleanupCtx == nil {
		ctx, cancel := context.WithCancel(context.Background())
		b.cleanupCtx = ctx
		b.cleanupCancel = cancel
		go b.cleanupLoop()
	}
}

// Stop stops the background cleanup.
func (b *KoditMCPBackend) Stop() {
	if b.cleanupCancel != nil {
		b.cleanupCancel()
	}
}

func (b *KoditMCPBackend) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-b.cleanupCtx.Done():
			return
		case <-ticker.C:
			b.cleanupExpired(5 * time.Minute)
		}
	}
}

func (b *KoditMCPBackend) cleanupExpired(ttl time.Duration) {
	var expired []string

	b.handlersMu.RLock()
	for key, h := range b.handlers {
		if h.isExpired(ttl) {
			expired = append(expired, key)
		}
	}
	b.handlersMu.RUnlock()

	if len(expired) == 0 {
		return
	}

	b.handlersMu.Lock()
	for _, key := range expired {
		delete(b.handlers, key)
	}
	b.handlersMu.Unlock()

	log.Debug().Int("count", len(expired)).Msg("cleaned up expired Kodit MCP session handlers")
}

// ServeHTTP implements MCPBackend.
func (b *KoditMCPBackend) ServeHTTP(w http.ResponseWriter, r *http.Request, user *types.User) {
	if !b.enabled {
		http.Error(w, "Kodit is not enabled", http.StatusNotImplemented)
		return
	}

	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		sessionID = user.SessionID
	}
	if sessionID == "" {
		http.Error(w, "session_id is required for Kodit MCP access", http.StatusBadRequest)
		return
	}

	handler, err := b.handlerForSession(r.Context(), sessionID, user)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("failed to resolve Kodit MCP scope")
		http.Error(w, "failed to initialize Kodit MCP: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Rewrite the request path from /api/v1/mcp/kodit/{path} to /mcp/{path}
	vars := mux.Vars(r)
	pathSuffix := vars["path"]

	targetPath := "/mcp"
	if pathSuffix != "" {
		targetPath = "/mcp/" + pathSuffix
	} else if strings.HasSuffix(r.URL.Path, "/") {
		targetPath = "/mcp/"
	}

	r2 := r.Clone(r.Context())
	r2.URL.Path = targetPath
	r2.URL.RawPath = targetPath
	r2.RequestURI = targetPath
	if r.URL.RawQuery != "" {
		r2.RequestURI = targetPath + "?" + r.URL.RawQuery
	}

	handler.ServeHTTP(w, r2)
}

func (b *KoditMCPBackend) handlerForSession(ctx context.Context, sessionID string, user *types.User) (http.Handler, error) {
	ttl := 5 * time.Minute

	b.handlersMu.RLock()
	if h, ok := b.handlers[sessionID]; ok && !h.isExpired(ttl) {
		h.touch()
		b.handlersMu.RUnlock()
		return h.handler, nil
	}
	b.handlersMu.RUnlock()

	scope, err := resolveKoditRepoScope(ctx, b.store, sessionID, user)
	if err != nil {
		return nil, err
	}

	handler := kodit.NewScopedMCPHandler(b.koditClient, scope.idSlice)

	now := time.Now()
	b.handlersMu.Lock()
	b.handlers[sessionID] = &sessionHandler{
		handler:  handler,
		lastUsed: now,
	}
	b.handlersMu.Unlock()

	log.Info().
		Str("session_id", sessionID).
		Int("allowed_repos", len(scope.idSlice)).
		Msg("created session-scoped Kodit MCP handler")

	return handler, nil
}
