package server

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/sharepoint"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// setupOAuthRoutes configures the OAuth routes
func (s *HelixAPIServer) setupOAuthRoutes(r *mux.Router) {
	// OAuth provider management routes
	r.HandleFunc("/oauth/providers", system.DefaultWrapper(s.handleListOAuthProviders)).Methods("GET")
	r.HandleFunc("/oauth/providers", system.DefaultWrapper(s.handleCreateOAuthProvider)).Methods("POST")
	r.HandleFunc("/oauth/providers/{id}", system.DefaultWrapper(s.handleGetOAuthProvider)).Methods("GET")
	r.HandleFunc("/oauth/providers/{id}", system.DefaultWrapper(s.handleUpdateOAuthProvider)).Methods("PUT")
	r.HandleFunc("/oauth/providers/{id}", system.DefaultWrapper(s.handleDeleteOAuthProvider)).Methods("DELETE")

	// OAuth connection management routes
	r.HandleFunc("/oauth/connections", system.DefaultWrapper(s.handleListOAuthConnections)).Methods("GET")
	r.HandleFunc("/oauth/connections/{id}", system.DefaultWrapper(s.handleGetOAuthConnection)).Methods("GET")
	r.HandleFunc("/oauth/connections/{id}", system.DefaultWrapper(s.handleDeleteOAuthConnection)).Methods("DELETE")
	r.HandleFunc("/oauth/connections/{id}/refresh", system.DefaultWrapper(s.handleRefreshOAuthConnection)).Methods("POST")
	r.HandleFunc("/oauth/connections/{id}/test", system.DefaultWrapper(s.handleTestOAuthConnection)).Methods("GET")

	// OAuth flow routes (except callback which is registered in insecureRouter)
	r.HandleFunc("/oauth/flow/start/{provider_id}", system.DefaultWrapper(s.handleStartOAuthFlow)).Methods("GET")

	// SharePoint helper routes
	r.HandleFunc("/oauth/sharepoint/resolve-site", system.DefaultWrapper(s.handleResolveSharePointSite)).Methods("POST")
}

// handleListOAuthProviders returns the list of available OAuth providers
// listOAuthProviders godoc
// @Summary List OAuth providers
// @Description List OAuth providers for the user.
// @Tags    oauth
// @Success 200 {array} types.OAuthProvider
// @Router /api/v1/oauth/providers [get]
// @Security BearerAuth
func (s *HelixAPIServer) handleListOAuthProviders(_ http.ResponseWriter, r *http.Request) ([]*types.OAuthProvider, error) {
	user := getRequestUser(r)

	// Only admin users can list all providers
	var providers []*types.OAuthProvider
	var err error

	log.Debug().
		Str("user_id", user.ID).
		Bool("is_admin", user.Admin).
		Msg("Listing OAuth providers")

	if user.Admin {
		providers, err = s.Store.ListOAuthProviders(r.Context(), nil)
	} else {
		// Regular users can only see enabled providers
		providers, err = s.Store.ListOAuthProviders(r.Context(), &store.ListOAuthProvidersQuery{
			Enabled: true,
		})
	}

	if err != nil {
		log.Error().Err(err).Msg("Error listing OAuth providers")
		return nil, fmt.Errorf("error listing providers: %w", err)
	}

	log.Info().
		Int("count", len(providers)).
		Bool("is_admin", user.Admin).
		Msg("Retrieved OAuth providers")

	for i, provider := range providers {
		log.Debug().
			Int("index", i).
			Str("id", provider.ID).
			Str("name", provider.Name).
			Str("type", string(provider.Type)).
			Bool("enabled", provider.Enabled).
			Msg("Provider details")
	}

	// Remove sensitive information for non-admin users
	if !user.Admin {
		for _, provider := range providers {
			provider.ClientSecret = ""
		}
	}

	return providers, nil
}

