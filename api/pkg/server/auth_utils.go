package server

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
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

func getRequestToken(r *http.Request) string {
	// First try to get from Authorization header
	token := getBearerToken(r)
	if token != "" {
		return token
	}

	// Then try to get from cookie
	cookie, err := r.Cookie("access_token")
	if err == nil && cookie != nil && cookie.Value != "" {
		return cookie.Value
	}

	// Finally fall back to query parameter
	return getQueryToken(r)
}

/*
-
Request Context
-
*/
func setRequestUser(ctx context.Context, user types.User) context.Context {
	return context.WithValue(ctx, userKey, user)
}

func getRequestUser(req *http.Request) *types.User {
	// Check if this is a socket request by looking at the underlying connection type
	if h, ok := req.Context().Value(http.LocalAddrContextKey).(*net.UnixAddr); ok && h != nil {
		return &types.User{
			ID:        "socket",
			Type:      types.OwnerTypeSocket,
			TokenType: types.TokenTypeSocket,
		}
	}
	userIntf := req.Context().Value(userKey)
	if userIntf == nil {
		return nil
	}
	user := userIntf.(types.User)

	return &user
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
	return user.ID != ""
}

func hasUserOrRunner(user *types.User) bool {
	return hasUser(user) || isRunner(user)
}

func isAdmin(user *types.User) bool {
	return hasUser(user) && user.Admin
}

func isRunner(user *types.User) bool {
	return user.Token != "" && user.TokenType == types.TokenTypeRunner
}

func canSeeSession(user *types.User, session *types.Session) bool {
	return canEditSession(user, session)
}

func canEditSession(user *types.User, session *types.Session) bool {
	if session.OwnerType == user.Type && session.Owner == user.ID {
		return true
	}
	if user.Admin {
		return true
	}
	return false
}
