package server

import (
	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
)

type authMiddleware struct {
	authenticator auth.Authenticator
	cfg           config.WebServer
	store         store.Store
}

func newAuthMiddleware(authenticator auth.Authenticator, cfg config.WebServer, store store.Store) *authMiddleware {
	return &authMiddleware{authenticator: authenticator, cfg: cfg, store: store}
}

// func (auth *authMiddleware) maybeOwnerFromRequest(r *http.Request) (*types.APIKey, error) {
// 	// in case the request is authenticated with an hl- token, rather than a
// 	// keycloak JWT, return the owner. Returns nil if it's not an hl- token.
// 	token := getRequestToken(r)

// 	if token == "" {
// 		return nil, nil
// 	}

// 	if strings.HasPrefix(token, types.API_KEY_PREIX) {
// 		apiKey, err := auth.store.GetAPIKey(r.Context(), token)
// 		if err != nil {
// 			return nil, fmt.Errorf("error getting API key: %s", err.Error())
// 		}
// 		if apiKey == nil {
// 			return nil, fmt.Errorf("error getting API key: no key found")
// 		}
// 		return apiKey, nil
// 	}

// 	// user didn't claim token was an lp token, so fallback to keycloak
// 	return nil, nil
// }

// func (auth *authMiddleware) jwtFromRequest(r *http.Request) (*jwt.Token, error) {
// 	token := getRequestToken(r)

// 	if token == "" {
// 		return nil, fmt.Errorf("token missing")
// 	}

// 	return auth.authenticator.ValidateUserToken(r.Context(), token)
// }

// func (auth *authMiddleware) userIDFromRequest(r *http.Request) (string, error) {
// 	token, err := auth.jwtFromRequest(r)
// 	if err != nil {
// 		return "", err
// 	}
// 	return getUserIdFromJWT(token), nil
// }

// // this will return a user id based on EITHER a database token OR a keycloak token
// func (auth *authMiddleware) userIDFromRequestBothModes(r *http.Request) (string, error) {
// 	databaseToken, err := auth.maybeOwnerFromRequest(r)
// 	if err != nil {
// 		return "", err
// 	}
// 	if databaseToken != nil {
// 		return databaseToken.Owner, nil
// 	}

// 	authToken, err := auth.jwtFromRequest(r)
// 	if err != nil {
// 		return "", err
// 	}
// 	return getUserIdFromJWT(authToken), nil
// }

// func getUserFromJWT(tok *jwt.Token) types.UserData {
// 	if tok == nil {
// 		return types.UserData{}
// 	}
// 	mc := tok.Claims.(jwt.MapClaims)
// 	uid := mc["sub"].(string)
// 	email := mc["email"].(string)
// 	name := mc["name"].(string)

// 	return types.UserData{
// 		ID:       uid,
// 		Email:    email,
// 		FullName: name,
// 		Token:    tok.Raw,
// 	}
// }

// func getUserIdFromJWT(tok *jwt.Token) string {
// 	user := getUserFromJWT(tok)
// 	return user.ID
// }

// func setRequestUser(ctx context.Context, user types.UserData) context.Context {
// 	ctx = context.WithValue(ctx, "userid", user.ID)
// 	ctx = context.WithValue(ctx, "email", user.Email)
// 	ctx = context.WithValue(ctx, "fullname", user.FullName)
// 	ctx = context.WithValue(ctx, "token", user.Token)
// 	return ctx
// }

// func getRequestUser(req *http.Request) types.UserData {
// 	id := req.Context().Value("userid")
// 	email := req.Context().Value("email")
// 	fullname := req.Context().Value("fullname")
// 	token := req.Context().Value("token")
// 	return types.UserData{
// 		ID:       id.(string),
// 		Email:    email.(string),
// 		FullName: fullname.(string),
// 		Token:    token.(string),
// 	}
// }

// // this happens in the very first middleware to populate the request context
// // based on EITHER the database api token OR then the keycloak JWT
// func (auth *authMiddleware) verifyToken(next http.Handler, enforce bool) http.Handler {
// 	f := func(w http.ResponseWriter, r *http.Request) {
// 		maybeOwner, err := auth.maybeOwnerFromRequest(r)
// 		if err != nil && enforce {
// 			http.Error(w, err.Error(), http.StatusUnauthorized)
// 			return
// 		}
// 		if maybeOwner == nil {
// 			// check keycloak JWT
// 			token, err := auth.jwtFromRequest(r)
// 			if err != nil && enforce {
// 				http.Error(w, err.Error(), http.StatusUnauthorized)
// 				return
// 			}
// 			r = r.WithContext(setRequestUser(r.Context(), getUserFromJWT(token)))
// 			next.ServeHTTP(w, r)
// 			return
// 		}
// 		// successful api_key auth
// 		r = r.WithContext(setRequestUser(r.Context(), types.UserData{
// 			ID:    maybeOwner.Owner,
// 			Token: maybeOwner.Key,
// 		}))
// 		next.ServeHTTP(w, r)
// 	}

// 	return http.HandlerFunc(f)
// }

// func (auth *authMiddleware) maybeVerifyToken(next http.Handler) http.Handler {
// 	return auth.verifyToken(next, false)
// }

// func (auth *authMiddleware) enforceVerifyToken(next http.Handler) http.Handler {
// 	return auth.verifyToken(next, true)
// }

// func (auth *authMiddleware) apiKeyAuth(f http.HandlerFunc) http.HandlerFunc {
// 	return func(rw http.ResponseWriter, req *http.Request) {
// 		maybeOwner, err := auth.maybeOwnerFromRequest(req)
// 		if err != nil || maybeOwner == nil {
// 			errorMessage := ""
// 			if err != nil {
// 				errorMessage = err.Error()
// 			} else {
// 				errorMessage = "unauthorized"
// 			}
// 			http.Error(rw, errorMessage, http.StatusUnauthorized)
// 			return
// 		}
// 		// successful api_key auth
// 		req = req.WithContext(setRequestUser(req.Context(), types.UserData{
// 			ID:    maybeOwner.Owner,
// 			Token: maybeOwner.Key,
// 		}))
// 		f.ServeHTTP(rw, req)
// 	}
// }
