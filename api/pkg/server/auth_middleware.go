package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

var (
	// Allowed paths for app API keys. Currently we support
	// only OpenAI compatible chat completions API
	AppAPIKeyPaths = map[string]bool{
		"/v1/chat/completions":  true,
		"/api/v1/sessions/chat": true,
	}
)

var (
	ErrNoUserIDFound           = errors.New("no user ID found")
	ErrAppAPIKeyPathNotAllowed = errors.New("path not allowed for app API keys, use your personal account key from your /account page instead")
)

type authMiddleware struct {
	oidcAuth auth.OIDCAuthenticator
	runnerAuth auth.RunnerTokenAuthenticator
	apikeyAuth auth.ApiKeyAuthenticator

	store store.Store
}

func newAuthMiddleware(
	oidcAuth auth.OIDCAuthenticator,
	runnerAuth auth.RunnerTokenAuthenticator,
	apikeyAuth auth.ApiKeyAuthenticator,

	store store.Store,
) *authMiddleware {
	return &authMiddleware{
		oidcAuth: oidcAuth,
		runnerAuth: runnerAuth,
		apikeyAuth: apikeyAuth,

		store: store,
	}
}

func (auth *authMiddleware) getUserFromToken(ctx context.Context, token string) (*types.User, error) {
	if token == "" {
		return nil, nil
	}

	result, err := auth.runnerAuth.ValidateAndReturnUser(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("runner token auth caused an error: %s", err)
	}
	if result != nil {
		return result, nil
	}

	result, err = auth.apikeyAuth.ValidateAndReturnUser(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("api key auth caused an error: %s", err)
	}
	if result != nil {
		return result, nil
	}

	result, err = auth.oidcAuth.ValidateAndReturnUser(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("oidc auth caused an error: %s", err)
	}

	return result, nil
}

// this will extract the token from the request and then load the correct
// user based on what type of token it is
// if there is no token, a default user object will be written to the
// request context
func (auth *authMiddleware) extractMiddleware(next http.Handler) http.Handler {
	f := func(w http.ResponseWriter, r *http.Request) {
		user, err := auth.getUserFromToken(r.Context(), getRequestToken(r))
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		if user == nil {
			user = &types.User{}
		}

		// If app API key, check if the path is in the allowed list
		if user.AppID != "" {
			if _, ok := AppAPIKeyPaths[r.URL.Path]; !ok {
				http.Error(w, ErrAppAPIKeyPathNotAllowed.Error(), http.StatusForbidden)
				return
			}
		}

		r = r.WithContext(setRequestUser(r.Context(), *user))
		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(f)
}

func (auth *authMiddleware) auth(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := auth.getUserFromToken(r.Context(), getRequestToken(r))
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		if user == nil {
			user = &types.User{}
		}

		if user.AppID != "" {
			if _, ok := AppAPIKeyPaths[r.URL.Path]; !ok {
				http.Error(w, ErrAppAPIKeyPathNotAllowed.Error(), http.StatusForbidden)
				return
			}
		}

		r = r.WithContext(setRequestUser(r.Context(), *user))

		f(w, r)
	}
}
