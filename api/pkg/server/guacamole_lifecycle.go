package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// GuacamoleLifecycleManager manages Guacamole connections throughout their lifecycle
type GuacamoleLifecycleManager struct {
	guacamoleProxy   *GuacamoleProxy
	store            store.Store
	rdpProxyManager  *RDPProxyManager
	runnerController *scheduler.RunnerController
	pubsub           pubsub.PubSub
}

// NewGuacamoleLifecycleManager creates a new lifecycle manager
func NewGuacamoleLifecycleManager(proxy *GuacamoleProxy, store store.Store, rdpProxy *RDPProxyManager, runnerController *scheduler.RunnerController, ps pubsub.PubSub) *GuacamoleLifecycleManager {
	return &GuacamoleLifecycleManager{
		guacamoleProxy:   proxy,
		store:            store,
		rdpProxyManager:  rdpProxy,
		runnerController: runnerController,
		pubsub:           ps,
	}
}

// OnRunnerConnect handles runner connection events
func (glm *GuacamoleLifecycleManager) OnRunnerConnect(ctx context.Context, runnerID string) error {
	log.Info().
		Str("runner_id", runnerID).
		Msg("Creating Guacamole connection for newly connected runner")

	// Clean up any stale connections before creating new ones
	if err := glm.CleanupStaleConnections(ctx); err != nil {
		log.Warn().Err(err).Msg("Failed to cleanup stale Guacamole connections, continuing anyway")
	}

	// Get or create agent runner record
	runner, err := glm.store.GetOrCreateAgentRunner(ctx, runnerID)
	if err != nil {
		return fmt.Errorf("failed to get/create agent runner: %w", err)
	}

	// Send the generated RDP password to the runner via ZedAgent WebSocket message
	// This ensures the runner uses the database password instead of the initial env password
	err = glm.sendPasswordConfigurationToRunner(ctx, runnerID, runner.RDPPassword)
	if err != nil {
		log.Warn().
			Err(err).
			Str("runner_id", runnerID).
			Msg("Failed to send password configuration to runner - runner will use initial password")
		// Don't fail the runner connection for this, but warn since it's a security issue
	}

	// Check if direct VNC proxy is enabled
	directProxy := os.Getenv("DIRECT_VNC_PROXY") == "true"
	
	var proxyPort int
	if directProxy {
		// Skip reverse dial proxy creation - connect directly to zed-runner
		proxyPort = 5902 // Direct wayvnc port (not used in direct mode)
		log.Info().
			Str("runner_id", runnerID).
			Msg("Skipping VNC proxy creation - using direct connection mode")
	} else {
		// Create VNC proxy for this runner (using reverse dial to forward to VNC port 5902)
		proxy, err := glm.rdpProxyManager.CreateRunnerRDPProxy(ctx, runnerID)
		if err != nil {
			return fmt.Errorf("failed to create VNC proxy for runner: %w", err)
		}
		proxyPort = proxy.LocalPort
		log.Info().
			Str("runner_id", runnerID).
			Int("proxy_port", proxyPort).
			Msg("Created VNC reverse dial proxy for runner")
	}

	// Create Guacamole VNC connection
	connectionID := fmt.Sprintf("runner-%s", runnerID)
	guacConnectionID, err := glm.createGuacamoleVNCConnection(ctx, connectionID, proxyPort, runner.RDPPassword, "runner")
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", runnerID).
			Msg("Failed to create Guacamole connection for runner")
		// Don't fail the runner connection for this
		return nil
	}

	if directProxy {
		log.Info().
			Str("runner_id", runnerID).
			Str("guac_connection_id", guacConnectionID).
			Msg("Successfully created Guacamole VNC connection for runner (direct mode)")
	} else {
		log.Info().
			Str("runner_id", runnerID).
			Str("guac_connection_id", guacConnectionID).
			Int("rdp_port", proxyPort).
			Msg("Successfully created Guacamole VNC connection for runner (reverse dial mode)")
	}

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

