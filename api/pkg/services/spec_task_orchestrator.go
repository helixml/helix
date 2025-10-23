package services

import (
	"context"
	"fmt"
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
	worktreeManager       *DesignDocsWorktreeManager
	wolfExecutor          WolfExecutorInterface // NEW: Wolf executor for external agents
	runningTasks          map[string]*OrchestratedTask
	mutex                 sync.RWMutex
	stopChan              chan struct{}
	wg                    sync.WaitGroup
	orchestrationInterval time.Duration
	liveProgressHandlers  []LiveProgressHandler
	testMode              bool
}

// WolfExecutorInterface defines the interface for Wolf executor
type WolfExecutorInterface interface {
	StartZedAgent(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error)
	StopZedAgent(ctx context.Context, sessionID string) error
}

// OrchestratedTask represents a task being orchestrated
type OrchestratedTask struct {
	SpecTask          *types.SpecTask           `json:"spec_task"`
	Agent             *ExternalAgentInstance    `json:"agent"`
	CurrentSessionID  string                    `json:"current_session_id"`
	DesignDocsPath    string                    `json:"design_docs_path"`
	RepoPath          string                    `json:"repo_path"`
	CurrentTaskIndex  int                       `json:"current_task_index"`
	TaskList          []TaskItem                `json:"task_list"`
	LastUpdate        time.Time                 `json:"last_update"`
	Phase             string                    `json:"phase"`
}

// LiveProgressHandler handles live progress updates for dashboard
type LiveProgressHandler func(progress *LiveAgentProgress)

// LiveAgentProgress represents current agent progress for dashboard
type LiveAgentProgress struct {
	AgentID      string      `json:"agent_id"`
	TaskID       string      `json:"task_id"`
	TaskName     string      `json:"task_name"`
	CurrentTask  *TaskItem   `json:"current_task"`
	TasksBefore  []TaskItem  `json:"tasks_before"`
	TasksAfter   []TaskItem  `json:"tasks_after"`
	LastUpdate   time.Time   `json:"last_update"`
	Phase        string      `json:"phase"`
}

// NewSpecTaskOrchestrator creates a new orchestrator
func NewSpecTaskOrchestrator(
	store store.Store,
	controller *controller.Controller,
	gitService *GitRepositoryService,
	specTaskService *SpecDrivenTaskService,
	agentPool *ExternalAgentPool,
	worktreeManager *DesignDocsWorktreeManager,
	wolfExecutor WolfExecutorInterface, // NEW: Wolf executor for external agents
) *SpecTaskOrchestrator {
	return &SpecTaskOrchestrator{
		store:                 store,
		controller:            controller,
		gitService:            gitService,
		specTaskService:       specTaskService,
		agentPool:             agentPool,
		worktreeManager:       worktreeManager,
		wolfExecutor:          wolfExecutor, // NEW
		runningTasks:          make(map[string]*OrchestratedTask),
		stopChan:              make(chan struct{}),
		orchestrationInterval: 10 * time.Second, // Check every 10 seconds
		liveProgressHandlers:  []LiveProgressHandler{},
		testMode:              false,
	}
}

// SetTestMode enables/disables test mode
func (o *SpecTaskOrchestrator) SetTestMode(enabled bool) {
	o.testMode = enabled
}

