package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	jwt "github.com/golang-jwt/jwt/v5"
	authpkg "github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
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
	ErrNoAPIKeyFound           = errors.New("no API key found")
	ErrNoUserIDFound           = errors.New("no user ID found")
	ErrAppAPIKeyPathNotAllowed = errors.New("path not allowed for app API keys, use your personal account key from your /account page instead")
)

type authMiddlewareConfig struct {
	adminUserIDs []string
	adminUserSrc config.AdminSrcType
	runnerToken  string
}

type authMiddleware struct {
	authenticator authpkg.Authenticator
	store         store.Store
	cfg           authMiddlewareConfig
}

func newAuthMiddleware(
	authenticator authpkg.Authenticator,
	store store.Store,
	cfg authMiddlewareConfig,
) *authMiddleware {
	return &authMiddleware{
		authenticator: authenticator,
		store:         store,
		cfg:           cfg,
	}
}

type tokenAcct struct {
	jwt    *jwt.Token
	userID string
}

type account struct {
	userID string
	token  *tokenAcct
}

type accountType string

const (
	accountTypeUser    accountType = "user"
	accountTypeToken   accountType = "token"
	accountTypeInvalid accountType = "invalid"
)

func (a *account) Type() accountType {
	switch {
	case a.userID != "":
		return accountTypeUser
	case a.token != nil:
		return accountTypeToken
	}
	return accountTypeInvalid
}

func (auth *authMiddleware) isAdmin(acct account) bool {
	if acct.Type() == accountTypeInvalid {
		return false
	}

	switch auth.cfg.adminUserSrc {
	case config.AdminSrcTypeEnv:
		if acct.Type() == accountTypeUser {
			return auth.isUserAdmin(acct.userID)
		}
		return auth.isUserAdmin(acct.token.userID)
	case config.AdminSrcTypeJWT:
		if acct.Type() != accountTypeToken {
			return false
		}
		return auth.isTokenAdmin(acct.token.jwt)
	}
	return false
}

func (auth *authMiddleware) isUserAdmin(userID string) bool {
	if userID == "" {
		return false
	}

	for _, adminID := range auth.cfg.adminUserIDs {
		// development mode everyone is an admin
		if adminID == types.AdminAllUsers {
			return true
		}
		if adminID == userID {
			return true
		}
	}
	return false
}

func (auth *authMiddleware) isTokenAdmin(token *jwt.Token) bool {
	if token == nil {
		return false
	}
	mc := token.Claims.(jwt.MapClaims)
	isAdmin := mc["admin"].(bool)
	return isAdmin
}

func (auth *authMiddleware) getUserFromToken(ctx context.Context, token string) (*types.User, error) {
	if token == "" {
		return nil, nil
	}

	if token == auth.cfg.runnerToken {
		// if the api key is our runner token then we are in runner mode
		return &types.User{
			ID:        "runner-system", // Add a system user ID for runner tokens
			Type:      types.OwnerTypeUser,
			Token:     token,
			TokenType: types.TokenTypeRunner,
		}, nil
	}

	if strings.HasPrefix(token, types.APIKeyPrefix) {
		if auth.authenticator == nil {
			return nil, fmt.Errorf("keycloak is required for Helix API key authentication")
		}
		// we have an API key - we should load it from the database and construct our user that way
		apiKey, err := auth.store.GetAPIKey(ctx, &types.ApiKey{
			Key: token,
		})
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
		user.Admin = auth.isAdmin(account{userID: user.ID})
		if apiKey.AppID != nil && apiKey.AppID.Valid {
			user.AppID = apiKey.AppID.String
		}

		// Ensure user_meta exists with slug for GitHub-style URLs
		if user.ID != "" {
			_, err := auth.store.EnsureUserMeta(ctx, types.UserMeta{
				ID: user.ID,
			})
			if err != nil {
				log.Warn().Err(err).Str("user_id", user.ID).Msg("failed to ensure user meta")
				// Don't fail auth if user_meta creation fails - just log the warning
			}
		}

		return user, nil
	}

	// otherwise we try to decode the token with keycloak
	user, err := auth.authenticator.ValidateUserToken(ctx, token)
	if err != nil {
		log.Error().Err(err).Str("token", token).Msg("error validating user token")
		return nil, err
	}

	// Ensure user_meta exists with slug for GitHub-style URLs
	if user != nil && user.ID != "" {
		_, err := auth.store.EnsureUserMeta(ctx, types.UserMeta{
			ID: user.ID,
		})
		if err != nil {
			log.Warn().Err(err).Str("user_id", user.ID).Msg("failed to ensure user meta")
			// Don't fail auth if user_meta creation fails - just log the warning
		}
	}

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
			// Check if error is due to server not ready vs invalid token
			// Return 503 for server errors so frontend doesn't auto-logout during API restart
			if errors.Is(err, authpkg.ErrProviderNotReady) {
				log.Warn().Err(err).Str("path", r.URL.Path).Msg("OIDC provider not ready during auth check")
				http.Error(w, "Authentication service temporarily unavailable", http.StatusServiceUnavailable)
				return
			}
			log.Debug().Err(err).Str("path", r.URL.Path).Msg("Auth error - returning 401")
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
			// Check if error is due to server not ready vs invalid token
			// Return 503 for server errors so frontend doesn't auto-logout during API restart
			if errors.Is(err, authpkg.ErrProviderNotReady) {
				log.Warn().Err(err).Str("path", r.URL.Path).Msg("OIDC provider not ready during auth check")
				http.Error(w, "Authentication service temporarily unavailable", http.StatusServiceUnavailable)
				return
			}
			log.Debug().Err(err).Str("path", r.URL.Path).Msg("Auth error - returning 401")
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
