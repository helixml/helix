package services

import (
	"context"
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
	wolfExecutor          WolfExecutorInterface // Wolf executor for external agents
	stopChan              chan struct{}
	wg                    sync.WaitGroup
	orchestrationInterval time.Duration
	prPollInterval        time.Duration // Interval for polling external PR status (default 1 minute)
	testMode              bool
}

// WolfExecutorInterface defines the interface for Wolf executor
type WolfExecutorInterface interface {
	StartDesktop(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error)
	StopDesktop(ctx context.Context, sessionID string) error
}

// NewSpecTaskOrchestrator creates a new orchestrator
func NewSpecTaskOrchestrator(
	store store.Store,
	controller *controller.Controller,
	gitService *GitRepositoryService,
	specTaskService *SpecDrivenTaskService,
	wolfExecutor WolfExecutorInterface, // Wolf executor for external agents
) *SpecTaskOrchestrator {
	return &SpecTaskOrchestrator{
		store:                 store,
		controller:            controller,
		gitService:            gitService,
		specTaskService:       specTaskService,
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

	// Start PR polling loop (runs every 1 minute to check external PR status)
	o.wg.Add(1)
	go o.prPollLoop(ctx)

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

	// Filter to only active tasks (PR polling handled by separate 1-minute loop)
	activeStatuses := map[types.SpecTaskStatus]bool{
		types.TaskStatusBacklog:              true,
		types.TaskStatusQueuedSpecGeneration: true,
		types.TaskStatusQueuedImplementation: true,
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
					Str("status", task.Status.String()).
					Msg("Task references deleted project - skipping")
			} else {
				log.Error().
					Err(err).
					Str("task_id", task.ID).
					Str("status", task.Status.String()).
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
	case types.TaskStatusQueuedSpecGeneration:
		return o.handleQueuedSpecGeneration(ctx, task)
	case types.TaskStatusQueuedImplementation:
		return o.handleQueuedImplementation(ctx, task)
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
	case types.TaskStatusPullRequest:
		return o.handlePullRequest(ctx, task)
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

	var planningLimit = 3

	if project.Metadata.BoardSettings != nil &&
		project.Metadata.BoardSettings.WIPLimits.Planning > 0 {
		planningLimit = project.Metadata.BoardSettings.WIPLimits.Planning
	}

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

	log.Info().
		Str("task_id", task.ID).
		Str("helix_app_id", task.HelixAppID).
		Msg("Auto-starting SpecTask planning phase")

	// Delegate to the canonical StartSpecGeneration implementation
	// This ensures both explicit start and auto-start use the same code path
	// Auto-start doesn't have user browser context, so pass empty options
	o.specTaskService.StartSpecGeneration(ctx, task)

	return nil
}

// handleQueuedSpecGeneration handles tasks in queued spec generation
func (o *SpecTaskOrchestrator) handleQueuedSpecGeneration(ctx context.Context, task *types.SpecTask) error {
	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		o.specTaskService.StartSpecGeneration(ctx, task)
	}()

	return nil
}

// handleQueuedImplementation handles tasks in queued implementation
func (o *SpecTaskOrchestrator) handleQueuedImplementation(ctx context.Context, task *types.SpecTask) error {
	// Check if implementation session is complete
	// This would integrate with existing SpecDrivenTaskService
	// For now, we'll assume implementation is ready when all implementation tasks exist
	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		o.specTaskService.StartSpecGeneration(ctx, task)
	}()

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

	// NOTE: PR status polling removed from here to avoid hammering ADO API every
	// 10 seconds. Now handled by dedicated prPollLoop which runs every 1 minute.
	// Tasks move to pull_request status when user approves implementation, then
	// prPollLoop polls ADO for merge status. See conversation with Karolis.

	// Task not tracked - this is OK for new reuse-agent pattern
	// Implementation progress is tracked via shell scripts in the sandbox
	log.Debug().
		Str("task_id", task.ID).
		Msg("Task in implementation (using reused agent pattern)")
	return nil
}

// handlePullRequest polls external repo for PR merge status
// Called from the dedicated PR polling loop (runs every 1 minute)
func (o *SpecTaskOrchestrator) handlePullRequest(ctx context.Context, task *types.SpecTask) error {
	if task.PullRequestID == "" {
		log.Warn().
			Str("task_id", task.ID).
			Msg("Task in pull_request status but no PullRequestID set")
		return nil
	}

	log.Debug().
		Str("task_id", task.ID).
		Str("pr_id", task.PullRequestID).
		Msg("Polling external PR status")

	return o.processExternalPullRequestStatus(ctx, task)
}

