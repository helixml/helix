package server

import (
	"net/http"

	"github.com/lukemarsden/helix/api/pkg/types"
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
		if id == types.ADMIN_ALL_USERS {
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
	if user == "" {
		return false
	}
	if auth.developmentMode {
		return true
	}
	for _, id := range auth.adminUserIDs {
		if id == user {
			return true
		}
	}
	return false
}

func (auth *adminAuth) isRequestAuthenticated(r *http.Request) bool {
	reqUser := getRequestUser(r)
	return auth.isUserAdmin(reqUser.ID)
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