// handleCreateOAuthProvider creates a new OAuth provider
// createOAuthProvider godoc
// @Summary Create a new OAuth provider
// @Description Create a new OAuth provider for the user.
// @Tags    oauth
// @Param request body types.OAuthProvider true "Request body with OAuth provider configuration."
// @Success 200 {object} types.OAuthProvider
// @Router /api/v1/oauth/providers [post]
// @Security BearerAuth
func (s *HelixAPIServer) handleCreateOAuthProvider(_ http.ResponseWriter, r *http.Request) (*types.OAuthProvider, error) {
	user := getRequestUser(r)

	// Only admin users can create providers
	if !user.Admin {
		return nil, fmt.Errorf("unauthorized")
	}

	// Parse the provider details from the request body
	var provider types.OAuthProvider
	if err := json.NewDecoder(r.Body).Decode(&provider); err != nil {
		log.Error().Err(err).Msg("Failed to decode OAuth provider from request body")
		return nil, fmt.Errorf("error decoding request: %w", err)
	}

	log.Info().
		Str("name", provider.Name).
		Str("type", string(provider.Type)).
		Str("user_id", user.ID).
		Msg("Creating new OAuth provider")

	// Set the creator information
	provider.CreatorID = user.ID
	provider.CreatorType = types.OwnerTypeUser

	// Create the provider
	result, err := s.Store.CreateOAuthProvider(r.Context(), &provider)
	if err != nil {
		log.Error().Err(err).
			Str("name", provider.Name).
			Str("type", string(provider.Type)).
			Msg("Failed to create OAuth provider")
		return nil, fmt.Errorf("error creating provider: %w", err)
	}

	log.Info().
		Str("id", result.ID).
		Str("name", result.Name).
		Str("type", string(result.Type)).
		Bool("enabled", result.Enabled).
		Msg("Successfully created OAuth provider")

	return result, nil
}

// handleListOAuthConnections returns the list of OAuth connections for the current user
// listOAuthConnections godoc
// @Summary List OAuth connections
// @Description List OAuth connections for the user.
// @Tags    oauth
// @Success 200 {array} types.OAuthConnection
// @Router /api/v1/oauth/connections [get]
// @Security BearerAuth
func (s *HelixAPIServer) handleListOAuthConnections(_ http.ResponseWriter, r *http.Request) ([]*types.OAuthConnection, error) {
	user := getRequestUser(r)

	// Get the user's connections
	connections, err := s.Store.ListOAuthConnections(r.Context(), &store.ListOAuthConnectionsQuery{
		UserID: user.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("error listing connections: %w", err)
	}

	// Remove sensitive information for all connections
	for _, connection := range connections {
		connection.AccessToken = ""
		connection.RefreshToken = ""
	}

	return connections, nil
}

// handleStartOAuthFlow initiates the OAuth flow
func (s *HelixAPIServer) handleStartOAuthFlow(_ http.ResponseWriter, r *http.Request) (map[string]string, error) {
	user := getRequestUser(r)

	// Extract the provider ID from the URL
	vars := mux.Vars(r)
	providerID := vars["provider_id"]

	log.Debug().Str("provider_id", providerID).Str("user_id", user.ID).Msg("Starting OAuth flow")

	// Get the redirect URL from the query parameters
	redirectURL := r.URL.Query().Get("redirect_url")
	if redirectURL == "" {
		log.Debug().Msg("No redirect URL provided, using default")
	} else {
		log.Debug().Str("redirect_url", redirectURL).Msg("Using provided redirect URL")
	}

	// Start the OAuth flow
	authURL, err := s.oauthManager.StartOAuthFlow(r.Context(), user.ID, providerID, redirectURL)
	if err != nil {
		log.Error().Err(err).Str("provider_id", providerID).Str("user_id", user.ID).Msg("Failed to start OAuth flow")
		return nil, fmt.Errorf("error starting OAuth flow: %w", err)
	}

	log.Debug().Str("provider_id", providerID).Str("user_id", user.ID).Str("auth_url", authURL).Msg("Successfully generated OAuth authorization URL")

	// Return the authorization URL
	return map[string]string{
		"auth_url": authURL,
	}, nil
}

// handleGetOAuthProvider returns a specific OAuth provider by ID
func (s *HelixAPIServer) handleGetOAuthProvider(_ http.ResponseWriter, r *http.Request) (*types.OAuthProvider, error) {
	user := getRequestUser(r)

	// Extract the provider ID from the URL
	vars := mux.Vars(r)
	providerID := vars["id"]

	provider, err := s.Store.GetOAuthProvider(r.Context(), providerID)
	if err != nil {
		return nil, fmt.Errorf("error getting provider: %w", err)
	}

	// Non-admin users can't see sensitive information
	if !user.Admin {
		if !provider.Enabled {
			return nil, fmt.Errorf("provider not found")
		}

		provider.ClientSecret = ""
	}

	return provider, nil
}

// handleUpdateOAuthProvider updates an existing OAuth provider
// updateOAuthProvider godoc
// @Summary Update an OAuth provider
// @Description Update an existing OAuth provider for the user.
// @Tags    oauth
// @Param   id path     string  true  "Provider ID"
// @Param   request body types.OAuthProvider true "Request body with OAuth provider configuration."
// @Success 200 {object} types.OAuthProvider
func (s *HelixAPIServer) handleUpdateOAuthProvider(_ http.ResponseWriter, r *http.Request) (*types.OAuthProvider, error) {
	user := getRequestUser(r)

	// Only admin users can update providers
	if !user.Admin {
		return nil, fmt.Errorf("unauthorized")
	}

	// Extract the provider ID from the URL
	vars := mux.Vars(r)
	providerID := vars["id"]

	// Get the existing provider
	existingProvider, err := s.Store.GetOAuthProvider(r.Context(), providerID)
	if err != nil {
		return nil, fmt.Errorf("error getting provider: %w", err)
	}

	// Parse the updated provider
	var updatedProvider types.OAuthProvider
	if err := json.NewDecoder(r.Body).Decode(&updatedProvider); err != nil {
		return nil, fmt.Errorf("error decoding request: %w", err)
	}

	// Ensure the ID matches
	if updatedProvider.ID != providerID {
		return nil, fmt.Errorf("provider ID mismatch")
	}

	// Preserve the original type - can't change provider type
	if updatedProvider.Type != existingProvider.Type {
		return nil, fmt.Errorf("cannot change provider type")
	}

	// Update the provider
	result, err := s.Store.UpdateOAuthProvider(r.Context(), &updatedProvider)
	if err != nil {
		return nil, fmt.Errorf("error updating provider: %w", err)
	}

	return result, nil
}

// handleDeleteOAuthProvider deletes an OAuth provider
// deleteOAuthProvider godoc
// @Summary Delete an OAuth provider
// @Description Delete an existing OAuth provider for the user.
// @Tags    oauth
// @Param   id path     string  true  "Provider ID"
// @Success 200
// @Router /api/v1/oauth/providers/{id} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) handleDeleteOAuthProvider(_ http.ResponseWriter, r *http.Request) (interface{}, error) {
	user := getRequestUser(r)

	// Only admin users can delete providers
	if !user.Admin {
		return nil, fmt.Errorf("unauthorized")
	}

	// Extract the provider ID from the URL
	vars := mux.Vars(r)
	providerID := vars["id"]

	// Delete the provider
	err := s.Store.DeleteOAuthProvider(r.Context(), providerID)
	if err != nil {
		return nil, fmt.Errorf("error deleting provider: %w", err)
	}

	return nil, nil
}

