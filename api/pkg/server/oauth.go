package server

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// setupOAuthRoutes configures the OAuth routes
func (s *HelixAPIServer) setupOAuthRoutes(r *mux.Router) {
	// OAuth provider management routes
	providerRouter := r.PathPrefix("/oauth/providers").Subrouter()
	providerRouter.HandleFunc("", system.DefaultWrapper(s.handleListOAuthProviders)).Methods("GET")
	providerRouter.HandleFunc("", system.DefaultWrapper(s.handleCreateOAuthProvider)).Methods("POST")
	providerRouter.HandleFunc("/{id}", system.DefaultWrapper(s.handleGetOAuthProvider)).Methods("GET")
	providerRouter.HandleFunc("/{id}", system.DefaultWrapper(s.handleUpdateOAuthProvider)).Methods("PUT")
	providerRouter.HandleFunc("/{id}", system.DefaultWrapper(s.handleDeleteOAuthProvider)).Methods("DELETE")

	// OAuth connection management routes
	connectionRouter := r.PathPrefix("/oauth/connections").Subrouter()
	connectionRouter.HandleFunc("", system.DefaultWrapper(s.handleListOAuthConnections)).Methods("GET")
	connectionRouter.HandleFunc("/{id}", system.DefaultWrapper(s.handleGetOAuthConnection)).Methods("GET")
	connectionRouter.HandleFunc("/{id}", system.DefaultWrapper(s.handleDeleteOAuthConnection)).Methods("DELETE")
	connectionRouter.HandleFunc("/{id}/refresh", system.DefaultWrapper(s.handleRefreshOAuthConnection)).Methods("POST")
	connectionRouter.HandleFunc("/{id}/test", system.DefaultWrapper(s.handleTestOAuthConnection)).Methods("GET")

	// OAuth flow routes (except callback which is registered in insecureRouter)
	flowRouter := r.PathPrefix("/oauth/flow").Subrouter()
	flowRouter.HandleFunc("/start/{provider_id}", system.DefaultWrapper(s.handleStartOAuthFlow)).Methods("GET")
}

// handleListOAuthProviders returns the list of available OAuth providers
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
func (s *HelixAPIServer) handleDeleteOAuthProvider(w http.ResponseWriter, r *http.Request) (interface{}, error) {
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
func (s *HelixAPIServer) handleGetOAuthConnection(w http.ResponseWriter, r *http.Request) (*types.OAuthConnection, error) {
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
func (s *HelixAPIServer) handleDeleteOAuthConnection(w http.ResponseWriter, r *http.Request) (interface{}, error) {
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

// handleRefreshOAuthConnection manually refreshes an OAuth connection
func (s *HelixAPIServer) handleRefreshOAuthConnection(w http.ResponseWriter, r *http.Request) (*types.OAuthConnection, error) {
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
				setTimeout(() => window.close(), 5000);
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
				setTimeout(() => window.close(), 5000);
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
		<p>This window will close automatically in a few seconds.</p>
		<script>
			// Send a message to the opener window
			window.opener && window.opener.postMessage({
				type: 'oauth-success', 
				connectionId: '%s',
				providerId: '%s'
			}, '*');
			
			// Close this window automatically after a short delay
			setTimeout(() => window.close(), 2000);
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
func (s *HelixAPIServer) handleTestOAuthConnection(w http.ResponseWriter, r *http.Request) (map[string]interface{}, error) {
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

	// Depending on the provider type, test the connection
	var result map[string]interface{}

	if provider.Type == types.OAuthProviderTypeGitHub {
		// Test GitHub connection
		result, err = s.oauthManager.TestGitHubConnection(ctx, &connectionWithProvider)
		if err != nil {
			return nil, fmt.Errorf("failed to test GitHub connection: %w", err)
		}
	} else {
		// For other providers, return a generic success response
		result = map[string]interface{}{
			"success": true,
			"message": "Connection is valid but testing is not implemented for this provider type",
		}
	}

	// Return the result
	return result, nil
}
