package controller

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"time"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/trigger/agent_work_queue"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// CreateSessionForWorkItem creates a new Helix session for a work item
func (c *Controller) CreateAgentSession(ctx context.Context, workItem *types.AgentWorkItem) (*types.Session, error) {
	// Create system prompt that includes the work item context
	systemPrompt := fmt.Sprintf(`You are an AI agent working on a specific task. Your task details:

**Task**: %s
**Description**: %s
**Source**: %s (%s)
**Priority**: %d

You have access to the "LoopInHuman" skill to request human assistance when needed, and the "JobCompleted" skill to signal when your work is done.

Your goal is to complete this task autonomously, but don't hesitate to ask for human help if you encounter complex decisions or need clarification.

Work methodically and document your progress. When you're done, use the JobCompleted skill to report your results.`,
		workItem.Name,
		workItem.Description,
		workItem.Source,
		workItem.SourceURL,
		workItem.Priority)

	// Create a new session
	session := &types.Session{
		ID:             system.GenerateSessionID(),
		Owner:          workItem.UserID,
		OwnerType:      types.OwnerTypeUser,
		OrganizationID: workItem.OrganizationID,
		Type:           types.SessionTypeText,
		Mode:           types.SessionModeInference,
		ModelName:      "claude-3-5-sonnet-20241022",
		ParentApp:      workItem.AppID,
		Created:        time.Now(),
		Updated:        time.Now(),
		Metadata: types.SessionMetadata{
			AgentType:    workItem.AgentType,
			SystemPrompt: systemPrompt,
			Priority:     workItem.Priority > 0,
		},
	}

	// Store the session
	createdSession, err := c.Options.Store.CreateSession(ctx, *session)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	log.Info().
		Str("session_id", createdSession.ID).
		Str("work_item_id", workItem.ID).
		Str("app_id", workItem.AppID).
		Str("agent_type", workItem.AgentType).
		Msg("Created Helix session for work item")

	return createdSession, nil
}

// LaunchExternalAgent launches an external agent (Zed, VS Code, etc.) for a session
func (c *Controller) LaunchExternalAgent(ctx context.Context, sessionID, agentType string) error {
	log.Info().
		Str("EXTERNAL_AGENT_DEBUG", "launch_external_agent").
		Str("session_id", sessionID).
		Str("agent_type", agentType).
		Msg("🚀 EXTERNAL_AGENT_DEBUG: LaunchExternalAgent called")

	switch agentType {
	case "zed":
		return c.launchZedAgent(ctx, sessionID)
	case "vscode":
		return c.launchVSCodeAgent(ctx, sessionID)
	default:
		log.Error().
			Str("EXTERNAL_AGENT_DEBUG", "unsupported_agent_type").
			Str("agent_type", agentType).
			Msg("❌ EXTERNAL_AGENT_DEBUG: Unsupported agent type")
		return fmt.Errorf("unsupported agent type: %s", agentType)
	}
}

