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
	SpecTask         *types.SpecTask        `json:"spec_task"`
	Agent            *ExternalAgentInstance `json:"agent"`
	CurrentSessionID string                 `json:"current_session_id"`
	DesignDocsPath   string                 `json:"design_docs_path"`
	RepoPath         string                 `json:"repo_path"`
	CurrentTaskIndex int                    `json:"current_task_index"`
	TaskList         []TaskItem             `json:"task_list"`
	LastUpdate       time.Time              `json:"last_update"`
	Phase            string                 `json:"phase"`
}

// LiveProgressHandler handles live progress updates for dashboard
type LiveProgressHandler func(progress *LiveAgentProgress)

// LiveAgentProgress represents current agent progress for dashboard
type LiveAgentProgress struct {
	AgentID     string     `json:"agent_id"`
	TaskID      string     `json:"task_id"`
	TaskName    string     `json:"task_name"`
	CurrentTask *TaskItem  `json:"current_task"`
	TasksBefore []TaskItem `json:"tasks_before"`
	TasksAfter  []TaskItem `json:"tasks_after"`
	LastUpdate  time.Time  `json:"last_update"`
	Phase       string     `json:"phase"`
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
		Msg("Starting SpecTask planning phase with external agent")

	// Get Helix Agent configuration
	app, err := o.store.GetApp(ctx, task.HelixAppID)
	if err != nil {
		return fmt.Errorf("failed to get Helix agent: %w", err)
	}

	// Create or get external agent for this SpecTask
	externalAgent, err := o.getOrCreateExternalAgent(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to get/create external agent: %w", err)
	}

	// Create planning Helix session
	planningSession, err := o.createPlanningSession(ctx, task, app, externalAgent)
	if err != nil {
		return fmt.Errorf("failed to create planning session: %w", err)
	}

	// Update task with session reference
	task.PlanningSessionID = planningSession.ID
	task.Status = types.TaskStatusSpecGeneration
	task.UpdatedAt = time.Now()

	err = o.store.UpdateSpecTask(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to update spec task: %w", err)
	}

	// Update external agent with new session
	externalAgent.HelixSessionIDs = append(externalAgent.HelixSessionIDs, planningSession.ID)
	externalAgent.LastActivity = time.Now()
	err = o.store.UpdateSpecTaskExternalAgent(ctx, externalAgent)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update external agent with planning session")
	}

	// Update activity tracking
	err = o.store.UpsertExternalAgentActivity(ctx, &types.ExternalAgentActivity{
		ExternalAgentID: externalAgent.ID,
		SpecTaskID:      task.ID,
		LastInteraction: time.Now(),
		AgentType:       "spectask",
		WolfAppID:       externalAgent.WolfAppID,
		WorkspaceDir:    externalAgent.WorkspaceDir,
		UserID:          task.CreatedBy,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to update external agent activity")
	}

	log.Info().
		Str("task_id", task.ID).
		Str("session_id", planningSession.ID).
		Str("external_agent_id", externalAgent.ID).
		Msg("Planning phase started successfully")

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

// createImplementationSession creates a Helix session for the implementation phase
func (o *SpecTaskOrchestrator) createImplementationSession(ctx context.Context, task *types.SpecTask, app *types.App, agent *types.SpecTaskExternalAgent) (*types.Session, error) {
	// Build system prompt for implementation phase
	systemPrompt := o.buildImplementationPrompt(task, app)

	// Create session
	// Extract organization ID from metadata if present
	orgID := ""
	if task.Metadata != nil {
		if id, ok := task.Metadata["organization_id"].(string); ok {
			orgID = id
		}
	}

	session := &types.Session{
		ID:             fmt.Sprintf("ses_impl_%s", task.ID),
		Owner:          task.CreatedBy,
		OwnerType:      types.OwnerTypeUser,
		ParentApp:      app.ID,
		Mode:           types.SessionModeInference,
		Type:           types.SessionTypeText,
		ModelName:      app.Config.Helix.Assistants[0].Model,
		OrganizationID: orgID,
		Metadata: types.SessionMetadata{
			SystemPrompt:    systemPrompt,
			SpecTaskID:      task.ID,
			ExternalAgentID: agent.ID,
			Phase:           "implementation",
		},
	}

	createdSession, err := o.store.CreateSession(ctx, *session)
	if err != nil {
		return nil, fmt.Errorf("failed to create implementation session: %w", err)
	}

	log.Info().
		Str("session_id", createdSession.ID).
		Str("spec_task_id", task.ID).
		Msg("Created implementation session")

	return createdSession, nil
}

// buildImplementationPrompt builds the system prompt for implementation phase with git workflow
func (o *SpecTaskOrchestrator) buildImplementationPrompt(task *types.SpecTask, app *types.App) string {
	basePrompt := ""
	if len(app.Config.Helix.Assistants) > 0 {
		basePrompt = app.Config.Helix.Assistants[0].SystemPrompt
	}

	// Get repositories from parent project - repos are now managed at project level
	projectRepos, err := o.store.ListGitRepositories(context.Background(), &types.ListGitRepositoriesRequest{
		ProjectID: task.ProjectID,
	})
	if err != nil {
		log.Warn().Err(err).Str("project_id", task.ProjectID).Msg("Failed to get project repositories for prompt building")
		projectRepos = nil
	}

	// Find primary repo path (use first repo as fallback)
	primaryRepoPath := "backend" // Default fallback
	if len(projectRepos) > 0 {
		// Use LocalPath from first repo (or we could fetch project.DefaultRepoID to find the primary)
		primaryRepoPath = projectRepos[0].LocalPath
		if primaryRepoPath == "" {
			primaryRepoPath = "backend" // Fallback if LocalPath not set
		}
	}

	// Generate task directory name (same as planning phase)
	dateStr := time.Now().Format("2006-01-02")
	sanitizedName := sanitizeForBranchName(task.OriginalPrompt)
	if len(sanitizedName) > 50 {
		sanitizedName = sanitizedName[:50]
	}
	taskDirName := fmt.Sprintf("%s_%s_%s", dateStr, sanitizedName, task.ID)

	// Build implementation prompt
	var promptBuilder strings.Builder
	promptBuilder.WriteString(basePrompt)
	promptBuilder.WriteString("\n\n## Task: Implement According to Specifications\n\n")
	promptBuilder.WriteString("You are running in a full external agent session with Zed editor and git access.\n")
	promptBuilder.WriteString("This is the SAME Zed instance from the planning phase - you can see the planning thread in Zed!\n\n")
	promptBuilder.WriteString(fmt.Sprintf("**SpecTask**: %s\n", task.Name))
	promptBuilder.WriteString(fmt.Sprintf("**Description**: %s\n\n", task.Description))
	promptBuilder.WriteString("**Your job is to:**\n\n")
	promptBuilder.WriteString(fmt.Sprintf("1. Fetch latest design documents from helix-specs branch\n"))
	promptBuilder.WriteString(fmt.Sprintf("2. Read design docs from: ~/work/helix-specs/tasks/%s/\n", taskDirName))
	promptBuilder.WriteString(fmt.Sprintf("3. Create feature branch: feature/%s\n", task.ID))
	promptBuilder.WriteString("4. Implement according to tasks.md\n")
	promptBuilder.WriteString("5. Mark tasks in progress [ ] -> [~] and complete [~] -> [x] in tasks.md\n")
	promptBuilder.WriteString("6. Push progress updates to helix-specs branch\n")
	promptBuilder.WriteString("7. Push feature branch when ready\n\n")
	promptBuilder.WriteString("**Git Workflow**:\n")
	promptBuilder.WriteString(fmt.Sprintf("- Work in: ~/work/%s\n", primaryRepoPath))
	promptBuilder.WriteString("- Design docs: ~/work/helix-specs\n")
	promptBuilder.WriteString("- Feature branch: feature/" + task.ID + "\n\n")
	promptBuilder.WriteString("**Context from Planning Phase**:\n\n")
	promptBuilder.WriteString(fmt.Sprintf("Requirements: %s\n\n", task.RequirementsSpec))
	promptBuilder.WriteString(fmt.Sprintf("Design: %s\n\n", task.TechnicalDesign))
	promptBuilder.WriteString(fmt.Sprintf("Implementation Plan: %s\n\n", task.ImplementationPlan))
	promptBuilder.WriteString("Work methodically. All repositories and Zed state persist across sessions.")

	return promptBuilder.String()
}

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

	// If no external agent or not running, check old orchestration pattern
	o.mutex.RLock()
	orchestratedTask, exists := o.runningTasks[task.ID]
	o.mutex.RUnlock()

	if !exists {
		// Task not tracked in old orchestration system - this is OK for new reuse-agent pattern
		log.Debug().
			Str("task_id", task.ID).
			Msg("Task in implementation but not in orchestrator tracking (using reused agent pattern)")
		return nil
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

// getOrCreateExternalAgent gets existing external agent or creates new one for SpecTask
func (o *SpecTaskOrchestrator) getOrCreateExternalAgent(ctx context.Context, task *types.SpecTask) (*types.SpecTaskExternalAgent, error) {
	// Try to get existing agent
	agent, err := o.store.GetSpecTaskExternalAgent(ctx, task.ID)
	if err == nil && agent.Status == "running" {
		log.Info().
			Str("agent_id", agent.ID).
			Str("spec_task_id", task.ID).
			Msg("Reusing existing external agent")
		return agent, nil
	}

	// Create new external agent
	agentID := fmt.Sprintf("zed-spectask-%s", task.ID)
	workspaceDir := fmt.Sprintf("/opt/helix/filestore/workspaces/spectasks/%s", task.ID)

	log.Info().
		Str("agent_id", agentID).
		Str("workspace_dir", workspaceDir).
		Msg("Creating new external agent for SpecTask")

	// Get project to access repository configuration
	project, err := o.store.GetProject(ctx, task.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get project for SpecTask: %w", err)
	}

	// Get all repositories for this project
	repos, err := o.store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{
		ProjectID: task.ProjectID,
	})
	if err != nil {
		log.Warn().Err(err).Msg("Failed to get project repositories, continuing without git")
	}

	// Extract repository IDs
	var repositoryIDs []string
	for _, repo := range repos {
		repositoryIDs = append(repositoryIDs, repo.ID)
	}

	// Determine primary repository
	primaryRepoID := project.DefaultRepoID
	if primaryRepoID == "" && len(repositoryIDs) > 0 {
		// Fall back to first repository if no default set
		primaryRepoID = repositoryIDs[0]
	}

	log.Info().
		Strs("repository_ids", repositoryIDs).
		Str("primary_repository_id", primaryRepoID).
		Msg("Attaching project repositories to external agent")

	// Get or create API key for user (needed for git HTTP authentication)
	userAPIKey, err := o.getOrCreateUserAPIKey(ctx, task.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("failed to get user API key for git operations: %w", err)
	}

	log.Info().
		Str("user_id", task.CreatedBy).
		Msg("Using user's API key for git operations (RBAC enforced)")

	// Create Wolf agent with per-SpecTask workspace
	agentReq := &types.ZedAgent{
		SessionID:           agentID, // Agent-level session ID (not tied to specific Helix session)
		UserID:              task.CreatedBy,
		WorkDir:             workspaceDir,
		ProjectPath:         "backend",     // Default primary repo path
		RepositoryIDs:       repositoryIDs, // Repositories to clone
		PrimaryRepositoryID: primaryRepoID, // Primary repository for design docs
		SpecTaskID:          task.ID,       // Link to SpecTask
		DisplayWidth:        2560,
		DisplayHeight:       1600,
		DisplayRefreshRate:  60,
		Env: []string{
			// Pass user's API key for git operations (NOT server's RunnerToken)
			// This ensures RBAC is enforced - agent can only access repos the user can access
			fmt.Sprintf("USER_API_TOKEN=%s", userAPIKey),
		},
	}

	agentResp, err := o.wolfExecutor.StartZedAgent(ctx, agentReq)
	if err != nil {
		return nil, fmt.Errorf("failed to start Wolf agent: %w", err)
	}

	// Create external agent record
	externalAgent := &types.SpecTaskExternalAgent{
		ID:              agentID,
		SpecTaskID:      task.ID,
		WolfAppID:       agentResp.WolfAppID,
		WorkspaceDir:    workspaceDir,
		HelixSessionIDs: []string{},
		ZedThreadIDs:    []string{},
		Status:          "running",
		Created:         time.Now(),
		LastActivity:    time.Now(),
		UserID:          task.CreatedBy,
	}

	err = o.store.CreateSpecTaskExternalAgent(ctx, externalAgent)
	if err != nil {
		return nil, fmt.Errorf("failed to create external agent record: %w", err)
	}

	// Update task with external agent ID
	task.ExternalAgentID = agentID
	err = o.store.UpdateSpecTask(ctx, task)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update task with external agent ID")
	}

	log.Info().
		Str("agent_id", agentID).
		Str("wolf_app_id", agentResp.WolfAppID).
		Msg("External agent created successfully")

	return externalAgent, nil
}

