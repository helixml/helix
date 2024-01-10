package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	gocloak "github.com/Nerzal/gocloak/v13"
	jwt "github.com/golang-jwt/jwt/v4"
	"github.com/lukemarsden/helix/api/pkg/store"
	"github.com/lukemarsden/helix/api/pkg/types"
)

const CLIENT_ID = "api"
const REALM = "helix"

type keycloak struct {
	gocloak      *gocloak.GoCloak // keycloak client
	externalUrl  string           // the URL of the keycloak server
	clientId     string           // clientId specified in Keycloak
	clientSecret string           // client secret specified in Keycloak
	realm        string           // realm specified in Keycloak
}

func newKeycloak(options ServerOptions) *keycloak {
	externalUrl := options.KeyCloakURL
	keycloak := &keycloak{
		gocloak:      gocloak.NewClient(externalUrl),
		externalUrl:  externalUrl,
		clientId:     CLIENT_ID,
		clientSecret: options.KeyCloakToken,
		realm:        REALM,
	}
	return keycloak
}

type keyCloakMiddleware struct {
	keycloak *keycloak
	options  ServerOptions
	store    store.Store
}

func newMiddleware(keycloak *keycloak, options ServerOptions, store store.Store) *keyCloakMiddleware {
	return &keyCloakMiddleware{keycloak: keycloak, options: options, store: store}
}

func (auth *keyCloakMiddleware) maybeOwnerFromRequest(r *http.Request) (*types.ApiKey, error) {
	// in case the request is authenticated with an hl- token, rather than a
	// keycloak JWT, return the owner. Returns nil if it's not an hl- token.
	token := r.Header.Get("Authorization")
	token = extractBearerToken(token)

	if token == "" {
		token = r.URL.Query().Get("access_token")
	}

	if token == "" {
		return nil, nil
	}

	if strings.HasPrefix(token, types.API_KEY_PREIX) {
		if owner, err := auth.store.CheckAPIKey(r.Context(), token); err != nil {
			return nil, fmt.Errorf("error checking API key: %s", err.Error())
		} else if owner == nil {
			// user claimed to provide hl- token, but it was invalid
			return nil, fmt.Errorf("invalid API key")
		} else {
			return owner, nil
		}
	}
	// user didn't claim token was an lp token, so fallback to keycloak
	return nil, nil
}

func (auth *keyCloakMiddleware) jwtFromRequest(r *http.Request) (*jwt.Token, error) {
	// try to extract Authorization parameter from the HTTP header
	token := r.Header.Get("Authorization")
	if token != "" {
		// extract Bearer token
		token = extractBearerToken(token)
		if token == "" {
			return nil, fmt.Errorf("bearer token missing")
		}
	} else {
		// try to extract access_token query parameter
		token = r.URL.Query().Get("access_token")
		if token == "" {
			return nil, fmt.Errorf("token missing")
		}
	}

	result, err := auth.keycloak.gocloak.RetrospectToken(r.Context(), token, CLIENT_ID, auth.options.KeyCloakToken, REALM)
	if err != nil {
		return nil, fmt.Errorf("RetrospectToken: invalid or malformed token: %s", err.Error())
	}

	j, _, err := auth.keycloak.gocloak.DecodeAccessToken(r.Context(), token, REALM)
	if err != nil {
		return nil, fmt.Errorf("DecodeAccessToken: invalid or malformed token: %s", err.Error())
	}

	// check if the token isn't expired and valid
	if !*result.Active {
		return nil, fmt.Errorf("invalid or expired token")
	}

	return j, nil
}

func (auth *keyCloakMiddleware) userIDFromRequest(r *http.Request) (string, error) {
	token, err := auth.jwtFromRequest(r)
	if err != nil {
		return "", err
	}
	return getUserIdFromJWT(token), nil
}

// this will return a user id based on EITHER a database token OR a keycloak token
// TODO: refactor this mess
func (auth *keyCloakMiddleware) userIDFromRequestBothModes(r *http.Request) (string, error) {
	databaseToken, err := auth.maybeOwnerFromRequest(r)
	if err != nil {
		return "", err
	}
	if databaseToken != nil {
		return databaseToken.Owner, nil
	}
	keycloakToken, err := auth.jwtFromRequest(r)
	if err != nil {
		return "", err
	}
	return getUserIdFromJWT(keycloakToken), nil
}

func getUserFromJWT(tok *jwt.Token) types.UserData {
	if tok == nil {
		return types.UserData{}
	}
	mc := tok.Claims.(jwt.MapClaims)
	uid := mc["sub"].(string)
	email := mc["email"].(string)
	name := mc["name"].(string)
	return types.UserData{
		ID:       uid,
		Email:    email,
		FullName: name,
	}
}

func getUserIdFromJWT(tok *jwt.Token) string {
	user := getUserFromJWT(tok)
	return user.ID
}

func setRequestUser(ctx context.Context, user types.UserData) context.Context {
	ctx = context.WithValue(ctx, "userid", user.ID)
	ctx = context.WithValue(ctx, "email", user.Email)
	ctx = context.WithValue(ctx, "fullname", user.FullName)
	return ctx
}

func getRequestUser(req *http.Request) types.UserData {
	id := req.Context().Value("userid")
	email := req.Context().Value("email")
	fullname := req.Context().Value("fullname")
	return types.UserData{
		ID:       id.(string),
		Email:    email.(string),
		FullName: fullname.(string),
	}
}

// this happens in the very first middleware to populate the request context
// based on EITHER the database api token OR then the keycloak JWT
func (auth *keyCloakMiddleware) verifyToken(next http.Handler, enforce bool) http.Handler {
	f := func(w http.ResponseWriter, r *http.Request) {
		maybeOwner, err := auth.maybeOwnerFromRequest(r)
		if err != nil && enforce {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		if maybeOwner == nil {
			// check keycloak JWT
			token, err := auth.jwtFromRequest(r)
			if err != nil && enforce {
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}
			r = r.WithContext(setRequestUser(r.Context(), getUserFromJWT(token)))
			next.ServeHTTP(w, r)
			return
		}
		// successful api_key auth
		r = r.WithContext(setRequestUser(r.Context(), types.UserData{
			ID: maybeOwner.Owner,
		}))
		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(f)
}

func (auth *keyCloakMiddleware) maybeVerifyToken(next http.Handler) http.Handler {
	return auth.verifyToken(next, false)
}

func (auth *keyCloakMiddleware) enforceVerifyToken(next http.Handler) http.Handler {
	return auth.verifyToken(next, true)
}