// launchZedAgent dispatches a Zed agent task to the runner pool
func (c *Controller) launchZedAgent(ctx context.Context, sessionID string) error {
	log.Info().
		Str("EXTERNAL_AGENT_DEBUG", "launch_zed_agent").
		Str("session_id", sessionID).
		Msg("🎯 EXTERNAL_AGENT_DEBUG: launchZedAgent called")

	// Get session information
	session, err := c.Options.Store.GetSession(ctx, sessionID)
	if err != nil {
		log.Error().
			Str("EXTERNAL_AGENT_DEBUG", "session_load_error").
			Err(err).
			Str("session_id", sessionID).
			Msg("❌ EXTERNAL_AGENT_DEBUG: Failed to get session for Zed agent launch")
		return fmt.Errorf("failed to get session: %w", err)
	}

	log.Debug().
		Str("EXTERNAL_AGENT_DEBUG", "session_loaded").
		Str("session_id", sessionID).
		Str("owner", session.Owner).
		Str("agent_type", session.Metadata.AgentType).
		Msg("📄 EXTERNAL_AGENT_DEBUG: Session loaded successfully")

	// Check if this session is part of a multi-session SpecTask
	var specTaskContext *types.SpecTask
	var workSessionContext *types.SpecTaskWorkSession
	var isMultiSession bool

	if session.Metadata.WorkSessionID != "" {
		// This session is part of a SpecTask work session
		workSession, err := c.Options.Store.GetSpecTaskWorkSession(ctx, session.Metadata.WorkSessionID)
		if err != nil {
			log.Warn().Err(err).Str("work_session_id", session.Metadata.WorkSessionID).Msg("Failed to get work session, proceeding as single session")
		} else {
			workSessionContext = workSession
			isMultiSession = true

			specTask, err := c.Options.Store.GetSpecTask(ctx, workSession.SpecTaskID)
			if err != nil {
				log.Warn().Err(err).Str("spec_task_id", workSession.SpecTaskID).Msg("Failed to get spec task, proceeding as single session")
				isMultiSession = false
			} else {
				specTaskContext = specTask
			}
		}
	}

	// Create Zed agent request
	zedAgent := &types.ZedAgent{
		SessionID: sessionID,
		UserID:    session.Owner,
		Input:     "Initialize Zed development environment",
	}

	// Configure Zed agent based on context (SpecTask vs regular session)
	if isMultiSession && specTaskContext != nil && workSessionContext != nil {
		// Multi-session SpecTask configuration
		zedAgent.ProjectPath = specTaskContext.ProjectPath
		if zedAgent.ProjectPath == "" {
			zedAgent.ProjectPath = "/workspace/" + specTaskContext.ID
		}
		zedAgent.WorkDir = zedAgent.ProjectPath

		// Use SpecTask ID as the Zed instance identifier
		zedAgent.InstanceID = specTaskContext.ZedInstanceID
		if zedAgent.InstanceID == "" {
			zedAgent.InstanceID = "zed_instance_" + specTaskContext.ID
		}

		// Set work session specific context
		zedAgent.ThreadID = session.Metadata.ZedThreadID
		if zedAgent.ThreadID == "" {
			zedAgent.ThreadID = "thread_" + workSessionContext.ID
		}

		// Add SpecTask context to environment
		zedAgent.Env = []string{
			"SPEC_TASK_ID=" + specTaskContext.ID,
			"WORK_SESSION_ID=" + workSessionContext.ID,
			"IMPLEMENTATION_TASK_TITLE=" + workSessionContext.ImplementationTaskTitle,
			"IMPLEMENTATION_TASK_INDEX=" + fmt.Sprintf("%d", workSessionContext.ImplementationTaskIndex),
		}

		// Add git repository information for cloning
		if specTaskContext.ProjectID != "" {
			// Generate git repository URL - assume git repository service creates repo with SpecTask ID
			gitRepoID := fmt.Sprintf("spec-task-%s", specTaskContext.ID)
			helixAPIServer := os.Getenv("HELIX_API_SERVER")
			if helixAPIServer == "" {
				helixAPIServer = "http://api:8080" // Default internal Docker/K8s address
			}

			gitRepoURL := fmt.Sprintf("%s/git/%s", helixAPIServer, gitRepoID)

			zedAgent.Env = append(zedAgent.Env,
				"GIT_REPO_URL="+gitRepoURL,
				"GIT_REPO_ID="+gitRepoID,
				"HELIX_API_SERVER="+helixAPIServer,
			)

			// Add API key for git authentication if available
			if apiKey := c.getAPIKeyForUser(session.Owner); apiKey != "" {
				zedAgent.Env = append(zedAgent.Env, "HELIX_API_KEY="+apiKey)
			}
		}

		// Add workspace config if available
		if len(specTaskContext.WorkspaceConfig) > 0 {
			var workspaceEnv map[string]interface{}
			if err := json.Unmarshal(specTaskContext.WorkspaceConfig, &workspaceEnv); err == nil {
				for key, value := range workspaceEnv {
					if strValue, ok := value.(string); ok {
						zedAgent.Env = append(zedAgent.Env, fmt.Sprintf("%s=%s", key, strValue))
					}
				}
			}
		}

		log.Info().
			Str("spec_task_id", specTaskContext.ID).
			Str("work_session_id", workSessionContext.ID).
			Str("zed_instance_id", zedAgent.InstanceID).
			Str("zed_thread_id", zedAgent.ThreadID).
			Msg("Launching Zed agent for multi-session SpecTask")

	} else {
		// Single session configuration (Desktop or Exploratory session)
		if session.Metadata.ExternalAgentConfig != nil {
			externalConfig := session.Metadata.ExternalAgentConfig

			// Apply display settings from external agent configuration
			if externalConfig.DisplayWidth > 0 {
				zedAgent.DisplayWidth = externalConfig.DisplayWidth
			}
			if externalConfig.DisplayHeight > 0 {
				zedAgent.DisplayHeight = externalConfig.DisplayHeight
			}
			if externalConfig.DisplayRefreshRate > 0 {
				zedAgent.DisplayRefreshRate = externalConfig.DisplayRefreshRate
			}
		}

		log.Info().
			Str("session_id", sessionID).
			Msg("Launching Zed agent for single session")
	}

	// Wolf executor will handle authentication via its own mechanisms
	log.Info().
		Str("session_id", sessionID).
		Msg("Dispatching external agent request to Wolf executor")

	// Dispatch to Zed runner pool via pub/sub (following GPTScript pattern)
	data, err := json.Marshal(zedAgent)
	if err != nil {
		log.Error().
			Str("EXTERNAL_AGENT_DEBUG", "marshal_error").
			Err(err).
			Msg("❌ EXTERNAL_AGENT_DEBUG: Failed to marshal Zed agent request")
		return fmt.Errorf("failed to marshal Zed agent request: %w", err)
	}

	header := map[string]string{
		"kind": "zed_agent",
	}

	// Add multi-session context to headers if applicable
	if isMultiSession && specTaskContext != nil {
		header["spec_task_id"] = specTaskContext.ID
		header["multi_session"] = "true"
		if workSessionContext != nil {
			header["work_session_id"] = workSessionContext.ID
		}
	}

	log.Info().
		Str("EXTERNAL_AGENT_DEBUG", "stream_request_start").
		Str("stream", pubsub.ZedAgentRunnerStream).
		Str("queue", pubsub.ZedAgentQueue).
		Interface("header", header).
		Msg("📤 EXTERNAL_AGENT_DEBUG: Sending StreamRequest to NATS")

	// Send to runner pool (runners will compete for the task) - same pattern as GPTScript
	_, err = c.Options.PubSub.StreamRequest(
		ctx,
		pubsub.ZedAgentRunnerStream,
		pubsub.ZedAgentQueue,
		data,
		header,
		30*time.Second,
	)
	if err != nil {
		log.Error().
			Str("EXTERNAL_AGENT_DEBUG", "stream_request_error").
			Err(err).
			Str("session_id", sessionID).
			Str("user_id", session.Owner).
			Str("stream", pubsub.ZedAgentRunnerStream).
			Str("queue", pubsub.ZedAgentQueue).
			Msg("❌ EXTERNAL_AGENT_DEBUG: Failed to dispatch Zed agent to runner pool")
		return fmt.Errorf("failed to dispatch Zed agent to runner pool: %w", err)
	}

	log.Info().
		Str("EXTERNAL_AGENT_DEBUG", "stream_request_success").
		Msg("✅ EXTERNAL_AGENT_DEBUG: StreamRequest completed successfully")

	// Note: NATS StreamRequest is for lifecycle management only
	// Session communication happens via WebSocket (handled in session_handlers.go)

	// Update Zed thread status if this is a multi-session work session
	if isMultiSession && workSessionContext != nil {
		err = c.updateZedThreadStatus(ctx, workSessionContext.ID, "pending")
		if err != nil {
			log.Warn().Err(err).Str("work_session_id", workSessionContext.ID).Msg("Failed to update Zed thread status")
		}
	}

	log.Info().
		Str("EXTERNAL_AGENT_DEBUG", "dispatch_complete").
		Str("session_id", sessionID).
		Str("user_id", session.Owner).
		Bool("multi_session", isMultiSession).
		Msg("🎉 EXTERNAL_AGENT_DEBUG: Zed agent dispatched to runner pool successfully")

	return nil
}

