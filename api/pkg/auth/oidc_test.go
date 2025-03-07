package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/suite"
	"golang.org/x/oauth2"
	"gopkg.in/go-jose/go-jose.v2"
	"gopkg.in/go-jose/go-jose.v2/jwt"
)

type OIDCSuite struct {
	suite.Suite
	mockOIDCServer *MockOIDCServer
}

func TestOIDCSuite(t *testing.T) {
	suite.Run(t, new(OIDCSuite))
}

func (s *OIDCSuite) SetupTest() {
	s.mockOIDCServer = NewMockOIDCServer()
}

func (s *OIDCSuite) TearDownTest() {
	s.mockOIDCServer.Close()
}

func (s *OIDCSuite) TestCreateOIDCClient() {
	ctx := context.Background()

	client, err := NewOIDCClient(ctx, OIDCConfig{
		ProviderURL:  s.mockOIDCServer.URL(),
		ClientID:     "api",
		ClientSecret: "REPLACE_ME",
		RedirectURL:  "http://localhost:8080/callback",
	})
	s.NoError(err)

	authURL := client.GetAuthURL("test-state", "test-nonce")
	log.Info().Str("auth_url", authURL).Msg("OIDC auth URL")
	s.NotEmpty(authURL)
}

func (s *OIDCSuite) TestExchange() {
	ctx := context.Background()

	client, err := NewOIDCClient(ctx, OIDCConfig{
		ProviderURL:  s.mockOIDCServer.URL(),
		ClientID:     "api",
		ClientSecret: "REPLACE_ME",
		RedirectURL:  "http://localhost:8080/callback",
	})
	s.NoError(err)

	oauth2Token, err := client.Exchange(ctx, "test-code")
	s.NoError(err)
	log.Info().Interface("oauth2_token", oauth2Token).Msg("OIDC oauth2 token")
	s.Equal(oauth2Token.AccessToken, "test-access-token")
}

func (s *OIDCSuite) TestVerifyIDToken() {
	ctx := context.Background()

	client, err := NewOIDCClient(ctx, OIDCConfig{
		ProviderURL:  s.mockOIDCServer.URL(),
		ClientID:     "api",
		ClientSecret: "REPLACE_ME",
		RedirectURL:  "http://localhost:8080/callback",
	})
	s.NoError(err)

	// Create claims
	claims := map[string]interface{}{
		"iss": s.mockOIDCServer.URL(),
		"sub": "test-subject",
		"aud": "api",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}

	// Sign the claims
	builder := jwt.Signed(s.mockOIDCServer.signer).Claims(claims)
	rawJWT, err := builder.CompactSerialize()
	s.NoError(err)

	// Use the raw JWT as the id_token
	idToken := rawJWT

	token := oauth2.Token{
		AccessToken:  "test-access-token",
		TokenType:    "Bearer",
		RefreshToken: "test-refresh-token",
		Expiry:       time.Now().Add(time.Hour),
	}
	token = *token.WithExtra(map[string]interface{}{
		"id_token": idToken,
	})

	t, err := client.VerifyIDToken(ctx, &token)
	s.NoError(err)
	log.Info().Interface("id_token", t).Msg("OIDC id token")
}

func (s *OIDCSuite) TestVerifyAccessToken() {
	ctx := context.Background()

	client, err := NewOIDCClient(ctx, OIDCConfig{
		ProviderURL:  s.mockOIDCServer.URL(),
		ClientID:     "api",
		ClientSecret: "REPLACE_ME",
		RedirectURL:  "http://localhost:8080/callback",
	})
	s.NoError(err)

	token, err := client.RefreshAccessToken(ctx, "test-refresh-token")
	s.NoError(err)
	log.Info().Interface("token", token).Msg("OIDC token")
}

func (s *OIDCSuite) TestRefreshAccessToken() {
	ctx := context.Background()

	client, err := NewOIDCClient(ctx, OIDCConfig{
		ProviderURL:  s.mockOIDCServer.URL(),
		ClientID:     "api",
		ClientSecret: "REPLACE_ME",
		RedirectURL:  "http://localhost:8080/callback",
	})
	s.NoError(err)

	token, err := client.RefreshAccessToken(ctx, "test-refresh-token")
	s.NoError(err)
	log.Info().Interface("token", token).Msg("OIDC token")
	s.Equal(token.AccessToken, "test-access-token")
}

