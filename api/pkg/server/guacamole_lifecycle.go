package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/rs/zerolog/log"
)

// GuacamoleLifecycleManager manages Guacamole connections throughout their lifecycle
type GuacamoleLifecycleManager struct {
	guacamoleProxy  *GuacamoleProxy
	store           store.Store
	rdpProxyManager *RDPProxyManager
}

// NewGuacamoleLifecycleManager creates a new lifecycle manager
func NewGuacamoleLifecycleManager(proxy *GuacamoleProxy, store store.Store, rdpProxy *RDPProxyManager) *GuacamoleLifecycleManager {
	return &GuacamoleLifecycleManager{
		guacamoleProxy:  proxy,
		store:           store,
		rdpProxyManager: rdpProxy,
	}
}

// OnRunnerConnect handles runner connection events
func (glm *GuacamoleLifecycleManager) OnRunnerConnect(ctx context.Context, runnerID string) error {
	log.Info().
		Str("runner_id", runnerID).
		Msg("Creating Guacamole connection for newly connected runner")

	// Get or create agent runner record
	runner, err := glm.store.GetOrCreateAgentRunner(ctx, runnerID)
	if err != nil {
		return fmt.Errorf("failed to get/create agent runner: %w", err)
	}

	// Create RDP proxy for this runner
	proxy, err := glm.rdpProxyManager.CreateRunnerRDPProxy(ctx, runnerID)
	if err != nil {
		return fmt.Errorf("failed to create RDP proxy for runner: %w", err)
	}

	// Create Guacamole connection
	connectionID := fmt.Sprintf("runner-%s", runnerID)
	guacConnectionID, err := glm.createGuacamoleConnection(ctx, connectionID, proxy.LocalPort, runner.RDPPassword, "runner")
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", runnerID).
			Msg("Failed to create Guacamole connection for runner")
		// Don't fail the runner connection for this
		return nil
	}

	log.Info().
		Str("runner_id", runnerID).
		Str("guac_connection_id", guacConnectionID).
		Int("rdp_port", proxy.LocalPort).
		Msg("Successfully created Guacamole connection for runner")

	return nil
}

// OnRunnerDisconnect handles runner disconnection events
func (glm *GuacamoleLifecycleManager) OnRunnerDisconnect(ctx context.Context, runnerID string) error {
	log.Info().
		Str("runner_id", runnerID).
		Msg("Cleaning up Guacamole connection for disconnected runner")

	// Update runner status to offline
	err := glm.store.UpdateAgentRunnerStatus(ctx, runnerID, "offline")
	if err != nil {
		log.Warn().Err(err).Str("runner_id", runnerID).Msg("Failed to update runner status")
	}

	// Clean up Guacamole connection
	connectionID := fmt.Sprintf("runner-%s", runnerID)
	err = glm.deleteGuacamoleConnection(ctx, connectionID)
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", runnerID).
			Msg("Failed to delete Guacamole connection for runner")
		// Don't fail for this
	}

	return nil
}

// OnSessionStart handles session startup with password rotation
func (glm *GuacamoleLifecycleManager) OnSessionStart(ctx context.Context, sessionID, runnerID string) error {
	log.Info().
		Str("session_id", sessionID).
		Str("runner_id", runnerID).
		Msg("Creating/updating Guacamole connection for new session")

	// Get current runner info (password may have been rotated)
	runner, err := glm.store.GetAgentRunner(ctx, runnerID)
	if err != nil {
		return fmt.Errorf("failed to get agent runner: %w", err)
	}

	// Create or get RDP proxy for this session
	proxy, err := glm.rdpProxyManager.CreateSessionRDPProxy(ctx, sessionID, runnerID)
	if err != nil {
		return fmt.Errorf("failed to create session RDP proxy: %w", err)
	}

	// Create/update Guacamole connection for session
	connectionID := fmt.Sprintf("session-%s", sessionID)
	guacConnectionID, err := glm.createGuacamoleConnection(ctx, connectionID, proxy.LocalPort, runner.RDPPassword, "session")
	if err != nil {
		log.Error().
			Err(err).
			Str("session_id", sessionID).
			Str("runner_id", runnerID).
			Msg("Failed to create Guacamole connection for session")
		// Don't fail the session for this
		return nil
	}

	log.Info().
		Str("session_id", sessionID).
		Str("runner_id", runnerID).
		Str("guac_connection_id", guacConnectionID).
		Int("rdp_port", proxy.LocalPort).
		Msg("Successfully created Guacamole connection for session")

	return nil
}

