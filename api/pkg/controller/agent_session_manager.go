package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
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
	switch agentType {
	case "zed":
		return c.launchZedAgent(ctx, sessionID)
	case "vscode":
		return c.launchVSCodeAgent(ctx, sessionID)
	default:
		return fmt.Errorf("unsupported agent type: %s", agentType)
	}
}

// launchZedAgent dispatches a Zed agent task to the runner pool
func (c *Controller) launchZedAgent(ctx context.Context, sessionID string) error {
	// Get session information
	session, err := c.Options.Store.GetSession(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session for Zed agent launch")
		return fmt.Errorf("failed to get session: %w", err)
	}

	// Extract external agent configuration from session metadata
	var externalConfig *types.ExternalAgentConfig
	if session.Metadata.ExternalAgentConfig != nil {
		externalConfig = session.Metadata.ExternalAgentConfig

		// Basic validation
		if err := externalConfig.Validate(); err != nil {
			log.Error().Err(err).
				Str("session_id", sessionID).
				Str("user_id", session.Owner).
				Msg("Invalid external agent configuration")
			return fmt.Errorf("invalid external agent configuration: %w", err)
		}
	}

	// Create Zed agent request
	zedAgent := &types.ZedAgent{
		SessionID:   sessionID,
		UserID:      session.Owner,
		Input:       "Initialize Zed development environment",
		ProjectPath: "",
		WorkDir:     "",
		Env:         []string{},
	}

	// Apply external agent configuration if provided
	if externalConfig != nil {
		if externalConfig.ProjectPath != "" {
			zedAgent.ProjectPath = externalConfig.ProjectPath
		}
		if externalConfig.WorkspaceDir != "" {
			zedAgent.WorkDir = externalConfig.WorkspaceDir
		}
		if len(externalConfig.EnvVars) > 0 {
			zedAgent.Env = externalConfig.EnvVars
		}
	}

	// Dispatch to Zed runner pool via pub/sub (following gptscript pattern)
	data, err := json.Marshal(zedAgent)
	if err != nil {
		return fmt.Errorf("failed to marshal Zed agent request: %w", err)
	}

	header := map[string]string{
		"kind": "zed_agent",
	}

	// Send to runner pool (runners will compete for the task)
	_, err = c.Options.PubSub.StreamRequest(
		ctx,
		pubsub.ZedAgentRunnerStream,
		pubsub.ZedAgentQueue,
		data,
		header,
		30*time.Second,
	)
	if err != nil {
		log.Error().Err(err).
			Str("session_id", sessionID).
			Str("user_id", session.Owner).
			Msg("Failed to dispatch Zed agent to runner pool")
		return fmt.Errorf("failed to dispatch Zed agent to runner pool: %w", err)
	}

	log.Info().
		Str("session_id", sessionID).
		Str("user_id", session.Owner).
		Msg("Zed agent dispatched to runner pool successfully")

	return nil
}

// launchVSCodeAgent launches a VS Code external agent for the session
func (c *Controller) launchVSCodeAgent(ctx context.Context, sessionID string) error {
	// TODO: Implement VS Code agent launcher
	return fmt.Errorf("VS Code agent not yet implemented")
}

// StopExternalAgent stops an external agent for a session
func (c *Controller) StopExternalAgent(ctx context.Context, sessionID string) error {
	// In runner pool pattern, we send a stop signal to the pool
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

// Query types for store operations
