package server

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type contextKey string

const userKey contextKey = "user"

/*
-
Middlewares
-
*/
func requireUser(next http.Handler) http.Handler {
	f := func(w http.ResponseWriter, r *http.Request) {
		// CORS preflight requests must reach the handler to receive
		// Access-Control-* headers. Auth has nothing useful to add.
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		user := getRequestUser(r)
		if !hasUser(user) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(f)
}

func requireAdmin(next http.Handler) http.Handler {
	f := func(w http.ResponseWriter, r *http.Request) {
		user := getRequestUser(r)
		if !isAdmin(user) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(f)
}

// requireAdminOrRunner admits global admins and the runner token. Used for
// infrastructure endpoints that both the admin UI and sandbox hosts call.
func requireAdminOrRunner(next http.Handler) http.Handler {
	f := func(w http.ResponseWriter, r *http.Request) {
		user := getRequestUser(r)
		if !isAdmin(user) && !isRunner(user) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(f)
}

func requireRunner(next http.Handler) http.Handler {
	f := func(w http.ResponseWriter, r *http.Request) {
		user := getRequestUser(r)
		if !isRunner(user) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(f)
}

/*
-
Token
-
*/
func extractBearerToken(token string) string {
	return strings.Replace(token, "Bearer ", "", 1)
}

func getBearerToken(r *http.Request) string {
	return extractBearerToken(r.Header.Get("Authorization"))
}

func getQueryToken(r *http.Request) string {
	return r.URL.Query().Get("access_token")
}

// namesOwnAPIKey reports whether the request carries a Helix API key it was
// given explicitly (a header, or ?access_token= on a WebSocket or EventSource,
// which cannot set headers), as opposed to one the browser sends by itself.
// Such a key says who the caller is, so it must win over the browser's
// session cookie: an embed in another product's page runs in a browser that
// may also hold the viewer's own Helix session.
func namesOwnAPIKey(r *http.Request) bool {
	for _, token := range []string{getBearerToken(r), r.Header.Get("x-api-key"), r.Header.Get("api-key"), getQueryToken(r)} {
		if strings.HasPrefix(token, types.APIKeyPrefix) {
			return true
		}
	}
	return false
}

func getRequestToken(r *http.Request) string {
	token, _ := getRequestTokenWithSource(r)
	return token
}

// getRequestTokenWithSource returns the credential the request carries and
// where it came from. The source is for the request audit log; the token itself
// is never recorded. The order matches the auth layer's precedence.
func getRequestTokenWithSource(r *http.Request) (string, string) {
	// First try to get from Authorization header
	token := getBearerToken(r)
	if token != "" {
		return token, "bearer"
	}

	// Try x-api-key header (used by Anthropic SDK clients like Zed)
	// When Zed is configured to use Helix as a proxy, it sends the Helix user token
	// in the x-api-key header (since that's what Anthropic's API expects)
	token = r.Header.Get("x-api-key")
	if token != "" {
		log.Trace().Str("source", "x-api-key").Str("path", r.URL.Path).Msg("token from x-api-key header")
		return token, "api-key"
	}

	// Try api-key header (used by Azure OpenAI SDK clients)
	token = r.Header.Get("api-key")
	if token != "" {
		log.Debug().Str("source", "api-key").Str("path", r.URL.Path).Msg("token from api-key header")
		return token, "api-key"
	}

	// Then try to get from cookie
	cookie, err := r.Cookie("access_token")
	if err == nil && cookie != nil && cookie.Value != "" {
		log.Debug().Str("source", "cookie").Str("path", r.URL.Path).Msg("token from access_token cookie")
		return cookie.Value, "cookie"
	}

	// Finally fall back to query parameter
	queryToken := getQueryToken(r)
	if queryToken != "" {
		log.Debug().Str("source", "query").Str("path", r.URL.Path).Msg("token from query parameter")
		return queryToken, "query"
	}
	return "", ""
}

/*
-
Request Context
-
*/
// detachContext creates a new context that is NOT tied to the HTTP request
// lifecycle. Use this for mutating operations (DB writes, state changes) that
// must complete even if the client disconnects or refreshes the page.
// The returned context preserves the authenticated user but has its own timeout.
func detachContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	newCtx, cancel := context.WithTimeout(context.Background(), timeout)
	// Preserve the authenticated user
	if userIntf := ctx.Value(userKey); userIntf != nil {
		newCtx = context.WithValue(newCtx, userKey, userIntf)
	}
	return newCtx, cancel
}

func setRequestUser(ctx context.Context, user types.User) context.Context {
	// Write through the request audit holder, if one is installed, so the outer
	// audit middleware sees the user this inner middleware authenticated.
	recordRequestAuditUser(ctx, user)
	return context.WithValue(ctx, userKey, user)
}

// socketUserID is the identity of unix-socket requests when no
// HELIX_EMBEDDINGS_SOCKET_USER_ID is configured.
const socketUserID = "socket"

func getRequestUser(req *http.Request) *types.User {
	// First check if user was set in context (e.g., by socket middleware)
	userIntf := req.Context().Value(userKey)
	if userIntf != nil {
		user := userIntf.(types.User)
		return &user
	}

	// Requests on the embeddings unix socket carry no credentials. Unless the
	// socket middleware set a configured user above, they run as the fixed
	// socket identity; nothing in the request selects the identity.
	if h, ok := req.Context().Value(http.LocalAddrContextKey).(*net.UnixAddr); ok && h != nil {
		return &types.User{
			ID:        socketUserID,
			Type:      types.OwnerTypeSocket,
			TokenType: types.TokenTypeSocket,
		}
	}

	return nil
}

// scrubUserCredentials clears the request-scoped credential fields of a user
// before the record is returned to another user or saved.
func scrubUserCredentials(user *types.User) {
	user.Token = ""
	user.TokenType = ""
}

func getOwnerContext(req *http.Request) types.OwnerContext {
	user := getRequestUser(req)
	return types.OwnerContext{
		Owner:     user.ID,
		OwnerType: types.OwnerTypeUser,
	}
}

/*
-
CORS
-
*/
func addCorsHeaders(w http.ResponseWriter) {
	// Set headers to allow requests from any origin
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
}

/*
-
Access Control
-
*/

func hasUser(user *types.User) bool {
	return user != nil && user.ID != ""
}

func hasUserOrRunner(user *types.User) bool {
	return hasUser(user) || isRunner(user)
}

func isAdmin(user *types.User) bool {
	return hasUser(user) && user.Admin
}

func isRunner(user *types.User) bool {
	return user != nil && user.Token != "" && user.TokenType == types.TokenTypeRunner
}