// OnSessionEnd handles session cleanup
func (glm *GuacamoleLifecycleManager) OnSessionEnd(ctx context.Context, sessionID string) error {
	log.Info().
		Str("session_id", sessionID).
		Msg("Cleaning up Guacamole connection for ended session")

	// Clean up Guacamole connection
	connectionID := fmt.Sprintf("session-%s", sessionID)
	err := glm.deleteGuacamoleConnection(ctx, connectionID)
	if err != nil {
		log.Error().
			Err(err).
			Str("session_id", sessionID).
			Msg("Failed to delete Guacamole connection for session")
		// Don't fail for this
	}

	return nil
}

// OnPasswordRotation handles password rotation events
func (glm *GuacamoleLifecycleManager) OnPasswordRotation(ctx context.Context, runnerID, newPassword string) error {
	log.Info().
		Str("runner_id", runnerID).
		Msg("Updating Guacamole connections for password rotation")

	// Update runner connection
	connectionID := fmt.Sprintf("runner-%s", runnerID)
	err := glm.updateGuacamoleConnectionPassword(ctx, connectionID, newPassword)
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", runnerID).
			Msg("Failed to update runner Guacamole connection password")
	}

	// Find and update all active session connections for this runner
	// This is a simplified approach - in production you'd want to track session-to-runner mappings
	activeConnections := glm.guacamoleProxy.getConnections()
	for connID, conn := range activeConnections {
		if conn.runnerID == runnerID && conn.connectionType == "session" {
			err := glm.updateGuacamoleConnectionPassword(ctx, connID, newPassword)
			if err != nil {
				log.Error().
					Err(err).
					Str("connection_id", connID).
					Str("session_id", conn.sessionID).
					Str("runner_id", runnerID).
					Msg("Failed to update session Guacamole connection password")
			}
		}
	}

	return nil
}

// createGuacamoleConnection creates a new connection in Guacamole
func (glm *GuacamoleLifecycleManager) createGuacamoleConnection(ctx context.Context, connectionID string, rdpPort int, rdpPassword, connectionType string) (string, error) {
	// Authenticate with Guacamole
	authToken, err := glm.authenticateWithGuacamole()
	if err != nil {
		return "", fmt.Errorf("failed to authenticate with Guacamole: %w", err)
	}

	// Create connection configuration
	connectionConfig := map[string]interface{}{
		"parentIdentifier": "ROOT",
		"name":             fmt.Sprintf("helix-%s", connectionID),
		"protocol":         "rdp",
		"parameters": map[string]string{
			"hostname":    "api", // Connect to API container where RDP proxy is running
			"port":        fmt.Sprintf("%d", rdpPort),
			"username":    "zed",
			"password":    rdpPassword,
			"width":       "1024",
			"height":      "768",
			"dpi":         "96",
			"color-depth": "32",
		},
		"attributes": map[string]string{
			"max-connections":          "",
			"max-connections-per-user": "",
			"weight":                   "",
			"failover-only":            "",
			"guacd-port":               "",
			"guacd-encryption":         "",
			"guacd-hostname":           "",
		},
	}

	// Call Guacamole REST API
	guacamoleURL := fmt.Sprintf("%s/guacamole/api/session/data/postgresql/connections?token=%s",
		glm.guacamoleProxy.guacamoleServerURL, authToken)

	configJSON, err := json.Marshal(connectionConfig)
	if err != nil {
		return "", fmt.Errorf("failed to marshal connection config: %w", err)
	}

	// Make HTTP POST request
	resp, err := http.Post(guacamoleURL, "application/json", strings.NewReader(string(configJSON)))
	if err != nil {
		return "", fmt.Errorf("failed to create connection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("Guacamole API returned status %d", resp.StatusCode)
	}

	// Parse response
	var guacConnection map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&guacConnection); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	guacConnectionID, ok := guacConnection["identifier"].(string)
	if !ok {
		guacConnectionID = fmt.Sprintf("guac-%s", connectionID)
	}

	log.Debug().
		Str("connection_id", connectionID).
		Str("guac_connection_id", guacConnectionID).
		Str("connection_type", connectionType).
		Msg("Created Guacamole connection")

	return guacConnectionID, nil
}

