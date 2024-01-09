package server

import (
	"fmt"
	"net/http"
)

type runnerAuth struct {
	token string
}

func newRunnerAuth(token string) (*runnerAuth, error) {
	if token == "" {
		return nil, fmt.Errorf("runner token is required")
	}
	auth := &runnerAuth{
		token: token,
	}
	return auth, nil
}

func (auth *runnerAuth) isRequestAuthenticated(r *http.Request) bool {
	return isRequestAuthenticatedAgainstToken(r, auth.token)
}

// verify a single shared secret if provided
func (auth *runnerAuth) middleware(next http.Handler) http.Handler {
	f := func(w http.ResponseWriter, r *http.Request) {
		if !auth.isRequestAuthenticated(r) {
			http.Error(w, "not authorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(f)
}