func (s *OIDCSuite) TestGetUserInfo() {
	ctx := context.Background()

	client, err := NewOIDCClient(ctx, OIDCConfig{
		ProviderURL:  s.mockOIDCServer.URL(),
		ClientID:     "api",
		ClientSecret: "REPLACE_ME",
		RedirectURL:  "http://localhost:8080/callback",
	})
	s.NoError(err)

	userInfo, err := client.GetUserInfo(ctx, "test-access-token")
	s.NoError(err)
	log.Info().Interface("user_info", userInfo).Msg("OIDC user info")
	s.Equal(userInfo.Email, "test@example.com")
	s.Equal(userInfo.Name, "Test User")
	s.Equal(userInfo.Subject, "test-user-id")
}

func (s *OIDCSuite) TestGetLogoutURL() {
	ctx := context.Background()

	client, err := NewOIDCClient(ctx, OIDCConfig{
		ProviderURL:  s.mockOIDCServer.URL(),
		ClientID:     "api",
		ClientSecret: "REPLACE_ME",
		RedirectURL:  "http://does-not-matter",
	})
	s.NoError(err)

	logoutURL, err := client.GetLogoutURL()
	s.NoError(err)
	expectedURL := s.mockOIDCServer.URL() + "/protocol/openid-connect/logout"
	s.Equal(expectedURL, logoutURL)
}

func (s *OIDCSuite) TestValidateUserToken() {
	ctx := context.Background()

	client, err := NewOIDCClient(ctx, OIDCConfig{
		ProviderURL:  s.mockOIDCServer.URL(),
		ClientID:     "api",
		ClientSecret: "REPLACE_ME",
		RedirectURL:  "http://does-not-matter",
	})
	s.NoError(err)

	user, err := client.ValidateUserToken(ctx, "test-access-token")
	s.NoError(err)
	log.Info().Interface("user", user).Msg("OIDC user")
	s.Equal(user.ID, "test-user-id")
	s.Equal(user.Email, "test@example.com")
	s.Equal(user.TokenType, types.TokenType("oidc"))
	s.Equal(user.Token, "test-access-token")
	s.Equal(user.Type, types.OwnerType("user"))
	s.Equal(user.Admin, false)
	s.Equal(user.FullName, "Test User")
}

type MockOIDCServer struct {
	server *httptest.Server
	signer jose.Signer
}

