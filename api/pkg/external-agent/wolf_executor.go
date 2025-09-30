package external_agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/wolf"
)

// WolfExecutor implements the Executor interface using Wolf API
type WolfExecutor struct {
	wolfClient *wolf.Client
	sessions   map[string]*ZedSession
	instances  map[string]*ZedInstanceInfo
	mutex      sync.RWMutex

	// Zed configuration
	zedImage      string
	helixAPIURL   string
	helixAPIToken string

	// Workspace configuration for dev stack
	workspaceBasePath string
}

// generateWolfAppID creates a consistent, numeric Wolf-compatible app ID
// Uses user ID and environment name to ensure the same environment always gets the same ID
// Wolf expects numeric-only IDs for Moonlight protocol compatibility
func (w *WolfExecutor) generateWolfAppID(userID, environmentName string) string {
	stableKey := fmt.Sprintf("%s-%s", userID, environmentName)
	// Create a numeric hash by summing byte values
	var numericHash uint64
	for _, b := range []byte(stableKey) {
		numericHash = numericHash*31 + uint64(b)
	}
	// Convert to string and limit to reasonable length for Wolf
	return fmt.Sprintf("%d", numericHash%1000000000) // Max 9 digits
}

// NewWolfExecutor creates a new Wolf-based executor
func NewWolfExecutor(wolfSocketPath, zedImage, helixAPIURL, helixAPIToken string) *WolfExecutor {
	return &WolfExecutor{
		wolfClient:        wolf.NewClient(wolfSocketPath),
		sessions:          make(map[string]*ZedSession),
		instances:         make(map[string]*ZedInstanceInfo),
		zedImage:          zedImage,
		helixAPIURL:       helixAPIURL,
		helixAPIToken:     helixAPIToken,
		workspaceBasePath: "/opt/helix/filestore/workspaces", // Default workspace base path
	}
}

// StartZedAgent implements the Executor interface
func (w *WolfExecutor) StartZedAgent(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	log.Info().
		Str("session_id", agent.SessionID).
		Str("project_path", agent.ProjectPath).
		Msg("Starting Zed agent via Wolf")

	// For now, skip Zed process apps since we're focusing on the Docker personal dev environments
	// TODO: Create a minimal process app constructor if needed
	return nil, fmt.Errorf("Zed process apps not supported with minimal Wolf client - use personal dev environments instead")
}

// buildZedCommand constructs the Zed execution command with proper environment variables
func (w *WolfExecutor) buildZedCommand(agent *types.ZedAgent) string {
	// Build environment variables for Zed
	envVars := []string{
		fmt.Sprintf("HELIX_API_URL=%s", w.helixAPIURL),
		fmt.Sprintf("HELIX_API_TOKEN=%s", w.helixAPIToken),
		fmt.Sprintf("ZED_SESSION_ID=%s", agent.SessionID),
		fmt.Sprintf("ZED_USER_ID=%s", agent.UserID),
		fmt.Sprintf("ZED_PROJECT_PATH=%s", agent.ProjectPath),
		fmt.Sprintf("ZED_WORK_DIR=%s", agent.WorkDir),
		"DISPLAY=:0",
		"WAYLAND_DISPLAY=wayland-1",
	}

	// Add any additional environment variables from the agent
	for _, env := range agent.Env {
		envVars = append(envVars, env)
	}

	// Construct the full command
	// This assumes we'll have a Zed container image or binary available
	cmd := fmt.Sprintf("env %s /usr/local/bin/zed --foreground",
		joinEnvVars(envVars))

	return cmd
}

// joinEnvVars joins environment variables with spaces
func joinEnvVars(envVars []string) string {
	result := ""
	for i, env := range envVars {
		if i > 0 {
			result += " "
		}
		result += env
	}
	return result
}

// StopZedAgent implements the Executor interface
func (w *WolfExecutor) StopZedAgent(ctx context.Context, sessionID string) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	log.Info().Str("session_id", sessionID).Msg("Stopping Zed agent via Wolf")

	session, exists := w.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	appID := fmt.Sprintf("zed-agent-%s", sessionID)

	// Stop Wolf session (this should stop the Zed process)
	err := w.wolfClient.StopSession(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to stop Wolf session")
		// Continue with cleanup even if stop fails
	}

	// Remove the app from Wolf
	err = w.wolfClient.RemoveApp(ctx, appID)
	if err != nil {
		log.Error().Err(err).Str("app_id", appID).Msg("Failed to remove Wolf app")
		// Continue with cleanup even if removal fails
	}

	// Update session status
	session.Status = "stopped"

	// Remove from our tracking
	delete(w.sessions, sessionID)

	log.Info().Str("session_id", sessionID).Msg("Zed agent stopped successfully")

	return nil
}

