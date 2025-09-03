package controller

import (
	"context"
	"fmt"
	"time"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/gptscript"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// CreateSessionForWorkItem creates a new Helix session for a work item
func (c *Controller) CreateSessionForWorkItem(ctx context.Context, workItem *types.AgentWorkItem) (*types.Session, error) {
	// Get the app to determine configuration
	app, err := c.Options.Store.GetApp(ctx, workItem.AppID)
	if err != nil {
		return nil, fmt.Errorf("failed to get app: %w", err)
	}

	// Create system prompt that includes the work item context
	systemPrompt := fmt.Sprintf(`You are an AI agent working on a specific task. Your task details:

**Task**: %s
**Description**: %s
**Source**: %s (%s)
**Priority**: %d

%s

You have access to the "LoopInHuman" skill to request human assistance when needed, and the "JobCompleted" skill to signal when your work is done.

Your goal is to complete this task autonomously, but don't hesitate to ask for human help if you encounter complex decisions or need clarification.

Work methodically and document your progress. When you're done, use the JobCompleted skill to report your results.`,
		workItem.Name,
		workItem.Description,
		workItem.Source,
		workItem.SourceURL,
		workItem.Priority,
		app.Config.Helix.SystemPrompt)

	// Create a new session
	session := &types.Session{
		ID:               system.GenerateSessionID(),
		Owner:            workItem.UserID,
		OwnerType:        types.OwnerTypeUser,
		OrganizationID:   workItem.OrganizationID,
		SessionType:      types.SessionTypeText,
		Mode:             types.SessionModeInference,
		ModelName:        app.Config.Helix.Model,
		ParentApp:        workItem.AppID,
		SystemPrompt:     systemPrompt,
		Created:          time.Now(),
		Updated:          time.Now(),
		InteractionCount: 0,
		Metadata: types.SessionMetadata{
			WorkItemID:  workItem.ID,
			AgentType:   workItem.AgentType,
			Source:      "agent_work_queue",
			Priority:    workItem.Priority,
			AutoManaged: true,
		},
	}

	// Store the session
	createdSession, err := c.Options.Store.CreateSession(ctx, session)
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

// launchZedAgent launches a Zed external agent for the session
func (c *Controller) launchZedAgent(ctx context.Context, sessionID string) error {
	// Create Zed agent configuration
	zedAgent := &types.ZedAgent{
		SessionID: sessionID,
		Config: types.ZedAgentConfig{
			HelixURL:    c.Options.Config.URL,
			HelixToken:  "", // Will be generated
			ProjectPath: "",
			Environment: map[string]string{
				"HELIX_SESSION_ID": sessionID,
				"HELIX_URL":        c.Options.Config.URL,
			},
		},
		Status:    "starting",
		CreatedAt: time.Now(),
	}

	// Use the existing external agent system
	if c.Options.ExternalAgentExecutor != nil {
		response, err := c.Options.ExternalAgentExecutor.StartZedAgent(ctx, zedAgent)
		if err != nil {
			log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to start Zed agent")
			return fmt.Errorf("failed to start Zed agent: %w", err)
		}

		log.Info().
			Str("session_id", sessionID).
			Str("rdp_url", response.RDPURL).
			Int("pid", response.PID).
			Msg("Successfully launched Zed external agent")

		// Update session metadata with RDP info
		session, err := c.Options.Store.GetSession(ctx, sessionID)
		if err == nil {
			if session.Metadata == nil {
				session.Metadata = make(types.SessionMetadata)
			}
			session.Metadata["rdp_url"] = response.RDPURL
			session.Metadata["agent_pid"] = response.PID
			session.Metadata["agent_status"] = response.Status
			c.Options.Store.UpdateSession(ctx, session)
		}

		return nil
	}

	// Fallback to GPTScript runner with Zed integration
	if c.Options.GPTScriptRunner != nil {
		runner := gptscript.NewZedAgentRunner(c.Options.GPTScriptRunner.Config, nil)
		return runner.StartAgent(ctx, sessionID)
	}

	return fmt.Errorf("no external agent executor available")
}

// launchVSCodeAgent launches a VS Code external agent for the session
func (c *Controller) launchVSCodeAgent(ctx context.Context, sessionID string) error {
	// TODO: Implement VS Code agent launcher
	return fmt.Errorf("VS Code agent not yet implemented")
}

// StopExternalAgent stops an external agent for a session
func (c *Controller) StopExternalAgent(ctx context.Context, sessionID string) error {
	if c.Options.ExternalAgentExecutor != nil {
		return c.Options.ExternalAgentExecutor.StopZedAgent(ctx, sessionID)
	}
	return fmt.Errorf("no external agent executor available")
}

// GetExternalAgentStatus gets the status of an external agent
func (c *Controller) GetExternalAgentStatus(ctx context.Context, sessionID string) (*external_agent.ZedSession, error) {
	if c.Options.ExternalAgentExecutor != nil {
		session, exists := c.Options.ExternalAgentExecutor.GetSession(sessionID)
		if !exists {
			return nil, fmt.Errorf("external agent session not found")
		}
		return session, nil
	}
	return nil, fmt.Errorf("no external agent executor available")
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
		success := completion.CompletionStatus == "fully_completed"
		err = c.Options.Store.CompleteWorkItem(ctx, completion.WorkItemID, success, completion.Summary)
		if err != nil {
			log.Warn().Err(err).Str("work_item_id", completion.WorkItemID).Msg("Failed to update work item status")
		}
	}

	log.Info().
		Str("session_id", sessionID).
		Str("completion_status", completion.CompletionStatus).
		Bool("review_needed", completion.ReviewNeeded).
		Msg("Agent completed work")

	return nil
}

// CleanupExpiredAgentSessions removes sessions that have been inactive for too long
func (c *Controller) CleanupExpiredAgentSessions(ctx context.Context, timeout time.Duration) error {
	log.Info().Dur("timeout", timeout).Msg("Starting cleanup of expired agent sessions")

	// Clean up expired sessions in the database
	err := c.Options.Store.CleanupExpiredSessions(ctx, timeout)
	if err != nil {
		return fmt.Errorf("failed to cleanup expired sessions in database: %w", err)
	}

	// Clean up external agent sessions if executor is available
	if c.Options.ExternalAgentExecutor != nil {
		c.Options.ExternalAgentExecutor.CleanupExpiredSessions(ctx, timeout)
	}

	return nil
}

// GetAgentDashboardData returns comprehensive dashboard data for agent management
func (c *Controller) GetAgentDashboardData(ctx context.Context) (*types.AgentDashboardSummary, error) {
	// Get base dashboard data
	baseDashboard, err := c.GetDashboardData(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get base dashboard data: %w", err)
	}

	// Get active sessions
	sessionsQuery := &ListAgentSessionsQuery{
		Page:       0,
		PageSize:   100,
		ActiveOnly: true,
	}
	sessionsResponse, err := c.Options.Store.ListAgentSessions(ctx, sessionsQuery)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get active sessions")
		sessionsResponse = &types.AgentSessionsResponse{Sessions: []*types.AgentSessionStatus{}}
	}

	// Get sessions needing help
	sessionsNeedingHelp, err := c.Options.Store.GetSessionsNeedingHelp(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get sessions needing help")
		sessionsNeedingHelp = []*types.AgentSessionStatus{}
	}

	// Get pending work
	pendingWorkQuery := &ListAgentWorkItemsQuery{
		Page:     0,
		PageSize: 50,
		Status:   "pending",
		OrderBy:  "priority ASC, created_at ASC",
	}
	pendingWorkResponse, err := c.Options.Store.ListAgentWorkItems(ctx, pendingWorkQuery)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get pending work")
		pendingWorkResponse = &types.AgentWorkItemsResponse{Items: []*types.AgentWorkItem{}}
	}

	// Get running work
	runningWorkQuery := &ListAgentWorkItemsQuery{
		Page:     0,
		PageSize: 50,
		Status:   "in_progress",
		OrderBy:  "started_at DESC",
	}
	runningWorkResponse, err := c.Options.Store.ListAgentWorkItems(ctx, runningWorkQuery)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get running work")
		runningWorkResponse = &types.AgentWorkItemsResponse{Items: []*types.AgentWorkItem{}}
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
		PendingWork:         pendingWorkResponse.Items,
		RunningWork:         runningWorkResponse.Items,
		RecentCompletions:   recentCompletions,
		PendingReviews:      pendingReviews,
		ActiveHelpRequests:  activeHelpRequests,
		WorkQueueStats:      workQueueStats,
		LastUpdated:         baseDashboard.Timestamp,
	}, nil
}

// Query types for store operations
type ListAgentSessionsQuery struct {
	Page           int
	PageSize       int
	Status         string
	AgentType      string
	UserID         string
	AppID          string
	OrganizationID string
	ActiveOnly     bool
	HealthStatus   string
	OrderBy        string
}

type ListAgentWorkItemsQuery struct {
	Page            int
	PageSize        int
	Status          string
	Source          string
	AgentType       string
	UserID          string
	AppID           string
	OrganizationID  string
	TriggerConfigID string
	AssignedOnly    bool
	UnassignedOnly  bool
	OrderBy         string
}