// handleGetOAuthConnection returns a specific OAuth connection
// getOAuthConnection godoc
// @Summary Get an OAuth connection
// @Description Get a specific OAuth connection by ID. Users can only access their own connections unless they are admin.
// @Tags    oauth
// @Param   id path     string  true  "Connection ID"
// @Success 200 {object} types.OAuthConnection
// @Router /api/v1/oauth/connections/{id} [get]
// @Security BearerAuth
func (s *HelixAPIServer) handleGetOAuthConnection(_ http.ResponseWriter, r *http.Request) (*types.OAuthConnection, error) {
	user := getRequestUser(r)

	// Extract the connection ID from the URL
	vars := mux.Vars(r)
	connectionID := vars["id"]

	// Get the connection
	connection, err := s.Store.GetOAuthConnection(r.Context(), connectionID)
	if err != nil {
		return nil, fmt.Errorf("error getting connection: %w", err)
	}

	// Check if the connection belongs to the user
	if connection.UserID != user.ID && !user.Admin {
		return nil, fmt.Errorf("unauthorized")
	}

	// Remove sensitive information for non-admin users
	if !user.Admin {
		connection.AccessToken = ""
		connection.RefreshToken = ""
	}

	return connection, nil
}

// handleDeleteOAuthConnection deletes an OAuth connection
// deleteOAuthConnection godoc
// @Summary Delete an OAuth connection
// @Description Delete an OAuth connection. Users can only delete their own connections unless they are admin.
// @Tags    oauth
// @Param   id path     string  true  "Connection ID"
// @Success 200
// @Router /api/v1/oauth/connections/{id} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) handleDeleteOAuthConnection(_ http.ResponseWriter, r *http.Request) (interface{}, error) {
	user := getRequestUser(r)

	// Extract the connection ID from the URL
	vars := mux.Vars(r)
	connectionID := vars["id"]

	// Get the connection to check ownership
	connection, err := s.Store.GetOAuthConnection(r.Context(), connectionID)
	if err != nil {
		return nil, fmt.Errorf("error getting connection: %w", err)
	}

	// Check if the connection belongs to the user
	if connection.UserID != user.ID && !user.Admin {
		return nil, fmt.Errorf("unauthorized")
	}

	// Delete the connection
	err = s.Store.DeleteOAuthConnection(r.Context(), connectionID)
	if err != nil {
		return nil, fmt.Errorf("error deleting connection: %w", err)
	}

	return nil, nil
}