// RegisterLiveProgressHandler registers a handler for live progress updates
func (o *SpecTaskOrchestrator) RegisterLiveProgressHandler(handler LiveProgressHandler) {
	o.liveProgressHandlers = append(o.liveProgressHandlers, handler)
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
			log.Error().
				Err(err).
				Str("task_id", task.ID).
				Str("status", task.Status).
				Msg("Failed to process task")
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

// handleBacklog handles tasks in backlog state
func (o *SpecTaskOrchestrator) handleBacklog(ctx context.Context, task *types.SpecTask) error {
	// Transition to spec generation
	log.Info().
		Str("task_id", task.ID).
		Msg("Moving task from backlog to spec generation")

	// This would trigger the existing spec generation flow
	// For now, just update status
	task.Status = types.TaskStatusSpecGeneration
	task.UpdatedAt = time.Now()

	return o.store.UpdateSpecTask(ctx, task)
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

// handleImplementationQueued handles tasks ready for implementation
func (o *SpecTaskOrchestrator) handleImplementationQueued(ctx context.Context, task *types.SpecTask) error {
	log.Info().
		Str("task_id", task.ID).
		Msg("Starting implementation phase")

	// Setup git repository and design docs
	repoPath, designDocsPath, err := o.setupTaskEnvironment(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to setup task environment: %w", err)
	}

	// Get or create agent for task
	agent, err := o.agentPool.GetOrCreateForTask(ctx, task, repoPath, designDocsPath)
	if err != nil {
		return fmt.Errorf("failed to get agent: %w", err)
	}

	// Parse task list from design docs
	tasks, err := o.worktreeManager.ParseTaskList(designDocsPath)
	if err != nil {
		return fmt.Errorf("failed to parse task list: %w", err)
	}

	// Create orchestrated task
	o.mutex.Lock()
	o.runningTasks[task.ID] = &OrchestratedTask{
		SpecTask:         task,
		Agent:            agent,
		CurrentSessionID: "",
		DesignDocsPath:   designDocsPath,
		RepoPath:         repoPath,
		CurrentTaskIndex: -1,
		TaskList:         tasks,
		LastUpdate:       time.Now(),
		Phase:            types.TaskStatusImplementation,
	}
	o.mutex.Unlock()

	// Start first task
	err = o.startNextTask(ctx, task.ID)
	if err != nil {
		return fmt.Errorf("failed to start first task: %w", err)
	}

	// Update task status
	task.Status = types.TaskStatusImplementation
	task.UpdatedAt = time.Now()

	return o.store.UpdateSpecTask(ctx, task)
}

// handleImplementation handles tasks in implementation
func (o *SpecTaskOrchestrator) handleImplementation(ctx context.Context, task *types.SpecTask) error {
	o.mutex.RLock()
	orchestratedTask, exists := o.runningTasks[task.ID]
	o.mutex.RUnlock()

	if !exists {
		// Task not yet tracked, queue it
		task.Status = types.TaskStatusImplementationQueued
		return o.store.UpdateSpecTask(ctx, task)
	}

	// Check current task progress via git commits
	currentTask, err := o.worktreeManager.GetCurrentTask(orchestratedTask.DesignDocsPath)
	if err != nil {
		return fmt.Errorf("failed to get current task: %w", err)
	}

	// If no current task, start next one
	if currentTask == nil {
		err = o.startNextTask(ctx, task.ID)
		if err != nil {
			return fmt.Errorf("failed to start next task: %w", err)
		}
	}

	// Broadcast live progress
	o.broadcastLiveProgress(orchestratedTask)

	// Check if all tasks complete
	allComplete := true
	for _, t := range orchestratedTask.TaskList {
		if t.Status != TaskStatusCompleted {
			allComplete = false
			break
		}
	}

	if allComplete {
		log.Info().
			Str("task_id", task.ID).
			Msg("All implementation tasks complete")

		task.Status = types.TaskStatusImplementationReview
		task.UpdatedAt = time.Now()
		task.CompletedAt = &task.UpdatedAt

		// Clean up orchestrated task
		o.mutex.Lock()
		delete(o.runningTasks, task.ID)
		o.mutex.Unlock()

		return o.store.UpdateSpecTask(ctx, task)
	}

	return nil
}

// startNextTask starts the next pending task
func (o *SpecTaskOrchestrator) startNextTask(ctx context.Context, taskID string) error {
	o.mutex.RLock()
	orchestratedTask, exists := o.runningTasks[taskID]
	o.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("orchestrated task %s not found", taskID)
	}

	// Find next pending task
	nextTask, err := o.worktreeManager.GetNextPendingTask(orchestratedTask.DesignDocsPath)
	if err != nil {
		return fmt.Errorf("failed to get next task: %w", err)
	}

	if nextTask == nil {
		log.Info().
			Str("task_id", taskID).
			Msg("No more pending tasks")
		return nil
	}

	// Mark task as in progress
	err = o.worktreeManager.MarkTaskInProgress(ctx, orchestratedTask.DesignDocsPath, nextTask.Index)
	if err != nil {
		return fmt.Errorf("failed to mark task in progress: %w", err)
	}

	// Update orchestrated task
	o.mutex.Lock()
	orchestratedTask.CurrentTaskIndex = nextTask.Index
	orchestratedTask.LastUpdate = time.Now()
	o.mutex.Unlock()

	// Broadcast update
	o.broadcastLiveProgress(orchestratedTask)

	log.Info().
		Str("task_id", taskID).
		Int("task_index", nextTask.Index).
		Str("description", nextTask.Description).
		Msg("Started next implementation task")

	return nil
}

// setupTaskEnvironment sets up git repo and design docs for a task
func (o *SpecTaskOrchestrator) setupTaskEnvironment(ctx context.Context, task *types.SpecTask) (repoPath, designDocsPath string, err error) {
	// Get or create git repository for task
	// This would use the GitRepositoryService to clone demo repo or setup project repo

	// For now, assume repo exists at standard path
	repoPath = fmt.Sprintf("/workspace/repos/%s/%s", task.CreatedBy, task.ProjectID)

	// Setup design docs worktree
	designDocsPath, err = o.worktreeManager.SetupWorktree(ctx, repoPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to setup worktree: %w", err)
	}

	return repoPath, designDocsPath, nil
}

// broadcastLiveProgress broadcasts live progress to dashboard
func (o *SpecTaskOrchestrator) broadcastLiveProgress(task *OrchestratedTask) {
	// Get task context for dashboard
	context, err := o.worktreeManager.GetTaskContext(task.DesignDocsPath, 2)
	if err != nil {
		log.Error().
			Err(err).
			Str("task_id", task.SpecTask.ID).
			Msg("Failed to get task context for broadcast")
		return
	}

	progress := &LiveAgentProgress{
		AgentID:     task.Agent.InstanceID,
		TaskID:      task.SpecTask.ID,
		TaskName:    task.SpecTask.Name,
		CurrentTask: context.CurrentTask,
		TasksBefore: context.TasksBefore,
		TasksAfter:  context.TasksAfter,
		LastUpdate:  time.Now(),
		Phase:       task.Phase,
	}

	// Call all registered handlers
	for _, handler := range o.liveProgressHandlers {
		handler(progress)
	}
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

// GetLiveProgress returns current live progress for all running tasks
func (o *SpecTaskOrchestrator) GetLiveProgress() []*LiveAgentProgress {
	o.mutex.RLock()
	defer o.mutex.RUnlock()

	progress := []*LiveAgentProgress{}

	for _, task := range o.runningTasks {
		context, err := o.worktreeManager.GetTaskContext(task.DesignDocsPath, 2)
		if err != nil {
			log.Error().
				Err(err).
				Str("task_id", task.SpecTask.ID).
				Msg("Failed to get task context")
			continue
		}

		progress = append(progress, &LiveAgentProgress{
			AgentID:     task.Agent.InstanceID,
			TaskID:      task.SpecTask.ID,
			TaskName:    task.SpecTask.Name,
			CurrentTask: context.CurrentTask,
			TasksBefore: context.TasksBefore,
			TasksAfter:  context.TasksAfter,
			LastUpdate:  task.LastUpdate,
			Phase:       task.Phase,
		})
	}

	return progress
}
