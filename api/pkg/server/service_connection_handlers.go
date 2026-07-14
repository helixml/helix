package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	azuredevops "github.com/helixml/helix/api/pkg/agent/skill/azure_devops"
	"github.com/helixml/helix/api/pkg/agent/skill/github"
	"github.com/helixml/helix/api/pkg/crypto"
	slackcore "github.com/helixml/helix/api/pkg/serviceconnection/slack"
	"github.com/helixml/helix/api/pkg/types"
)

// errNothingToValidate is returned by testServiceConnection for a
// connection that exposes no credential a test can probe — e.g. a REST
// slack_app holds only a signing secret, which has no API to check. It is
// NOT a failure: callers leave the connection untested (neutral status)
// instead of recording a false error or a misleading "ok".
var errNothingToValidate = errors.New("nothing to validate: a REST Slack app's signing secret can't be probed via the API — install the app into a workspace and test that connection")

// notifyServiceConnectionChange fires the optional post-mutation hook a
// subsystem may have registered, after a connection is created, updated
// (deleted=false) or deleted (deleted=true). This handler stays generic:
// it knows the connection types and their credentials but nothing about
// which subsystem reacts or why (helix-org reacts to slack_app changes,
// registered in registerHelixOrgRoutes). No-op when nothing is registered.
func (s *HelixAPIServer) notifyServiceConnectionChange(ctx context.Context, conn *types.ServiceConnection, deleted bool) {
	if s.onServiceConnectionChange != nil {
		s.onServiceConnectionChange(ctx, conn, deleted)
	}
}

