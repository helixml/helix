package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// Anthropic OAuth constants
const (
	// OAuth endpoints
	anthropicAuthorizeURL = "https://console.anthropic.com/oauth/authorize"
	anthropicTokenURL     = "https://console.anthropic.com/oauth/token"

	// Default scopes for Claude Code
	// org:create_api_key - allows creating API keys
	// user:profile - access user profile info
	// user:inference - allows making inference requests
	anthropicDefaultScopes = "org:create_api_key user:profile user:inference"

	// PKCE state expiration
	pkceStateExpiration = 10 * time.Minute

	// Well-known provider name for Anthropic
	anthropicOAuthProviderName = "Anthropic (Claude Code)"
)

// PKCEState stores PKCE challenge data during OAuth flow
type PKCEState struct {
	CodeVerifier string
	UserID       string
	RedirectURI  string
	ProviderID   string
	CreatedAt    time.Time
}

// oauthStateStore stores PKCE states in memory (keyed by state parameter)
// In production, this should use Redis or database storage
var (
	oauthStateStore     = make(map[string]*PKCEState)
	oauthStateStoreLock sync.RWMutex
)

// generateRandomString generates a cryptographically random string
func generateRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

// generatePKCEChallenge generates code_verifier and code_challenge for PKCE
func generatePKCEChallenge() (verifier string, challenge string, err error) {
	// Generate 32 random bytes for verifier (will be 43 chars base64url encoded)
	verifier, err = generateRandomString(32)
	if err != nil {
		return "", "", err
	}

	// Create S256 challenge
	hash := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(hash[:])

	return verifier, challenge, nil
}

// AnthropicOAuthStartResponse is returned when starting OAuth flow
type AnthropicOAuthStartResponse struct {
	AuthURL string `json:"auth_url"`
	State   string `json:"state"`
}