// GetSession implements the Executor interface
func (w *WolfExecutor) GetSession(sessionID string) (*ZedSession, error) {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

	session, exists := w.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	// Update last access time
	session.LastAccess = time.Now()

	return session, nil
}

// CleanupExpiredSessions implements the Executor interface
func (w *WolfExecutor) CleanupExpiredSessions(ctx context.Context, timeout time.Duration) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	cutoff := time.Now().Add(-timeout)
	var expiredSessions []string

	for sessionID, session := range w.sessions {
		if session.LastAccess.Before(cutoff) {
			expiredSessions = append(expiredSessions, sessionID)
		}
	}

	for _, sessionID := range expiredSessions {
		log.Info().
			Str("session_id", sessionID).
			Dur("timeout", timeout).
			Msg("Cleaning up expired Zed session")

		err := w.StopZedAgent(ctx, sessionID)
		if err != nil {
			log.Error().
				Err(err).
				Str("session_id", sessionID).
				Msg("Failed to stop expired session")
		}
	}
}

// ListSessions implements the Executor interface
func (w *WolfExecutor) ListSessions() []*ZedSession {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

	sessions := make([]*ZedSession, 0, len(w.sessions))
	for _, session := range w.sessions {
		sessions = append(sessions, session)
	}

	return sessions
}

// Multi-session SpecTask methods (for future use)

// StartZedInstance implements the Executor interface
func (w *WolfExecutor) StartZedInstance(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error) {
	// For now, delegate to single-session method
	return w.StartZedAgent(ctx, agent)
}

// CreateZedThread implements the Executor interface
func (w *WolfExecutor) CreateZedThread(ctx context.Context, instanceID, threadID string, config map[string]interface{}) error {
	// TODO: Implement multi-threading support when needed
	return fmt.Errorf("multi-threading not yet implemented in Wolf executor")
}

// StopZedInstance implements the Executor interface
func (w *WolfExecutor) StopZedInstance(ctx context.Context, instanceID string) error {
	// For now, delegate to single-session method
	return w.StopZedAgent(ctx, instanceID)
}

// GetInstanceStatus implements the Executor interface
func (w *WolfExecutor) GetInstanceStatus(instanceID string) (*ZedInstanceStatus, error) {
	session, err := w.GetSession(instanceID)
	if err != nil {
		return nil, err
	}

	return &ZedInstanceStatus{
		InstanceID:    instanceID,
		Status:        session.Status,
		ThreadCount:   1,
		ActiveThreads: 1,
		LastActivity:  &session.LastAccess,
		ProjectPath:   session.ProjectPath,
	}, nil
}

// ListInstanceThreads implements the Executor interface
func (w *WolfExecutor) ListInstanceThreads(instanceID string) ([]*ZedThreadInfo, error) {
	session, err := w.GetSession(instanceID)
	if err != nil {
		return nil, err
	}

	// Return single thread for now
	return []*ZedThreadInfo{
		{
			ThreadID:      instanceID,
			WorkSessionID: session.SessionID,
			Status:        session.Status,
			CreatedAt:     session.StartTime,
			LastActivity:  &session.LastAccess,
			Config:        map[string]interface{}{},
		},
	}, nil
}

// Personal Development Environment Management

// CreatePersonalDevEnvironment creates a new personal development environment with default display settings
func (w *WolfExecutor) CreatePersonalDevEnvironment(ctx context.Context, userID, appID, environmentName string) (*ZedInstanceInfo, error) {
	return w.CreatePersonalDevEnvironmentWithDisplay(ctx, userID, appID, environmentName, 2360, 1640, 120)
}

