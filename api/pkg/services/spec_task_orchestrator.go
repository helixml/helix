package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// SpecTaskOrchestrator orchestrates SpecTasks through the complete workflow
// Pushes agents through design → approval → implementation
// Manages agent lifecycle and reuses sessions across Helix interactions
type SpecTaskOrchestrator struct {
	store                 store.Store
	controller            *controller.Controller
	gitService            *GitRepositoryService
	specTaskService       *SpecDrivenTaskService
	agentPool             *ExternalAgentPool
	wolfExecutor          WolfExecutorInterface // Wolf executor for external agents
	mutex                 sync.RWMutex
	stopChan              chan struct{}
	wg                    sync.WaitGroup
	orchestrationInterval time.Duration
	testMode              bool
}

// WolfExecutorInterface defines the interface for Wolf executor
type WolfExecutorInterface interface {
	StartZedAgent(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error)
	StopZedAgent(ctx context.Context, sessionID string) error
}


// NewSpecTaskOrchestrator creates a new orchestrator
func NewSpecTaskOrchestrator(
	store store.Store,
	controller *controller.Controller,
	gitService *GitRepositoryService,
	specTaskService *SpecDrivenTaskService,
	agentPool *ExternalAgentPool,
	wolfExecutor WolfExecutorInterface, // Wolf executor for external agents
) *SpecTaskOrchestrator {
	return &SpecTaskOrchestrator{
		store:                 store,
		controller:            controller,
		gitService:            gitService,
		specTaskService:       specTaskService,
		agentPool:             agentPool,
		wolfExecutor:          wolfExecutor,
		stopChan:              make(chan struct{}),
		orchestrationInterval: 10 * time.Second, // Check every 10 seconds
		testMode:              false,
	}
}

// SetTestMode enables/disables test mode
func (o *SpecTaskOrchestrator) SetTestMode(enabled bool) {
	o.testMode = enabled
}

// Start begins the orchestration loop
func (o *SpecTaskOrchestrator) Start(ctx context.Context) error {
	log.Info().Msg("Starting SpecTask orchestrator")

	// Start main orchestration loop
	o.wg.Add(1)
	go o.orchestrationLoop(ctx)

	// Start cleanup routine
	o.wg.Add(1)
	go o.cleanupLoop(ctx)

	return nil
}

// Stop stops the orchestrator
func (o *SpecTaskOrchestrator) Stop() error {
	log.Info().Msg("Stopping SpecTask orchestrator")
	close(o.stopChan)
	o.wg.Wait()
	return nil
}

// orchestrationLoop is the main loop that processes tasks
func (o *SpecTaskOrchestrator) orchestrationLoop(ctx context.Context) {
	defer o.wg.Done()

	ticker := time.NewTicker(o.orchestrationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-o.stopChan:
			return
		case <-ticker.C:
			o.processTasks(ctx)
		}
	}
}

// processTasks processes all active tasks
func (o *SpecTaskOrchestrator) processTasks(ctx context.Context) {
	// Get all tasks (we'll filter active ones)
	tasks, err := o.store.ListSpecTasks(ctx, &types.SpecTaskFilters{})
	if err != nil {
		log.Error().Err(err).Msg("Failed to list tasks for orchestration")
		return
	}

	// Filter to only active tasks
	activeStatuses := map[string]bool{
		types.TaskStatusBacklog:              true,
		types.TaskStatusSpecGeneration:       true,
		types.TaskStatusSpecReview:           true,
		types.TaskStatusSpecRevision:         true,
		types.TaskStatusImplementationQueued: true,
		types.TaskStatusImplementation:       true,
	}

	for _, task := range tasks {
		if !activeStatuses[task.Status] {
			continue
		}

		err := o.processTask(ctx, task)
		if err != nil {
			// Tasks with deleted projects are expected - don't spam logs
			if strings.Contains(err.Error(), "record not found") || strings.Contains(err.Error(), "not found") {
				log.Trace().
					Err(err).
					Str("task_id", task.ID).
					Str("status", task.Status).
					Msg("Task references deleted project - skipping")
			} else {
				log.Error().
					Err(err).
					Str("task_id", task.ID).
					Str("status", task.Status).
					Msg("Failed to process task")
			}
		}
	}
}

// processTask processes a single task through its workflow
func (o *SpecTaskOrchestrator) processTask(ctx context.Context, task *types.SpecTask) error {
	// State machine for task workflow
	switch task.Status {
	case types.TaskStatusBacklog:
		return o.handleBacklog(ctx, task)
	case types.TaskStatusSpecGeneration:
		return o.handleSpecGeneration(ctx, task)
	case types.TaskStatusSpecReview:
		return o.handleSpecReview(ctx, task)
	case types.TaskStatusSpecRevision:
		return o.handleSpecRevision(ctx, task)
	case types.TaskStatusImplementationQueued:
		return o.handleImplementationQueued(ctx, task)
	case types.TaskStatusImplementation:
		return o.handleImplementation(ctx, task)
	default:
		return nil
	}
}