// deleteGuacamoleConnection removes a connection from Guacamole
func (glm *GuacamoleLifecycleManager) deleteGuacamoleConnection(ctx context.Context, connectionID string) error {
	// Authenticate with Guacamole
	authToken, err := glm.authenticateWithGuacamole()
	if err != nil {
		return fmt.Errorf("failed to authenticate with Guacamole: %w", err)
	}

	// Find the connection by name (since we don't store the Guacamole ID)
	connections, err := glm.listGuacamoleConnections(authToken)
	if err != nil {
		return fmt.Errorf("failed to list connections: %w", err)
	}

	var guacConnectionID string
	expectedName := fmt.Sprintf("helix-%s", connectionID)
	for id, conn := range connections {
		if name, ok := conn["name"].(string); ok && name == expectedName {
			guacConnectionID = id
			break
		}
	}

	if guacConnectionID == "" {
		log.Debug().Str("connection_id", connectionID).Msg("Guacamole connection not found, may already be deleted")
		return nil
	}

	// Delete the connection
	deleteURL := fmt.Sprintf("%s/guacamole/api/session/data/postgresql/connections/%s?token=%s",
		glm.guacamoleProxy.guacamoleServerURL, guacConnectionID, authToken)

	req, err := http.NewRequest("DELETE", deleteURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create delete request: %w", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete connection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("delete failed with status %d", resp.StatusCode)
	}

	log.Debug().
		Str("connection_id", connectionID).
		Str("guac_connection_id", guacConnectionID).
		Msg("Deleted Guacamole connection")

	return nil
}

// updateGuacamoleConnectionPassword updates the password for an existing connection
func (glm *GuacamoleLifecycleManager) updateGuacamoleConnectionPassword(ctx context.Context, connectionID, newPassword string) error {
	// Authenticate with Guacamole
	authToken, err := glm.authenticateWithGuacamole()
	if err != nil {
		return fmt.Errorf("failed to authenticate with Guacamole: %w", err)
	}

	// Find the connection
	connections, err := glm.listGuacamoleConnections(authToken)
	if err != nil {
		return fmt.Errorf("failed to list connections: %w", err)
	}

	var guacConnectionID string
	expectedName := fmt.Sprintf("helix-%s", connectionID)
	for id, conn := range connections {
		if name, ok := conn["name"].(string); ok && name == expectedName {
			guacConnectionID = id
			break
		}
	}

	if guacConnectionID == "" {
		log.Debug().Str("connection_id", connectionID).Msg("Guacamole connection not found for password update")
		return nil
	}

	// Update the password parameter
	updateData := map[string]interface{}{
		"parameters": map[string]string{
			"password": newPassword,
		},
	}

	updateJSON, err := json.Marshal(updateData)
	if err != nil {
		return fmt.Errorf("failed to marshal update data: %w", err)
	}

	updateURL := fmt.Sprintf("%s/guacamole/api/session/data/postgresql/connections/%s?token=%s",
		glm.guacamoleProxy.guacamoleServerURL, guacConnectionID, authToken)

	req, err := http.NewRequest("PUT", updateURL, strings.NewReader(string(updateJSON)))
	if err != nil {
		return fmt.Errorf("failed to create update request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update connection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update failed with status %d", resp.StatusCode)
	}

	log.Debug().
		Str("connection_id", connectionID).
		Str("guac_connection_id", guacConnectionID).
		Msg("Updated Guacamole connection password")

	return nil
}