// CreatePersonalDevEnvironmentWithDisplay creates a new personal development environment with custom display settings
func (w *WolfExecutor) CreatePersonalDevEnvironmentWithDisplay(ctx context.Context, userID, appID, environmentName string, displayWidth, displayHeight, displayFPS int) (*ZedInstanceInfo, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	// Validate display parameters
	if err := validateDisplayParams(displayWidth, displayHeight, displayFPS); err != nil {
		return nil, fmt.Errorf("invalid display configuration: %w", err)
	}

	instanceID := fmt.Sprintf("personal-dev-%s-%d", userID, time.Now().Unix())

	log.Info().
		Str("instance_id", instanceID).
		Str("user_id", userID).
		Str("app_id", appID).
		Str("environment_name", environmentName).
		Int("display_width", displayWidth).
		Int("display_height", displayHeight).
		Int("display_fps", displayFPS).
		Msg("Creating personal development environment via Wolf")

	// Create persistent workspace directory
	workspaceDir, err := w.createWorkspaceDirectory(instanceID)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace directory: %w", err)
	}

	log.Info().
		Str("instance_id", instanceID).
		Str("workspace_dir", workspaceDir).
		Msg("Created persistent workspace directory")

	// Create Sway configuration for this personal dev environment
	err = w.createSwayConfig(instanceID)
	if err != nil {
		return nil, fmt.Errorf("failed to create Sway config: %w", err)
	}

	log.Info().
		Str("instance_id", instanceID).
		Msg("Created Sway compositor configuration")

	// Create Wolf app for this personal dev environment
	wolfAppID := w.generateWolfAppID(userID, environmentName)

	// Use the OpenAPI-based constructor with custom Docker configuration
	env := []string{
		"GOW_REQUIRED_DEVICES=/dev/input/* /dev/dri/* /dev/nvidia*", // Exact same as XFCE working config
		"RUN_SWAY=1", // Enable Sway compositor mode in GOW launcher
		// Pass through API key for Zed AI functionality
		fmt.Sprintf("ANTHROPIC_API_KEY=%s", os.Getenv("ANTHROPIC_API_KEY")),
		// Additional environment variables for Helix integration
		fmt.Sprintf("HELIX_API_URL=%s", w.helixAPIURL),
		fmt.Sprintf("HELIX_API_TOKEN=%s", w.helixAPIToken),
		// Enable user startup script execution
		"HELIX_STARTUP_SCRIPT=/home/retro/work/startup.sh",
	}
	mounts := []string{
		fmt.Sprintf("%s:/home/retro/work", workspaceDir),                                    // Mount persistent workspace
		fmt.Sprintf("%s/zed-build/zed:/usr/local/bin/zed:ro", os.Getenv("HELIX_HOST_HOME")), // Mount Zed binary from host path
	}
	baseCreateJSON := `{
  "HostConfig": {
    "IpcMode": "host",
    "NetworkMode": "helix_default",
    "Privileged": false,
    "CapAdd": ["SYS_ADMIN", "SYS_NICE", "SYS_PTRACE", "NET_RAW", "MKNOD", "NET_ADMIN"],
    "SecurityOpt": ["seccomp=unconfined", "apparmor=unconfined"],
    "DeviceCgroupRules": ["c 13:* rmw", "c 244:* rmw"]
  }
}`

	// Add startup script bind mount for fast iteration
	mounts = append(mounts, fmt.Sprintf("%s/wolf/sway-config/startup-app.sh:/opt/gow/startup-app.sh:ro", os.Getenv("HELIX_HOST_HOME")))
	// Mount Docker socket for full host Docker access
	mounts = append(mounts, "/var/run/docker.sock:/var/run/docker.sock")

	// Use minimal app creation that exactly matches the working XFCE configuration
	app := wolf.NewMinimalDockerApp(
		wolfAppID, // ID
		fmt.Sprintf("Personal Dev Environment %s", environmentName), // Include user's environment name
		fmt.Sprintf("PersonalDev_%s", wolfAppID),                    // Name - shorter but unique using Wolf app ID
		"helix-sway:latest",                                         // Custom Sway image with modern Wayland support and Helix branding
		env,
		mounts,
		baseCreateJSON,
		displayWidth,  // Pass user-configured display width
		displayHeight, // Pass user-configured display height
		displayFPS,    // Pass user-configured display FPS
	)

	// Try to remove any existing app with the same ID to prevent duplicates
	log.Info().Str("wolf_app_id", wolfAppID).Msg("Attempting to remove any existing Wolf app to prevent duplicates")
	err = w.wolfClient.RemoveApp(ctx, wolfAppID)
	if err != nil {
		log.Debug().Err(err).Str("wolf_app_id", wolfAppID).Msg("No existing Wolf app to remove (expected)")
	}

	// Add the app to Wolf
	err = w.wolfClient.AddApp(ctx, app)
	if err != nil {
		return nil, fmt.Errorf("failed to add personal dev app to Wolf: %w", err)
	}

	// Wolf will auto-start the container when the app is added (if auto_start_containers = true)
	// No need for fake client background sessions - Wolf handles container lifecycle directly
	wolfSessionID := "" // No session ID needed for auto-started containers

	// Create instance info
	instance := &ZedInstanceInfo{
		InstanceID:      instanceID,
		SpecTaskID:      "", // Empty for personal dev environments
		UserID:          userID,
		AppID:           wolfAppID, // Use Wolf's numeric app ID for Wolf API calls
		InstanceType:    "personal_dev",
		Status:          "starting",
		CreatedAt:       time.Now(),
		LastActivity:    time.Now(),
		ProjectPath:     fmt.Sprintf("/workspace/%s", environmentName),
		ThreadCount:     1,
		IsPersonalEnv:   true,
		EnvironmentName: environmentName,
		ConfiguredTools: []string{}, // TODO: Load from App configuration
		DataSources:     []string{}, // TODO: Load from App configuration
		StreamURL:       fmt.Sprintf("http://localhost:8090/?session=%s", wolfSessionID),
		WolfSessionID:   wolfSessionID, // Store Wolf's session ID for later API calls
		DisplayWidth:    displayWidth,  // Store user-configured display resolution
		DisplayHeight:   displayHeight,
		DisplayFPS:      displayFPS,
		ContainerName:   fmt.Sprintf("PersonalDev_%s", wolfAppID), // Store container name for direct network access
		VNCPort:         5901,                                     // VNC port inside container
	}

	w.instances[instanceID] = instance

	log.Info().
		Str("instance_id", instanceID).
		Str("wolf_session_id", wolfSessionID).
		Str("wolf_app_id", wolfAppID).
		Msg("Personal development environment created successfully via Wolf")

	return instance, nil
}

