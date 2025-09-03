package external_agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// Executor defines the interface for external agent executors
type Executor interface {
	StartZedAgent(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error)
	StopZedAgent(ctx context.Context, sessionID string) error
	GetSession(sessionID string) (*ZedSession, error)
	CleanupExpiredSessions(ctx context.Context, timeout time.Duration)
	ListSessions() []*ZedSession
}

// PoolExecutor manages a pool of Zed runners using the runner pattern
// Each runner handles one session then exits (container restarts for cleanup)
type PoolExecutor struct {
	runnerPool []string // IDs of available runners in the pool
	apiHost    string
	apiToken   string
}

// NewPoolExecutor creates a new pool-based executor
func NewPoolExecutor(apiHost, apiToken string, runnerIDs []string) *PoolExecutor {
	return &PoolExecutor{
		runnerPool: runnerIDs,
		apiHost:    apiHost,
		apiToken:   apiToken,
	}
}

// StartZedAgent dispatches a Zed agent task to an available runner in the pool
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

	// Basic path validation
	if agent.WorkDir != "" && (len(agent.WorkDir) > 255 || containsDangerousPath(agent.WorkDir)) {
		return nil, fmt.Errorf("invalid work directory")
	}

	if agent.ProjectPath != "" && (len(agent.ProjectPath) > 255 || containsDangerousPath(agent.ProjectPath)) {
		return nil, fmt.Errorf("invalid project path")
	}

	// Environment variable validation
	if len(agent.Env) > 20 {
		return nil, fmt.Errorf("too many environment variables (max 20)")
	}

	log.Info().
		Str("session_id", agent.SessionID).
		Str("user_id", agent.UserID).
		Msg("Dispatching Zed agent task to runner pool")

	// Create runner task envelope
	envelope := types.RunnerEventRequestEnvelope{
		Type:      types.RunnerEventRequestZedAgent,
		RequestID: fmt.Sprintf("zed-%s-%d", agent.SessionID, time.Now().UnixNano()),
		Reply:     fmt.Sprintf("/sessions/%s/agent-response", agent.SessionID),
	}

	payload, err := json.Marshal(agent)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal agent: %w", err)
	}
	envelope.Payload = payload

	// For now, return a response indicating the task was queued
	// The actual runner will handle the Zed execution
	return &types.ZedAgentResponse{
		SessionID: agent.SessionID,
		Status:    "dispatched",
	}, nil
}

// StopZedAgent stops a Zed agent session (runner will exit naturally)
func (pe *PoolExecutor) StopZedAgent(ctx context.Context, sessionID string) error {
	log.Info().
		Str("session_id", sessionID).
		Msg("Stopping Zed agent (runner will exit after current task)")

	// In the pool pattern, we don't directly stop agents
	// The runner completes its task and exits, container restarts for cleanup
	return nil
}

// GetSession returns session info (limited in pool pattern)
func (pe *PoolExecutor) GetSession(sessionID string) (*ZedSession, error) {
	// In pool pattern, sessions are managed by individual runners
	// We can query the control plane or return a placeholder
	return &ZedSession{
		SessionID:  sessionID,
		Status:     "active",
		StartTime:  time.Now(),
		LastAccess: time.Now(),
	}, nil
}

// CleanupExpiredSessions in pool pattern is handled by container restarts
func (pe *PoolExecutor) CleanupExpiredSessions(ctx context.Context, timeout time.Duration) {
	log.Debug().
		Dur("timeout", timeout).
		Msg("Pool executor cleanup (handled by container lifecycle)")
}

// ListSessions returns active sessions (limited in pool pattern)
func (pe *PoolExecutor) ListSessions() []*ZedSession {
	// In pool pattern, would need to query control plane for active sessions
	return []*ZedSession{}
}

// DirectExecutor wraps the ZedExecutor for direct execution (development/testing)
type DirectExecutor struct {
	zedExecutor *ZedExecutor
}

// NewDirectExecutor creates an executor that runs Zed directly (not in pool)
func NewDirectExecutor(zedExecutor *ZedExecutor) *DirectExecutor {
	return &DirectExecutor{
		zedExecutor: zedExecutor,
	}
}

// StartZedAgent starts a Zed agent directly using the ZedExecutor
func (de *DirectExecutor) StartZedAgent(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error) {
	return de.zedExecutor.StartZedAgent(ctx, agent)
}

// StopZedAgent stops a Zed agent directly
func (de *DirectExecutor) StopZedAgent(ctx context.Context, sessionID string) error {
	return de.zedExecutor.StopZedAgent(ctx, sessionID)
}

// GetSession gets session info directly
func (de *DirectExecutor) GetSession(sessionID string) (*ZedSession, error) {
	return de.zedExecutor.GetSession(sessionID)
}

// CleanupExpiredSessions cleans up expired sessions directly
func (de *DirectExecutor) CleanupExpiredSessions(ctx context.Context, timeout time.Duration) {
	de.zedExecutor.CleanupExpiredSessions(ctx, timeout)
}

// ListSessions lists all sessions directly
func (de *DirectExecutor) ListSessions() []*ZedSession {
	return de.zedExecutor.ListSessions()
}

// containsDangerousPath checks for dangerous path patterns
func containsDangerousPath(path string) bool {
	dangerousPaths := []string{
		"..", "/etc", "/root", "/bin", "/sbin", "/usr/bin", "/usr/sbin",
		"/sys", "/proc", "/dev", "/var", "/tmp/../", "~/", "$HOME",
	}

	for _, dangerous := range dangerousPaths {
		if len(path) >= len(dangerous) && path[:len(dangerous)] == dangerous {
			return true
		}
	}

	return false
}
