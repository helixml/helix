package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"gopkg.in/go-jose/go-jose.v2"
	"gopkg.in/go-jose/go-jose.v2/jwt"
)

type OIDCSuite struct {
	suite.Suite
	mockOIDCServer *MockOIDCServer
	mockStore      *store.MockStore
	ctrl           *gomock.Controller
	client         *OIDCClient
	ctx            context.Context
	testToken      string
}

func TestOIDCSuite(t *testing.T) {
	suite.Run(t, new(OIDCSuite))
}

func (s *OIDCSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.ctx = context.Background()

	s.mockOIDCServer = NewMockOIDCServer()
	s.mockStore = store.NewMockStore(s.ctrl)

	client, err := NewOIDCClient(s.ctx, OIDCConfig{
		ProviderURL:  s.mockOIDCServer.URL(),
		ClientID:     "api",
		ClientSecret: "REPLACE_ME",
		RedirectURL:  "http://localhost:8080/callback",
		Audience:     "test-aud",
		Store:        s.mockStore,
	})
	s.NoError(err)
	s.client = client

	s.mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(&types.User{
		ID:       "test-user-id",
		Email:    "test@example.com",
		FullName: "Test User",
	}, nil).AnyTimes()

	// Generate a valid access token
	s.testToken = s.generateToken("test-aud")
}

func (s *OIDCSuite) generateToken(aud string) string {
	claims := jwt.Claims{
		Audience: jwt.Audience{aud},
		Subject:  "test-sub",
		Expiry:   jwt.NewNumericDate(time.Now().Add(time.Hour)),
		Issuer:   s.mockOIDCServer.URL(),
	}

	builder := jwt.Signed(s.mockOIDCServer.signer).Claims(claims)
	tokenString, err := builder.CompactSerialize()
	s.NoError(err)
	log.Info().Str("token", tokenString).Msg("Generated token")
	return tokenString
}

func (s *OIDCSuite) TearDownTest() {
	s.mockOIDCServer.Close()
}

func (s *OIDCSuite) TestAuthFlow() {
	// Test the complete auth flow
	tests := []struct {
		name      string
		state     string
		nonce     string
		wantError bool
	}{
		{
			name:      "valid auth flow",
			state:     "test-state",
			nonce:     "test-nonce",
			wantError: false,
		},
		{
			name:      "empty state",
			state:     "",
			nonce:     "test-nonce",
			wantError: false,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			// 1. Get auth URL
			authURL := s.client.GetAuthURL(tt.state, tt.nonce)
			s.NotEmpty(authURL)

			// 2. Exchange code
			token, err := s.client.Exchange(s.ctx, "test-code")
			if tt.wantError {
				s.Error(err)
				return
			}
			s.NoError(err)
			s.Equal("test-access-token", token.AccessToken)
			log.Info().
				Str("access_token", token.AccessToken).
				Str("token_type", token.TokenType).
				Str("refresh_token", token.RefreshToken).
				Time("expiry", token.Expiry).
				Interface("id_token", token.Extra("id_token")).
				Msg("Complete token with extras")

			// 3. Verify ID token
			idToken, err := s.client.VerifyIDToken(s.ctx, token)
			s.NoError(err)
			s.NotNil(idToken)
		})
	}
}

func (s *OIDCSuite) TestUserOperations() {
	// Note: We don't test JWT audience validation here because ValidateUserToken
	// intentionally doesn't verify access tokens as JWTs. Keycloak access tokens
	// have aud="account" not the client_id, so we validate by calling the userinfo
	// endpoint instead. If the token is invalid, the userinfo call will fail.
	tests := []struct {
		name        string
		accessToken string
		wantEmail   string
		wantName    string
		wantError   bool
	}{
		{
			name:        "valid user info",
			accessToken: s.testToken,
			wantEmail:   "test@example.com",
			wantName:    "Test User",
			wantError:   false,
		},
		{
			name:        "invalid token",
			accessToken: "invalid-token",
			wantError:   true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			// Test GetUserInfo
			userInfo, err := s.client.GetUserInfo(s.ctx, tt.accessToken)
			if tt.wantError {
				s.Error(err)
				return
			}
			s.NoError(err)
			s.Equal(tt.wantEmail, userInfo.Email)
			s.Equal(tt.wantName, userInfo.Name)

			// Test ValidateUserToken
			user, err := s.client.ValidateUserToken(s.ctx, tt.accessToken)
			s.NoError(err)
			s.Equal(tt.wantEmail, user.Email)
			s.Equal(tt.wantName, user.FullName)
			s.Equal(types.TokenType("oidc"), user.TokenType)
			s.Equal(types.OwnerType("user"), user.Type)
		})
	}
}