// buildPersonalDevZedCommand constructs the Zed execution command for personal dev environments
func (w *WolfExecutor) buildPersonalDevZedCommand(userID, appID, instanceID string) string {
	// Build environment variables for personal dev Zed
	envVars := []string{
		fmt.Sprintf("HELIX_API_URL=%s", w.helixAPIURL),
		fmt.Sprintf("HELIX_API_TOKEN=%s", w.helixAPIToken),
		fmt.Sprintf("ZED_INSTANCE_ID=%s", instanceID),
		fmt.Sprintf("ZED_USER_ID=%s", userID),
		fmt.Sprintf("ZED_APP_ID=%s", appID),
		fmt.Sprintf("ZED_INSTANCE_TYPE=personal_dev"),
		fmt.Sprintf("ZED_WORK_DIR=/workspace"),
		"DISPLAY=:0",
		"WAYLAND_DISPLAY=wayland-1",
	}

	// Construct the full command
	cmd := fmt.Sprintf("env %s /usr/local/bin/zed --foreground",
		joinEnvVars(envVars))

	return cmd
}

// GetPersonalDevEnvironments returns all personal dev environments for a user
func (w *WolfExecutor) GetPersonalDevEnvironments(ctx context.Context, userID string) ([]*ZedInstanceInfo, error) {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

	var personalEnvs []*ZedInstanceInfo
	for _, instance := range w.instances {
		if instance.IsPersonalEnv && instance.UserID == userID {
			personalEnvs = append(personalEnvs, instance)
		}
	}

	log.Info().
		Str("user_id", userID).
		Int("environment_count", len(personalEnvs)).
		Msg("Retrieved personal dev environments")

	return personalEnvs, nil
}