// handleBacklog handles tasks in backlog state - creates external agent and starts planning
func (o *SpecTaskOrchestrator) handleBacklog(ctx context.Context, task *types.SpecTask) error {
	// Check if project has auto-start enabled
	project, err := o.store.GetProject(ctx, task.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	if !project.AutoStartBacklogTasks {
		// Auto-start is disabled - don't process backlog tasks automatically
		log.Trace().
			Str("task_id", task.ID).
			Str("project_id", task.ProjectID).
			Msg("Skipping backlog task - auto-start is disabled for this project")
		return nil
	}

	// Check WIP limits for planning column before auto-starting
	// Get default project to load board settings
	defaultProject, err := o.store.GetProject(ctx, "default")
	if err == nil && len(defaultProject.Metadata) > 0 {
		var metadata types.ProjectMetadata
		if err := json.Unmarshal(defaultProject.Metadata, &metadata); err == nil {
			if metadata.BoardSettings != nil && metadata.BoardSettings.WIPLimits != nil {
				planningLimit, ok := metadata.BoardSettings.WIPLimits["planning"]
				if ok && planningLimit > 0 {
					// Count tasks currently in planning for THIS project
					planningTasks, err := o.store.ListSpecTasks(ctx, &types.SpecTaskFilters{
						ProjectID: task.ProjectID,
						Status:    types.TaskStatusSpecGeneration,
					})
					if err != nil {
						log.Warn().Err(err).Msg("Failed to check planning column WIP limit")
					} else if len(planningTasks) >= planningLimit {
						// Planning column is at WIP limit - don't auto-start
						log.Info().
							Str("task_id", task.ID).
							Str("project_id", task.ProjectID).
							Int("planning_count", len(planningTasks)).
							Int("wip_limit", planningLimit).
							Msg("Skipping backlog task - planning column at WIP limit")
						return nil
					}
				}
			}
		}
	}

	log.Info().
		Str("task_id", task.ID).
		Str("helix_app_id", task.HelixAppID).
		Msg("Auto-starting SpecTask planning phase")

	// Delegate to the canonical StartSpecGeneration implementation
	// This ensures both explicit start and auto-start use the same code path
	o.specTaskService.StartSpecGeneration(ctx, task)

	return nil
}

// handleSpecGeneration handles tasks in spec generation
func (o *SpecTaskOrchestrator) handleSpecGeneration(ctx context.Context, task *types.SpecTask) error {
	// Check if spec generation session is complete
	// This would integrate with existing SpecDrivenTaskService
	// For now, we'll assume spec is ready when all spec documents exist

	// Check if task has spec documents (requirements, design, implementation plan)
	if task.RequirementsSpec != "" && task.TechnicalDesign != "" && task.ImplementationPlan != "" {
		// Spec is ready, move to review
		log.Info().
			Str("task_id", task.ID).
			Msg("Spec generation complete, moving to review")

		task.Status = types.TaskStatusSpecReview
		task.UpdatedAt = time.Now()

		return o.store.UpdateSpecTask(ctx, task)
	}

	return nil
}

// handleSpecReview handles tasks in spec review
func (o *SpecTaskOrchestrator) handleSpecReview(ctx context.Context, task *types.SpecTask) error {
	// Waiting for human approval
	// This is handled via API endpoint, no automatic transition
	return nil
}

// handleSpecRevision handles tasks needing spec revision
func (o *SpecTaskOrchestrator) handleSpecRevision(ctx context.Context, task *types.SpecTask) error {
	// Similar to spec generation, regenerate specs based on feedback
	// For now, move back to spec generation
	task.Status = types.TaskStatusSpecGeneration
	task.UpdatedAt = time.Now()

	return o.store.UpdateSpecTask(ctx, task)
}

// handleImplementationQueued handles tasks ready for implementation - reuses external agent
// This is a legacy state - new flow (design review approval) bypasses this entirely
func (o *SpecTaskOrchestrator) handleImplementationQueued(ctx context.Context, task *types.SpecTask) error {
	log.Info().
		Str("task_id", task.ID).
		Msg("Task in implementation_queued - moving directly to implementation")

	// Just move to implementation status - agent is already running from planning
	task.Status = types.TaskStatusImplementation
	task.UpdatedAt = time.Now()

	return o.store.UpdateSpecTask(ctx, task)
}

// NOTE: Implementation prompts are now handled by agent_instruction_service.go:SendApprovalInstruction
// That function sends the actual implementation instructions when specs are approved.

// handleImplementation handles tasks in implementation
func (o *SpecTaskOrchestrator) handleImplementation(ctx context.Context, task *types.SpecTask) error {
	// Since we reuse the planning agent, external agents are already running
	// No need to queue or create new agents - just verify agent is still active

	// Check if external agent exists and is running
	if task.ExternalAgentID != "" {
		externalAgent, err := o.store.GetSpecTaskExternalAgent(ctx, task.ID)
		if err == nil && externalAgent.Status == "running" {
			// Agent is running, update activity timestamp
			externalAgent.LastActivity = time.Now()
			o.store.UpdateSpecTaskExternalAgent(ctx, externalAgent)
			return nil
		}
	}

	// Task not tracked - this is OK for new reuse-agent pattern
	// Implementation progress is tracked via shell scripts in the sandbox
	log.Debug().
		Str("task_id", task.ID).
		Msg("Task in implementation (using reused agent pattern)")
	return nil
}

// cleanupLoop periodically cleans up stale agents
func (o *SpecTaskOrchestrator) cleanupLoop(ctx context.Context) {
	defer o.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-o.stopChan:
			return
		case <-ticker.C:
			err := o.agentPool.CleanupStaleAgents(ctx, 30*time.Minute)
			if err != nil {
				log.Error().Err(err).Msg("Failed to cleanup stale agents")
			}
		}
	}
}