func (o *SpecTaskOrchestrator) processExternalPullRequestStatus(ctx context.Context, task *types.SpecTask) error {
	project, err := o.store.GetProject(ctx, task.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	pr, err := o.gitService.GetPullRequest(ctx, project.DefaultRepoID, task.PullRequestID)
	if err != nil {
		return fmt.Errorf("failed to get pull request: %w", err)
	}

	switch pr.State {
	case "active":
		// Active - still open, nothing to do
		log.Debug().
			Str("task_id", task.ID).
			Str("pr_id", task.PullRequestID).
			Msg("PR still active, awaiting merge")
	case "completed":
		// PR merged - move to done
		now := time.Now()
		task.Status = types.TaskStatusDone
		task.MergedToMain = true
		task.MergedAt = &now
		task.CompletedAt = &now
		task.UpdatedAt = now
		log.Info().
			Str("task_id", task.ID).
			Str("pr_id", task.PullRequestID).
			Msg("PR merged! Moving task to done")
		return o.store.UpdateSpecTask(ctx, task)
	case "abandoned":
		// PR abandoned - archive the task
		task.Archived = true
		task.UpdatedAt = time.Now()
		log.Info().
			Str("task_id", task.ID).
			Str("pr_id", task.PullRequestID).
			Msg("PR abandoned, archiving task")
		return o.store.UpdateSpecTask(ctx, task)
	}

	return nil
}

// prPollLoop polls external repos for PR merge status every minute
func (o *SpecTaskOrchestrator) prPollLoop(ctx context.Context) {
	defer o.wg.Done()

	// Use configured interval or default to 1 minute
	interval := o.prPollInterval
	if interval == 0 {
		interval = 1 * time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Info().Dur("interval", interval).Msg("Starting PR poll loop")

	for {
		select {
		case <-ctx.Done():
			return
		case <-o.stopChan:
			return
		case <-ticker.C:
			o.pollPullRequests(ctx)
		}
	}
}

// pollPullRequests checks all tasks in pull_request status for merge status
func (o *SpecTaskOrchestrator) pollPullRequests(ctx context.Context) {
	tasks, err := o.store.ListSpecTasks(ctx, &types.SpecTaskFilters{
		Status: types.TaskStatusPullRequest,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to list PR tasks for polling")
		return
	}

	if len(tasks) > 0 {
		log.Debug().Int("count", len(tasks)).Msg("Polling external PR status for tasks")
	}

	for _, task := range tasks {
		err := o.handlePullRequest(ctx, task)
		if err != nil {
			// Tasks with deleted projects are expected - don't spam logs
			if strings.Contains(err.Error(), "record not found") || strings.Contains(err.Error(), "not found") {
				log.Trace().
					Err(err).
					Str("task_id", task.ID).
					Msg("PR task references deleted project - skipping")
			} else {
				log.Error().
					Err(err).
					Str("task_id", task.ID).
					Msg("Failed to poll PR status")
			}
		}
	}
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
			repoInstructions += fmt.Sprintf("- `%s` at `/home/retro/work/%s`\n", repo.Name, repo.Name)
		}
		repoInstructions += "\n"
	}

	// Use DesignDocPath if set (new human-readable format), fall back to task ID
	taskDirName := task.DesignDocPath
	if taskDirName == "" {
		taskDirName = task.ID // Backwards compatibility for old tasks
	}

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
	promptBuilder.WriteString("`/home/retro/work/helix-specs`\n\n")
	promptBuilder.WriteString("Create a dated task directory and navigate to it:\n\n")
	promptBuilder.WriteString("```bash\n")
	promptBuilder.WriteString("cd /home/retro/work/helix-specs/tasks\n")
	promptBuilder.WriteString(fmt.Sprintf("mkdir -p %s\n", taskDirName))
	promptBuilder.WriteString(fmt.Sprintf("cd %s\n", taskDirName))
	promptBuilder.WriteString("```\n\n")
	promptBuilder.WriteString("**Step 3: Create design documents**\n\n")
	promptBuilder.WriteString("Write these markdown files in `/home/retro/work/helix-specs/tasks/` (the current directory):\n\n")
	promptBuilder.WriteString("1. **requirements.md** - User stories + EARS acceptance criteria\n")
	promptBuilder.WriteString("2. **design.md** - Architecture, diagrams, data models\n")
	promptBuilder.WriteString("3. **tasks.md** - Implementation tasks with [ ]/[~]/[x] markers\n")
	promptBuilder.WriteString("4. **task-metadata.json** - {\"name\": \"...\", \"description\": \"...\", \"type\": \"feature|bug|refactor\"}\n\n")
	promptBuilder.WriteString("**Step 4: Commit and then push to upstream repository**\n\n")
	promptBuilder.WriteString("This is **CRITICAL** - you must commit and then push to get design docs back to Helix:\n\n")
	promptBuilder.WriteString("```bash\n")
	promptBuilder.WriteString("git add .\n")
	promptBuilder.WriteString(fmt.Sprintf("git commit -m \"Add design docs for %s\"\n", task.Name))
	promptBuilder.WriteString("git push origin helix-specs\n")
	promptBuilder.WriteString("```\n\n")
	promptBuilder.WriteString("The helix-specs branch is **forward-only** (never rolled back).\n")
	promptBuilder.WriteString("Pushing to upstream is how the Helix UI retrieves your design docs to display to the user.\n\n")
	promptBuilder.WriteString("**All work persists in `/home/retro/work/` across sessions.**")

	return promptBuilder.String()
}