// StopPersonalDevEnvironment stops a personal development environment
func (w *WolfExecutor) StopPersonalDevEnvironment(ctx context.Context, userID, instanceID string) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	instance, exists := w.instances[instanceID]
	if !exists {
		return fmt.Errorf("personal dev environment %s not found", instanceID)
	}

	if !instance.IsPersonalEnv {
		return fmt.Errorf("instance %s is not a personal dev environment", instanceID)
	}

	if instance.UserID != userID {
		return fmt.Errorf("access denied: environment belongs to different user")
	}

	log.Info().Str("instance_id", instanceID).Msg("Stopping personal dev environment via Wolf")

	// Use consistent ID generation
	wolfAppID := w.generateWolfAppID(instance.UserID, instance.EnvironmentName)

	// Stop Wolf session using the stored Wolf session ID
	if instance.WolfSessionID != "" {
		err := w.wolfClient.StopSession(ctx, instance.WolfSessionID)
		if err != nil {
			log.Error().Err(err).Str("wolf_session_id", instance.WolfSessionID).Str("instance_id", instanceID).Msg("Failed to stop Wolf session")
			// Continue with cleanup even if stop fails
		}
	} else {
		log.Warn().Str("instance_id", instanceID).Msg("No Wolf session ID stored, cannot stop Wolf session")
	}

	// Remove the app from Wolf
	err := w.wolfClient.RemoveApp(ctx, wolfAppID)
	if err != nil {
		log.Error().Err(err).Str("wolf_app_id", wolfAppID).Msg("Failed to remove Wolf app")
		// Continue with cleanup even if removal fails
	}

	// Clean up Sway configuration file
	swayConfigPath := fmt.Sprintf("/tmp/sway-config-%s", instanceID)
	if err := os.Remove(swayConfigPath); err != nil {
		log.Warn().Err(err).Str("config_path", swayConfigPath).Msg("Failed to remove Sway config file")
	} else {
		log.Info().Str("config_path", swayConfigPath).Msg("Removed Sway config file")
	}

	// Update instance status
	instance.Status = "stopped"

	// Remove from our tracking
	delete(w.instances, instanceID)

	log.Info().Str("instance_id", instanceID).Msg("Personal dev environment stopped and cleaned up successfully")

	return nil
}

// GetPersonalDevEnvironment returns a specific personal dev environment for a user
func (w *WolfExecutor) GetPersonalDevEnvironment(ctx context.Context, userID, environmentID string) (*ZedInstanceInfo, error) {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

	instance, exists := w.instances[environmentID]
	if !exists {
		return nil, fmt.Errorf("environment not found: %s", environmentID)
	}

	if !instance.IsPersonalEnv {
		return nil, fmt.Errorf("instance %s is not a personal dev environment", environmentID)
	}

	if instance.UserID != userID {
		return nil, fmt.Errorf("access denied: environment belongs to different user")
	}

	return instance, nil
}

// ReconcilePersonalDevEnvironments cleans up orphaned configuration files
func (w *WolfExecutor) ReconcilePersonalDevEnvironments(ctx context.Context) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	log.Info().Msg("Starting personal dev environment reconciliation (Wolf apps + config cleanup)")

	// Step 1: Reconcile Wolf apps against Helix instances
	err := w.reconcileWolfApps(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to reconcile Wolf apps")
		// Continue with config cleanup even if Wolf reconciliation fails
	}

	// Clean up orphaned Sway config files
	orphanedConfigs := 0
	configPattern := "/tmp/sway-config-personal-dev-*"
	configFiles, err := filepath.Glob(configPattern)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to find Sway config files for cleanup")
	} else {
		for _, configFile := range configFiles {
			// Extract instance ID from config filename
			basename := filepath.Base(configFile)
			instanceID := strings.TrimPrefix(basename, "sway-config-")

			// Check if we have this instance tracked
			if _, exists := w.instances[instanceID]; !exists {
				log.Info().
					Str("config_file", configFile).
					Str("instance_id", instanceID).
					Msg("Found orphaned Sway config file, removing")

				err = os.Remove(configFile)
				if err != nil {
					log.Error().Err(err).Str("config_file", configFile).Msg("Failed to remove orphaned Sway config")
				} else {
					orphanedConfigs++
				}
			}
		}
	}

	log.Info().
		Int("orphaned_configs", orphanedConfigs).
		Msg("Personal dev environment reconciliation completed")

	return nil
}

