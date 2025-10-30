package controller

import (
	"context"
	"fmt"
	"time"

	"encoding/json"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
)

// GetAgentDashboardData returns comprehensive dashboard data including agent sessions and work items
func (c *Controller) GetAgentDashboardData(ctx context.Context) (*types.AgentDashboardData, error) {
	// Get the base dashboard data
	baseDashboardData, err := c.GetDashboardData(ctx)
	if err != nil {
		return nil, err
	}

	// Get active agent sessions
	sessionsQuery := &store.ListAgentSessionsQuery{
		Page:       0,
		PageSize:   100,
		ActiveOnly: true,
	}
	sessionsResponse, err := c.Options.Store.ListAgentSessionStatus(ctx, sessionsQuery)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get active agent sessions")
		sessionsResponse = &types.AgentSessionsResponse{Sessions: []*types.AgentSessionStatus{}}
	}

	// Get pending work items
	workQuery := &store.ListAgentWorkItemsQuery{
		Page:     0,
		PageSize: 50,
		Status:   "pending",
		OrderBy:  "priority ASC, created_at ASC",
	}
	workResponse, err := c.Options.Store.ListAgentWorkItems(ctx, workQuery)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get pending work items")
		workResponse = &types.AgentWorkItemsListResponse{WorkItems: []*types.AgentWorkItem{}}
	}

	// Get active help requests
	activeHelpRequests, err := c.Options.Store.ListActiveHelpRequests(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get active help requests")
		activeHelpRequests = []*types.HelpRequest{}
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

	return &types.AgentDashboardData{
		DashboardData:       baseDashboardData,
		ActiveSessions:      sessionsResponse.Sessions,
		PendingWork:         workResponse.WorkItems,
		HelpRequests:        activeHelpRequests,
		SessionsNeedingHelp: sessionsNeedingHelpResponse.Sessions,
		RecentCompletions:   recentCompletions,
		PendingReviews:      pendingReviews,
	}, nil
}

// GetAgentSessionStatus returns detailed status for a specific agent session
func (c *Controller) GetAgentSessionStatus(ctx context.Context, sessionID string) (*types.AgentSession, error) {
	session, err := c.Options.Store.GetAgentSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// Update last activity to now since we're checking on it
	session.LastActivity = time.Now()
	if err := c.Options.Store.UpdateAgentSession(ctx, session); err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("failed to update session last activity")
	}

	return session, nil
}

// UpdateAgentSessionStatus updates the status of an agent session
func (c *Controller) UpdateAgentSessionStatus(ctx context.Context, sessionID, status, currentTask string) error {
	session, err := c.Options.Store.GetAgentSession(ctx, sessionID)
	if err != nil {
		// Create session if it doesn't exist
		session = &types.AgentSession{
			ID:           fmt.Sprintf("session-%s", sessionID),
			SessionID:    sessionID,
			AgentType:    "helix", // Default to helix
			Status:       status,
			CurrentTask:  currentTask,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
			LastActivity: time.Now(),
			HealthStatus: "unknown",
		}
		return c.Options.Store.CreateAgentSession(ctx, session)
	}

	// Update existing session
	session.Status = status
	session.CurrentTask = currentTask
	session.LastActivity = time.Now()
	session.UpdatedAt = time.Now()

	return c.Options.Store.UpdateAgentSession(ctx, session)
}

// CreateWorkItem creates a new work item in the agent scheduler
func (c *Controller) CreateWorkItem(ctx context.Context, req *CreateWorkItemRequest) (*types.AgentWorkItem, error) {
	// Convert map[string]interface{} to datatypes.JSON
	var workData, config, metadata datatypes.JSON

	if req.WorkData != nil {
		workDataBytes, err := json.Marshal(req.WorkData)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal work data: %w", err)
		}
		workData = datatypes.JSON(workDataBytes)
	}

	if req.Config != nil {
		configBytes, err := json.Marshal(req.Config)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal config: %w", err)
		}
		config = datatypes.JSON(configBytes)
	}

	if req.Metadata != nil {
		metadataBytes, err := json.Marshal(req.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
		metadata = datatypes.JSON(metadataBytes)
	}

	workItem := &types.AgentWorkItem{
		ID:          fmt.Sprintf("work-%d", time.Now().UnixNano()),
		Name:        req.Name,
		Description: req.Description,
		Source:      req.Source,
		SourceID:    req.SourceID,
		Priority:    req.Priority,
		Status:      "pending",
		AgentType:   req.AgentType,
		UserID:      req.UserID,
		AppID:       req.AppID,
		WorkData:    workData,
		Config:      config,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		MaxRetries:  3,
		Metadata:    metadata,
	}

	if err := c.Options.Store.CreateAgentWorkItem(ctx, workItem); err != nil {
		return nil, err
	}

	log.Info().
		Str("work_item_id", workItem.ID).
		Str("name", workItem.Name).
		Str("source", workItem.Source).
		Str("agent_type", workItem.AgentType).
		Msg("created new work item")

	return workItem, nil
}