// @Summary Refresh an OAuth connection
// @Description Manually refresh an OAuth connection
// @Tags    oauth
// @Produce json
// @Param   id path     string  true  "Connection ID"
// @Success 200 {object} types.OAuthConnection
// @Router /api/v1/oauth/connections/{id}/refresh [post]
// @Security BearerAuth
func (s *HelixAPIServer) handleRefreshOAuthConnection(_ http.ResponseWriter, r *http.Request) (*types.OAuthConnection, error) {
	user := getRequestUser(r)

	// Extract the connection ID from the URL
	vars := mux.Vars(r)
	connectionID := vars["id"]

	// Get the connection to check ownership
	connection, err := s.Store.GetOAuthConnection(r.Context(), connectionID)
	if err != nil {
		return nil, fmt.Errorf("error getting connection: %w", err)
	}

	// Check if the connection belongs to the user
	if connection.UserID != user.ID && !user.Admin {
		return nil, fmt.Errorf("unauthorized")
	}

	// Refresh the connection
	err = s.oauthManager.RefreshConnection(r.Context(), connection)
	if err != nil {
		return nil, fmt.Errorf("error refreshing connection: %w", err)
	}

	// Get the updated connection
	connection, err = s.Store.GetOAuthConnection(r.Context(), connectionID)
	if err != nil {
		return nil, fmt.Errorf("error getting updated connection: %w", err)
	}

	// Remove sensitive information for non-admin users
	if !user.Admin {
		connection.AccessToken = ""
		connection.RefreshToken = ""
	}

	return connection, nil
}

