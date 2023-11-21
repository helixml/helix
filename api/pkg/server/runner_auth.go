package server

import (
	"net/http"
)

type runnerAuth struct {
	token string
}

func newRunnerAuth(token string) *runnerAuth {
	auth := &runnerAuth{
		token: token,
	}
	return auth
}

func (auth *runnerAuth) isRequestAuthenticated(r *http.Request) bool {
	if auth.token == "" {
		return true
	}
	token := r.Header.Get("Authorization")
	if token == "" {
		token = r.URL.Query().Get("access_token")
	} else {
		token = extractBearerToken(token)
	}
	return token == auth.token
}

// verify a single shared secret if provided
func (auth *runnerAuth) verifyToken(next http.Handler) http.Handler {
	f := func(w http.ResponseWriter, r *http.Request) {
		if !auth.isRequestAuthenticated(r) {
			http.Error(w, "no token", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(f)
}