func (s *OIDCSuite) TestTokenOperations() {
	// Test token refresh
	token, err := s.client.RefreshAccessToken(s.ctx, "test-refresh-token")
	s.NoError(err)
	s.Equal("test-access-token", token.AccessToken)

	// Test logout URL
	logoutURL, err := s.client.GetLogoutURL()
	s.NoError(err)
	s.Equal(s.mockOIDCServer.URL()+"/protocol/openid-connect/logout", logoutURL)
}

type MockOIDCServer struct {
	server *httptest.Server
	signer jose.Signer
	jwk    jose.JSONWebKey
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
		jwk:    jwk,
	}
	mux := http.NewServeMux()

	// .well-known/openid-configuration endpoint
	mux.HandleFunc("/.well-known/openid-configuration", m.handleConfiguration)

	// Certs
	mux.HandleFunc("/protocol/openid-connect/certs", m.handleCerts)

	// Userinfo endpoint
	mux.HandleFunc("/protocol/openid-connect/userinfo", m.handleUserInfo)

	// token_endpoint
	mux.HandleFunc("/protocol/openid-connect/token", m.handleToken)

	m.server = httptest.NewServer(mux)
	return m
}

func (m *MockOIDCServer) handleConfiguration(w http.ResponseWriter, _ *http.Request) {
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
	err := json.NewEncoder(w).Encode(config)
	if err != nil {
		http.Error(w, "Failed to encode config", http.StatusInternalServerError)
		return
	}
}

func (m *MockOIDCServer) handleCerts(w http.ResponseWriter, _ *http.Request) {
	keys := map[string]interface{}{
		"keys": []map[string]interface{}{
			{
				"kid": m.jwk.KeyID,
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"n":   base64.RawURLEncoding.EncodeToString(m.jwk.Public().Key.(*rsa.PublicKey).N.Bytes()),
				"e":   "AQAB",
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(keys)
	if err != nil {
		http.Error(w, "Failed to encode keys", http.StatusInternalServerError)
		return
	}
}

func (m *MockOIDCServer) handleUserInfo(w http.ResponseWriter, r *http.Request) {
	// Parse bearer token from header
	token := r.Header.Get("Authorization")
	if token == "" {
		http.Error(w, "No token provided", http.StatusUnauthorized)
		return
	}
	token = strings.TrimPrefix(token, "Bearer ")
	if token == "invalid-token" {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(map[string]interface{}{
		"sub":            "test-user-id",
		"name":           "Test User",
		"email":          "test@example.com",
		"email_verified": true,
	})
	if err != nil {
		http.Error(w, "Failed to encode userinfo", http.StatusInternalServerError)
		return
	}
}

func (m *MockOIDCServer) handleToken(w http.ResponseWriter, r *http.Request) {
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

	// Create claims
	claims := map[string]interface{}{
		"iss": m.server.URL,
		"sub": "test-subject",
		"aud": "api",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}

	// Sign the claims
	builder := jwt.Signed(m.signer).Claims(claims)
	rawJWT, err := builder.CompactSerialize()
	if err != nil {
		http.Error(w, "Failed to sign token", http.StatusInternalServerError)
		return
	}

	// Create the token response manually to ensure all fields are included
	tokenResponse := struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		IDToken      string `json:"id_token"`
	}{
		AccessToken:  "test-access-token",
		TokenType:    "Bearer",
		RefreshToken: "test-refresh-token",
		ExpiresIn:    3600,
		IDToken:      rawJWT,
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(tokenResponse)
	if err != nil {
		http.Error(w, "Failed to encode token", http.StatusInternalServerError)
		return
	}
}

func (m *MockOIDCServer) Close() {
	if m.server != nil {
		m.server.Close()
	}
}

func (m *MockOIDCServer) URL() string {
	return m.server.URL
}