// handleOAuthCallback handles the OAuth callback
func (s *HelixAPIServer) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	// Get the OAuth parameters from the query
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errorMsg := r.URL.Query().Get("error")

	log.Error().Str("code", code).Str("state", state).Str("error", errorMsg).Msg("OAuth callback failed")

	// Check for errors
	if errorMsg != "" {
		// Create a user-friendly error page based on the error type
		errorTitle := "Authentication Error"
		errorMessage := fmt.Sprintf("%v", html.EscapeString(errorMsg))
		errorColor := "#d32f2f" // Red by default

		// Check for specific error types
		if strings.Contains(errorMessage, "duplicate key") || strings.Contains(errorMessage, "unique constraint") {
			errorTitle = "Already Connected"
			errorMessage = "You are already connected to this service. You can manage your connections in your account settings."
			errorColor = "#ff9800" // Orange for warnings
		}

		htmlError := fmt.Sprintf(`<html>
		<head><title>%s</title>
		<style>
			body { font-family: Arial, sans-serif; margin: 40px; text-align: center; }
			h1 { color: %s; }
			p { font-size: 16px; }
			.close-button { background: %s; color: white; border: none; padding: 10px 20px; 
				margin-top: 20px; border-radius: 4px; cursor: pointer; }
		</style></head>
		<body>
			<h1>%s</h1>
			<p>%s</p>
			<button class="close-button" onclick="window.close()">Close Window</button>
			<script>
				window.opener && window.opener.postMessage({
					type: 'oauth-failure', 
					error: '%s'
				}, '*');
				// setTimeout(() => window.close(), 5000);
			</script>
		</body></html>`, errorTitle, errorColor, errorColor, errorTitle, errorMessage, errorMessage)

		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadRequest)
		if _, writeErr := w.Write([]byte(htmlError)); writeErr != nil {
			log.Error().Err(writeErr).Msg("error writing OAuth error response")
		}
		return
	}

	// Get the request token from the database
	requestTokens, err := s.Store.GetOAuthRequestTokenByState(r.Context(), state)
	if err != nil || len(requestTokens) == 0 {
		http.Error(w, "Invalid or expired state", http.StatusBadRequest)
		return
	}

	// Use the most recent request token
	requestToken := requestTokens[0]

	// Check if the user exists (we don't need the user object)
	_, err = s.Store.GetUser(r.Context(), &store.GetUserQuery{ID: requestToken.UserID})
	if err != nil {
		http.Error(w, "User not found", http.StatusInternalServerError)
		return
	}

	// Complete the OAuth flow
	connection, err := s.oauthManager.CompleteOAuthFlow(r.Context(), requestToken.UserID, requestToken.ProviderID, code)
	if err != nil {
		log.Error().
			Err(err).
			Str("provider_id", requestToken.ProviderID).
			Str("user_id", requestToken.UserID).
			Str("code", code).
			Str("state", state).
			Msg("OAuth callback failed")
		// Create a user-friendly error page based on the error type
		errorTitle := "Authentication Error"
		errorMessage := fmt.Sprintf("%v", err)
		errorColor := "#d32f2f" // Red by default

		// Check for specific error types
		if strings.Contains(errorMessage, "duplicate key") || strings.Contains(errorMessage, "unique constraint") {
			errorTitle = "Already Connected"
			errorMessage = "You are already connected to this service. You can manage your connections in your account settings."
			errorColor = "#ff9800" // Orange for warnings
		}

		htmlError := fmt.Sprintf(`<html>
		<head><title>%s</title>
		<style>
			body { font-family: Arial, sans-serif; margin: 40px; text-align: center; }
			h1 { color: %s; }
			p { font-size: 16px; }
			.close-button { background: %s; color: white; border: none; padding: 10px 20px; 
				margin-top: 20px; border-radius: 4px; cursor: pointer; }
		</style></head>
		<body>
			<h1>%s</h1>
			<p>%s</p>
			<button class="close-button" onclick="window.close()">Close Window</button>
			<script>
				window.opener && window.opener.postMessage({
					type: 'oauth-failure', 
					error: '%s'
				}, '*');
			</script>
		</body></html>`, errorTitle, errorColor, errorColor, errorTitle, errorMessage, errorMessage)

		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadRequest)
		if _, writeErr := w.Write([]byte(htmlError)); writeErr != nil {
			log.Error().Err(writeErr).Msg("error writing OAuth error response")
		}
		return
	}

	// Get the provider for a better success message
	provider, err := s.Store.GetOAuthProvider(r.Context(), requestToken.ProviderID)
	providerName := "service"
	if err == nil && provider != nil {
		providerName = provider.Name
	}

	// Set a custom success page with auto-close
	htmlResponse := fmt.Sprintf(`<html>
	<head><title>Connection Successful</title>
	<meta charset="UTF-8">
	<style>
		body { font-family: Arial, sans-serif; margin: 40px; text-align: center; }
		h1 { color: #4caf50; }
		p { font-size: 16px; }
		.icon { font-size: 64px; color: #4caf50; margin-bottom: 20px; }
	</style></head>
	<body>
		<div class="icon">&#10004;</div>
		<h1>Connection Successful</h1>
		<p>You have successfully connected to %s.</p>
		<p>You can now close this window.</p>
		<script>
			// Send a message to the opener window
			window.opener && window.opener.postMessage({
				type: 'oauth-success', 
				connectionId: '%s',
				providerId: '%s'
			}, '*');
		</script>
	</body></html>`, providerName, connection.ID, requestToken.ProviderID)

	w.Header().Set("Content-Type", "text/html")
	// Use http.Error for any write errors - even though we've already set the header
	// This ensures the error is logged properly and won't cause linter warnings
	if _, err = w.Write([]byte(htmlResponse)); err != nil {
		log.Error().Err(err).Msg("error writing OAuth callback response")
	}
}

