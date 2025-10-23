package external_agent

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// WebSocketConnectionChecker provides a way to check if a Zed instance has connected via WebSocket
type WebSocketConnectionChecker interface {
	IsExternalAgentConnected(sessionID string) bool
}

// Executor defines the interface for external agent executors
type Executor interface {
	// Single-session methods (legacy)
	StartZedAgent(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error)
	StopZedAgent(ctx context.Context, sessionID string) error
	GetSession(sessionID string) (*ZedSession, error)
	CleanupExpiredSessions(ctx context.Context, timeout time.Duration)
	ListSessions() []*ZedSession

	// Multi-session SpecTask methods
	StartZedInstance(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error)
	CreateZedThread(ctx context.Context, instanceID, threadID string, config map[string]interface{}) error
	StopZedInstance(ctx context.Context, instanceID string) error
	GetInstanceStatus(instanceID string) (*ZedInstanceStatus, error)
	ListInstanceThreads(instanceID string) ([]*ZedThreadInfo, error)

	// Personal Dev Environment methods
	CreatePersonalDevEnvironment(ctx context.Context, userID, appID, environmentName string) (*ZedInstanceInfo, error)
	CreatePersonalDevEnvironmentWithDisplay(ctx context.Context, userID, appID, environmentName string, displayWidth, displayHeight, displayFPS int) (*ZedInstanceInfo, error)
	GetPersonalDevEnvironments(ctx context.Context, userID string) ([]*ZedInstanceInfo, error)
	GetPersonalDevEnvironment(ctx context.Context, userID, environmentID string) (*ZedInstanceInfo, error)
	StopPersonalDevEnvironment(ctx context.Context, userID, environmentID string) error

	// Screenshot support
	FindContainerBySessionID(ctx context.Context, helixSessionID string) (string, error)
}

// NATSExecutorAdapter adapts NATSExternalAgentService to the Executor interface
type NATSExecutorAdapter struct {
	natsService *NATSExternalAgentService
}

// PoolExecutor manages a pool of Zed runners using the runner pattern
type PoolExecutor struct {
	runnerPool []string // IDs of available runners in the pool
	apiHost    string
	apiToken   string

	// Instance tracking for multi-session support
	instances map[string]*ZedInstanceInfo
	sessions  map[string]*ZedSession
	mutex     sync.RWMutex
}