// createPlanningSession creates a Helix session for the planning phase
func (o *SpecTaskOrchestrator) createPlanningSession(ctx context.Context, task *types.SpecTask, app *types.App, agent *types.SpecTaskExternalAgent) (*types.Session, error) {
	// Build system prompt for planning phase
	systemPrompt := o.buildPlanningPrompt(task, app)

	// Extract organization ID from metadata if present
	orgID := ""
	if task.Metadata != nil {
		if id, ok := task.Metadata["organization_id"].(string); ok {
			orgID = id
		}
	}

	// Create session
	session := &types.Session{
		ID:             fmt.Sprintf("ses_planning_%s", task.ID),
		Name:           fmt.Sprintf("Planning: %s", task.Name),
		Owner:          task.CreatedBy,
		OwnerType:      types.OwnerTypeUser,
		ParentApp:      app.ID,
		Mode:           types.SessionModeInference,
		Type:           types.SessionTypeText,
		ModelName:      app.Config.Helix.Assistants[0].Model,
		OrganizationID: orgID,
		Metadata: types.SessionMetadata{
			SystemPrompt:    systemPrompt,
			SpecTaskID:      task.ID,
			ExternalAgentID: agent.ID,
			Phase:           "planning",
		},
	}

	createdSession, err := o.store.CreateSession(ctx, *session)
	if err != nil {
		return nil, fmt.Errorf("failed to create planning session: %w", err)
	}

	log.Info().
		Str("session_id", createdSession.ID).
		Str("spec_task_id", task.ID).
		Msg("Created planning session")

	return createdSession, nil
}

