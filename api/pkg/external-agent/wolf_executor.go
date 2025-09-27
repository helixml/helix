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

	// Create Wolf app for this Zed instance
	appID := fmt.Sprintf("zed-agent-%s", agent.SessionID)

	// Build Zed command with environment variables
	zedCmd := w.buildZedCommand(agent)

	app := &wolf.App{
		ID:                     appID,
		Title:                  fmt.Sprintf("Zed Agent - %s", agent.SessionID),
		IconPngPath:            nil,
		H264GstPipeline:        "", // Use Wolf defaults
		HEVCGstPipeline:        "", // Use Wolf defaults
		AV1GstPipeline:         "", // Use Wolf defaults
		OpusGstPipeline:        "", // Use Wolf defaults
		RenderNode:             "", // Use Wolf defaults
		StartVirtualCompositor: true,
		StartAudioServer:       true,
		SupportHDR:             false,
		Runner: wolf.AppRunner{
			Type:   "process",
			RunCmd: zedCmd,
		},
	}

	// Add the app to Wolf
	err := w.wolfClient.AddApp(ctx, app)
	if err != nil {
		return nil, fmt.Errorf("failed to add Zed app to Wolf: %w", err)
	}

	// Create a Wolf session for this app
	session := &wolf.Session{
		AppID:             appID,
		ClientID:          "342532221405053742", // Use valid paired client ID from Wolf config
		ClientIP:          "127.0.0.1",
		VideoWidth:        1920,
		VideoHeight:       1080,
		VideoRefreshRate:  60,
		AudioChannelCount: 2,
		ClientSettings: wolf.ClientSettings{
			RunUID:              1000,
			RunGID:              1000,
			MouseAcceleration:   1.0,
			HScrollAcceleration: 1.0,
			VScrollAcceleration: 1.0,
			ControllersOverride: []string{},
		},
		AESKey:     "9d804e47a6aa6624b7d4b502b32cc522", // 32-char hex string for 16-byte AES key
		AESIV:      "0123456789abcdef",                 // 16-char hex string for 8-byte IV
		RTSPFakeIP: "192.168.1.100",                   // Fake IP address for RTSP streaming
	}

	wolfSessionID, err := w.wolfClient.CreateSession(ctx, session)
	if err != nil {
		// Clean up the app if session creation fails
		w.wolfClient.RemoveApp(ctx, appID)
		return nil, fmt.Errorf("failed to create Wolf session: %w", err)
	}

	// Store session info
	zedSession := &ZedSession{
		SessionID:   agent.SessionID,
		UserID:      agent.UserID,
		Status:      "running",
		StartTime:   time.Now(),
		LastAccess:  time.Now(),
		ProjectPath: agent.ProjectPath,
		// RDP URL would come from Wolf streaming - for now we'll use a placeholder
		RDPURL:      fmt.Sprintf("http://localhost:8090/?session=%s", wolfSessionID),
		RDPPassword: "", // Wolf handles auth differently
	}

	w.sessions[agent.SessionID] = zedSession

	log.Info().
		Str("session_id", agent.SessionID).
		Str("wolf_session_id", wolfSessionID).
		Str("app_id", appID).
		Msg("Zed agent started successfully via Wolf")

	return &types.ZedAgentResponse{
		SessionID: agent.SessionID,
		RDPURL:    zedSession.RDPURL,
		Status:    "running",
		PID:       0, // Wolf manages the process
	}, nil
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

