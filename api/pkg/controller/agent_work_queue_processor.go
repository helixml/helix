package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/trigger/agent_work_queue"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
)

// AgentWorkQueueProcessor handles pending agent work items by creating sessions for them
type AgentWorkQueueProcessor struct {
	store        store.Store
	controller   *Controller
	trigger      *agent_work_queue.AgentWorkQueueTrigger
	pollInterval time.Duration
	maxRetries   int
}

// NewAgentWorkQueueProcessor creates a new agent work queue processor
func NewAgentWorkQueueProcessor(store store.Store, controller *Controller) *AgentWorkQueueProcessor {
	return &AgentWorkQueueProcessor{
		store:        store,
		controller:   controller,
		trigger:      agent_work_queue.NewAgentWorkQueueTrigger(store, controller),
		pollInterval: 5 * time.Second, // Poll every 5 seconds
		maxRetries:   3,               // Max 3 retry attempts
	}
}

// Start begins processing the agent work queue
func (p *AgentWorkQueueProcessor) Start(ctx context.Context) {
	log.Info().Msg("Starting agent work queue processor")

	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	// Process any existing items immediately
	p.processWorkQueue(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping agent work queue processor")
			return
		case <-ticker.C:
			p.processWorkQueue(ctx)
		}
	}
}

// processWorkQueue processes pending work items
func (p *AgentWorkQueueProcessor) processWorkQueue(ctx context.Context) {
	// Get pending work items ordered by priority and creation time
	query := &store.ListAgentWorkItemsQuery{
		Page:     0,
		PageSize: 10, // Process up to 10 items at a time
		Status:   "pending",
		OrderBy:  "priority ASC, created_at ASC",
	}

	response, err := p.store.ListAgentWorkItems(ctx, query)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list pending work items")
		return
	}

	if len(response.WorkItems) == 0 {
		log.Trace().Msg("No pending work items to process")
		return
	}

	log.Info().Int("count", len(response.WorkItems)).Msg("Processing pending work items")

	for _, workItem := range response.WorkItems {
		if err := p.processWorkItem(ctx, workItem); err != nil {
			log.Error().Err(err).
				Str("work_item_id", workItem.ID).
				Str("work_item_name", workItem.Name).
				Msg("Failed to process work item")

			// Update retry count
			workItem.RetryCount++
			if workItem.RetryCount >= p.maxRetries {
				log.Warn().
					Str("work_item_id", workItem.ID).
					Int("retry_count", workItem.RetryCount).
					Msg("Work item exceeded max retries, marking as failed")

				workItem.Status = "failed"
				workItem.LastError = fmt.Sprintf("Exceeded max retries (%d): %v", p.maxRetries, err)
			} else {
				log.Info().
					Str("work_item_id", workItem.ID).
					Int("retry_count", workItem.RetryCount).
					Int("max_retries", p.maxRetries).
					Msg("Will retry work item")
			}

			if updateErr := p.store.UpdateAgentWorkItem(ctx, workItem); updateErr != nil {
				log.Error().Err(updateErr).
					Str("work_item_id", workItem.ID).
					Msg("Failed to update work item retry count")
			}
		}
	}
}