// listServiceConnections returns all service connections for the organization
// @Summary List service connections
// @Description List all service connections (GitHub Apps, ADO Service Principals) for the organization
// @Tags service-connections
// @Produce json
// @Param organization_id query string false "Organization ID (optional, defaults to user's org)"
// @Success 200 {array} types.ServiceConnectionResponse
// @Failure 401 {object} types.APIError
// @Failure 403 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/service-connections [get]
// @Security BearerAuth
func (s *HelixAPIServer) listServiceConnections(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Only admins can manage service connections
	if !user.Admin {
		http.Error(w, "Forbidden: admin access required", http.StatusForbidden)
		return
	}

	organizationID := r.URL.Query().Get("organization_id")

	connections, err := s.Store.ListServiceConnections(r.Context(), organizationID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list service connections")
		http.Error(w, fmt.Sprintf("Failed to list connections: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// The admin panel manages deployment-global, admin-owned connections
	// (GitHub App, ADO, global Slack app). Org-scoped installs like
	// slack_workspace belong to an org and are managed in that org's own
	// settings, so when no org filter is given (the global view) drop any
	// connection that belongs to an org.
	if organizationID == "" {
		global := connections[:0]
		for _, c := range connections {
			if c.OrganizationID == "" {
				global = append(global, c)
			}
		}
		connections = global
	}

	// Convert to response objects (hide sensitive fields)
	responses := make([]*types.ServiceConnectionResponse, len(connections))
	for i, conn := range connections {
		responses[i] = conn.ToResponse()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responses)
}

// getServiceConnection returns a specific service connection
// @Summary Get service connection
// @Description Get a specific service connection by ID
// @Tags service-connections
// @Produce json
// @Param id path string true "Connection ID"
// @Success 200 {object} types.ServiceConnectionResponse
// @Failure 401 {object} types.APIError
// @Failure 403 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Router /api/v1/service-connections/{id} [get]
// @Security BearerAuth
func (s *HelixAPIServer) getServiceConnection(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if !user.Admin {
		http.Error(w, "Forbidden: admin access required", http.StatusForbidden)
		return
	}

	connectionID := mux.Vars(r)["id"]
	if connectionID == "" {
		http.Error(w, "Connection ID is required", http.StatusBadRequest)
		return
	}

	connection, err := s.Store.GetServiceConnection(r.Context(), connectionID)
	if err != nil {
		http.Error(w, "Connection not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(connection.ToResponse())
}

// createServiceConnection creates a new service connection
// @Summary Create service connection
// @Description Create a new service connection (GitHub App or ADO Service Principal)
// @Tags service-connections
// @Accept json
// @Produce json
// @Param request body types.ServiceConnectionCreateRequest true "Connection details"
// @Success 201 {object} types.ServiceConnectionResponse
// @Failure 400 {object} types.APIError
// @Failure 401 {object} types.APIError
// @Failure 403 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/service-connections [post]
// @Security BearerAuth
func (s *HelixAPIServer) createServiceConnection(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if !user.Admin {
		http.Error(w, "Forbidden: admin access required", http.StatusForbidden)
		return
	}

	var req types.ServiceConnectionCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %s", err.Error()), http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	if req.Type == "" {
		http.Error(w, "Connection type is required", http.StatusBadRequest)
		return
	}

	// Validate based on connection type
	var providerType types.ExternalRepositoryType
	switch req.Type {
	case types.ServiceConnectionTypeGitHubApp:
		if req.GitHubAppID == 0 {
			http.Error(w, "GitHub App ID is required", http.StatusBadRequest)
			return
		}
		if req.GitHubInstallationID == 0 {
			http.Error(w, "GitHub Installation ID is required", http.StatusBadRequest)
			return
		}
		if req.GitHubPrivateKey == "" {
			http.Error(w, "GitHub Private Key is required", http.StatusBadRequest)
			return
		}
		providerType = types.ExternalRepositoryTypeGitHub

	case types.ServiceConnectionTypeADOServicePrincipal:
		if req.ADOOrganizationURL == "" {
			http.Error(w, "Azure DevOps Organization URL is required", http.StatusBadRequest)
			return
		}
		if req.ADOTenantID == "" {
			http.Error(w, "Azure AD Tenant ID is required", http.StatusBadRequest)
			return
		}
		if req.ADOClientID == "" {
			http.Error(w, "Azure AD Client ID is required", http.StatusBadRequest)
			return
		}
		if req.ADOClientSecret == "" {
			http.Error(w, "Azure AD Client Secret is required", http.StatusBadRequest)
			return
		}
		providerType = types.ExternalRepositoryTypeADO

	case types.ServiceConnectionTypeSlackApp:
		// The global Slack app. REST mode needs client id/secret +
		// signing secret; Socket Mode needs app token + bot token. We
		// don't hard-require a particular subset here — the operator may
		// fill REST now and Socket later — but at least one credential
		// must be present so an empty row can't masquerade as configured.
		if req.SlackClientID == "" && req.SlackSigningSecret == "" && req.SlackAppToken == "" && req.SlackBotToken == "" {
			http.Error(w, "Slack app requires at least one of: client id/signing secret (REST) or app/bot token (Socket Mode)", http.StatusBadRequest)
			return
		}
		// Slack isn't a git provider — leave ProviderType unset; the
		// connection Type (slack_app) already identifies it.

	default:
		http.Error(w, fmt.Sprintf("Unsupported connection type: %s", req.Type), http.StatusBadRequest)
		return
	}

	// Test the connection before saving.
	testErr := s.testServiceConnection(r.Context(), req)
	var lastError string
	var lastTestedAt *time.Time
	switch {
	case errors.Is(testErr, errNothingToValidate):
		// Nothing to probe (e.g. a REST slack_app) — leave it untested so
		// the UI shows a neutral status, not a false error or "ok".
	case testErr != nil:
		now := time.Now()
		lastTestedAt = &now
		lastError = testErr.Error()
	default:
		now := time.Now()
		lastTestedAt = &now
	}

	// Get encryption key
	encryptionKey, err := s.getEncryptionKey()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get encryption key")
		http.Error(w, "Failed to encrypt credentials", http.StatusInternalServerError)
		return
	}

	// Create connection with encrypted sensitive fields
	connection := &types.ServiceConnection{
		ID:             uuid.New().String(),
		OrganizationID: "", // Can be set based on user's org if needed
		Name:           req.Name,
		Description:    req.Description,
		Type:           req.Type,
		ProviderType:   providerType,
		BaseURL:        req.BaseURL,
		LastTestedAt:   lastTestedAt,
		LastError:      lastError,
	}

	// Encrypt and store GitHub App credentials
	if req.Type == types.ServiceConnectionTypeGitHubApp {
		connection.GitHubAppID = req.GitHubAppID
		connection.GitHubInstallationID = req.GitHubInstallationID
		encryptedKey, err := crypto.EncryptAES256GCM([]byte(req.GitHubPrivateKey), encryptionKey)
		if err != nil {
			log.Error().Err(err).Msg("Failed to encrypt GitHub private key")
			http.Error(w, "Failed to encrypt credentials", http.StatusInternalServerError)
			return
		}
		connection.GitHubPrivateKey = encryptedKey
	}

	// Encrypt and store ADO Service Principal credentials
	if req.Type == types.ServiceConnectionTypeADOServicePrincipal {
		connection.ADOOrganizationURL = req.ADOOrganizationURL
		connection.ADOTenantID = req.ADOTenantID
		connection.ADOClientID = req.ADOClientID
		encryptedSecret, err := crypto.EncryptAES256GCM([]byte(req.ADOClientSecret), encryptionKey)
		if err != nil {
			log.Error().Err(err).Msg("Failed to encrypt ADO client secret")
			http.Error(w, "Failed to encrypt credentials", http.StatusInternalServerError)
			return
		}
		connection.ADOClientSecret = encryptedSecret
	}

	// Encrypt and store global Slack app credentials
	if req.Type == types.ServiceConnectionTypeSlackApp {
		connection.SlackClientID = req.SlackClientID
		connection.SlackIngressMode = req.SlackIngressMode
		for _, f := range []struct {
			val string
			dst *string
		}{
			{req.SlackClientSecret, &connection.SlackClientSecret},
			{req.SlackSigningSecret, &connection.SlackSigningSecret},
			{req.SlackAppToken, &connection.SlackAppToken},
			{req.SlackBotToken, &connection.SlackBotToken},
		} {
			if f.val == "" {
				continue
			}
			enc, err := crypto.EncryptAES256GCM([]byte(f.val), encryptionKey)
			if err != nil {
				log.Error().Err(err).Msg("Failed to encrypt slack credential")
				http.Error(w, "Failed to encrypt credentials", http.StatusInternalServerError)
				return
			}
			*f.dst = enc
		}
	}

	if err := s.Store.CreateServiceConnection(r.Context(), connection); err != nil {
		log.Error().Err(err).Msg("Failed to create service connection")
		http.Error(w, fmt.Sprintf("Failed to create connection: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	s.notifyServiceConnectionChange(r.Context(), connection, false)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(connection.ToResponse())
}

// updateServiceConnection updates a service connection
// @Summary Update service connection
// @Description Update a service connection
// @Tags service-connections
// @Accept json
// @Produce json
// @Param id path string true "Connection ID"
// @Param request body types.ServiceConnectionUpdateRequest true "Connection details"
// @Success 200 {object} types.ServiceConnectionResponse
// @Failure 400 {object} types.APIError
// @Failure 401 {object} types.APIError
// @Failure 403 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/service-connections/{id} [put]
// @Security BearerAuth
func (s *HelixAPIServer) updateServiceConnection(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if !user.Admin {
		http.Error(w, "Forbidden: admin access required", http.StatusForbidden)
		return
	}

	connectionID := mux.Vars(r)["id"]
	if connectionID == "" {
		http.Error(w, "Connection ID is required", http.StatusBadRequest)
		return
	}

	connection, err := s.Store.GetServiceConnection(r.Context(), connectionID)
	if err != nil {
		http.Error(w, "Connection not found", http.StatusNotFound)
		return
	}

	var req types.ServiceConnectionUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %s", err.Error()), http.StatusBadRequest)
		return
	}

	// Update fields if provided
	if req.Name != nil {
		connection.Name = *req.Name
	}
	if req.Description != nil {
		connection.Description = *req.Description
	}
	if req.BaseURL != nil {
		connection.BaseURL = *req.BaseURL
	}

	encryptionKey, err := s.getEncryptionKey()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get encryption key")
		http.Error(w, "Failed to encrypt credentials", http.StatusInternalServerError)
		return
	}

	// Update GitHub App fields if provided
	if req.GitHubAppID != nil {
		connection.GitHubAppID = *req.GitHubAppID
	}
	if req.GitHubInstallationID != nil {
		connection.GitHubInstallationID = *req.GitHubInstallationID
	}
	if req.GitHubPrivateKey != nil && *req.GitHubPrivateKey != "" {
		encryptedKey, err := crypto.EncryptAES256GCM([]byte(*req.GitHubPrivateKey), encryptionKey)
		if err != nil {
			log.Error().Err(err).Msg("Failed to encrypt GitHub private key")
			http.Error(w, "Failed to encrypt credentials", http.StatusInternalServerError)
			return
		}
		connection.GitHubPrivateKey = encryptedKey
	}

	// Update ADO Service Principal fields if provided
	if req.ADOOrganizationURL != nil {
		connection.ADOOrganizationURL = *req.ADOOrganizationURL
	}
	if req.ADOTenantID != nil {
		connection.ADOTenantID = *req.ADOTenantID
	}
	if req.ADOClientID != nil {
		connection.ADOClientID = *req.ADOClientID
	}
	if req.ADOClientSecret != nil && *req.ADOClientSecret != "" {
		encryptedSecret, err := crypto.EncryptAES256GCM([]byte(*req.ADOClientSecret), encryptionKey)
		if err != nil {
			log.Error().Err(err).Msg("Failed to encrypt ADO client secret")
			http.Error(w, "Failed to encrypt credentials", http.StatusInternalServerError)
			return
		}
		connection.ADOClientSecret = encryptedSecret
	}

	// Update global Slack app fields if provided
	if req.SlackClientID != nil {
		connection.SlackClientID = *req.SlackClientID
	}
	if req.SlackIngressMode != nil {
		connection.SlackIngressMode = *req.SlackIngressMode
	}
	for _, f := range []struct {
		val *string
		dst *string
	}{
		{req.SlackClientSecret, &connection.SlackClientSecret},
		{req.SlackSigningSecret, &connection.SlackSigningSecret},
		{req.SlackAppToken, &connection.SlackAppToken},
		{req.SlackBotToken, &connection.SlackBotToken},
	} {
		if f.val == nil || *f.val == "" {
			continue
		}
		enc, err := crypto.EncryptAES256GCM([]byte(*f.val), encryptionKey)
		if err != nil {
			log.Error().Err(err).Msg("Failed to encrypt slack credential")
			http.Error(w, "Failed to encrypt credentials", http.StatusInternalServerError)
			return
		}
		*f.dst = enc
	}

	if err := s.Store.UpdateServiceConnection(r.Context(), connection); err != nil {
		log.Error().Err(err).Msg("Failed to update service connection")
		http.Error(w, fmt.Sprintf("Failed to update connection: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	s.notifyServiceConnectionChange(r.Context(), connection, false)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(connection.ToResponse())
}

// deleteServiceConnection deletes a service connection
// @Summary Delete service connection
// @Description Delete a service connection
// @Tags service-connections
// @Param id path string true "Connection ID"
// @Success 204
// @Failure 401 {object} types.APIError
// @Failure 403 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/service-connections/{id} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) deleteServiceConnection(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if !user.Admin {
		http.Error(w, "Forbidden: admin access required", http.StatusForbidden)
		return
	}

	connectionID := mux.Vars(r)["id"]
	if connectionID == "" {
		http.Error(w, "Connection ID is required", http.StatusBadRequest)
		return
	}

	// Verify connection exists
	conn, err := s.Store.GetServiceConnection(r.Context(), connectionID)
	if err != nil {
		http.Error(w, "Connection not found", http.StatusNotFound)
		return
	}

	if err := s.Store.DeleteServiceConnection(r.Context(), connectionID); err != nil {
		log.Error().Err(err).Str("connection_id", connectionID).Msg("Failed to delete service connection")
		http.Error(w, fmt.Sprintf("Failed to delete connection: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	s.notifyServiceConnectionChange(r.Context(), conn, true)

	w.WriteHeader(http.StatusNoContent)
}

// testServiceConnectionEndpoint tests a service connection
// @Summary Test service connection
// @Description Test a service connection by attempting to authenticate
// @Tags service-connections
// @Param id path string true "Connection ID"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} types.APIError
// @Failure 403 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/service-connections/{id}/test [post]
// @Security BearerAuth
func (s *HelixAPIServer) testServiceConnectionEndpoint(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if !user.Admin {
		http.Error(w, "Forbidden: admin access required", http.StatusForbidden)
		return
	}

	connectionID := mux.Vars(r)["id"]
	if connectionID == "" {
		http.Error(w, "Connection ID is required", http.StatusBadRequest)
		return
	}

	connection, err := s.Store.GetServiceConnection(r.Context(), connectionID)
	if err != nil {
		http.Error(w, "Connection not found", http.StatusNotFound)
		return
	}

	// Decrypt credentials for testing
	encryptionKey, err := s.getEncryptionKey()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get encryption key")
		http.Error(w, "Failed to decrypt credentials", http.StatusInternalServerError)
		return
	}

	// Build request for testing
	testReq := types.ServiceConnectionCreateRequest{
		Type:                 connection.Type,
		GitHubAppID:          connection.GitHubAppID,
		GitHubInstallationID: connection.GitHubInstallationID,
		ADOOrganizationURL:   connection.ADOOrganizationURL,
		ADOTenantID:          connection.ADOTenantID,
		ADOClientID:          connection.ADOClientID,
		BaseURL:              connection.BaseURL,
	}

	// Decrypt sensitive fields
	if connection.GitHubPrivateKey != "" {
		decryptedKey, err := crypto.DecryptAES256GCM(connection.GitHubPrivateKey, encryptionKey)
		if err != nil {
			http.Error(w, "Failed to decrypt credentials", http.StatusInternalServerError)
			return
		}
		testReq.GitHubPrivateKey = string(decryptedKey)
	}
	if connection.ADOClientSecret != "" {
		decryptedSecret, err := crypto.DecryptAES256GCM(connection.ADOClientSecret, encryptionKey)
		if err != nil {
			http.Error(w, "Failed to decrypt credentials", http.StatusInternalServerError)
			return
		}
		testReq.ADOClientSecret = string(decryptedSecret)
	}
	// Slack tokens — decrypt so the validators can probe Slack.
	for _, f := range []struct {
		enc string
		dst *string
	}{
		{connection.SlackAppToken, &testReq.SlackAppToken},
		{connection.SlackBotToken, &testReq.SlackBotToken},
	} {
		if f.enc == "" {
			continue
		}
		dec, err := crypto.DecryptAES256GCM(f.enc, encryptionKey)
		if err != nil {
			http.Error(w, "Failed to decrypt credentials", http.StatusInternalServerError)
			return
		}
		*f.dst = string(dec)
	}

	// Test the connection.
	testErr := s.testServiceConnection(r.Context(), testReq)

	w.Header().Set("Content-Type", "application/json")

	// Nothing to probe (e.g. a REST slack_app): not a failure. Leave the
	// status untested (neutral) and tell the caller it was skipped rather
	// than flipping the row to an error.
	if errors.Is(testErr, errNothingToValidate) {
		connection.LastError = ""
		connection.LastTestedAt = nil
		if err := s.Store.UpdateServiceConnection(r.Context(), connection); err != nil {
			log.Error().Err(err).Str("connection_id", connectionID).Msg("Failed to clear connection status")
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"skipped": true,
			"message": testErr.Error(),
		})
		return
	}

	// Update connection status
	now := time.Now()
	connection.LastTestedAt = &now
	if testErr != nil {
		connection.LastError = testErr.Error()
	} else {
		connection.LastError = ""
	}
	if err := s.Store.UpdateServiceConnection(r.Context(), connection); err != nil {
		log.Error().Err(err).Str("connection_id", connectionID).Msg("Failed to update connection status after test")
	}

	if testErr != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   testErr.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Connection test successful",
	})
}

// testServiceConnection tests a service connection by attempting to authenticate
func (s *HelixAPIServer) testServiceConnection(ctx context.Context, req types.ServiceConnectionCreateRequest) error {
	switch req.Type {
	case types.ServiceConnectionTypeGitHubApp:
		// Create GitHub App client and try to get authenticated user
		client, err := github.NewClientWithGitHubApp(
			req.GitHubAppID,
			req.GitHubInstallationID,
			req.GitHubPrivateKey,
			req.BaseURL,
		)
		if err != nil {
			return fmt.Errorf("failed to create GitHub App client: %w", err)
		}

		// Try to list repositories to verify the connection works
		_, err = client.ListRepositories(ctx)
		if err != nil {
			return fmt.Errorf("failed to authenticate with GitHub App: %w", err)
		}
		return nil

	case types.ServiceConnectionTypeADOServicePrincipal:
		// Create ADO client with Service Principal and try to authenticate
		client, err := azuredevops.NewAzureDevOpsClientWithServicePrincipal(
			ctx,
			req.ADOOrganizationURL,
			req.ADOTenantID,
			req.ADOClientID,
			req.ADOClientSecret,
		)
		if err != nil {
			return fmt.Errorf("failed to create ADO Service Principal client: %w", err)
		}

		// Try to list projects to verify the connection works
		_, err = client.ListProjects(ctx)
		if err != nil {
			return fmt.Errorf("failed to authenticate with ADO Service Principal: %w", err)
		}
		return nil

	case types.ServiceConnectionTypeSlackApp, types.ServiceConnectionTypeSlackWorkspace:
		// Validate whatever credential actually reaches Slack:
		//   - a Socket Mode app-level token (xapp-) via apps.connections.open
		//     (returns a wss URL but opens no persistent connection)
		//   - a bot token (xoxb-) via auth.test
		// A REST app exposes only a signing secret, which has no API to
		// probe — report that honestly rather than a false "ok".
		if req.SlackAppToken != "" {
			if err := slackcore.ValidateAppToken(ctx, req.SlackAppToken, ""); err != nil {
				return fmt.Errorf("Slack app-level token rejected: %w", err)
			}
			return nil
		}
		if req.SlackBotToken != "" {
			if err := slackcore.ValidateBotToken(ctx, req.SlackBotToken, ""); err != nil {
				return fmt.Errorf("Slack bot token rejected: %w", err)
			}
			return nil
		}
		return errNothingToValidate

	default:
		return fmt.Errorf("unsupported connection type: %s", req.Type)
	}
}