// listGuacamoleConnections retrieves all connections from Guacamole
func (glm *GuacamoleLifecycleManager) listGuacamoleConnections(authToken string) (map[string]map[string]interface{}, error) {
	listURL := fmt.Sprintf("%s/guacamole/api/session/data/postgresql/connections?token=%s",
		glm.guacamoleProxy.guacamoleServerURL, authToken)

	resp, err := http.Get(listURL)
	if err != nil {
		return nil, fmt.Errorf("failed to list connections: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list failed with status %d", resp.StatusCode)
	}

	var connections map[string]map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&connections); err != nil {
		return nil, fmt.Errorf("failed to decode connections: %w", err)
	}

	return connections, nil
}

// authenticateWithGuacamole authenticates with Guacamole and returns auth token
func (glm *GuacamoleLifecycleManager) authenticateWithGuacamole() (string, error) {
	authURL := fmt.Sprintf("%s/guacamole/api/tokens", glm.guacamoleProxy.guacamoleServerURL)

	// Use form data for authentication
	authData := url.Values{}
	authData.Set("username", "guacadmin")
	authData.Set("password", "guacadmin")

	resp, err := http.PostForm(authURL, authData)
	if err != nil {
		return "", fmt.Errorf("failed to authenticate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("authentication failed with status %d", resp.StatusCode)
	}

	var authResponse map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&authResponse); err != nil {
		return "", fmt.Errorf("failed to decode auth response: %w", err)
	}

	token, ok := authResponse["authToken"].(string)
	if !ok {
		return "", fmt.Errorf("no auth token in response")
	}

	return token, nil
}

// CleanupStaleConnections removes old/stale Guacamole connections
func (glm *GuacamoleLifecycleManager) CleanupStaleConnections(ctx context.Context) error {
	log.Info().Msg("Cleaning up stale Guacamole connections")

	authToken, err := glm.authenticateWithGuacamole()
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	connections, err := glm.listGuacamoleConnections(authToken)
	if err != nil {
		return fmt.Errorf("failed to list connections: %w", err)
	}

	// Clean up connections that start with "helix-" but don't have active proxies
	activeProxyConnections := make(map[string]bool)
	if glm.guacamoleProxy != nil {
		proxyConns := glm.guacamoleProxy.getConnections()
		for connID := range proxyConns {
			activeProxyConnections[fmt.Sprintf("helix-%s", connID)] = true
		}
	}

	for guacID, conn := range connections {
		if name, ok := conn["name"].(string); ok && strings.HasPrefix(name, "helix-") {
			if !activeProxyConnections[name] {
				log.Info().
					Str("guac_connection_id", guacID).
					Str("connection_name", name).
					Msg("Cleaning up stale Guacamole connection")

				deleteURL := fmt.Sprintf("%s/guacamole/api/session/data/postgresql/connections/%s?token=%s",
					glm.guacamoleProxy.guacamoleServerURL, guacID, authToken)

				req, err := http.NewRequest("DELETE", deleteURL, nil)
				if err != nil {
					log.Error().Err(err).Str("guac_connection_id", guacID).Msg("Failed to create delete request")
					continue
				}

				client := &http.Client{}
				resp, err := client.Do(req)
				if err != nil {
					log.Error().Err(err).Str("guac_connection_id", guacID).Msg("Failed to delete stale connection")
					continue
				}
				resp.Body.Close()
			}
		}
	}

	return nil
}