// createWorkspaceDirectory creates a persistent workspace directory for an instance
func (w *WolfExecutor) createWorkspaceDirectory(instanceID string) (string, error) {
	workspaceDir := filepath.Join(w.workspaceBasePath, instanceID)

	// Create the workspace directory
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create workspace directory: %w", err)
	}

	// Create a startup script template
	startupScriptPath := filepath.Join(workspaceDir, "startup.sh")
	if _, err := os.Stat(startupScriptPath); os.IsNotExist(err) {
		startupScript := `#!/bin/bash
# Personal Development Environment Startup Script
# This script runs when your dev environment starts up
# You can add commands here to install packages, configure tools, etc.

echo "Starting up personal dev environment: ` + instanceID + `"

# Ensure workspace directory has correct permissions
sudo chown -R retro:retro ~/work

# Install JupyterLab and OnlyOffice
echo "Installing JupyterLab and OnlyOffice..."
sudo apt update
sudo apt install -y python3-pip
pip3 install jupyterlab

# Install OnlyOffice
sudo apt install -y snapd
sudo snap install onlyoffice-desktopeditors

# Example: Configure git
# git config --global user.name "Your Name"
# git config --global user.email "your.email@example.com"

# Example: Install development tools
# curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash -
# sudo apt install -y nodejs

echo "Startup script completed!"
echo "JupyterLab can be started with: jupyter lab --ip=0.0.0.0 --allow-root"
echo "OnlyOffice is available in applications menu"
`
		if err := os.WriteFile(startupScriptPath, []byte(startupScript), 0755); err != nil {
			return "", fmt.Errorf("failed to create startup script: %w", err)
		}
	}

	// Copy the welcome README for users to see when they open Zed
	welcomeReadmePath := filepath.Join(workspaceDir, "README.md")
	if _, err := os.Stat(welcomeReadmePath); os.IsNotExist(err) {
		// Read the template README
		templatePath := "/opt/helix/WORKDIR_README.md"
		welcomeContent, err := os.ReadFile(templatePath)
		if err != nil {
			// Fallback to inline content if template not found
			welcomeContent = []byte(`# Welcome to HADES! 🚀

**Helix Agentic Development Environment Service**

You're running in a **Sway tiling window manager** environment - a keyboard-driven, efficient workspace designed for developers.

## Quick Tips

- **Move windows**: ` + "`Alt + Left Drag`" + `
- **Resize windows**: ` + "`Alt + Right Drag`" + `
- **Launch apps**: Click the bar at the top
- **New terminal**: Look for the terminal icon in the top bar

## What is this?

This is your personal development container, complete with a desktop environment accessible via remote streaming. Everything you need to code, test, and deploy - all in one place.

**Have fun building! 🛠️**
`)
		}
		if err := os.WriteFile(welcomeReadmePath, welcomeContent, 0644); err != nil {
			return "", fmt.Errorf("failed to create welcome README: %w", err)
		}
	}

	return workspaceDir, nil
}

// createSwayConfig creates a Sway compositor configuration for the personal dev environment
func (w *WolfExecutor) createSwayConfig(instanceID string) error {
	swayConfigPath := fmt.Sprintf("/tmp/sway-config-%s", instanceID)

	swayConfig := `# Sway configuration for Helix Personal Dev Environment
# Generated for instance: ` + instanceID + `

# Set mod key to Super (Windows key)
set $mod Mod4

# Font for window titles
font pango:Monospace 8

# Use Mouse+$mod to drag floating windows
floating_modifier $mod

# Start a terminal
bindsym $mod+Return exec kitty

# Kill focused window
bindsym $mod+Shift+q kill

# Start launcher (fuzzel for Wayland)
bindsym $mod+d exec fuzzel

# Change focus
bindsym $mod+j focus left
bindsym $mod+k focus down
bindsym $mod+l focus up
bindsym $mod+semicolon focus right

# Move focused window
bindsym $mod+Shift+j move left
bindsym $mod+Shift+k move down
bindsym $mod+Shift+l move up
bindsym $mod+Shift+semicolon move right

# Split orientation
bindsym $mod+h split h
bindsym $mod+v split v

# Fullscreen mode
bindsym $mod+f fullscreen toggle

# Change container layout
bindsym $mod+s layout stacking
bindsym $mod+w layout tabbed
bindsym $mod+e layout toggle split

# Toggle floating
bindsym $mod+Shift+space floating toggle

# Workspaces
set $ws1 "1"
set $ws2 "2"
set $ws3 "3"
set $ws4 "4"

# Switch to workspace
bindsym $mod+1 workspace $ws1
bindsym $mod+2 workspace $ws2
bindsym $mod+3 workspace $ws3
bindsym $mod+4 workspace $ws4

# Move focused container to workspace
bindsym $mod+Shift+1 move container to workspace $ws1
bindsym $mod+Shift+2 move container to workspace $ws2
bindsym $mod+Shift+3 move container to workspace $ws3
bindsym $mod+Shift+4 move container to workspace $ws4

# Reload configuration
bindsym $mod+Shift+c reload

# Restart Sway
bindsym $mod+Shift+r restart

# Exit Sway
bindsym $mod+Shift+e exec swaynag -t warning -m 'Exit Sway?' -b 'Yes' 'swaymsg exit'

# Auto-start applications for development
exec kitty --working-directory=/home/user/work
exec --no-startup-id swaybg -c "#2e3440"

# Window rules
for_window [app_id="kitty"] focus
for_window [app_id="zed"] focus

# Output configuration for Wolf streaming
output * {
    mode 1920x1080@60Hz
    pos 0 0
}

# Input configuration
input * {
    xkb_layout "us"
    xkb_variant ""
    xkb_options ""
}
`

	if err := os.WriteFile(swayConfigPath, []byte(swayConfig), 0644); err != nil {
		return fmt.Errorf("failed to create Sway config: %w", err)
	}

	return nil
}

