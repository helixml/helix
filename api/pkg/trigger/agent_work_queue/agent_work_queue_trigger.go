package agent_work_queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// AgentWorkQueueTrigger handles work queue triggers for agent scheduling
type AgentWorkQueueTrigger struct {
	store      store.Store
	controller HelixController
}

// HelixController interface for creating sessions and managing external agents
type HelixController interface {
	CreateSession(ctx context.Context, req *CreateSessionRequest) (*types.Session, error)
	LaunchExternalAgent(ctx context.Context, sessionID string, agentType string) error
}

// CreateSessionRequest represents a request to create a new Helix session
type CreateSessionRequest struct {
	UserID         string
	AppID          string
	OrganizationID string
	SessionMode    types.SessionMode
	SystemPrompt   string
	WorkItemID     string
}

// NewAgentWorkQueueTrigger creates a new agent work queue trigger handler
func NewAgentWorkQueueTrigger(store store.Store, controller HelixController) *AgentWorkQueueTrigger {
	return &AgentWorkQueueTrigger{
		store:      store,
		controller: controller,
	}
}

// ProcessTrigger processes a work queue trigger configuration
func (t *AgentWorkQueueTrigger) ProcessTrigger(ctx context.Context, triggerConfig *types.TriggerConfiguration, payload map[string]interface{}) (*types.TriggerExecuteResponse, error) {
	if triggerConfig.Trigger.AgentWorkQueue == nil {
		return nil, fmt.Errorf("agent work queue trigger configuration is nil")
	}

	trigger := triggerConfig.Trigger.AgentWorkQueue

	log.Info().
		Str("trigger_config_id", triggerConfig.ID).
		Str("agent_type", trigger.AgentType).
		Bool("enabled", trigger.Enabled).
		Msg("Processing agent work queue trigger")

	if !trigger.Enabled {
		return nil, fmt.Errorf("agent work queue trigger is disabled")
	}

	// Extract work item details from payload
	workItem, err := t.createWorkItemFromPayload(triggerConfig, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create work item from payload: %w", err)
	}

	// Create the work item in the database
	err = t.store.CreateAgentWorkItem(ctx, workItem)
	if err != nil {
		return nil, fmt.Errorf("failed to create work item: %w", err)
	}

	log.Info().
		Str("work_item_id", workItem.ID).
		Str("trigger_config_id", triggerConfig.ID).
		Str("name", workItem.Name).
		Int("priority", workItem.Priority).
		Msg("Created agent work item from trigger")

	// Create a new Helix session for this work item
	sessionResp, err := t.createSessionForWorkItem(ctx, workItem)
	if err != nil {
		log.Error().Err(err).Str("work_item_id", workItem.ID).Msg("Failed to create session for work item")
		// Don't fail the trigger - work item is still created
	}

	sessionID := ""
	if sessionResp != nil {
		sessionID = sessionResp.SessionID
	}

	// Return the trigger response
	return &types.TriggerExecuteResponse{
		SessionID: sessionID,
		Content:   fmt.Sprintf("Work item '%s' created and assigned to new %s agent session", workItem.Name, workItem.AgentType),
	}, nil
}