// handleTestOAuthConnection tests an OAuth connection by making an API call to the provider
// testOAuthConnection godoc
// @Summary Test an OAuth connection
// @Description Test an OAuth connection by making an API call to the provider
// @Tags    oauth
// @Produce json
// @Param   id path     string  true  "Connection ID"
// @Success 200 {object} types.OAuthConnectionTestResult
func (s *HelixAPIServer) handleTestOAuthConnection(_ http.ResponseWriter, r *http.Request) (*types.OAuthConnectionTestResult, error) {
	ctx := r.Context()

	// Get the connection ID from the URL
	vars := mux.Vars(r)
	connectionID := vars["id"]
	if connectionID == "" {
		return nil, fmt.Errorf("connection ID is required")
	}

	// Get the user from the context
	user := getRequestUser(r)
	if user == nil {
		return nil, fmt.Errorf("unauthorized")
	}

	// Get the connection from the database
	connection, err := s.Store.GetOAuthConnection(ctx, connectionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	// Check if the connection belongs to the user
	if connection.UserID != user.ID {
		return nil, fmt.Errorf("unauthorized")
	}

	// Load the provider to populate the connection.Provider field
	provider, err := s.Store.GetOAuthProvider(ctx, connection.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}

	// Make a copy of the connection with the provider details
	connectionWithProvider := *connection
	connectionWithProvider.Provider = *provider

	switch provider.Type {
	case types.OAuthProviderTypeGitHub:
		// Test GitHub connection
		result, err := s.oauthManager.TestGitHubConnection(ctx, &connectionWithProvider)
		if err != nil {
			return nil, fmt.Errorf("failed to test GitHub connection: %w", err)
		}
		return &types.OAuthConnectionTestResult{
			Success:         true,
			Message:         "Connection is valid",
			ProviderDetails: result,
		}, nil
	default:
		// For other providers, return a generic success response
		return &types.OAuthConnectionTestResult{
			Success: true,
			Message: "Connection is valid but testing is not implemented for this provider type",
		}, nil
	}
}

// SharePointSiteResolveRequest is the request body for resolving a SharePoint site URL
type SharePointSiteResolveRequest struct {
	SiteURL    string `json:"site_url"`
	ProviderID string `json:"provider_id"`
}

// SharePointSiteResolveResponse is the response for resolving a SharePoint site URL
type SharePointSiteResolveResponse struct {
	SiteID      string `json:"site_id"`
	DisplayName string `json:"display_name"`
	WebURL      string `json:"web_url"`
}

// handleResolveSharePointSite resolves a SharePoint site URL to a site ID
// resolveSharePointSite godoc
// @Summary Resolve SharePoint site URL to site ID
// @Description Resolve a SharePoint site URL to its site ID using Microsoft Graph API
// @Tags    oauth
// @Accept  json
// @Produce json
// @Param   request body SharePointSiteResolveRequest true "Request body with site URL and provider ID"
// @Success 200 {object} SharePointSiteResolveResponse
// @Router /api/v1/oauth/sharepoint/resolve-site [post]
// @Security BearerAuth
func (s *HelixAPIServer) handleResolveSharePointSite(_ http.ResponseWriter, r *http.Request) (*SharePointSiteResolveResponse, error) {
	ctx := r.Context()
	user := getRequestUser(r)

	// Parse request body
	var req SharePointSiteResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, fmt.Errorf("invalid request body: %w", err)
	}

	// Validate request
	if req.SiteURL == "" {
		return nil, fmt.Errorf("site_url is required")
	}
	if req.ProviderID == "" {
		return nil, fmt.Errorf("provider_id is required")
	}

	// Validate URL format
	if !strings.Contains(req.SiteURL, "sharepoint.com") {
		return nil, fmt.Errorf("invalid SharePoint URL: must be a sharepoint.com URL (e.g., https://contoso.sharepoint.com/sites/MySite)")
	}

	// Get the OAuth connection for the user and provider
	connection, err := s.oauthManager.GetConnection(ctx, user.ID, req.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get OAuth connection: %w (have you connected to this OAuth provider?)", err)
	}

	// Create SharePoint client with the access token
	spClient := sharepoint.NewClient(connection.AccessToken, s.Cfg.Tools.TLSSkipVerify)

	// Resolve the site URL to a site object
	site, err := spClient.GetSiteByURL(ctx, req.SiteURL)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve SharePoint site: %w", err)
	}

	log.Info().
		Str("user_id", user.ID).
		Str("site_url", req.SiteURL).
		Str("site_id", site.ID).
		Str("site_name", site.DisplayName).
		Msg("Resolved SharePoint site URL to site ID")

	return &SharePointSiteResolveResponse{
		SiteID:      site.ID,
		DisplayName: site.DisplayName,
		WebURL:      site.WebURL,
	}, nil
}