// ZedInstanceInfo tracks information about a Zed instance
type ZedInstanceInfo struct {
	InstanceID   string    `json:"instanceID"`
	SpecTaskID   string    `json:"specTaskID"`   // Optional - null for personal dev environments
	UserID       string    `json:"userID"`       // Always required
	AppID        string    `json:"appID"`        // Helix App ID for configuration (MCP servers, tools, etc.)
	InstanceType string    `json:"instanceType"` // "spec_task", "personal_dev", "shared_workspace"
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"createdAt"`
	LastActivity time.Time `json:"lastActivity"`
	ProjectPath  string    `json:"projectPath"`
	ThreadCount  int       `json:"threadCount"`

	// Personal dev environment specific
	IsPersonalEnv   bool     `json:"is_personal_env"`
	EnvironmentName string   `json:"environment_name,omitempty"` // User-friendly name
	ConfiguredTools []string `json:"configured_tools,omitempty"` // MCP servers enabled
	DataSources     []string `json:"data_sources,omitempty"`     // Connected data sources
	StreamURL       string   `json:"stream_url,omitempty"`       // Wolf streaming URL
	WolfSessionID   string   `json:"wolf_session_id,omitempty"`  // Wolf's numeric session ID for API calls

	// Display configuration for streaming
	DisplayWidth  int `json:"display_width,omitempty"`  // Streaming resolution width
	DisplayHeight int `json:"display_height,omitempty"` // Streaming resolution height
	DisplayFPS    int `json:"display_fps,omitempty"`    // Streaming framerate

	// Container information for direct network access
	ContainerName string `json:"container_name,omitempty"` // Docker container name (PersonalDev_{wolfAppID})
	VNCPort       int    `json:"vnc_port,omitempty"`       // VNC port inside container (5901)
}

// ZedInstanceStatus represents the current status of a Zed instance
type ZedInstanceStatus struct {
	InstanceID    string     `json:"instance_id"`
	SpecTaskID    string     `json:"spec_task_id,omitempty"`
	Status        string     `json:"status"`
	ThreadCount   int        `json:"thread_count"`
	ActiveThreads int        `json:"active_threads"`
	LastActivity  *time.Time `json:"last_activity,omitempty"`
	ProjectPath   string     `json:"project_path,omitempty"`
}

// ZedThreadInfo represents information about a thread within an instance
type ZedThreadInfo struct {
	ThreadID      string                 `json:"thread_id"`
	WorkSessionID string                 `json:"work_session_id"`
	Status        string                 `json:"status"`
	CreatedAt     time.Time              `json:"created_at"`
	LastActivity  *time.Time             `json:"last_activity,omitempty"`
	Config        map[string]interface{} `json:"config,omitempty"`
}

// ZedSession represents a single Zed session
type ZedSession struct {
	SessionID      string    `json:"session_id"`       // Agent session ID (key for external agents)
	HelixSessionID string    `json:"helix_session_id"` // Helix session ID (for screenshot lookup)
	UserID         string    `json:"user_id"`
	Status         string    `json:"status"`
	StartTime      time.Time `json:"start_time"`
	LastAccess     time.Time `json:"last_access"`
	ProjectPath    string    `json:"project_path,omitempty"`
	WolfAppID      string    `json:"wolf_app_id,omitempty"`     // Deprecated: Used for old app-based approach
	WolfSessionID  int64     `json:"wolf_session_id,omitempty"` // Deprecated: Used for old session-based approach
	WolfLobbyID    string    `json:"wolf_lobby_id,omitempty"`   // NEW: Lobby ID for auto-start approach
	WolfLobbyPIN   string    `json:"wolf_lobby_pin,omitempty"`  // NEW: Lobby PIN for reconnection
	ContainerName  string    `json:"container_name,omitempty"`  // Container hostname for DNS lookup

	// Keepalive session tracking (prevents stale buffer crash on rejoin)
	KeepaliveStatus    string     `json:"keepalive_status"`               // "active", "starting", "failed", "disabled"
	KeepaliveStartTime *time.Time `json:"keepalive_start_time,omitempty"` // When keepalive was started
	KeepaliveLastCheck *time.Time `json:"keepalive_last_check,omitempty"` // Last health check time
	KeepaliveError     string     `json:"keepalive_error,omitempty"`      // Error message if keepalive failed
}

// NewPoolExecutor creates a new pool-based executor
func NewPoolExecutor(apiHost, apiToken string, runnerIDs []string) *PoolExecutor {
	return &PoolExecutor{
		runnerPool: runnerIDs,
		apiHost:    apiHost,
		apiToken:   apiToken,
		instances:  make(map[string]*ZedInstanceInfo),
		sessions:   make(map[string]*ZedSession),
	}
}

// StartZedAgent dispatches a Zed agent task (handles both single and multi-session)
func (pe *PoolExecutor) StartZedAgent(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error) {
	// Validate agent request
	if agent.SessionID == "" {
		return nil, fmt.Errorf("session ID is required")
	}
	if agent.UserID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	// Basic input validation
	if len(agent.Input) > 50000 {
		return nil, fmt.Errorf("input too long (max 50000 characters)")
	}

	// Check if this is a multi-session SpecTask request
	if agent.InstanceID != "" {
		return pe.handleMultiSessionRequest(ctx, agent)
	}

	// Single session workflow (existing behavior)
	return pe.handleSingleSessionRequest(ctx, agent)
}

// handleSingleSessionRequest handles legacy single-session Zed agent requests
func (pe *PoolExecutor) handleSingleSessionRequest(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error) {
	log.Info().
		Str("session_id", agent.SessionID).
		Str("user_id", agent.UserID).
		Msg("Starting single-session Zed agent")

	// Basic path validation
	if agent.WorkDir != "" && (len(agent.WorkDir) > 255 || containsDangerousPath(agent.WorkDir)) {
		return nil, fmt.Errorf("invalid work directory")
	}
	if agent.ProjectPath != "" && (len(agent.ProjectPath) > 255 || containsDangerousPath(agent.ProjectPath)) {
		return nil, fmt.Errorf("invalid project path")
	}

	// Create session info
	session := &ZedSession{
		SessionID:  agent.SessionID,
		UserID:     agent.UserID,
		Status:     "starting",
		StartTime:  time.Now(),
		LastAccess: time.Now(),
	}

	pe.mutex.Lock()
	pe.sessions[agent.SessionID] = session
	pe.mutex.Unlock()

	response := &types.ZedAgentResponse{
		SessionID: agent.SessionID,
		Status:    "starting",
	}

	return response, nil
}

// handleMultiSessionRequest handles multi-session SpecTask Zed requests
func (pe *PoolExecutor) handleMultiSessionRequest(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error) {
	log.Info().
		Str("session_id", agent.SessionID).
		Str("instance_id", agent.InstanceID).
		Str("thread_id", agent.ThreadID).
		Str("user_id", agent.UserID).
		Msg("Handling multi-session Zed request")

	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	// Check if instance already exists
	instance, exists := pe.instances[agent.InstanceID]
	if !exists {
		// Create new Zed instance
		instance = &ZedInstanceInfo{
			InstanceID:   agent.InstanceID,
			SpecTaskID:   agent.SessionID, // SessionID contains SpecTask ID for instances
			Status:       "creating",
			CreatedAt:    time.Now(),
			LastActivity: time.Now(),
			ProjectPath:  agent.ProjectPath,
			ThreadCount:  0,
		}
		pe.instances[agent.InstanceID] = instance

		log.Info().
			Str("instance_id", agent.InstanceID).
			Str("spec_task_id", instance.SpecTaskID).
			Str("project_path", agent.ProjectPath).
			Msg("Created new Zed instance")
	}

	// Create thread if specified
	if agent.ThreadID != "" {
		threadConfig := map[string]interface{}{
			"session_id": agent.SessionID,
			"user_id":    agent.UserID,
			"env":        agent.Env,
		}

		err := pe.createThreadInInstance(agent.InstanceID, agent.ThreadID, threadConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create thread: %w", err)
		}

		instance.ThreadCount++
		instance.LastActivity = time.Now()
	}

	return &types.ZedAgentResponse{
		SessionID: agent.SessionID,
		// Wolf handles streaming via Moonlight protocol
	}, nil
}

// StartZedInstance creates a new Zed instance (explicit method for service layer)
func (pe *PoolExecutor) StartZedInstance(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error) {
	return pe.handleMultiSessionRequest(ctx, agent)
}

// CreateZedThread creates a new thread within an existing Zed instance
func (pe *PoolExecutor) CreateZedThread(ctx context.Context, instanceID, threadID string, config map[string]interface{}) error {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	return pe.createThreadInInstance(instanceID, threadID, config)
}

// StopZedAgent stops a single-session Zed agent (legacy)
func (pe *PoolExecutor) StopZedAgent(ctx context.Context, sessionID string) error {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	session, exists := pe.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.Status = "stopped"
	delete(pe.sessions, sessionID)

	log.Info().
		Str("session_id", sessionID).
		Msg("Stopped single-session Zed agent")

	return nil
}

// StopZedInstance stops a Zed instance and all its threads
func (pe *PoolExecutor) StopZedInstance(ctx context.Context, instanceID string) error {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	instance, exists := pe.instances[instanceID]
	if !exists {
		return fmt.Errorf("instance not found: %s", instanceID)
	}

	instance.Status = "stopped"
	delete(pe.instances, instanceID)

	log.Info().
		Str("instance_id", instanceID).
		Str("spec_task_id", instance.SpecTaskID).
		Msg("Stopped Zed instance")

	return nil
}

// GetSession returns information about a single session (legacy)
func (pe *PoolExecutor) GetSession(sessionID string) (*ZedSession, error) {
	pe.mutex.RLock()
	defer pe.mutex.RUnlock()

	session, exists := pe.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	return session, nil
}

// GetInstanceStatus returns the status of a Zed instance
func (pe *PoolExecutor) GetInstanceStatus(instanceID string) (*ZedInstanceStatus, error) {
	pe.mutex.RLock()
	defer pe.mutex.RUnlock()

	instance, exists := pe.instances[instanceID]
	if !exists {
		return nil, fmt.Errorf("instance not found: %s", instanceID)
	}

	return &ZedInstanceStatus{
		InstanceID:   instance.InstanceID,
		SpecTaskID:   instance.SpecTaskID,
		Status:       instance.Status,
		ThreadCount:  instance.ThreadCount,
		LastActivity: &instance.LastActivity,
		ProjectPath:  instance.ProjectPath,
	}, nil
}

// ListInstanceThreads returns all threads for a Zed instance
func (pe *PoolExecutor) ListInstanceThreads(instanceID string) ([]*ZedThreadInfo, error) {
	pe.mutex.RLock()
	defer pe.mutex.RUnlock()

	// In a real implementation, this would query the actual Zed instance
	// For now, return mock data
	return []*ZedThreadInfo{
		{
			ThreadID:      "thread_1",
			WorkSessionID: "ws_1",
			Status:        "active",
			CreatedAt:     time.Now(),
		},
	}, nil
}

// ListSessions returns all active single sessions (legacy)
func (pe *PoolExecutor) ListSessions() []*ZedSession {
	pe.mutex.RLock()
	defer pe.mutex.RUnlock()

	sessions := make([]*ZedSession, 0, len(pe.sessions))
	for _, session := range pe.sessions {
		sessions = append(sessions, session)
	}

	return sessions
}

// CleanupExpiredSessions removes old sessions and instances
func (pe *PoolExecutor) CleanupExpiredSessions(ctx context.Context, timeout time.Duration) {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	now := time.Now()

	// Cleanup expired single sessions
	for sessionID, session := range pe.sessions {
		if now.Sub(session.LastAccess) > timeout {
			delete(pe.sessions, sessionID)
			log.Info().
				Str("session_id", sessionID).
				Dur("age", now.Sub(session.LastAccess)).
				Msg("Cleaned up expired single session")
		}
	}

	// Cleanup expired instances
	for instanceID, instance := range pe.instances {
		if now.Sub(instance.LastActivity) > timeout {
			delete(pe.instances, instanceID)
			log.Info().
				Str("instance_id", instanceID).
				Str("spec_task_id", instance.SpecTaskID).
				Dur("age", now.Sub(instance.LastActivity)).
				Msg("Cleaned up expired Zed instance")
		}
	}
}

// FindContainerBySessionID implements Executor interface
func (pe *PoolExecutor) FindContainerBySessionID(ctx context.Context, helixSessionID string) (string, error) {
	return "", fmt.Errorf("screenshot support not available for pool-based executors")
}

// Personal Dev Environment methods (not supported by PoolExecutor)
func (pe *PoolExecutor) CreatePersonalDevEnvironment(ctx context.Context, userID, appID, environmentName string) (*ZedInstanceInfo, error) {
	return nil, fmt.Errorf("personal dev environments not supported by pool-based executors")
}

func (pe *PoolExecutor) CreatePersonalDevEnvironmentWithDisplay(ctx context.Context, userID, appID, environmentName string, displayWidth, displayHeight, displayFPS int) (*ZedInstanceInfo, error) {
	return nil, fmt.Errorf("personal dev environments not supported by pool-based executors")
}

func (pe *PoolExecutor) GetPersonalDevEnvironments(ctx context.Context, userID string) ([]*ZedInstanceInfo, error) {
	return nil, fmt.Errorf("personal dev environments not supported by pool-based executors")
}

func (pe *PoolExecutor) GetPersonalDevEnvironment(ctx context.Context, userID, environmentID string) (*ZedInstanceInfo, error) {
	return nil, fmt.Errorf("personal dev environments not supported by pool-based executors")
}

func (pe *PoolExecutor) StopPersonalDevEnvironment(ctx context.Context, userID, environmentID string) error {
	return fmt.Errorf("personal dev environments not supported by pool-based executors")
}

// Private helper methods

func (pe *PoolExecutor) createThreadInInstance(instanceID, threadID string, config map[string]interface{}) error {
	instance, exists := pe.instances[instanceID]
	if !exists {
		return fmt.Errorf("instance not found: %s", instanceID)
	}

	// In a real implementation, this would send thread creation command to the Zed instance
	log.Info().
		Str("instance_id", instanceID).
		Str("thread_id", threadID).
		Interface("config", config).
		Msg("Creating thread in Zed instance")

	instance.LastActivity = time.Now()
	return nil
}

func generatePassword() string {
	// Generate a cryptographically secure random password for RDP access
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 16

	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		// Fail securely - no fallback passwords
		log.Error().Err(err).Msg("Failed to generate secure random password - cannot proceed")
		panic(fmt.Sprintf("Failed to generate secure RDP password: %v", err))
	}

	// Convert random bytes to charset characters
	password := make([]byte, length)
	for i := 0; i < length; i++ {
		password[i] = charset[b[i]%byte(len(charset))]
	}

	return fmt.Sprintf("zed_%s_%d", string(password), time.Now().Unix())
}

func containsDangerousPath(path string) bool {
	// Basic path validation to prevent directory traversal
	dangerous := []string{"../", "..\\", "/etc/", "/root/", "C:\\Windows\\"}
	for _, d := range dangerous {
		if len(path) > len(d) && path[:len(d)] == d {
			return true
		}
	}
	return false
}

// NewNATSExecutorAdapter creates an adapter for NATSExternalAgentService
func NewNATSExecutorAdapter(natsService *NATSExternalAgentService) Executor {
	return &NATSExecutorAdapter{
		natsService: natsService,
	}
}

// StartZedAgent implements Executor interface
func (adapter *NATSExecutorAdapter) StartZedAgent(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error) {
	log.Info().
		Str("session_id", agent.SessionID).
		Str("user_id", agent.UserID).
		Str("project_path", agent.ProjectPath).
		Msg("NATS adapter starting external agent")

	response, err := adapter.natsService.AssignExternalAgent(ctx, agent)
	if err != nil {
		log.Error().
			Err(err).
			Str("session_id", agent.SessionID).
			Msg("NATS adapter failed to assign external agent")
		return nil, err
	}

	log.Info().
		Str("session_id", agent.SessionID).
		Str("status", response.Status).
		Msg("NATS adapter successfully assigned external agent")

	return response, nil
}

// StopZedAgent implements Executor interface
func (adapter *NATSExecutorAdapter) StopZedAgent(ctx context.Context, sessionID string) error {
	log.Info().Str("session_id", sessionID).Msg("NATS adapter stopping external agent")

	err := adapter.natsService.StopAgentSession(ctx, sessionID)
	if err != nil {
		log.Error().
			Err(err).
			Str("session_id", sessionID).
			Msg("NATS adapter failed to stop external agent")
	}

	return err
}

// GetSession implements Executor interface
func (adapter *NATSExecutorAdapter) GetSession(sessionID string) (*ZedSession, error) {
	log.Debug().Str("session_id", sessionID).Msg("NATS adapter getting session")

	session, err := adapter.natsService.GetAgentSession(sessionID)
	if err != nil {
		log.Error().
			Err(err).
			Str("session_id", sessionID).
			Msg("NATS adapter failed to get session")

		// Debug: List all available sessions
		allSessions := adapter.natsService.ListActiveSessions()
		log.Debug().
			Int("total_sessions", len(allSessions)).
			Str("requested_session_id", sessionID).
			Msg("Available sessions in NATS service")

		for _, s := range allSessions {
			log.Debug().
				Str("available_session_id", s.ID).
				Str("agent_id", s.AgentID).
				Str("status", s.Status).
				Msg("Available NATS session")
		}

		return nil, err
	}

	log.Debug().
		Str("session_id", sessionID).
		Str("agent_id", session.AgentID).
		Str("status", session.Status).
		Msg("NATS adapter found session")

	// Convert AgentSession to ZedSession
	return &ZedSession{
		SessionID:  session.ID,
		UserID:     session.HelixSession, // Using HelixSession as UserID placeholder
		Status:     session.Status,
		StartTime:  session.CreatedAt,
		LastAccess: session.LastActivity,
	}, nil
}

// CleanupExpiredSessions implements Executor interface
func (adapter *NATSExecutorAdapter) CleanupExpiredSessions(ctx context.Context, timeout time.Duration) {
	// The NATS service handles cleanup internally
	log.Debug().Msg("Cleanup handled by NATS service")
}

// ListSessions implements Executor interface
func (adapter *NATSExecutorAdapter) ListSessions() []*ZedSession {
	sessions := adapter.natsService.ListActiveSessions()
	zedSessions := make([]*ZedSession, len(sessions))

	log.Debug().Int("session_count", len(sessions)).Msg("NATS adapter listing sessions")

	for i, session := range sessions {
		zedSessions[i] = &ZedSession{
			SessionID:  session.ID,
			UserID:     session.HelixSession,
			Status:     session.Status,
			StartTime:  session.CreatedAt,
			LastAccess: session.LastActivity,
		}

		log.Debug().
			Str("session_id", session.ID).
			Str("status", session.Status).
			Str("agent_id", session.AgentID).
			Msg("Listed NATS session")
	}

	return zedSessions
}

// StartZedInstance implements Executor interface
func (adapter *NATSExecutorAdapter) StartZedInstance(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error) {
	return adapter.natsService.AssignExternalAgent(ctx, agent)
}

// CreateZedThread implements Executor interface
func (adapter *NATSExecutorAdapter) CreateZedThread(ctx context.Context, instanceID, threadID string, config map[string]interface{}) error {
	// For NATS-based agents, thread creation is handled by the agent itself
	log.Info().
		Str("instance_id", instanceID).
		Str("thread_id", threadID).
		Msg("Thread creation delegated to external agent")
	return nil
}

// StopZedInstance implements Executor interface
func (adapter *NATSExecutorAdapter) StopZedInstance(ctx context.Context, instanceID string) error {
	// Find session by instance ID and stop it
	sessions := adapter.natsService.ListActiveSessions()
	for _, session := range sessions {
		if session.InstanceID == instanceID {
			return adapter.natsService.StopAgentSession(ctx, session.ID)
		}
	}
	return fmt.Errorf("instance not found: %s", instanceID)
}

// GetInstanceStatus implements Executor interface
func (adapter *NATSExecutorAdapter) GetInstanceStatus(instanceID string) (*ZedInstanceStatus, error) {
	sessions := adapter.natsService.ListActiveSessions()
	for _, session := range sessions {
		if session.InstanceID == instanceID {
			return &ZedInstanceStatus{
				InstanceID:   instanceID,
				SpecTaskID:   session.HelixSession,
				Status:       session.Status,
				ThreadCount:  len(session.ZedContexts),
				LastActivity: &session.LastActivity,
			}, nil
		}
	}
	return nil, fmt.Errorf("instance not found: %s", instanceID)
}

// ListInstanceThreads implements Executor interface
func (adapter *NATSExecutorAdapter) ListInstanceThreads(instanceID string) ([]*ZedThreadInfo, error) {
	sessions := adapter.natsService.ListActiveSessions()
	for _, session := range sessions {
		if session.InstanceID == instanceID {
			threads := make([]*ZedThreadInfo, 0, len(session.ZedContexts))
			for contextID, interactionID := range session.ZedContexts {
				threads = append(threads, &ZedThreadInfo{
					ThreadID:      contextID,
					WorkSessionID: interactionID,
					Status:        "active",
					CreatedAt:     session.CreatedAt,
					LastActivity:  &session.LastActivity,
				})
			}
			return threads, nil
		}
	}
	return nil, fmt.Errorf("instance not found: %s", instanceID)
}

// FindContainerBySessionID implements Executor interface
func (adapter *NATSExecutorAdapter) FindContainerBySessionID(ctx context.Context, helixSessionID string) (string, error) {
	return "", fmt.Errorf("screenshot support not available for NATS-based external agents")
}

// Personal Dev Environment methods (not supported by NATSExecutorAdapter)
func (adapter *NATSExecutorAdapter) CreatePersonalDevEnvironment(ctx context.Context, userID, appID, environmentName string) (*ZedInstanceInfo, error) {
	return nil, fmt.Errorf("personal dev environments not supported by NATS-based external agents")
}

func (adapter *NATSExecutorAdapter) CreatePersonalDevEnvironmentWithDisplay(ctx context.Context, userID, appID, environmentName string, displayWidth, displayHeight, displayFPS int) (*ZedInstanceInfo, error) {
	return nil, fmt.Errorf("personal dev environments not supported by NATS-based external agents")
}

func (adapter *NATSExecutorAdapter) GetPersonalDevEnvironments(ctx context.Context, userID string) ([]*ZedInstanceInfo, error) {
	return nil, fmt.Errorf("personal dev environments not supported by NATS-based external agents")
}

func (adapter *NATSExecutorAdapter) GetPersonalDevEnvironment(ctx context.Context, userID, environmentID string) (*ZedInstanceInfo, error) {
	return nil, fmt.Errorf("personal dev environments not supported by NATS-based external agents")
}

func (adapter *NATSExecutorAdapter) StopPersonalDevEnvironment(ctx context.Context, userID, environmentID string) error {
	return fmt.Errorf("personal dev environments not supported by NATS-based external agents")
}