// reconcileWolfApps ensures Wolf has apps for all running personal dev environments
func (w *WolfExecutor) reconcileWolfApps(ctx context.Context) error {
	log.Info().Msg("Reconciling Wolf apps against Helix personal dev environments")

	// We check each instance individually and try to recreate Wolf apps as needed
	reconciledCount := 0
	recreatedCount := 0

	for instanceID, instance := range w.instances {
		if !instance.IsPersonalEnv {
			continue // Skip non-personal dev environments
		}

		if instance.Status != "running" && instance.Status != "starting" {
			continue // Skip stopped environments
		}

		// Check if Wolf app exists before trying to create it
		// Use consistent ID generation (from instance lookup)
		instance, exists := w.instances[instanceID]
		if !exists {
			return fmt.Errorf("instance not found: %s", instanceID)
		}
		wolfAppID := w.generateWolfAppID(instance.UserID, instance.EnvironmentName)

		log.Info().
			Str("instance_id", instanceID).
			Str("wolf_app_id", wolfAppID).
			Str("status", instance.Status).
			Msg("Checking if Wolf app exists for personal dev environment")

		// First check if Wolf app already exists
		appExists, checkErr := w.checkWolfAppExists(ctx, wolfAppID)
		if checkErr != nil {
			log.Error().
				Err(checkErr).
				Str("wolf_app_id", wolfAppID).
				Msg("Failed to check if Wolf app exists")
			continue // Skip this instance and continue with others
		}

		if appExists {
			log.Debug().
				Str("instance_id", instanceID).
				Str("wolf_app_id", wolfAppID).
				Msg("Wolf app already exists, skipping recreation")
			continue // App exists, no need to recreate
		}

		log.Info().
			Str("instance_id", instanceID).
			Str("wolf_app_id", wolfAppID).
			Msg("Wolf app missing, recreating")

		// Try to recreate the Wolf app for this instance
		err := w.recreateWolfAppForInstance(ctx, instance)
		if err != nil {
			log.Error().
				Err(err).
				Str("instance_id", instanceID).
				Msg("Failed to recreate Wolf app for personal dev environment")
			// Mark instance as stopped since Wolf app creation failed
			instance.Status = "stopped"
		} else {
			log.Info().
				Str("instance_id", instanceID).
				Msg("Successfully ensured Wolf app exists for personal dev environment")
			recreatedCount++
		}
		reconciledCount++
	}

	log.Info().
		Int("reconciled_count", reconciledCount).
		Int("recreated_count", recreatedCount).
		Msg("Completed Wolf app reconciliation")

	return nil
}

