package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	gocloak "github.com/Nerzal/gocloak/v13"
	"github.com/bacalhau-project/lilysaas/api/pkg/store"
	"github.com/bacalhau-project/lilysaas/api/pkg/types"
	jwt "github.com/golang-jwt/jwt/v4"
)

const CLIENT_ID = "api"
const REALM = "lilypad"

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

func extractBearerToken(token string) string {
	return strings.Replace(token, "Bearer ", "", 1)
}

func (auth *keyCloakMiddleware) maybeOwnerFromRequest(r *http.Request) (*types.ApiKey, error) {
	// in case the request is authenticated with an lp- token, rather than a
	// keycloak JWT, return the owner. Returns nil if it's not an lp- token.
	token := r.Header.Get("Authorization")
	token = extractBearerToken(token)

	if strings.HasPrefix(token, "lp-") {
		if owner, err := auth.store.CheckAPIKey(r.Context(), token); err != nil {
			return nil, fmt.Errorf("error checking API key: %s", err.Error())
		} else if owner == nil {
			// user claimed to provide lp- token, but it was invalid
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

func getUserIdFromJWT(tok *jwt.Token) string {
	mc := tok.Claims.(jwt.MapClaims)
	uid := mc["sub"].(string)
	return uid
}

func setRequestUser(ctx context.Context, userid string) context.Context {
	return context.WithValue(ctx, "userid", userid)
}

func getRequestUser(req *http.Request) string {
	// return the "userid" value from the request context
	return req.Context().Value("userid").(string)
}

func (auth *keyCloakMiddleware) verifyToken(next http.Handler) http.Handler {

	f := func(w http.ResponseWriter, r *http.Request) {
		maybeOwner, err := auth.maybeOwnerFromRequest(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		if maybeOwner == nil {
			// check keycloak JWT
			token, err := auth.jwtFromRequest(r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}
			r = r.WithContext(setRequestUser(r.Context(), getUserIdFromJWT(token)))
			next.ServeHTTP(w, r)
			return
		}
		// successful api_key auth
		r = r.WithContext(setRequestUser(r.Context(), maybeOwner.Owner))
		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(f)
}