// CreatePersonalDevEnvironment creates a new personal development environment
func (w *WolfExecutor) CreatePersonalDevEnvironment(ctx context.Context, userID, appID, environmentName string) (*ZedInstanceInfo, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	instanceID := fmt.Sprintf("personal-dev-%s-%d", userID, time.Now().Unix())

	log.Info().
		Str("instance_id", instanceID).
		Str("user_id", userID).
		Str("app_id", appID).
		Str("environment_name", environmentName).
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
	wolfAppID := fmt.Sprintf("helix-personal-dev-%s", instanceID)

	// Note: Using exact XFCE configuration for stability
	// TODO: Re-enable custom environment variables once XFCE baseline is working
	// envVars := []string{
	//	fmt.Sprintf("HELIX_API_URL=%s", w.helixAPIURL),
	//	fmt.Sprintf("HELIX_API_TOKEN=%s", w.helixAPIToken),
	//	fmt.Sprintf("ZED_INSTANCE_ID=%s", instanceID),
	//	fmt.Sprintf("ZED_USER_ID=%s", userID),
	//	fmt.Sprintf("ZED_APP_ID=%s", appID),
	//	fmt.Sprintf("ZED_INSTANCE_TYPE=personal_dev"),
	//	fmt.Sprintf("ZED_WORK_DIR=/home/user/work"),
	//	"GOW_REQUIRED_DEVICES=/dev/input/* /dev/dri/* /dev/nvidia*",
	//	"HELIX_STARTUP_SCRIPT=/home/user/work/startup.sh",
	// }

	app := &wolf.App{
		ID:                     wolfAppID,
		Title:                  fmt.Sprintf("Personal Dev: %s", environmentName),
		IconPngPath:            nil,
		H264GstPipeline:        "", // Use Wolf defaults
		HEVCGstPipeline:        "", // Use Wolf defaults
		AV1GstPipeline:         "", // Use Wolf defaults
		OpusGstPipeline:        "", // Use Wolf defaults
		RenderNode:             "", // Use Wolf defaults
		StartVirtualCompositor: true,
		StartAudioServer:       true,
		SupportHDR:             false,
		Runner: wolf.AppRunner{
			Type:  "docker",
			Image: "ghcr.io/games-on-whales/xfce:edge", // Use exact same XFCE image as working config
			Name:  fmt.Sprintf("WolfXFCE_%s", instanceID),  // Use similar naming pattern
			Env: []string{
				"GOW_REQUIRED_DEVICES=/dev/input/* /dev/dri/* /dev/nvidia*", // Exact same as XFCE working config
			},
			Mounts: []string{
				"./zed-build:/zed-build", // Mount Zed binary for updates
				fmt.Sprintf("%s:/home/user/work", workspaceDir), // Mount persistent workspace
			},
			Devices: []string{}, // Use empty devices array like XFCE config
			Ports:   []string{}, // No external ports needed for personal dev environments
			BaseCreateJSON: `{
  "HostConfig": {
    "IpcMode": "host",
    "Privileged": false,
    "CapAdd": ["SYS_ADMIN", "SYS_NICE", "SYS_PTRACE", "NET_RAW", "MKNOD", "NET_ADMIN"],
    "SecurityOpt": ["seccomp=unconfined", "apparmor=unconfined"],
    "DeviceCgroupRules": ["c 13:* rmw", "c 244:* rmw"]
  }
}`,
		},
	}

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

	// Create a Wolf session for this app
	session := &wolf.Session{
		AppID:             wolfAppID,
		ClientID:          "342532221405053742", // Use valid paired client ID from Wolf config
		ClientIP:          "127.0.0.1",
		VideoWidth:        1920,
		VideoHeight:       1080,
		VideoRefreshRate:  60,
		AudioChannelCount: 2,
		ClientSettings: wolf.ClientSettings{
			RunUID:              1000,
			RunGID:              1000,
			MouseAcceleration:   1.0,
			HScrollAcceleration: 1.0,
			VScrollAcceleration: 1.0,
			ControllersOverride: []string{},
		},
		AESKey:     "9d804e47a6aa6624b7d4b502b32cc522", // 32-char hex string for 16-byte AES key
		AESIV:      "0123456789abcdef",                 // 16-char hex string for 8-byte IV
		RTSPFakeIP: "192.168.1.100",                   // Fake IP address for RTSP streaming
	}

	wolfSessionID, err := w.wolfClient.CreateSession(ctx, session)
	if err != nil {
		// Clean up the app if session creation fails
		w.wolfClient.RemoveApp(ctx, wolfAppID)
		return nil, fmt.Errorf("failed to create Wolf session: %w", err)
	}

	// Create instance info
	instance := &ZedInstanceInfo{
		InstanceID:      instanceID,
		SpecTaskID:      "", // Empty for personal dev environments
		UserID:          userID,
		AppID:           appID,
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

	wolfAppID := fmt.Sprintf("helix-personal-dev-%s", instanceID)

	// Stop Wolf session
	err := w.wolfClient.StopSession(ctx, instanceID)
	if err != nil {
		log.Error().Err(err).Str("instance_id", instanceID).Msg("Failed to stop Wolf session")
		// Continue with cleanup even if stop fails
	}

	// Remove the app from Wolf
	err = w.wolfClient.RemoveApp(ctx, wolfAppID)
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

	log.Info().Msg("Starting personal dev environment reconciliation (config cleanup)")

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

	// Create a README explaining the workspace
	readmePath := filepath.Join(workspaceDir, "README.md")
	if _, err := os.Stat(readmePath); os.IsNotExist(err) {
		readme := `# Personal Development Environment Workspace

This is your persistent workspace directory for instance: ` + instanceID + `

## Contents

- **startup.sh**: Custom startup script that runs when your environment starts
- **Your files**: Any files you create here will persist across environment restarts

## Pre-installed Tools

Your environment comes with:
- **JupyterLab**: Web-based interactive development environment for notebooks
- **OnlyOffice**: Full office suite for documents, spreadsheets, and presentations

## Startup Script

Edit the ` + "`startup.sh`" + ` file to customize your environment:
- Install additional packages with ` + "`sudo apt install`" + `
- Configure development tools
- Set up environment variables
- Clone repositories
- Run initialization commands

The startup script runs with sudo privileges, so you can install system packages.

## Persistence

This entire directory is mounted as ` + "`/home/user/work`" + ` in your development environment.
All files and changes you make here will persist even if you stop and restart the environment.

## Tips

- Use ` + "`sudo apt update && sudo apt install package-name`" + ` to install new packages
- Your startup script runs every time the environment starts, so make commands idempotent
- Large files and dependencies installed via the startup script will persist
- Consider using package managers like npm, pip, cargo for language-specific tools
- Start JupyterLab with: ` + "`jupyter lab --ip=0.0.0.0 --allow-root`" + `
- OnlyOffice applications are available in the desktop applications menu
`
		if err := os.WriteFile(readmePath, []byte(readme), 0644); err != nil {
			return "", fmt.Errorf("failed to create README: %w", err)
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

// GetWolfClient returns the Wolf client for direct access to Wolf API
func (w *WolfExecutor) GetWolfClient() *wolf.Client {
	return w.wolfClient
}