// recreateWolfAppForInstance recreates a Wolf app for an existing personal dev environment instance
func (w *WolfExecutor) recreateWolfAppForInstance(ctx context.Context, instance *ZedInstanceInfo) error {
	// Use consistent ID generation
	wolfAppID := w.generateWolfAppID(instance.UserID, instance.EnvironmentName)

	// Get workspace directory (should already exist)
	workspaceDir := filepath.Join(w.workspaceBasePath, instance.InstanceID)

	// Create Wolf app using the same Sway configuration as the main creation function
	env := []string{
		"GOW_REQUIRED_DEVICES=/dev/input/* /dev/dri/* /dev/nvidia*",
		"RUN_SWAY=1", // Enable Sway compositor mode in GOW launcher
		// Pass through API keys for Zed AI functionality
		fmt.Sprintf("ANTHROPIC_API_KEY=%s", os.Getenv("ANTHROPIC_API_KEY")),
		fmt.Sprintf("OPENAI_API_KEY=%s", os.Getenv("OPENAI_API_KEY")),
		fmt.Sprintf("TOGETHER_API_KEY=%s", os.Getenv("TOGETHER_API_KEY")),
		fmt.Sprintf("HF_TOKEN=%s", os.Getenv("HF_TOKEN")),
		// Additional environment variables for development
		fmt.Sprintf("HELIX_API_URL=%s", w.helixAPIURL),
		fmt.Sprintf("HELIX_API_TOKEN=%s", w.helixAPIToken),
		// Enable user startup script execution
		"HELIX_STARTUP_SCRIPT=/home/retro/work/startup.sh",
	}
	mounts := []string{
		fmt.Sprintf("%s:/home/retro/work", workspaceDir),                                    // Mount persistent workspace
		fmt.Sprintf("%s/zed-build/zed:/usr/local/bin/zed:ro", os.Getenv("HELIX_HOST_HOME")), // Mount Zed binary from host path
	}
	baseCreateJSON := `{
  "HostConfig": {
    "IpcMode": "host",
    "NetworkMode": "helix_default",
    "Privileged": false,
    "CapAdd": ["SYS_ADMIN", "SYS_NICE", "SYS_PTRACE", "NET_RAW", "MKNOD", "NET_ADMIN"],
    "SecurityOpt": ["seccomp=unconfined", "apparmor=unconfined"],
    "DeviceCgroupRules": ["c 13:* rmw", "c 244:* rmw"]
  }
}`

	// Use minimal app creation that exactly matches the working XFCE configuration
	app := wolf.NewMinimalDockerApp(
		wolfAppID, // ID
		fmt.Sprintf("Personal Dev %s", instance.EnvironmentName), // Title (no colon to avoid Docker volume syntax issues)
		fmt.Sprintf("PersonalDev_%s", wolfAppID),                 // Name - shorter but unique using Wolf app ID
		"helix-sway:latest",                                      // Custom Sway image with modern Wayland support and Helix branding
		env,
		mounts,
		baseCreateJSON,
		instance.DisplayWidth,  // Use stored display configuration
		instance.DisplayHeight, // Use stored display configuration
		instance.DisplayFPS,    // Use stored display configuration
	)

	// Try to remove any existing app first to avoid conflicts
	err := w.wolfClient.RemoveApp(ctx, wolfAppID)
	if err != nil {
		log.Debug().Err(err).Str("wolf_app_id", wolfAppID).Msg("No existing Wolf app to remove (expected)")
	}

	// Add the app to Wolf
	err = w.wolfClient.AddApp(ctx, app)
	if err != nil {
		return fmt.Errorf("failed to recreate Wolf app: %w", err)
	}

	log.Info().
		Str("instance_id", instance.InstanceID).
		Str("wolf_app_id", wolfAppID).
		Msg("Successfully recreated Wolf app for personal dev environment")

	return nil
}

// NOTE: Background session creation has been moved to Wolf's native auto_persistent_sessions feature.
// Wolf now handles container lifecycle directly through auto_start_containers = true configuration.
// No need for Helix to create fake background sessions - Wolf automatically starts containers
// when apps are added, and real Moonlight clients can connect to running containers seamlessly.

// generateRandomIP generates a unique fake IP address for RTSP routing
func generateRandomIP() string {
	// Generate a random IP in the 192.168.1.x range to avoid conflicts
	// Wolf uses fake IPs to route RTSP connections back to the correct session
	return fmt.Sprintf("192.168.1.%d", 100+time.Now().UnixNano()%155) // 100-254 range
}

// checkWolfAppExists checks if a Wolf app with the given ID already exists
func (w *WolfExecutor) checkWolfAppExists(ctx context.Context, appID string) (bool, error) {
	apps, err := w.wolfClient.ListApps(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to list Wolf apps: %w", err)
	}

	for _, app := range apps {
		if app.ID == appID {
			return true, nil
		}
	}

	return false, nil
}

// GetWolfClient returns the Wolf client for direct access to Wolf API
func (w *WolfExecutor) GetWolfClient() *wolf.Client {
	return w.wolfClient
}

// validateDisplayParams validates display configuration parameters
func validateDisplayParams(width, height, fps int) error {
	if width < 800 || width > 7680 {
		return fmt.Errorf("invalid display width: %d (must be 800-7680)", width)
	}
	if height < 600 || height > 4320 {
		return fmt.Errorf("invalid display height: %d (must be 600-4320)", height)
	}
	if fps < 30 || fps > 144 {
		return fmt.Errorf("invalid display fps: %d (must be 30-144)", fps)
	}

	// Validate aspect ratio is reasonable
	aspectRatio := float64(width) / float64(height)
	if aspectRatio < 0.5 || aspectRatio > 4.0 {
		return fmt.Errorf("invalid aspect ratio: %.2f (must be 0.5-4.0)", aspectRatio)
	}

	return nil
}