func NewMockOIDCServer() *MockOIDCServer {
	// Generate an RSA private key instead of ECDSA
	privateKey, _ := rsa.GenerateKey(rand.New(rand.NewSource(time.Now().UnixNano())), 2048)

	// Create a signing key for testing
	jwk := jose.JSONWebKey{
		Key:       privateKey,
		KeyID:     "test-key",
		Algorithm: string(jose.RS256),
		Use:       "sig",
	}

	// Create signer
	signer, _ := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jwk}, nil)

	m := &MockOIDCServer{
		signer: signer,
	}
	mux := http.NewServeMux()

	// .well-known/openid-configuration endpoint
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		config := map[string]interface{}{
			"issuer":                                                          m.server.URL,
			"authorization_endpoint":                                          m.server.URL + "/protocol/openid-connect/auth",
			"token_endpoint":                                                  m.server.URL + "/protocol/openid-connect/token",
			"introspection_endpoint":                                          m.server.URL + "/protocol/openid-connect/token/introspect",
			"userinfo_endpoint":                                               m.server.URL + "/protocol/openid-connect/userinfo",
			"end_session_endpoint":                                            m.server.URL + "/protocol/openid-connect/logout",
			"frontchannel_logout_session_supported":                           true,
			"frontchannel_logout_supported":                                   true,
			"jwks_uri":                                                        m.server.URL + "/protocol/openid-connect/certs",
			"check_session_iframe":                                            m.server.URL + "/protocol/openid-connect/login-status-iframe.html",
			"grant_types_supported":                                           []string{"authorization_code", "implicit", "refresh_token", "password", "client_credentials", "urn:openid:params:grant-type:ciba", "urn:ietf:params:oauth:grant-type:device_code"},
			"acr_values_supported":                                            []string{"0", "1"},
			"response_types_supported":                                        []string{"code", "none", "id_token", "token", "id_token token", "code id_token", "code token", "code id_token token"},
			"subject_types_supported":                                         []string{"public", "pairwise"},
			"id_token_signing_alg_values_supported":                           []string{"PS384", "ES384", "RS384", "HS256", "HS512", "ES256", "RS256", "HS384", "ES512", "PS256", "PS512", "RS512"},
			"id_token_encryption_alg_values_supported":                        []string{"RSA-OAEP", "RSA-OAEP-256", "RSA1_5"},
			"id_token_encryption_enc_values_supported":                        []string{"A256GCM", "A192GCM", "A128GCM", "A128CBC-HS256", "A192CBC-HS384", "A256CBC-HS512"},
			"userinfo_signing_alg_values_supported":                           []string{"PS384", "ES384", "RS384", "HS256", "HS512", "ES256", "RS256", "HS384", "ES512", "PS256", "PS512", "RS512", "none"},
			"userinfo_encryption_alg_values_supported":                        []string{"RSA-OAEP", "RSA-OAEP-256", "RSA1_5"},
			"userinfo_encryption_enc_values_supported":                        []string{"A256GCM", "A192GCM", "A128GCM", "A128CBC-HS256", "A192CBC-HS384", "A256CBC-HS512"},
			"request_object_signing_alg_values_supported":                     []string{"PS384", "ES384", "RS384", "HS256", "HS512", "ES256", "RS256", "HS384", "ES512", "PS256", "PS512", "RS512", "none"},
			"request_object_encryption_alg_values_supported":                  []string{"RSA-OAEP", "RSA-OAEP-256", "RSA1_5"},
			"request_object_encryption_enc_values_supported":                  []string{"A256GCM", "A192GCM", "A128GCM", "A128CBC-HS256", "A192CBC-HS384", "A256CBC-HS512"},
			"response_modes_supported":                                        []string{"query", "fragment", "form_post", "query.jwt", "fragment.jwt", "form_post.jwt", "jwt"},
			"registration_endpoint":                                           m.server.URL + "/clients-registrations/openid-connect",
			"token_endpoint_auth_methods_supported":                           []string{"private_key_jwt", "client_secret_basic", "client_secret_post", "tls_client_auth", "client_secret_jwt"},
			"token_endpoint_auth_signing_alg_values_supported":                []string{"PS384", "ES384", "RS384", "HS256", "HS512", "ES256", "RS256", "HS384", "ES512", "PS256", "PS512", "RS512"},
			"introspection_endpoint_auth_methods_supported":                   []string{"private_key_jwt", "client_secret_basic", "client_secret_post", "tls_client_auth", "client_secret_jwt"},
			"introspection_endpoint_auth_signing_alg_values_supported":        []string{"PS384", "ES384", "RS384", "HS256", "HS512", "ES256", "RS256", "HS384", "ES512", "PS256", "PS512", "RS512"},
			"authorization_signing_alg_values_supported":                      []string{"PS384", "ES384", "RS384", "HS256", "HS512", "ES256", "RS256", "HS384", "ES512", "PS256", "PS512", "RS512"},
			"authorization_encryption_alg_values_supported":                   []string{"RSA-OAEP", "RSA-OAEP-256", "RSA1_5"},
			"authorization_encryption_enc_values_supported":                   []string{"A256GCM", "A192GCM", "A128GCM", "A128CBC-HS256", "A192CBC-HS384", "A256CBC-HS512"},
			"claims_supported":                                                []string{"aud", "sub", "iss", "auth_time", "name", "given_name", "family_name", "preferred_username", "email", "acr"},
			"claim_types_supported":                                           []string{"normal"},
			"claims_parameter_supported":                                      true,
			"scopes_supported":                                                []string{"openid", "roles", "email", "offline_access", "profile", "microprofile-jwt", "phone", "web-origins", "address", "acr"},
			"request_parameter_supported":                                     true,
			"request_uri_parameter_supported":                                 true,
			"require_request_uri_registration":                                true,
			"code_challenge_methods_supported":                                []string{"plain", "S256"},
			"tls_client_certificate_bound_access_tokens":                      true,
			"revocation_endpoint":                                             m.server.URL + "/protocol/openid-connect/revoke",
			"revocation_endpoint_auth_methods_supported":                      []string{"private_key_jwt", "client_secret_basic", "client_secret_post", "tls_client_auth", "client_secret_jwt"},
			"revocation_endpoint_auth_signing_alg_values_supported":           []string{"PS384", "ES384", "RS384", "HS256", "HS512", "ES256", "RS256", "HS384", "ES512", "PS256", "PS512", "RS512"},
			"backchannel_logout_supported":                                    true,
			"backchannel_logout_session_supported":                            true,
			"device_authorization_endpoint":                                   m.server.URL + "/protocol/openid-connect/auth/device",
			"backchannel_token_delivery_modes_supported":                      []string{"poll", "ping"},
			"backchannel_authentication_endpoint":                             m.server.URL + "/protocol/openid-connect/ext/ciba/auth",
			"backchannel_authentication_request_signing_alg_values_supported": []string{"PS384", "ES384", "RS384", "ES256", "RS256", "ES512", "PS256", "PS512", "RS512"},
			"require_pushed_authorization_requests":                           false,
			"pushed_authorization_request_endpoint":                           m.server.URL + "/protocol/openid-connect/ext/par/request",
			"mtls_endpoint_aliases": map[string]interface{}{
				"token_endpoint":                        m.server.URL + "/protocol/openid-connect/token",
				"revocation_endpoint":                   m.server.URL + "/protocol/openid-connect/revoke",
				"introspection_endpoint":                m.server.URL + "/protocol/openid-connect/token/introspect",
				"device_authorization_endpoint":         m.server.URL + "/protocol/openid-connect/auth/device",
				"registration_endpoint":                 m.server.URL + "/clients-registrations/openid-connect",
				"userinfo_endpoint":                     m.server.URL + "/protocol/openid-connect/userinfo",
				"pushed_authorization_request_endpoint": m.server.URL + "/protocol/openid-connect/ext/par/request",
				"backchannel_authentication_endpoint":   m.server.URL + "/protocol/openid-connect/ext/ciba/auth",
			},
			"authorization_response_iss_parameter_supported": true,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config)
	})

	// Certs
	mux.HandleFunc("/protocol/openid-connect/certs", func(w http.ResponseWriter, r *http.Request) {
		keys := map[string]interface{}{
			"keys": []map[string]interface{}{
				{
					"kid": jwk.KeyID,
					"kty": "RSA",
					"alg": "RS256",
					"use": "sig",
					"n":   base64.RawURLEncoding.EncodeToString(jwk.Public().Key.(*rsa.PublicKey).N.Bytes()),
					"e":   "AQAB",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(keys)
	})

	// Userinfo endpoint
	mux.HandleFunc("/protocol/openid-connect/userinfo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"sub":            "test-user-id",
			"name":           "Test User",
			"email":          "test@example.com",
			"email_verified": true,
		})
	})

	// // JWKS endpoint
	// mux.HandleFunc("/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
	// 	jwks := map[string]interface{}{
	// 		"keys": []map[string]interface{}{
	// 			{
	// 				"kid": "test-key-id",
	// 				"kty": "RSA",
	// 				"alg": "RS256",
	// 				"use": "sig",
	// 				"n":   "test-modulus",
	// 				"e":   "AQAB",
	// 			},
	// 		},
	// 	}
	// 	w.Header().Set("Content-Type", "application/json")
	// 	json.NewEncoder(w).Encode(jwks)
	// })

	// // Auth endpoint
	// // /auth/realms/helix/protocol/openid-connect/auth?client_id=api&nonce=oG4-yM8BlmT4iTZh_ZioYpl07-0T_uRel1-q4WfGZGcVVfNN22dSjkyy5ZTEtszxuwtCGBvgCE-OchIBD9U7OA&redirect_uri=http%3A%2F%2Flocalhost%3A8080%2Fapi%2Fv1%2Fauth%2Fcallback&response_type=code&scope=openid+profile+email&state=cmQsB0KGMUn90h2nKVhjsOGEod9U3_Jamnmx4_4l0gwWI7g1pGtqGL3dIqPjJJXeN-3I-3oM2_4MmC0LlwRXgQ
	// mux.HandleFunc("/protocol/openid-connect/auth", func(w http.ResponseWriter, r *http.Request) {
	// 	if err := r.ParseForm(); err != nil {
	// 		http.Error(w, "Invalid request", http.StatusBadRequest)
	// 		return
	// 	}

	// 	clientID := r.FormValue("client_id")
	// 	nonce := r.FormValue("nonce")
	// 	redirectURI := r.FormValue("redirect_uri")
	// 	responseType := r.FormValue("response_type")
	// 	scope := r.FormValue("scope")
	// 	state := r.FormValue("state")

	// 	// Log all of this
	// 	log.Info().
	// 		Str("client_id", clientID).
	// 		Str("nonce", nonce).
	// 		Str("redirect_uri", redirectURI).
	// 		Str("response_type", responseType).
	// 		Str("scope", scope).
	// 		Str("state", state).
	// 		Msg("OIDC auth request")
	// })

	// // Token endpoint
	// mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
	// 	if err := r.ParseForm(); err != nil {
	// 		http.Error(w, "Invalid request", http.StatusBadRequest)
	// 		return
	// 	}

	// 	response := map[string]interface{}{
	// 		"access_token":  "test-access-token",
	// 		"token_type":    "Bearer",
	// 		"refresh_token": "test-refresh-token",
	// 		"expires_in":    3600,
	// 		"id_token":      "test-id-token",
	// 	}
	// 	w.Header().Set("Content-Type", "application/json")
	// 	json.NewEncoder(w).Encode(response)
	// })

	// // Userinfo endpoint
	// mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
	// 	userInfo := map[string]interface{}{
	// 		"sub":            "test-user-id",
	// 		"name":           "Test User",
	// 		"email":          "test@example.com",
	// 		"email_verified": true,
	// 	}
	// 	w.Header().Set("Content-Type", "application/json")
	// 	json.NewEncoder(w).Encode(userInfo)
	// })

	// mux.HandleFunc("/auth/realms/helix/login-actions/authenticate", func(w http.ResponseWriter, r *http.Request) {
	// 	queryParams := r.URL.Query()
	// 	// session_code=_8EpnFwzpwX4G4v7rgD8smonJ7cPk8kD86f903TjGGQ&execution=3d058947-71c9-45b7-a52a-c76439d0058c&client_id=api&tab_id=hLQ8Veu0vsQ
	// 	sessionCode := queryParams.Get("session_code")
	// 	execution := queryParams.Get("execution")
	// 	clientID := queryParams.Get("client_id")
	// 	tabID := queryParams.Get("tab_id")

	// 	log.Info().
	// 		Str("session_code", sessionCode).
	// 		Str("execution", execution).
	// 		Str("client_id", clientID).
	// 		Str("tab_id", tabID).Msg("OIDC login action authenticate")
	// 	w.Header().Set("Content-Type", "application/json")
	// 	json.NewEncoder(w).Encode(map[string]interface{}{
	// 		"active": true,
	// 	})
	// })

	// token_endpoint
	mux.HandleFunc("/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
		// Parse form data
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		// Log all of this
		log.Info().
			Str("code", r.FormValue("client_id")).
			Str("grant_type", r.FormValue("grant_type")).
			Str("response_type", r.FormValue("response_type")).
			Str("username", r.FormValue("username")).
			Str("password", r.FormValue("password")).
			Msg("OIDC token request")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "test-access-token",
		})
	})

	// Catch all handler to log any requests
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Info().Str("method", r.Method).Str("url", r.URL.String()).Msg("OIDC request")
	})

	m.server = httptest.NewServer(mux)
	return m
}

func (m *MockOIDCServer) Close() {
	if m.server != nil {
		m.server.Close()
	}
}

func (m *MockOIDCServer) URL() string {
	return m.server.URL
}