// ensureAnthropicOAuthProvider ensures the Anthropic OAuth provider exists in the database
// Returns the provider ID
func (apiServer *HelixAPIServer) ensureAnthropicOAuthProvider(ctx context.Context) (string, error) {
	clientID := apiServer.Cfg.Providers.Anthropic.OAuthClientID
	clientSecret := apiServer.Cfg.Providers.Anthropic.OAuthClientSecret

	if clientID == "" {
		return "", fmt.Errorf("Anthropic OAuth not configured")
	}

	// Check if provider already exists by looking for one with type anthropic
	providers, err := apiServer.Store.ListOAuthProviders(ctx, &store.ListOAuthProvidersQuery{
		Enabled: true,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list OAuth providers: %w", err)
	}

	for _, p := range providers {
		if p.Type == types.OAuthProviderTypeAnthropic {
			// Update credentials if changed
			if p.ClientID != clientID || p.ClientSecret != clientSecret {
				p.ClientID = clientID
				p.ClientSecret = clientSecret
				_, err = apiServer.Store.UpdateOAuthProvider(ctx, p)
				if err != nil {
					log.Warn().Err(err).Msg("failed to update Anthropic OAuth provider credentials")
				}
			}
			return p.ID, nil
		}
	}

	// Create new provider
	provider := &types.OAuthProvider{
		ID:           uuid.New().String(),
		Name:         anthropicOAuthProviderName,
		Description:  "Anthropic OAuth for Claude Code integration",
		Type:         types.OAuthProviderTypeAnthropic,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		AuthURL:      anthropicAuthorizeURL,
		TokenURL:     anthropicTokenURL,
		Scopes:       strings.Split(anthropicDefaultScopes, " "),
		CreatorID:    "system",
		CreatorType:  types.OwnerTypeSystem,
		Enabled:      true,
	}

	created, err := apiServer.Store.CreateOAuthProvider(ctx, provider)
	if err != nil {
		return "", fmt.Errorf("failed to create Anthropic OAuth provider: %w", err)
	}

	log.Info().Str("provider_id", created.ID).Msg("created Anthropic OAuth provider")
	return created.ID, nil
}

// startAnthropicOAuth godoc
// @Summary Start Anthropic OAuth flow
// @Description Initiates OAuth flow with Anthropic to connect Claude subscription
// @Tags Authentication
// @Produce json
// @Param redirect_uri query string false "Redirect URI after OAuth completion"
// @Success 200 {object} AnthropicOAuthStartResponse
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/auth/anthropic/authorize [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) startAnthropicOAuth(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(rw, "unauthorized", http.StatusUnauthorized)
		return
	}

	ctx := r.Context()

	// Ensure provider exists and get ID
	providerID, err := apiServer.ensureAnthropicOAuthProvider(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to ensure Anthropic OAuth provider")
		http.Error(rw, err.Error(), http.StatusServiceUnavailable)
		return
	}

	// Get OAuth client ID from config
	clientID := apiServer.Cfg.Providers.Anthropic.OAuthClientID

	// Generate PKCE challenge
	codeVerifier, codeChallenge, err := generatePKCEChallenge()
	if err != nil {
		log.Error().Err(err).Msg("failed to generate PKCE challenge")
		http.Error(rw, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Generate state parameter
	state, err := generateRandomString(16)
	if err != nil {
		log.Error().Err(err).Msg("failed to generate state")
		http.Error(rw, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Determine redirect URI
	redirectURI := r.URL.Query().Get("redirect_uri")
	if redirectURI == "" {
		// Default to current host callback endpoint
		scheme := "https"
		if r.TLS == nil && !strings.HasPrefix(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "http"
		}
		host := r.Host
		if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
			host = fwdHost
		}
		redirectURI = fmt.Sprintf("%s://%s/api/v1/auth/anthropic/callback", scheme, host)
	}

	// Store PKCE state
	oauthStateStoreLock.Lock()
	oauthStateStore[state] = &PKCEState{
		CodeVerifier: codeVerifier,
		UserID:       user.ID,
		RedirectURI:  redirectURI,
		ProviderID:   providerID,
		CreatedAt:    time.Now(),
	}
	oauthStateStoreLock.Unlock()

	// Build authorization URL
	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", anthropicDefaultScopes)
	params.Set("state", state)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")

	authURL := anthropicAuthorizeURL + "?" + params.Encode()

	writeResponse(rw, &AnthropicOAuthStartResponse{
		AuthURL: authURL,
		State:   state,
	}, http.StatusOK)
}

// AnthropicTokenResponse is the response from Anthropic token endpoint
type AnthropicTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// handleAnthropicOAuthCallback godoc
// @Summary Handle Anthropic OAuth callback
// @Description Handles OAuth callback from Anthropic after user authorization
// @Tags Authentication
// @Produce html
// @Param code query string true "Authorization code"
// @Param state query string true "State parameter"
// @Success 302 {string} string "Redirect to success page"
// @Failure 400 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/auth/anthropic/callback [get]
func (apiServer *HelixAPIServer) handleAnthropicOAuthCallback(rw http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errorParam := r.URL.Query().Get("error")

	if errorParam != "" {
		errorDesc := r.URL.Query().Get("error_description")
		log.Warn().Str("error", errorParam).Str("description", errorDesc).Msg("OAuth error from Anthropic")
		http.Error(rw, fmt.Sprintf("OAuth error: %s - %s", errorParam, errorDesc), http.StatusBadRequest)
		return
	}

	if code == "" || state == "" {
		http.Error(rw, "Missing code or state parameter", http.StatusBadRequest)
		return
	}

	// Look up and validate state
	oauthStateStoreLock.Lock()
	pkceState, exists := oauthStateStore[state]
	if exists {
		delete(oauthStateStore, state)
	}
	oauthStateStoreLock.Unlock()

	if !exists {
		http.Error(rw, "Invalid or expired state parameter", http.StatusBadRequest)
		return
	}

	// Check expiration
	if time.Since(pkceState.CreatedAt) > pkceStateExpiration {
		http.Error(rw, "OAuth session expired", http.StatusBadRequest)
		return
	}

	// Exchange code for tokens
	clientID := apiServer.Cfg.Providers.Anthropic.OAuthClientID
	clientSecret := apiServer.Cfg.Providers.Anthropic.OAuthClientSecret

	tokenResp, err := exchangeAnthropicCode(code, pkceState.CodeVerifier, pkceState.RedirectURI, clientID, clientSecret)
	if err != nil {
		log.Error().Err(err).Msg("failed to exchange OAuth code")
		http.Error(rw, "Failed to exchange authorization code", http.StatusInternalServerError)
		return
	}

	// Store tokens in OAuthConnection
	ctx := r.Context()

	// Calculate expiry time
	var expiresAt time.Time
	if tokenResp.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	// Parse scopes
	var scopes []string
	if tokenResp.Scope != "" {
		scopes = strings.Split(tokenResp.Scope, " ")
	} else {
		scopes = strings.Split(anthropicDefaultScopes, " ")
	}

	// Check if connection already exists
	existingConn, err := apiServer.Store.GetOAuthConnectionByUserAndProvider(ctx, pkceState.UserID, pkceState.ProviderID)
	if err == nil && existingConn != nil {
		// Update existing connection
		existingConn.AccessToken = tokenResp.AccessToken
		existingConn.RefreshToken = tokenResp.RefreshToken
		existingConn.ExpiresAt = expiresAt
		existingConn.Scopes = scopes

		_, err = apiServer.Store.UpdateOAuthConnection(ctx, existingConn)
		if err != nil {
			log.Error().Err(err).Str("user_id", pkceState.UserID).Msg("failed to update OAuth connection")
			http.Error(rw, "Failed to store credentials", http.StatusInternalServerError)
			return
		}
		log.Info().Str("user_id", pkceState.UserID).Str("connection_id", existingConn.ID).Msg("updated Anthropic OAuth connection")
	} else {
		// Create new connection
		connection := &types.OAuthConnection{
			ID:           uuid.New().String(),
			UserID:       pkceState.UserID,
			ProviderID:   pkceState.ProviderID,
			AccessToken:  tokenResp.AccessToken,
			RefreshToken: tokenResp.RefreshToken,
			ExpiresAt:    expiresAt,
			Scopes:       scopes,
		}

		_, err = apiServer.Store.CreateOAuthConnection(ctx, connection)
		if err != nil {
			log.Error().Err(err).Str("user_id", pkceState.UserID).Msg("failed to create OAuth connection")
			http.Error(rw, "Failed to store credentials", http.StatusInternalServerError)
			return
		}
		log.Info().Str("user_id", pkceState.UserID).Str("connection_id", connection.ID).Msg("created Anthropic OAuth connection")
	}

	// Redirect to success page or close popup
	successHTML := `<!DOCTYPE html>
<html>
<head>
    <title>Claude Connected</title>
    <style>
        body { font-family: system-ui, sans-serif; display: flex; align-items: center; justify-content: center; height: 100vh; margin: 0; background: #1a1a2e; color: #eee; }
        .container { text-align: center; padding: 2rem; }
        h1 { color: #4CAF50; margin-bottom: 1rem; }
        p { color: #aaa; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Claude Connected Successfully</h1>
        <p>You can close this window and return to Helix.</p>
        <script>
            // Notify parent window and close
            if (window.opener) {
                window.opener.postMessage({ type: 'anthropic-oauth-success' }, '*');
                setTimeout(() => window.close(), 1500);
            }
        </script>
    </div>
</body>
</html>`

	rw.Header().Set("Content-Type", "text/html")
	rw.WriteHeader(http.StatusOK)
	rw.Write([]byte(successHTML))
}

// exchangeAnthropicCode exchanges authorization code for tokens
func exchangeAnthropicCode(code, codeVerifier, redirectURI, clientID, clientSecret string) (*AnthropicTokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("code_verifier", codeVerifier)
	data.Set("client_id", clientID)
	if clientSecret != "" {
		data.Set("client_secret", clientSecret)
	}

	req, err := http.NewRequest("POST", anthropicTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed: %s - %s", resp.Status, string(body))
	}

	var tokenResp AnthropicTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &tokenResp, nil
}

// RefreshAnthropicToken refreshes an expired OAuth token
func RefreshAnthropicToken(refreshToken, clientID, clientSecret string) (*AnthropicTokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", clientID)
	if clientSecret != "" {
		data.Set("client_secret", clientSecret)
	}

	req, err := http.NewRequest("POST", anthropicTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token refresh failed: %s - %s", resp.Status, string(body))
	}

	var tokenResp AnthropicTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &tokenResp, nil
}

// cleanupExpiredPKCEStates removes expired PKCE states from memory
func cleanupExpiredPKCEStates() {
	oauthStateStoreLock.Lock()
	defer oauthStateStoreLock.Unlock()

	now := time.Now()
	for state, pkce := range oauthStateStore {
		if now.Sub(pkce.CreatedAt) > pkceStateExpiration {
			delete(oauthStateStore, state)
		}
	}
}

func init() {
	// Start background cleanup goroutine
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		for range ticker.C {
			cleanupExpiredPKCEStates()
		}
	}()
}