// createGuacamoleVNCConnection creates a new VNC connection in Guacamole
func (glm *GuacamoleLifecycleManager) createGuacamoleVNCConnection(ctx context.Context, connectionID string, vncProxyPort int, vncPassword, connectionType string) (string, error) {
	// Authenticate with Guacamole
	authToken, err := glm.authenticateWithGuacamole()
	if err != nil {
		return "", fmt.Errorf("failed to authenticate with Guacamole: %w", err)
	}

	// Check if direct VNC proxy is enabled (bypass reverse dial for local development)
	directProxy := os.Getenv("DIRECT_VNC_PROXY") == "true"
	
	var hostname, port string
	if directProxy {
		// Direct connection to zed-runner container for local development
		hostname = "zed-runner"
		port = "5901" // Direct wayvnc port
		log.Info().
			Str("connection_id", connectionID).
			Msg("Using direct VNC proxy (bypassing reverse dial)")
	} else {
		// Use reverse dial proxy through API container (production mode)
		hostname = "api"
		port = fmt.Sprintf("%d", vncProxyPort)
		log.Info().
			Str("connection_id", connectionID).
			Int("proxy_port", vncProxyPort).
			Msg("Using reverse dial VNC proxy")
	}

	// Create VNC connection configuration
	connectionConfig := map[string]interface{}{
		"parentIdentifier": "ROOT",
		"name":             fmt.Sprintf("helix-%s", connectionID),
		"protocol":         "vnc",
		"parameters": map[string]string{
			"hostname":    hostname,
			"port":        port,
			"password":    vncPassword, // VNC only uses password, no username
			"color-depth": "32",
			"swap-red-blue": "false",
			"cursor":      "remote",
			"read-only":   "false",
			"clipboard":   "remote",
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
		Msg("Created Guacamole VNC connection")

	return guacConnectionID, nil
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
	authData.Set("username", glm.guacamoleProxy.guacamoleUsername)
	authData.Set("password", glm.guacamoleProxy.guacamolePassword)

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

// sendPasswordConfigurationToRunner sends the RDP password to a specific runner via ZedAgent WebSocket message
func (glm *GuacamoleLifecycleManager) sendPasswordConfigurationToRunner(ctx context.Context, runnerID, rdpPassword string) error {
	log.Info().
		Str("runner_id", runnerID).
		Msg("Sending RDP password configuration to runner via ZedAgent WebSocket")

	// Create a password configuration ZedAgent task
	// This will be sent via the existing WebSocket connection and processed by the runner
	passwordConfigAgent := &types.ZedAgent{
		SessionID:   fmt.Sprintf("password-config-%s", runnerID),
		UserID:      "system",
		Input:       "Configure initial RDP password for runner",
		RDPPassword: rdpPassword,
		RDPUser:     "zed",
		RDPPort:     5900,
	}

	// Marshal the ZedAgent task
	data, err := json.Marshal(passwordConfigAgent)
	if err != nil {
		return fmt.Errorf("failed to marshal password config task: %w", err)
	}

	// Send via the ZedAgent NATS stream - this will be picked up by the runner's WebSocket connection
	// The runner will process this like any other ZedAgent task and call configureRDPServer
	_, err = glm.pubsub.StreamRequest(
		ctx,
		pubsub.ZedAgentRunnerStream,
		pubsub.ZedAgentQueue,
		data,
		map[string]string{"kind": "zed_agent"},
		30*time.Second,
	)
	if err != nil {
		return fmt.Errorf("failed to send password config via ZedAgent stream: %w", err)
	}

	log.Info().
		Str("runner_id", runnerID).
		Msg("Successfully sent RDP password configuration to runner via ZedAgent WebSocket")

	return nil
}

// CleanupAllGuacamoleConnections removes all helix-* connections from Guacamole
func (glm *GuacamoleLifecycleManager) CleanupAllGuacamoleConnections(ctx context.Context) error {
	log.Info().Msg("Starting cleanup of all Helix Guacamole connections")

	// Authenticate with Guacamole
	authToken, err := glm.authenticateWithGuacamole()
	if err != nil {
		return fmt.Errorf("failed to authenticate with Guacamole: %w", err)
	}

	// List all connections
	connections, err := glm.listGuacamoleConnections(authToken)
	if err != nil {
		return fmt.Errorf("failed to list connections: %w", err)
	}

	// Find and delete all helix-* connections
	deletedCount := 0
	for id, conn := range connections {
		if name, ok := conn["name"].(string); ok && strings.HasPrefix(name, "helix-") {
			deleteURL := fmt.Sprintf("%s/guacamole/api/session/data/postgresql/connections/%s?token=%s",
				glm.guacamoleProxy.guacamoleServerURL, id, authToken)

			req, err := http.NewRequest("DELETE", deleteURL, nil)
			if err != nil {
				log.Error().Err(err).Str("connection_id", id).Msg("Failed to create delete request")
				continue
			}

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				log.Error().Err(err).Str("connection_id", id).Msg("Failed to delete connection")
				continue
			}
			resp.Body.Close()

			if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
				log.Info().Str("connection_name", name).Str("connection_id", id).Msg("Deleted Guacamole connection")
				deletedCount++
			} else {
				log.Error().Int("status", resp.StatusCode).Str("connection_id", id).Msg("Failed to delete connection")
			}
		}
	}

	log.Info().Int("deleted_count", deletedCount).Msg("Completed cleanup of Helix Guacamole connections")
	return nil
}