// AssignWorkToAgent assigns a work item to an agent session
func (c *Controller) AssignWorkToAgent(ctx context.Context, workItemID, sessionID string) error {
	// Get the work item
	workItem, err := c.Options.Store.GetAgentWorkItem(ctx, workItemID)
	if err != nil {
		return err
	}

	// Update work item status
	workItem.Status = "in_progress"
	workItem.AssignedSessionID = sessionID
	workItem.UpdatedAt = time.Now()
	workItem.StartedAt = &time.Time{}
	*workItem.StartedAt = time.Now()

	if err := c.Options.Store.UpdateAgentWorkItem(ctx, workItem); err != nil {
		return err
	}

	// Update agent session
	if err := c.UpdateAgentSessionStatus(ctx, sessionID, "active", workItem.Name); err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("failed to update agent session status")
	}

	log.Info().
		Str("work_item_id", workItemID).
		Str("session_id", sessionID).
		Msg("assigned work item to agent")

	return nil
}

// CompleteWorkItem marks a work item as completed
func (c *Controller) CompleteWorkItem(ctx context.Context, workItemID string, success bool, result string) error {
	workItem, err := c.Options.Store.GetAgentWorkItem(ctx, workItemID)
	if err != nil {
		return err
	}

	if success {
		workItem.Status = "completed"
	} else {
		workItem.Status = "failed"
		workItem.LastError = result
	}

	workItem.UpdatedAt = time.Now()
	workItem.CompletedAt = &time.Time{}
	*workItem.CompletedAt = time.Now()

	if err := c.Options.Store.UpdateAgentWorkItem(ctx, workItem); err != nil {
		return err
	}

	// Update agent session status
	if workItem.AssignedSessionID != "" {
		sessionStatus := "completed"
		if !success {
			sessionStatus = "failed"
		}
		if err := c.UpdateAgentSessionStatus(ctx, workItem.AssignedSessionID, sessionStatus, "Work completed"); err != nil {
			log.Warn().Err(err).Str("session_id", workItem.AssignedSessionID).Msg("failed to update agent session status")
		}
	}

	log.Info().
		Str("work_item_id", workItemID).
		Bool("success", success).
		Msg("completed work item")

	return nil
}

// ResolveHelpRequest resolves a help request with human input
func (c *Controller) ResolveHelpRequest(ctx context.Context, requestID, resolverUserID, resolution string) error {
	request, err := c.Options.Store.GetHelpRequestByID(ctx, requestID)
	if err != nil {
		return err
	}

	request.Status = "resolved"
	request.ResolvedBy = resolverUserID
	request.Resolution = resolution
	request.UpdatedAt = time.Now()
	request.ResolvedAt = &time.Time{}
	*request.ResolvedAt = time.Now()

	if err := c.Options.Store.UpdateHelpRequest(ctx, request); err != nil {
		return err
	}

	// Update agent session to active
	if err := c.UpdateAgentSessionStatus(ctx, request.SessionID, "active", "Resumed after human help"); err != nil {
		log.Warn().Err(err).Str("session_id", request.SessionID).Msg("failed to update agent session status")
	}

	log.Info().
		Str("request_id", requestID).
		Str("resolved_by", resolverUserID).
		Str("session_id", request.SessionID).
		Msg("resolved help request")

	return nil
}

// CreateWorkItemRequest represents a request to create a new work item
type CreateWorkItemRequest struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Source      string                 `json:"source"`
	SourceID    string                 `json:"source_id"`
	Priority    int                    `json:"priority"`
	AgentType   string                 `json:"agent_type"`
	UserID      string                 `json:"user_id"`
	AppID       string                 `json:"app_id"`
	WorkData    map[string]interface{} `json:"work_data"`
	Config      map[string]interface{} `json:"config"`
	Metadata    map[string]interface{} `json:"metadata"`
}