// getAPIKeyForUser gets an API key for a user (simplified implementation)
func (c *Controller) getAPIKeyForUser(userID string) string {
	// TODO: Implement proper API key retrieval from store
	// For now, return empty string - the git HTTP server will handle authentication
	// In production, this would:
	// 1. Query the api_keys table for active keys belonging to the user
	// 2. Return the first active key
	// 3. Or create a temporary git-scoped key
	return ""
}

// generateSecurePassword generates a cryptographically secure random password
func generateSecurePassword() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 16

	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	password := make([]byte, length)
	for i := 0; i < length; i++ {
		password[i] = charset[b[i]%byte(len(charset))]
	}

	return string(password), nil
}

// updateZedThreadStatus updates the status of a Zed thread for a work session
func (c *Controller) updateZedThreadStatus(ctx context.Context, workSessionID string, status string) error {
	zedThread, err := c.Options.Store.GetSpecTaskZedThreadByWorkSession(ctx, workSessionID)
	if err != nil {
		return fmt.Errorf("failed to get Zed thread: %w", err)
	}

	zedThread.Status = types.SpecTaskZedStatus(status)
	if status == "active" {
		now := time.Now()
		zedThread.LastActivityAt = &now
	}

	err = c.Options.Store.UpdateSpecTaskZedThread(ctx, zedThread)
	if err != nil {
		return fmt.Errorf("failed to update Zed thread status: %w", err)
	}

	log.Info().
		Str("work_session_id", workSessionID).
		Str("zed_thread_id", zedThread.ID).
		Str("status", status).
		Msg("Updated Zed thread status")

	return nil
}

