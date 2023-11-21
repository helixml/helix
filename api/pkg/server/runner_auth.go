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

// verify a single shared secret if provided
func (auth *runnerAuth) verifyToken(next http.Handler) http.Handler {
	f := func(w http.ResponseWriter, r *http.Request) {
		if auth.token == "" {
			next.ServeHTTP(w, r)
			return
		}
		token := r.Header.Get("Authorization")
		if token == "" {
			token = r.URL.Query().Get("access_token")
		} else {
			token = extractBearerToken(token)
		}

		if token == "" {
			http.Error(w, "no token", http.StatusUnauthorized)
			return
		}

		if token != auth.token {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(f)
}
