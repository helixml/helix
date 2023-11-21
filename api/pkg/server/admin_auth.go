package server

import (
	"net/http"
)

type adminAuth struct {
	adminUserIDs []string
	// this means ALL users
	// if '*' is included in the list
	developmentMode bool
}

func newAdminAuth(adminUserIDs []string) *adminAuth {
	developmentMode := false
	for _, id := range adminUserIDs {
		if id == "*" {
			developmentMode = true
			break
		}
	}
	auth := &adminAuth{
		adminUserIDs:    adminUserIDs,
		developmentMode: developmentMode,
	}
	return auth
}

func (auth *adminAuth) isUserAdmin(user string) bool {
	if auth.developmentMode {
		return true
	}
	if user == "" {
		return false
	}
	for _, id := range auth.adminUserIDs {
		if id == user {
			return true
		}
	}
	return false
}

func (auth *adminAuth) isRequestAuthenticated(r *http.Request) bool {
	return auth.isUserAdmin(getRequestUser(r))
}

func (auth *adminAuth) middleware(next http.Handler) http.Handler {
	f := func(w http.ResponseWriter, r *http.Request) {
		if !auth.isRequestAuthenticated(r) {
			http.Error(w, "not admin", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(f)
}