// launchVSCodeAgent launches a VS Code external agent for the session
func (c *Controller) launchVSCodeAgent(ctx context.Context, sessionID string) error {
	// TODO: Implement VS Code agent launcher
	return fmt.Errorf("VS Code agent not yet implemented")
}

// StopExternalAgent stops an external agent for a session
func (c *Controller) StopExternalAgent(ctx context.Context, sessionID string) error {
	// In runner pool pattern, we send a stop signal to the pool (following GPTScript pattern)
	// The runner will complete its current task and exit (container restarts for cleanup)
	stopRequest := map[string]string{
		"action":     "stop",
		"session_id": sessionID,
	}

	data, err := json.Marshal(stopRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal stop request: %w", err)
	}

	header := map[string]string{
		"kind": "stop_zed_agent",
	}

	_, err = c.Options.PubSub.StreamRequest(
		ctx,
		pubsub.ZedAgentRunnerStream,
		pubsub.ZedAgentQueue,
		data,
		header,
		10*time.Second,
	)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to send stop signal to Zed runner pool")
		return fmt.Errorf("failed to send stop signal to runner pool: %w", err)
	}

	log.Info().Str("session_id", sessionID).Msg("Stop signal sent to Zed runner pool")
	return nil
}

// GetExternalAgentStatus gets the status of an external agent
func (c *Controller) GetExternalAgentStatus(ctx context.Context, sessionID string) (*external_agent.ZedSession, error) {
	// In runner pool pattern, we check session status from the database/store
	// The runners manage their own state and report back via pub/sub

	// For now, return a basic status if we have a session record
	session, err := c.Options.Store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	// Check if this session has external agent metadata
	if session.Metadata.AgentType == "zed_external" {
		return &external_agent.ZedSession{
			SessionID:  sessionID,
			Status:     "active", // Assume active if session exists
			StartTime:  session.Created,
			LastAccess: session.Updated,
		}, nil
	}

	return nil, fmt.Errorf("session is not using external agent")
}

