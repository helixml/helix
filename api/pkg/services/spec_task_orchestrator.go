package services

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
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

// handleBacklog handles tasks in backlog state - creates external agent and starts planning
func (o *SpecTaskOrchestrator) handleBacklog(ctx context.Context, task *types.SpecTask) error {
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
func (o *SpecTaskOrchestrator) handleImplementationQueued(ctx context.Context, task *types.SpecTask) error {
	log.Info().
		Str("task_id", task.ID).
		Str("external_agent_id", task.ExternalAgentID).
		Msg("Starting implementation phase with existing external agent")

	// Get Helix Agent configuration (same agent used for planning)
	app, err := o.store.GetApp(ctx, task.HelixAppID)
	if err != nil {
		return fmt.Errorf("failed to get Helix agent: %w", err)
	}

	// Get EXISTING external agent (already running from planning phase!)
	externalAgent, err := o.store.GetSpecTaskExternalAgent(ctx, task.ID)
	if err != nil {
		return fmt.Errorf("failed to get external agent: %w", err)
	}

	// Check if agent needs resurrection (was terminated due to idle)
	if externalAgent.Status != "running" {
		log.Info().
			Str("agent_id", externalAgent.ID).
			Msg("External agent was terminated, resurrecting with same workspace")

		// Resurrect agent with SAME workspace
		agentReq := &types.ZedAgent{
			SessionID:          externalAgent.ID,
			UserID:             task.CreatedBy,
			WorkDir:            externalAgent.WorkspaceDir, // SAME workspace!
			ProjectPath:        "backend",
			DisplayWidth:       2560,
			DisplayHeight:      1600,
			DisplayRefreshRate: 60,
		}

		agentResp, err := o.wolfExecutor.StartZedAgent(ctx, agentReq)
		if err != nil {
			return fmt.Errorf("failed to resurrect external agent: %w", err)
		}

		externalAgent.WolfAppID = agentResp.WolfAppID
		externalAgent.Status = "running"
		externalAgent.LastActivity = time.Now()

		err = o.store.UpdateSpecTaskExternalAgent(ctx, externalAgent)
		if err != nil {
			return fmt.Errorf("failed to update resurrected agent: %w", err)
		}
	}

	// Create NEW Helix session for implementation (creates new Zed thread in existing instance)
	implSession, err := o.createImplementationSession(ctx, task, app, externalAgent)
	if err != nil {
		return fmt.Errorf("failed to create implementation session: %w", err)
	}

	// Update task with session reference
	task.ImplementationSessionID = implSession.ID
	task.Status = types.TaskStatusImplementation
	task.UpdatedAt = time.Now()

	err = o.store.UpdateSpecTask(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to update spec task: %w", err)
	}

	// Update external agent with new session
	externalAgent.HelixSessionIDs = append(externalAgent.HelixSessionIDs, implSession.ID)
	externalAgent.LastActivity = time.Now()
	err = o.store.UpdateSpecTaskExternalAgent(ctx, externalAgent)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update external agent with implementation session")
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
		Str("session_id", implSession.ID).
		Str("external_agent_id", externalAgent.ID).
		Int("total_sessions", len(externalAgent.HelixSessionIDs)).
		Msg("Implementation phase started successfully")

	return nil
}

// createImplementationSession creates a Helix session for the implementation phase
func (o *SpecTaskOrchestrator) createImplementationSession(ctx context.Context, task *types.SpecTask, app *types.App, agent *types.SpecTaskExternalAgent) (*types.Session, error) {
	// Build system prompt for implementation phase
	systemPrompt := o.buildImplementationPrompt(task, app)

	// Create session
	session := &types.Session{
		ID:             fmt.Sprintf("ses_impl_%s", task.ID),
		UserID:         task.CreatedBy,
		AppID:          app.ID,
		Mode:           types.SessionModeInference,
		Type:           types.SessionTypeText,
		ModelName:      app.Config.Helix.Assistants[0].Model,
		SystemPrompt:   systemPrompt,
		ParentApp:      app.ID,
		OrganizationID: task.Metadata.String(),
		Metadata: types.SessionMetadata{
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

	// Parse attached repositories
	var repos []types.AttachedRepository
	if len(task.AttachedRepositories) > 0 {
		json.Unmarshal(task.AttachedRepositories, &repos)
	}

	// Find primary repo and task directory
	primaryRepoPath := "backend"
	for _, repo := range repos {
		if repo.IsPrimary {
			primaryRepoPath = repo.LocalPath
			break
		}
	}

	// Generate task directory name (same as planning phase)
	dateStr := time.Now().Format("2006-01-02")
	sanitizedName := sanitizeForBranchName(task.OriginalPrompt)
	if len(sanitizedName) > 50 {
		sanitizedName = sanitizedName[:50]
	}
	taskDirName := fmt.Sprintf("%s_%s_%s", dateStr, sanitizedName, task.ID)

	return fmt.Sprintf(`%s

## Task: Implement According to Specifications

You are running in a full external agent session with Zed editor and git access.
This is the SAME Zed instance from the planning phase - you can see the planning thread in Zed!

**SpecTask**: %s
**Description**: %s

**Your job is to:**

**Step 1: Fetch latest design documents**

```bash
cd /home/retro/work/%s
git fetch origin helix-design-docs
```

**Step 2: Read design documents from helix-design-docs worktree**

```bash
cd .git-worktrees/helix-design-docs/tasks/%s
cat requirements.md
cat design.md
cat tasks.md
cat task-metadata.json
```

**Step 3: Create feature branch for implementation**

```bash
cd /home/retro/work/%s
git checkout -b feature/%s
```

**Step 4: Implement according to tasks.md**

Follow the implementation plan in tasks.md. For each task:

1. Mark task in progress:
```bash
cd /home/retro/work/%s/.git-worktrees/helix-design-docs
# Edit tasks/%s/tasks.md: change [ ] to [~]
git add tasks/%s/tasks.md
git commit -m "🤖 Agent: Started task X"
git push origin helix-design-docs
```

2. Implement the task in the feature branch
3. Commit implementation:
```bash
cd /home/retro/work/%s
git add .
git commit -m "Implement task X"
```

4. Mark task complete:
```bash
cd .git-worktrees/helix-design-docs
# Edit tasks/%s/tasks.md: change [~] to [x]
git add tasks/%s/tasks.md
git commit -m "🤖 Agent: Completed task X"
git push origin helix-design-docs
```

**Step 5: Push feature branch when ready**

```bash
cd /home/retro/work/%s
git push -u origin feature/%s
```

**Context from Planning Phase**:

**Requirements**: %s

**Design**: %s

**Implementation Plan**: %s

Work methodically. All repositories and Zed state persist across sessions. You can reference the planning thread in Zed.
`, basePrompt, task.Name, task.Description, primaryRepoPath, taskDirName, primaryRepoPath, task.ID, primaryRepoPath, taskDirName, taskDirName, primaryRepoPath, taskDirName, taskDirName, primaryRepoPath, task.ID, task.RequirementsSpec, task.TechnicalDesign, task.ImplementationPlan)
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

	// Create Wolf agent with per-SpecTask workspace
	agentReq := &types.ZedAgent{
		SessionID:      agentID, // Agent-level session ID (not tied to specific Helix session)
		UserID:         task.CreatedBy,
		WorkDir:        workspaceDir,
		ProjectPath:    "backend", // Default primary repo path
		DisplayWidth:   2560,
		DisplayHeight:  1600,
		DisplayRefreshRate: 60,
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

	// Create session
	session := &types.Session{
		ID:             fmt.Sprintf("ses_planning_%s", task.ID),
		UserID:         task.CreatedBy,
		AppID:          app.ID,
		Mode:           types.SessionModeInference,
		Type:           types.SessionTypeText,
		ModelName:      app.Config.Helix.Assistants[0].Model,
		SystemPrompt:   systemPrompt,
		ParentApp:      app.ID,
		OrganizationID: task.Metadata.String(), // Extract from metadata if needed
		Metadata: types.SessionMetadata{
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

	// Parse attached repositories
	var repos []types.AttachedRepository
	if len(task.AttachedRepositories) > 0 {
		json.Unmarshal(task.AttachedRepositories, &repos)
	}

	// Build repository clone commands
	repoInstructions := ""
	if len(repos) > 0 {
		repoInstructions = "\n**Step 1: Clone all attached repositories**\n\n```bash\ncd /home/retro/work\n"
		for _, repo := range repos {
			repoInstructions += fmt.Sprintf("git clone %s %s\n", repo.CloneURL, repo.LocalPath)
		}
		repoInstructions += "```\n"
	}

	// Find primary repo for helix-design-docs
	primaryRepoPath := "backend" // Default
	for _, repo := range repos {
		if repo.IsPrimary {
			primaryRepoPath = repo.LocalPath
			break
		}
	}

	// Generate task directory name
	dateStr := time.Now().Format("2006-01-02")
	sanitizedName := sanitizeForBranchName(task.OriginalPrompt)
	if len(sanitizedName) > 50 {
		sanitizedName = sanitizedName[:50]
	}
	taskDirName := fmt.Sprintf("%s_%s_%s", dateStr, sanitizedName, task.ID)

	return fmt.Sprintf(`%s

## Task: Generate Specifications from User Request

You are running in a full external agent session with Zed editor and git access.

The user has provided the following task description:

---
%s
---

**Your job is to:**
%s
**Step 2: Setup helix-design-docs branch and worktree**

```bash
cd /home/retro/work/%s
git branch helix-design-docs 2>/dev/null || true
git worktree add .git-worktrees/helix-design-docs helix-design-docs 2>/dev/null || true
cd .git-worktrees/helix-design-docs
mkdir -p tasks
cd tasks
mkdir -p %s
cd %s
```

**Step 3: Write design documents**

Create the following markdown files:

1. **requirements.md** - User stories + EARS acceptance criteria
2. **design.md** - Architecture, sequence diagrams, data models, API contracts
3. **tasks.md** - Discrete implementation tasks with [ ]/[~]/[x] markers
4. **task-metadata.json** - Extract: {"name": "...", "description": "...", "type": "feature|bug|refactor"}

**Step 4: Commit and push to Helix git server**

```bash
cd /home/retro/work/%s/.git-worktrees/helix-design-docs
git add tasks/%s/requirements.md
git commit -m "Add requirements specification for %s"
git add tasks/%s/design.md
git commit -m "Add technical design for %s"
git add tasks/%s/tasks.md
git commit -m "Add implementation plan for %s"
git add tasks/%s/task-metadata.json
git commit -m "Add task metadata for %s"
git push -u origin helix-design-docs
```

**CRITICAL**:
- The helix-design-docs branch is **forward-only** (never rolled back)
- Push to Helix git server so UI can read design docs
- Use the [ ]/[~]/[x] markers in tasks.md for progress tracking

Work in the persistent workspace at /home/retro/work/ - everything persists across sessions.
`, basePrompt, task.OriginalPrompt, repoInstructions, primaryRepoPath, taskDirName, taskDirName, primaryRepoPath, taskDirName, task.ID, taskDirName, task.ID, taskDirName, task.ID, taskDirName, task.ID)
}

// sanitizeForBranchName converts text to branch-name-safe format
func sanitizeForBranchName(text string) string {
	// Simple sanitization for git branch names
	text = strings.ToLower(text)
	text = strings.ReplaceAll(text, " ", "-")
	text = strings.ReplaceAll(text, "_", "-")
	// Remove special characters
	reg := regexp.MustCompile("[^a-z0-9-]")
	text = reg.ReplaceAllString(text, "")
	return text
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
