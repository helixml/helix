package server

import (
	"encoding/json"
	"fmt"
	"net/http"

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

	// OAuth flow routes (except callback which is registered in insecureRouter)
	flowRouter := r.PathPrefix("/oauth/flow").Subrouter()
	flowRouter.HandleFunc("/start/{provider_id}", system.DefaultWrapper(s.handleStartOAuthFlow)).Methods("GET")
}

// handleListOAuthProviders returns the list of available OAuth providers
func (s *HelixAPIServer) handleListOAuthProviders(w http.ResponseWriter, r *http.Request) ([]*types.OAuthProvider, error) {
	user := getRequestUser(r)

	// Only admin users can list all providers
	var providers []*types.OAuthProvider
	var err error

	if user.Admin {
		providers, err = s.Store.ListOAuthProviders(r.Context(), nil)
	} else {
		// Regular users can only see enabled providers
		providers, err = s.Store.ListOAuthProviders(r.Context(), &store.ListOAuthProvidersQuery{
			Enabled: true,
		})
	}

	if err != nil {
		return nil, fmt.Errorf("error listing providers: %w", err)
	}

	// Remove sensitive information for non-admin users
	if !user.Admin {
		for _, provider := range providers {
			provider.ClientSecret = ""
			provider.PrivateKey = ""
		}
	}

	return providers, nil
}

// handleCreateOAuthProvider creates a new OAuth provider
func (s *HelixAPIServer) handleCreateOAuthProvider(w http.ResponseWriter, r *http.Request) (*types.OAuthProvider, error) {
	user := getRequestUser(r)

	// Only admin users can create providers
	if !user.Admin {
		return nil, fmt.Errorf("unauthorized")
	}

	// Parse the provider details from the request body
	var provider types.OAuthProvider
	if err := json.NewDecoder(r.Body).Decode(&provider); err != nil {
		return nil, fmt.Errorf("error decoding request: %w", err)
	}

	// Set the creator information
	provider.CreatorID = user.ID
	provider.CreatorType = types.OwnerTypeUser

	// Create the provider
	result, err := s.Store.CreateOAuthProvider(r.Context(), &provider)
	if err != nil {
		return nil, fmt.Errorf("error creating provider: %w", err)
	}

	return result, nil
}

// handleListOAuthConnections returns the user's OAuth connections
func (s *HelixAPIServer) handleListOAuthConnections(w http.ResponseWriter, r *http.Request) ([]*types.OAuthConnection, error) {
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
		connection.TokenSecret = ""
	}

	return connections, nil
}

// handleStartOAuthFlow initiates the OAuth flow
func (s *HelixAPIServer) handleStartOAuthFlow(w http.ResponseWriter, r *http.Request) (map[string]string, error) {
	user := getRequestUser(r)

	// Extract the provider ID from the URL
	vars := mux.Vars(r)
	providerID := vars["provider_id"]

	// Get the redirect URL from the query parameters
	redirectURL := r.URL.Query().Get("redirect_url")

	// Start the OAuth flow
	authURL, err := s.oauthManager.StartOAuthFlow(r.Context(), user.ID, providerID, redirectURL)
	if err != nil {
		return nil, fmt.Errorf("error starting OAuth flow: %w", err)
	}

	// Return the authorization URL
	return map[string]string{
		"auth_url": authURL,
	}, nil
}

// handleGetOAuthProvider returns a specific OAuth provider by ID
func (s *HelixAPIServer) handleGetOAuthProvider(w http.ResponseWriter, r *http.Request) (*types.OAuthProvider, error) {
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
		provider.PrivateKey = ""
	}

	return provider, nil
}

// handleUpdateOAuthProvider updates an existing OAuth provider
func (s *HelixAPIServer) handleUpdateOAuthProvider(w http.ResponseWriter, r *http.Request) (*types.OAuthProvider, error) {
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
		connection.TokenSecret = ""
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
		connection.TokenSecret = ""
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
		http.Error(w, fmt.Sprintf("OAuth error: %s", errorMsg), http.StatusBadRequest)
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
		http.Error(w, fmt.Sprintf("Error completing OAuth flow: %v", err), http.StatusInternalServerError)
		return
	}

	// Set a custom success page
	htmlResponse := fmt.Sprintf("<html><body><h1>Connection Successful</h1><p>You can close this window now.</p><script>window.opener && window.opener.postMessage({type: 'oauth-success', connectionId: '%s'}, '*'); setTimeout(() => window.close(), 2000);</script></body></html>", connection.ID)

	w.Header().Set("Content-Type", "text/html")
	// Use http.Error for any write errors - even though we've already set the header
	// This ensures the error is logged properly and won't cause linter warnings
	if _, err = w.Write([]byte(htmlResponse)); err != nil {
		log.Error().Err(err).Msg("error writing OAuth callback response")
	}
}