// UpdateAgentSessionActivity updates the last activity timestamp for an agent session
func (c *Controller) UpdateAgentSessionActivity(ctx context.Context, sessionID string) error {
	session, err := c.Options.Store.GetAgentSessionStatus(ctx, sessionID)
	if err != nil {
		// Create session status if it doesn't exist
		session = &types.AgentSessionStatus{
			ID:           fmt.Sprintf("agent-session-%s", sessionID),
			SessionID:    sessionID,
			Status:       "active",
			HealthStatus: "healthy",
			CreatedAt:    time.Now(),
		}
		return c.Options.Store.CreateAgentSessionStatus(ctx, session)
	}

	session.LastActivity = time.Now()
	session.HealthStatus = "healthy"
	return c.Options.Store.UpdateAgentSessionStatus(ctx, session)
}

// HandleAgentHelp handles a help request from an agent
func (c *Controller) HandleAgentHelp(ctx context.Context, sessionID string, helpRequest *types.HelpRequest) error {
	// Mark the session as waiting for help
	err := c.Options.Store.MarkSessionAsNeedingHelp(ctx, sessionID, helpRequest.Context)
	if err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("Failed to mark session as needing help")
	}

	// Store the help request
	err = c.Options.Store.CreateHelpRequest(ctx, helpRequest)
	if err != nil {
		return fmt.Errorf("failed to store help request: %w", err)
	}

	// Send notifications (if configured)
	// TODO: Implement notification service integration

	log.Info().
		Str("session_id", sessionID).
		Str("help_type", helpRequest.HelpType).
		Str("urgency", helpRequest.Urgency).
		Msg("Agent requested human help")

	return nil
}

// HandleAgentCompletion handles a job completion from an agent
func (c *Controller) HandleAgentCompletion(ctx context.Context, sessionID string, completion *types.JobCompletion) error {
	// Store the completion record
	err := c.Options.Store.CreateJobCompletion(ctx, completion)
	if err != nil {
		return fmt.Errorf("failed to store job completion: %w", err)
	}

	// Update session status
	status := "completed"
	if completion.ReviewNeeded {
		status = "pending_review"
	}
	err = c.Options.Store.MarkSessionAsCompleted(ctx, sessionID, status)
	if err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("Failed to mark session as completed")
	}

	// Update work item status if applicable
	if completion.WorkItemID != "" {
		workItem, err := c.Options.Store.GetAgentWorkItem(ctx, completion.WorkItemID)
		if err != nil {
			log.Warn().Err(err).Str("work_item_id", completion.WorkItemID).Msg("Failed to get work item")
		} else {
			success := completion.CompletionStatus == "fully_completed"
			if success {
				workItem.Status = "completed"
			} else {
				workItem.Status = "failed"
				workItem.LastError = completion.Summary
			}

			workItem.UpdatedAt = time.Now()
			workItem.CompletedAt = &time.Time{}
			*workItem.CompletedAt = time.Now()

			if err := c.Options.Store.UpdateAgentWorkItem(ctx, workItem); err != nil {
				log.Warn().Err(err).Str("work_item_id", completion.WorkItemID).Msg("Failed to update work item status")
			}
		}
	}

	log.Info().
		Str("session_id", sessionID).
		Str("completion_status", completion.CompletionStatus).
		Bool("review_needed", completion.ReviewNeeded).
		Msg("Agent completed work")

	return nil
}

// CleanupExpiredAgentSessions cleans up expired agent sessions
func (c *Controller) CleanupExpiredAgentSessions(ctx context.Context, timeout time.Duration) error {
	log.Info().Dur("timeout", timeout).Msg("Starting cleanup of expired agent sessions")

	// Clean up expired sessions in the database
	err := c.Options.Store.CleanupExpiredSessions(ctx, timeout)
	if err != nil {
		return fmt.Errorf("failed to cleanup expired sessions in database: %w", err)
	}

	// In runner pool pattern, cleanup is handled by container lifecycle
	// Runners exit after completing tasks, containers restart fresh
	log.Debug().Msg("External agent cleanup handled by runner pool container lifecycle")

	return nil
}

