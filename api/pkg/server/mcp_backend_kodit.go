//go:build !nokodit

package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/kodit"
)

// KoditMCPBackend implements MCPBackend for Kodit code intelligence.
// Every request is served by a handler scoped to the repositories the caller
// may read through the requested session.
//
// The scope is resolved on every request rather than cached per session: it
// depends on the caller as well as the session (owners and org owners can
// both read a session), and changes such as turning code intelligence off or
// detaching a repository must take effect for a client that keeps polling.
// Building the scoped Kodit handler costs tens of microseconds, and its MCP
// sessions are stateless, so a fresh handler per request serves an existing
// MCP session unchanged.
type KoditMCPBackend struct {
	koditClient    *kodit.Client
	store          store.Store
	enabled        bool
	authorize      SessionAuthorizer
	authorizeRepos RepositoryAuthorizer
}

// NewKoditMCPBackend creates a new Kodit MCP backend with session-aware scoping.
func NewKoditMCPBackend(koditClient *kodit.Client, enabled bool, store store.Store) *KoditMCPBackend {
	if !enabled {
		return &KoditMCPBackend{enabled: false}
	}
	return &KoditMCPBackend{
		koditClient: koditClient,
		store:       store,
		enabled:     koditClient != nil,
	}
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
}

// SetSessionAuthorizer wires the session RBAC check applied to every request.
func (b *KoditMCPBackend) SetSessionAuthorizer(fn SessionAuthorizer) {
	b.authorize = fn
}

// SetRepositoryAuthorizer wires the repository RBAC check that filters each
// request's Kodit scope down to the repositories the caller may read.
func (b *KoditMCPBackend) SetRepositoryAuthorizer(fn RepositoryAuthorizer) {
	b.authorizeRepos = fn
}

// authorizedSession loads the session whose repository scope the caller asks
// for and checks the caller may read it.
func (b *KoditMCPBackend) authorizedSession(ctx context.Context, sessionID string, user *types.User) (*types.Session, error) {
	if b.authorize == nil || user == nil {
		return nil, errors.New("not authorized for this session")
	}
	session, err := b.store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, errors.New("not authorized for this session")
	}
	if err := b.authorize(ctx, user, session, types.ActionGet); err != nil {
		return nil, err
	}
	return session, nil
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

	session, err := b.authorizedSession(r.Context(), sessionID, user)
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	handler, err := b.handlerFor(r.Context(), session, user)
	if errors.Is(err, errNoKoditRepositories) {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
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

// handlerFor builds the Kodit MCP handler for the repositories user may read
// through session.
func (b *KoditMCPBackend) handlerFor(ctx context.Context, session *types.Session, user *types.User) (http.Handler, error) {
	scope, err := resolveKoditRepoScope(ctx, b.store, b.authorizeRepos, session, user)
	if err != nil {
		return nil, err
	}
	handler, err := scopedKoditMCPHandler(b.koditClient, scope.idSlice)
	if err != nil {
		return nil, err
	}
	return koditScopeGuard(scope.repoIDs, handler), nil
}

// errNoKoditRepositories is returned when a session may see no indexed
// repositories.
var errNoKoditRepositories = errors.New("no code-intelligence repositories are available to this session")

// scopedKoditMCPHandler builds the Kodit MCP handler for a fixed set of
// repositories. kodit.NewScopedMCPHandler treats an empty set as unscoped
// (every repository on the instance), so an empty set is refused here
// rather than passed on.
func scopedKoditMCPHandler(client *kodit.Client, repoIDs []int64) (http.Handler, error) {
	if len(repoIDs) == 0 {
		return nil, errNoKoditRepositories
	}
	return kodit.NewScopedMCPHandler(client, repoIDs), nil
}