// createWorkItemFromPayload creates a work item from the trigger payload
func (t *AgentWorkQueueTrigger) createWorkItemFromPayload(triggerConfig *types.TriggerConfiguration, payload map[string]interface{}) (*types.AgentWorkItem, error) {
	trigger := triggerConfig.Trigger.AgentWorkQueue

	// Extract basic fields from payload
	name, _ := payload["name"].(string)
	description, _ := payload["description"].(string)
	source, _ := payload["source"].(string)
	sourceID, _ := payload["source_id"].(string)
	sourceURL, _ := payload["source_url"].(string)

	// Default values
	if name == "" {
		name = fmt.Sprintf("Work Item - %s", time.Now().Format("2006-01-02 15:04:05"))
	}
	if source == "" {
		source = "trigger"
	}
	if description == "" {
		description = "Work item created from trigger"
	}

	// Extract priority from payload or use trigger default
	priority := trigger.Priority
	if payloadPriority, ok := payload["priority"].(float64); ok {
		priority = int(payloadPriority)
	}

	// Create work data JSON
	workDataJSON, _ := json.Marshal(payload)

	// Create the work item
	workItem := &types.AgentWorkItem{
		ID:              fmt.Sprintf("work-%d", time.Now().UnixNano()),
		TriggerConfigID: triggerConfig.ID,
		Name:            name,
		Description:     description,
		Source:          source,
		SourceID:        sourceID,
		SourceURL:       sourceURL,
		Priority:        priority,
		Status:          "pending",
		AgentType:       trigger.AgentType,
		UserID:          triggerConfig.Owner,
		AppID:           triggerConfig.AppID,
		OrganizationID:  triggerConfig.OrganizationID,
		WorkData:        workDataJSON,
		MaxRetries:      trigger.MaxRetries,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// Set default max retries if not specified
	if workItem.MaxRetries == 0 {
		workItem.MaxRetries = 3
	}

	// Set deadline if timeout is specified
	if trigger.TimeoutMins > 0 {
		deadline := time.Now().Add(time.Duration(trigger.TimeoutMins) * time.Minute)
		workItem.DeadlineAt = &deadline
	}

	// Set scheduled time if specified in payload
	if scheduledFor, ok := payload["scheduled_for"].(string); ok {
		if parsedTime, err := time.Parse(time.RFC3339, scheduledFor); err == nil {
			workItem.ScheduledFor = &parsedTime
		}
	}

	return workItem, nil
}

// createSessionForWorkItem creates a new Helix session for the work item and launches an external agent
func (t *AgentWorkQueueTrigger) createSessionForWorkItem(ctx context.Context, workItem *types.AgentWorkItem) (*types.TriggerExecuteResponse, error) {
	// Get the app to determine the system prompt and configuration
	app, err := t.store.GetApp(ctx, workItem.AppID)
	if err != nil {
		return nil, fmt.Errorf("failed to get app: %w", err)
	}

	// Get the assistant system prompt
	var assistantSystemPrompt string
	if len(app.Config.Helix.Assistants) == 0 {
		return nil, fmt.Errorf("app %s has no agents configured", workItem.AppID)
	}
	assistantSystemPrompt = app.Config.Helix.Assistants[0].SystemPrompt

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
		assistantSystemPrompt)

	// Create session request
	sessionReq := &CreateSessionRequest{
		UserID:         workItem.UserID,
		AppID:          workItem.AppID,
		OrganizationID: workItem.OrganizationID,
		SessionMode:    types.SessionModeInference, // Use appropriate mode
		SystemPrompt:   systemPrompt,
		WorkItemID:     workItem.ID,
	}

	// Create the Helix session
	session, err := t.controller.CreateSession(ctx, sessionReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Update work item with session ID
	workItem.AssignedSessionID = session.ID
	workItem.Status = "assigned"
	workItem.StartedAt = &time.Time{}
	*workItem.StartedAt = time.Now()
	err = t.store.UpdateAgentWorkItem(ctx, workItem)
	if err != nil {
		log.Warn().Err(err).Str("work_item_id", workItem.ID).Msg("Failed to update work item with session ID")
	}

	// Create agent session status record
	agentSession := &types.AgentSessionStatus{
		ID:              fmt.Sprintf("agent-session-%s", session.ID),
		SessionID:       session.ID,
		AgentType:       workItem.AgentType,
		Status:          "starting",
		CurrentTask:     fmt.Sprintf("Starting work on: %s", workItem.Name),
		CurrentWorkItem: workItem.ID,
		UserID:          workItem.UserID,
		AppID:           workItem.AppID,
		OrganizationID:  workItem.OrganizationID,
		HealthStatus:    "unknown",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		LastActivity:    time.Now(),
	}

	err = t.store.CreateAgentSessionStatus(ctx, agentSession)
	if err != nil {
		log.Warn().Err(err).Str("session_id", session.ID).Msg("Failed to create agent session status")
	}

	// Launch external agent (Zed, VS Code, etc.)
	err = t.controller.LaunchExternalAgent(ctx, session.ID, workItem.AgentType)
	if err != nil {
		log.Error().Err(err).Str("session_id", session.ID).Str("agent_type", workItem.AgentType).Msg("Failed to launch external agent")
		// Update status to failed
		agentSession.Status = "failed"
		agentSession.HealthStatus = "unhealthy"
		t.store.UpdateAgentSessionStatus(ctx, agentSession)
		return nil, fmt.Errorf("failed to launch external agent: %w", err)
	}

	// Update status to active
	agentSession.Status = "active"
	agentSession.HealthStatus = "healthy"
	agentSession.CurrentTask = fmt.Sprintf("Working on: %s", workItem.Name)
	err = t.store.UpdateAgentSessionStatus(ctx, agentSession)
	if err != nil {
		log.Warn().Err(err).Str("session_id", session.ID).Msg("Failed to update agent session status to active")
	}

	log.Info().
		Str("work_item_id", workItem.ID).
		Str("session_id", session.ID).
		Str("agent_type", workItem.AgentType).
		Msg("Created session and launched external agent for work item")

	return &types.TriggerExecuteResponse{
		SessionID: session.ID,
		Content:   fmt.Sprintf("Created session and launched %s agent for work item", workItem.AgentType),
	}, nil
}

// HandleGitHubIssue creates a work item from a GitHub issue
func (t *AgentWorkQueueTrigger) HandleGitHubIssue(ctx context.Context, triggerConfig *types.TriggerConfiguration, issuePayload map[string]interface{}) error {
	// Extract GitHub issue details
	issue, ok := issuePayload["issue"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid GitHub issue payload")
	}

	title, _ := issue["title"].(string)
	body, _ := issue["body"].(string)
	number, _ := issue["number"].(float64)
	htmlURL, _ := issue["html_url"].(string)

	// Create work item payload
	payload := map[string]interface{}{
		"name":        fmt.Sprintf("GitHub Issue #%.0f: %s", number, title),
		"description": body,
		"source":      "github",
		"source_id":   fmt.Sprintf("%.0f", number),
		"source_url":  htmlURL,
		"priority":    5, // Default priority for GitHub issues
		"github_issue": map[string]interface{}{
			"number":   number,
			"title":    title,
			"body":     body,
			"html_url": htmlURL,
		},
	}

	// Process as regular trigger
	_, err := t.ProcessTrigger(ctx, triggerConfig, payload)
	return err
}

// HandleManualWorkItem creates a work item from manual input
func (t *AgentWorkQueueTrigger) HandleManualWorkItem(ctx context.Context, triggerConfig *types.TriggerConfiguration, request map[string]interface{}) error {
	// Validate required fields
	name, ok := request["name"].(string)
	if !ok || name == "" {
		return fmt.Errorf("name is required for manual work items")
	}

	// Use the request as the payload
	_, err := t.ProcessTrigger(ctx, triggerConfig, request)
	return err
}

// GetWorkItemMetrics returns metrics for work items created by this trigger
func (t *AgentWorkQueueTrigger) GetWorkItemMetrics(ctx context.Context, triggerConfigID string) (map[string]interface{}, error) {
	// Get work items for this trigger config
	query := &store.ListAgentWorkItemsQuery{
		Page:            0,
		PageSize:        1000, // Get all for metrics
		TriggerConfigID: triggerConfigID,
	}

	response, err := t.store.ListAgentWorkItems(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get work items: %w", err)
	}

	// Calculate metrics
	metrics := map[string]interface{}{
		"total_items":             response.Total,
		"pending":                 0,
		"in_progress":             0,
		"completed":               0,
		"failed":                  0,
		"average_completion_time": 0.0,
	}

	var completionTimes []float64
	statusCounts := make(map[string]int)

	for _, item := range response.WorkItems {
		statusCounts[item.Status]++

		// Calculate completion time if completed
		if item.CompletedAt != nil && item.StartedAt != nil {
			duration := item.CompletedAt.Sub(*item.StartedAt)
			completionTimes = append(completionTimes, duration.Minutes())
		}
	}

	// Update metrics with counts
	for status, count := range statusCounts {
		metrics[status] = count
	}

	// Calculate average completion time
	if len(completionTimes) > 0 {
		var total float64
		for _, duration := range completionTimes {
			total += duration
		}
		metrics["average_completion_time"] = total / float64(len(completionTimes))
	}

	return metrics, nil
}