// GetAgentDashboardSummary returns comprehensive dashboard data for agent management
func (c *Controller) GetAgentDashboardSummary(ctx context.Context) (*types.AgentDashboardSummary, error) {
	// Get base dashboard data
	baseDashboard, err := c.GetDashboardData(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get base dashboard data: %w", err)
	}

	// Get active sessions
	sessionsQuery := &store.ListAgentSessionsQuery{
		Page:       0,
		PageSize:   100,
		ActiveOnly: true,
	}
	sessionsResponse, err := c.Options.Store.ListAgentSessionStatus(ctx, sessionsQuery)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get active sessions")
		sessionsResponse = &types.AgentSessionsResponse{Sessions: []*types.AgentSessionStatus{}}
	}

	// Get sessions needing help
	sessionsNeedingHelpQuery := &store.ListAgentSessionsQuery{
		Page:       0,
		PageSize:   50,
		Status:     "waiting_for_help",
		ActiveOnly: false,
	}
	sessionsNeedingHelpResponse, err := c.Options.Store.ListAgentSessionStatus(ctx, sessionsNeedingHelpQuery)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get sessions needing help")
		sessionsNeedingHelpResponse = &types.AgentSessionsResponse{Sessions: []*types.AgentSessionStatus{}}
	}
	sessionsNeedingHelp := sessionsNeedingHelpResponse.Sessions

	// Get pending work
	pendingWorkQuery := &store.ListAgentWorkItemsQuery{
		Page:     0,
		PageSize: 50,
		Status:   "pending",
		OrderBy:  "priority ASC, created_at ASC",
	}
	pendingWorkResponse, err := c.Options.Store.ListAgentWorkItems(ctx, pendingWorkQuery)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get pending work")
		pendingWorkResponse = &types.AgentWorkItemsListResponse{WorkItems: []*types.AgentWorkItem{}}
	}

	// Get running work
	runningWorkQuery := &store.ListAgentWorkItemsQuery{
		Page:     0,
		PageSize: 50,
		Status:   "in_progress",
		OrderBy:  "started_at DESC",
	}
	runningWorkResponse, err := c.Options.Store.ListAgentWorkItems(ctx, runningWorkQuery)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get running work")
		runningWorkResponse = &types.AgentWorkItemsListResponse{WorkItems: []*types.AgentWorkItem{}}
	}

	// Get recent completions
	recentCompletions, err := c.Options.Store.GetRecentCompletions(ctx, 20)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get recent completions")
		recentCompletions = []*types.JobCompletion{}
	}

	// Get pending reviews
	pendingReviews, err := c.Options.Store.GetPendingReviews(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get pending reviews")
		pendingReviews = []*types.JobCompletion{}
	}

	// Get active help requests
	activeHelpRequests, err := c.Options.Store.ListActiveHelpRequests(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get active help requests")
		activeHelpRequests = []*types.HelpRequest{}
	}

	// Get work queue stats
	workQueueStats, err := c.Options.Store.GetAgentWorkQueueStats(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get work queue stats")
		workQueueStats = &types.AgentWorkQueueStats{
			ByAgentType: make(map[string]int),
			BySource:    make(map[string]int),
			ByPriority:  make(map[string]int),
		}
	}

	return &types.AgentDashboardSummary{
		DashboardData:       baseDashboard,
		ActiveSessions:      sessionsResponse.Sessions,
		SessionsNeedingHelp: sessionsNeedingHelp,
		PendingWork:         pendingWorkResponse.WorkItems,
		RunningWork:         runningWorkResponse.WorkItems,
		RecentCompletions:   recentCompletions,
		PendingReviews:      pendingReviews,
		ActiveHelpRequests:  activeHelpRequests,
		WorkQueueStats:      workQueueStats,
		LastUpdated:         time.Now(),
	}, nil
}

// CreateSession adapter method to match HelixController interface used by AgentWorkQueueTrigger
func (c *Controller) CreateSession(ctx context.Context, req *agent_work_queue.CreateSessionRequest) (*types.Session, error) {
	// Convert CreateSessionRequest to AgentWorkItem for the existing method
	workItem := &types.AgentWorkItem{
		ID:             req.WorkItemID,
		UserID:         req.UserID,
		AppID:          req.AppID,
		OrganizationID: req.OrganizationID,
	}

	return c.CreateAgentSession(ctx, workItem)
}

// Query types for store operations
