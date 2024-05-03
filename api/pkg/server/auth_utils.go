package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
)

type contextKey string

const userKey contextKey = "user"

// func isRequestAuthenticatedAgainstToken(r *http.Request, actualToken string) bool {
// 	providedToken := getRequestToken(r)
// 	return providedToken == actualToken
// }

// func (apiServer *HelixAPIServer) requireAdmin(req *http.Request) error {
// 	isAdmin := apiServer.isAdmin(req)
// 	if !isAdmin {
// 		return fmt.Errorf("access denied")
// 	} else {
// 		return nil
// 	}
// }

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
	token := getBearerToken(r)
	if token == "" {
		token = getQueryToken(r)
	}
	return token
}

/*
-
Request Context
-
*/
func setRequestUser(ctx context.Context, user types.User) context.Context {
	return context.WithValue(ctx, userKey, user)
}

func getRequestUser(req *http.Request) types.User {
	user := req.Context().Value(userKey).(types.User)
	return user
}

func getRequestContext(req *http.Request) types.RequestContext {
	user := getRequestUser(req)
	return types.RequestContext{
		Ctx:  req.Context(),
		User: user,
	}
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

func corsMiddleware(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		addCorsHeaders(w)

		// If method is OPTIONS, return just the headers and finish the request
		if r.Method == "OPTIONS" {
			return
		}

		f.ServeHTTP(w, r)
	}
}

/*
-
Access Control
-
*/
// if any of the admin users IDs is "*" then we are in dev mode and every user is an admin
func isDevelopmentMode(adminUserIDs []string) bool {
	for _, id := range adminUserIDs {
		if id == types.ADMIN_ALL_USERS {
			return true
		}
	}
	return false
}

func hasUser(user types.User) bool {
	return user.ID != ""
}

func isAdmin(user types.User) bool {
	return user.ID != "" && user.Admin
}

func isRunner(user types.User) bool {
	return user.Token != "" && user.TokenType == types.TokenTypeRunner
}

func doesOwnSession(user types.User, session *types.Session) bool {
	return session.OwnerType == user.Type && session.Owner == user.ID
}

func canSeeSession(user types.User, session *types.Session) bool {
	canEdit := canEditSession(user, session)
	if canEdit {
		return true
	}
	if session.Metadata.Shared {
		return true
	}
	return false
}

func canEditSession(user types.User, session *types.Session) bool {
	if session.OwnerType == user.Type && session.Owner == user.ID {
		return true
	}
	if user.Admin {
		return true
	}
	return false
}
