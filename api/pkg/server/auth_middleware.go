package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	jwt "github.com/golang-jwt/jwt/v5"
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
	ErrNoAPIKeyFound = errors.New("no API key found")
	ErrNoUserIDFound = errors.New("no user ID found")
)

type authMiddlewareConfig struct {
	adminUserIDs []string
	runnerToken  string
}

type authMiddleware struct {
	authenticator auth.Authenticator
	store         store.Store
	adminUserIDs  []string
	runnerToken   string
	// this means ALL users
	// if '*' is included in the list
	developmentMode bool
}

func newAuthMiddleware(
	authenticator auth.Authenticator,
	store store.Store,
	cfg authMiddlewareConfig,
) *authMiddleware {
	return &authMiddleware{
		authenticator:   authenticator,
		store:           store,
		adminUserIDs:    cfg.adminUserIDs,
		runnerToken:     cfg.runnerToken,
		developmentMode: isDevelopmentMode(cfg.adminUserIDs),
	}
}

func (auth *authMiddleware) isUserAdmin(userID string) bool {
	if auth.developmentMode {
		return true
	}
	for _, adminID := range auth.adminUserIDs {
		if adminID == userID {
			return true
		}
	}
	return false
}

func (auth *authMiddleware) getUserFromToken(ctx context.Context, token string) (*types.User, error) {
	if token == "" {
		return nil, nil
	}

	if token == auth.runnerToken {
		// if the api key is our runner token then we are in runner mode
		return &types.User{
			Token:     token,
			TokenType: types.TokenTypeRunner,
		}, nil
	}

	if strings.HasPrefix(token, types.API_KEY_PREIX) {
		// we have an API key - we should load it from the database and construct our user that way
		apiKey, err := auth.store.GetAPIKey(ctx, token)
		if err != nil {
			return nil, fmt.Errorf("error getting API key: %s", err.Error())
		}
		if apiKey == nil {
			return nil, fmt.Errorf("error getting API key: %w", ErrNoAPIKeyFound)
		}

		user, err := auth.authenticator.GetUserByID(ctx, apiKey.Owner)
		if err != nil {
			return user, fmt.Errorf("error loading user from keycloak: %s", err.Error())
		}

		user.Token = token
		user.TokenType = types.TokenTypeAPIKey
		user.ID = apiKey.Owner
		user.Type = apiKey.OwnerType
		user.Admin = auth.isUserAdmin(user.ID)
		if apiKey.AppID != nil && apiKey.AppID.Valid {
			user.AppID = apiKey.AppID.String
		}

		return user, nil
	}

	// otherwise we try to decode the token with keycloak
	keycloakJWT, err := auth.authenticator.ValidateUserToken(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("error validating keycloak token: %s", err.Error())
	}
	mc := keycloakJWT.Claims.(jwt.MapClaims)
	keycloakUserID := mc["sub"].(string)

	if keycloakUserID == "" {
		return nil, fmt.Errorf("error getting keycloak user ID: %w", ErrNoUserIDFound)
	}

	user, err := auth.authenticator.GetUserByID(ctx, keycloakUserID)
	if err != nil {
		return user, fmt.Errorf("error loading user from keycloak: %s", err.Error())
	}

	user.Token = token
	user.TokenType = types.TokenTypeKeycloak
	user.ID = keycloakUserID
	user.Type = types.OwnerTypeUser
	user.Admin = auth.isUserAdmin(user.ID)

	return user, nil
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
				http.Error(w, "path not allowed for app API keys", http.StatusForbidden)
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
				http.Error(w, "path not allowed for app API keys", http.StatusForbidden)
				return
			}
		}

		r = r.WithContext(setRequestUser(r.Context(), *user))

		f(w, r)
	}
}