// processWorkItem processes a single work item by creating an agent session
func (p *AgentWorkQueueProcessor) processWorkItem(ctx context.Context, workItem *types.AgentWorkItem) error {
	log.Info().
		Str("work_item_id", workItem.ID).
		Str("work_item_name", workItem.Name).
		Str("agent_type", workItem.AgentType).
		Int("priority", workItem.Priority).
		Msg("Processing work item")

	// Skip work items without user_id (required)
	if workItem.UserID == "" {
		log.Warn().
			Str("work_item_id", workItem.ID).
			Str("user_id", workItem.UserID).
			Msg("Skipping work item with missing required field: user_id")

		workItem.Status = "failed"
		workItem.LastError = "Missing required field: user_id"
		workItem.CompletedAt = &[]time.Time{time.Now()}[0]
		if err := p.store.UpdateAgentWorkItem(ctx, workItem); err != nil {
			log.Error().Err(err).Str("work_item_id", workItem.ID).Msg("Failed to update work item status")
		}
		return fmt.Errorf("work item missing required user_id")
	}

	// Use default app if none provided
	if workItem.AppID == "" {
		// Get user's first app or create a default one
		defaultApp, err := p.getOrCreateDefaultApp(ctx, workItem.UserID)
		if err != nil {
			log.Error().Err(err).Str("work_item_id", workItem.ID).Msg("Failed to get default app")
			workItem.Status = "failed"
			workItem.LastError = fmt.Sprintf("Failed to get default app: %v", err)
			workItem.CompletedAt = &[]time.Time{time.Now()}[0]
			if updateErr := p.store.UpdateAgentWorkItem(ctx, workItem); updateErr != nil {
				log.Error().Err(updateErr).Str("work_item_id", workItem.ID).Msg("Failed to update work item status")
			}
			return fmt.Errorf("failed to get default app for work item")
		}
		workItem.AppID = defaultApp.ID
		log.Info().
			Str("work_item_id", workItem.ID).
			Str("app_id", workItem.AppID).
			Msg("Using default app for work item")
	}

	// Mark work item as in progress
	workItem.Status = "in_progress"
	workItem.StartedAt = &[]time.Time{time.Now()}[0] // Get pointer to current time
	if err := p.store.UpdateAgentWorkItem(ctx, workItem); err != nil {
		return fmt.Errorf("failed to update work item status to in_progress: %w", err)
	}

	// Create a trigger configuration for processing this work item
	triggerConfig := &types.TriggerConfiguration{
		ID:      fmt.Sprintf("work-item-%s", workItem.ID),
		Name:    fmt.Sprintf("Work Item: %s", workItem.Name),
		Enabled: true,
		Trigger: types.Trigger{
			AgentWorkQueue: &types.AgentWorkQueueTrigger{
				AgentType: workItem.AgentType,
				Enabled:   true,
			},
		},
	}

	// Create payload from work item
	payload := map[string]interface{}{
		"work_item_id":  workItem.ID,
		"name":          workItem.Name,
		"description":   workItem.Description,
		"agent_type":    workItem.AgentType,
		"app_id":        workItem.AppID,
		"user_id":       workItem.UserID,
		"priority":      workItem.Priority,
		"source":        workItem.Source,
		"source_id":     workItem.SourceID,
		"work_data":     workItem.WorkData,
		"configuration": workItem.Config,
		"metadata":      workItem.Metadata,
	}

	// Process the work item using the agent work queue trigger
	response, err := p.trigger.ProcessTrigger(ctx, triggerConfig, payload)
	if err != nil {
		// Mark work item as failed
		workItem.Status = "failed"
		workItem.LastError = err.Error()
		workItem.CompletedAt = &[]time.Time{time.Now()}[0]
		if updateErr := p.store.UpdateAgentWorkItem(ctx, workItem); updateErr != nil {
			log.Error().Err(updateErr).
				Str("work_item_id", workItem.ID).
				Msg("Failed to update work item status to failed")
		}
		return fmt.Errorf("failed to process work item with trigger: %w", err)
	}

	// Update work item with session information
	if response != nil && response.SessionID != "" {
		workItem.AssignedSessionID = response.SessionID
		log.Info().
			Str("work_item_id", workItem.ID).
			Str("session_id", response.SessionID).
			Msg("Work item assigned to session")
	}

	// The work item status will be updated by the session completion callback
	// For now, we leave it as "in_progress" until the session completes
	if err := p.store.UpdateAgentWorkItem(ctx, workItem); err != nil {
		log.Error().Err(err).
			Str("work_item_id", workItem.ID).
			Msg("Failed to update work item with session ID")
	}

	log.Info().
		Str("work_item_id", workItem.ID).
		Str("session_id", workItem.AssignedSessionID).
		Msg("Successfully created agent session for work item")

	return nil
}

// getOrCreateDefaultApp gets the user's first app or creates a default one
func (p *AgentWorkQueueProcessor) getOrCreateDefaultApp(ctx context.Context, userID string) (*types.App, error) {
	// Try to get an existing app for the user
	apps, err := p.store.ListApps(ctx, &store.ListAppsQuery{
		Owner:     userID,
		OwnerType: types.OwnerTypeUser,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query user apps: %w", err)
	}

	if len(apps) > 0 {
		return apps[0], nil
	}

	// Create a default app for the user
	defaultApp := &types.App{
		ID:        system.GenerateAppID(),
		Owner:     userID,
		OwnerType: types.OwnerTypeUser,
		Global:    false,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{
					{
						Name:         "default-assistant",
						Description:  "Default assistant for agent work",
						Model:        "claude-3-5-sonnet-20241022",
						SystemPrompt: "You are a helpful AI assistant that can work autonomously on tasks.",
					},
				},
			},
		},
		Created: time.Now(),
		Updated: time.Now(),
	}

	createdApp, err := p.store.CreateApp(ctx, defaultApp)
	if err != nil {
		return nil, fmt.Errorf("failed to create default app: %w", err)
	}

	log.Info().
		Str("user_id", userID).
		Str("app_id", createdApp.ID).
		Msg("Created default app for user")

	return createdApp, nil
}