// buildPlanningPrompt builds the system prompt for planning phase with complete git workflow
func (o *SpecTaskOrchestrator) buildPlanningPrompt(task *types.SpecTask, app *types.App) string {
	basePrompt := ""
	if len(app.Config.Helix.Assistants) > 0 {
		basePrompt = app.Config.Helix.Assistants[0].SystemPrompt
	}

	// Get repositories from parent project - repos are now managed at project level
	projectRepos, err := o.store.ListGitRepositories(context.Background(), &types.ListGitRepositoriesRequest{
		ProjectID: task.ProjectID,
	})
	if err != nil {
		log.Warn().Err(err).Str("project_id", task.ProjectID).Msg("Failed to get project repositories for planning prompt")
		projectRepos = nil
	}

	// Note which repositories are available (already cloned by API server)
	repoInstructions := ""
	if len(projectRepos) > 0 {
		repoInstructions = "\n**Available Repositories (already cloned):**\n\n"
		for _, repo := range projectRepos {
			repoPath := repo.Name
			if repo.RepoType == "internal" {
				repoPath = ".helix-project"
			}
			repoInstructions += fmt.Sprintf("- `%s` at `~/work/%s`\n", repo.Name, repoPath)
		}
		repoInstructions += "\n"
	}

	// Generate task directory name
	dateStr := time.Now().Format("2006-01-02")
	sanitizedName := sanitizeForBranchName(task.OriginalPrompt)
	if len(sanitizedName) > 50 {
		sanitizedName = sanitizedName[:50]
	}
	taskDirName := fmt.Sprintf("%s_%s_%s", dateStr, sanitizedName, task.ID)

	// Build planning prompt using string builder to avoid nested backticks
	var promptBuilder strings.Builder
	promptBuilder.WriteString(basePrompt)
	promptBuilder.WriteString("\n\n## Task: Generate Specifications from User Request\n\n")
	promptBuilder.WriteString("You are running in a full external agent session with Zed editor and git access.\n\n")
	promptBuilder.WriteString("The user has provided the following task description:\n\n")
	promptBuilder.WriteString("---\n")
	promptBuilder.WriteString(task.OriginalPrompt)
	promptBuilder.WriteString("\n---\n\n")
	promptBuilder.WriteString(repoInstructions)
	promptBuilder.WriteString("**Your job is to:**\n\n")
	promptBuilder.WriteString("1. Analyze the existing codebase in the primary repository and other attached repositories\n")
	promptBuilder.WriteString("2. Create design documents based on the user request and current codebase\n")
	promptBuilder.WriteString("3. Commit and then push the design docs to the upstream repository\n\n")
	promptBuilder.WriteString("**Step 1: Analyze the existing codebase**\n\n")
	promptBuilder.WriteString("Read the application source code from the primary repository and any other relevant repositories.\n")
	promptBuilder.WriteString("Understand the current architecture, patterns, and conventions before designing new features.\n\n")
	promptBuilder.WriteString("**Step 2: Navigate to the design docs directory**\n\n")
	promptBuilder.WriteString("The helix-specs git worktree is already set up at:\n")
	promptBuilder.WriteString("`~/work/helix-specs`\n\n")
	promptBuilder.WriteString("Create a dated task directory and navigate to it:\n\n")
	promptBuilder.WriteString("```bash\n")
	promptBuilder.WriteString("cd ~/work/helix-specs/tasks\n")
	promptBuilder.WriteString(fmt.Sprintf("mkdir -p %s\n", taskDirName))
	promptBuilder.WriteString(fmt.Sprintf("cd %s\n", taskDirName))
	promptBuilder.WriteString("```\n\n")
	promptBuilder.WriteString("**Step 3: Create design documents**\n\n")
	promptBuilder.WriteString("Write these markdown files in `~/work/helix-specs/tasks/` (the current directory):\n\n")
	promptBuilder.WriteString("1. **requirements.md** - User stories + EARS acceptance criteria\n")
	promptBuilder.WriteString("2. **design.md** - Architecture, diagrams, data models\n")
	promptBuilder.WriteString("3. **tasks.md** - Implementation tasks with [ ]/[~]/[x] markers\n")
	promptBuilder.WriteString("4. **task-metadata.json** - {\"name\": \"...\", \"description\": \"...\", \"type\": \"feature|bug|refactor\"}\n\n")
	promptBuilder.WriteString("**Step 4: Commit and then push to upstream repository**\n\n")
	promptBuilder.WriteString("This is **CRITICAL** - you must commit and then push to get design docs back to Helix:\n\n")
	promptBuilder.WriteString("```bash\n")
	promptBuilder.WriteString("git add .\n")
	promptBuilder.WriteString(fmt.Sprintf("git commit -m \"Add design docs for %s\"\n", sanitizedName))
	promptBuilder.WriteString("git push origin helix-specs\n")
	promptBuilder.WriteString("```\n\n")
	promptBuilder.WriteString("The helix-specs branch is **forward-only** (never rolled back).\n")
	promptBuilder.WriteString("Pushing to upstream is how the Helix UI retrieves your design docs to display to the user.\n\n")
	promptBuilder.WriteString("**All work persists in `~/work/` across sessions.**")

	return promptBuilder.String()
}

// sanitizeForBranchName is already defined in design_docs_helpers.go

// getOrCreateUserAPIKey gets user's existing API key for git operations
func (o *SpecTaskOrchestrator) getOrCreateUserAPIKey(ctx context.Context, userID string) (string, error) {
	// List user's existing API keys
	keys, err := o.store.ListAPIKeys(ctx, &store.ListAPIKeysQuery{
		Owner:     userID,
		OwnerType: types.OwnerTypeUser,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list user API keys: %w", err)
	}

	// Use user's first API key
	if len(keys) > 0 {
		log.Debug().
			Str("user_id", userID).
			Str("api_key_name", keys[0].Name).
			Msg("Using user's existing API key for git operations")
		return keys[0].Key, nil
	}

	// User has no API keys - this is an error, they need to create one first
	return "", fmt.Errorf("user %s has no API keys - cannot perform git operations (create one in Account Settings)", userID)
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
