package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

//go:generate mockgen -source $GOFILE -destination spec_task_orchestrator_mocks.go -package $GOPACKAGE

// SpecTaskOrchestrator orchestrates SpecTasks through the complete workflow
// Pushes agents through design → approval → implementation
// Manages agent lifecycle and reuses sessions across Helix interactions
// EnsurePRsFunc is a callback that creates PRs for all project repos that have
// the task's feature branch. Set by the server so the orchestrator can retry
// PR creation for repos whose branches weren't ready at initial "Open PR" time.
type EnsurePRsFunc func(ctx context.Context, task *types.SpecTask, primaryRepoID string) error

type SpecTaskOrchestrator struct {
	store                 store.Store
	gitService            *GitRepositoryService
	specTaskService       *SpecDrivenTaskService
	containerExecutor     ContainerExecutor // Executor for external agent containers
	goldenBuildService    *GoldenBuildService
	attentionService      *AttentionService
	ensurePRs             EnsurePRsFunc // Callback to create missing PRs (set by server)
	stopChan              chan struct{}
	wg                    sync.WaitGroup
	backlogProjectLocks   sync.Map // map[project_id]*sync.Mutex
	orchestrationInterval time.Duration
	prPollInterval        time.Duration // Interval for polling external PR status (default 1 minute)
	testMode              bool
}

// ContainerExecutor defines the interface for container lifecycle management
type ContainerExecutor interface {
	StartDesktop(ctx context.Context, agent *types.DesktopAgent) (*types.DesktopAgentResponse, error)
	StopDesktop(ctx context.Context, sessionID string) error
	HasRunningContainer(ctx context.Context, sessionID string) bool
	GetGoldenBuildResult(ctx context.Context, sandboxID, projectID string) (*hydra.GoldenBuildResult, error)
}

// NewSpecTaskOrchestrator creates a new orchestrator
func NewSpecTaskOrchestrator(
	store store.Store,
	gitService *GitRepositoryService,
	specTaskService *SpecDrivenTaskService,
	containerExecutor ContainerExecutor, // Executor for external agent containers
) *SpecTaskOrchestrator {
	return &SpecTaskOrchestrator{
		store:                 store,
		gitService:            gitService,
		specTaskService:       specTaskService,
		containerExecutor:     containerExecutor,
		stopChan:              make(chan struct{}),
		orchestrationInterval: 10 * time.Second, // Check every 10 seconds
		testMode:              false,
	}
}

// SetEnsurePRsFunc sets the callback used to create PRs for repos that may not
// have had their feature branch ready when the user first clicked "Open PR".
func (o *SpecTaskOrchestrator) SetEnsurePRsFunc(fn EnsurePRsFunc) {
	o.ensurePRs = fn
}

// SetTestMode enables/disables test mode
func (o *SpecTaskOrchestrator) SetTestMode(enabled bool) {
	o.testMode = enabled
}

// SetGoldenBuildService sets the golden build service for triggering cache warm-ups on merge.
func (o *SpecTaskOrchestrator) SetGoldenBuildService(svc *GoldenBuildService) {
	o.goldenBuildService = svc
}

// SetAttentionService sets the attention service for emitting human-needed events.
func (o *SpecTaskOrchestrator) SetAttentionService(svc *AttentionService) {
	o.attentionService = svc
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

	// Individual task update channel
	taskCh := make(chan *types.SpecTask)

	subscription, err := o.store.SubscribeForTasks(ctx, &store.SpecTaskSubscriptionFilter{
		Statuses: []types.SpecTaskStatus{
			types.TaskStatusBacklog,
			types.TaskStatusQueuedSpecGeneration,
			types.TaskStatusQueuedImplementation,
			types.TaskStatusDone, // For shutdown
		},
	}, func(task *types.SpecTask) error {
		taskCh <- task
		return nil
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to subscribe to tasks")
		return
	}
	defer func() {
		err := subscription.Unsubscribe()
		if err != nil {
			log.Error().Err(err).Msg("failed to unsubscribe from tasks")
		}
	}()

	// Subscribe to failure/PR status transitions to emit attention events.
	// This is separate from the main orchestration subscription because these
	// statuses don't need orchestration — they just need human notification.
	if o.attentionService != nil {
		attentionSub, err := o.store.SubscribeForTasks(ctx, &store.SpecTaskSubscriptionFilter{
			Statuses: []types.SpecTaskStatus{
				types.TaskStatusSpecFailed,
				types.TaskStatusImplementationFailed,
			},
		}, func(task *types.SpecTask) error {
			var eventType types.AttentionEventType
			switch task.Status {
			case types.TaskStatusSpecFailed:
				eventType = types.AttentionEventSpecFailed
			case types.TaskStatusImplementationFailed:
				eventType = types.AttentionEventImplementationFailed
			default:
				return nil
			}
			go func(t *types.SpecTask, et types.AttentionEventType) {
				_, emitErr := o.attentionService.EmitEvent(
					context.Background(),
					et,
					t,
					"", // no qualifier — one event per failure
					nil,
				)
				if emitErr != nil {
					log.Warn().Err(emitErr).
						Str("spec_task_id", t.ID).
						Str("event_type", string(et)).
						Msg("Failed to emit failure attention event")
				}
			}(task, eventType)
			return nil
		})
		if err != nil {
			log.Warn().Err(err).Msg("Failed to subscribe for attention events on failure statuses")
		} else {
			defer func() {
				if err := attentionSub.Unsubscribe(); err != nil {
					log.Warn().Err(err).Msg("Failed to unsubscribe attention event subscription")
				}
			}()
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-o.stopChan:
			return
		case <-ticker.C:
			o.processTasks(ctx)
		case task := <-taskCh:
			err := o.processTask(ctx, task)
			if err != nil {
				log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to process task")
			}
		}
	}
}

// processTasks processes all active tasks
func (o *SpecTaskOrchestrator) processTasks(ctx context.Context) {
	// Periodic cleanup of expired attention events (dismissed > 7 days ago).
	// This runs on every orchestration tick but the query is cheap (indexed).
	if o.attentionService != nil {
		go func() {
			_, err := o.store.CleanupExpiredAttentionEvents(context.Background(), 7*24*time.Hour)
			if err != nil {
				log.Warn().Err(err).Msg("Failed to cleanup expired attention events")
			}
		}()
	}

	// Get all tasks (we'll filter active ones)
	tasks, err := o.store.ListSpecTasks(ctx, &types.SpecTaskFilters{
		WithDependsOn: true, // Validate dependencies before processing
	})
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
		types.TaskStatusSpecApproved:         true,
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

// processTask processes a single task through its workflow.
// NOTE TO LLMS: this function should be fast and non blocking, when handling a task avoid long processes
// as it will block the whole orchestration loop. It first needs to change task status and then
// start the goroutine if needed for long running commands.
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
	case types.TaskStatusSpecApproved:
		return o.handleSpecApproved(ctx, task)
	case types.TaskStatusImplementationQueued:
		return o.handleImplementationQueued(ctx, task)
	case types.TaskStatusImplementation:
		return o.handleImplementation(ctx, task)
	case types.TaskStatusPullRequest:
		return o.handlePullRequest(ctx, task)
	case types.TaskStatusDone:
		return o.handleDone(ctx, task)
	default:
		return nil
	}
}

// handleBacklog handles tasks in backlog state - creates external agent and starts planning
func (o *SpecTaskOrchestrator) handleBacklog(ctx context.Context, task *types.SpecTask) error {
	projectLock, err := o.getBacklogProjectLock(task.ProjectID)
	if err != nil {
		return err
	}
	projectLock.Lock()
	defer projectLock.Unlock()

	latestTask, err := o.store.GetSpecTask(ctx, task.ID)
	if err != nil {
		return fmt.Errorf("failed to get latest task: %w", err)
	}

	if latestTask.Status != types.TaskStatusBacklog {
		return nil
	}

	// Check if project has auto-start enabled
	project, err := o.store.GetProject(ctx, latestTask.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	if !project.AutoStartBacklogTasks {
		// Auto-start is disabled - don't process backlog tasks automatically
		log.Trace().
			Str("task_id", latestTask.ID).
			Str("project_id", latestTask.ProjectID).
			Msg("Skipping backlog task - auto-start is disabled for this project")
		return nil
	}

	allProjectTasks, err := o.store.ListSpecTasks(ctx, &types.SpecTaskFilters{
		ProjectID:     latestTask.ProjectID,
		WithDependsOn: true,
	})
	if err != nil {
		log.Warn().
			Err(err).
			Str("task_id", latestTask.ID).
			Str("project_id", latestTask.ProjectID).
			Msg("Failed to list project tasks for WIP checks")
		return nil
	}

	var latestTaskWithDependencies *types.SpecTask
	for _, projectTask := range allProjectTasks {
		if projectTask.ID == latestTask.ID {
			latestTaskWithDependencies = projectTask
			break
		}
	}
	if latestTaskWithDependencies == nil {
		log.Info().
			Str("task_id", latestTask.ID).
			Str("project_id", latestTask.ProjectID).
			Msg("Skipping backlog task - unable to load task dependencies")
		return nil
	}

	if dependencyReady, blockingDependency := areBacklogDependenciesReady(latestTaskWithDependencies.DependsOn); !dependencyReady {
		log.Info().
			Str("task_id", latestTask.ID).
			Str("project_id", latestTask.ProjectID).
			Str("dependency_task_id", blockingDependency).
			Msg("Skipping backlog task - waiting for dependency task to be done or archived")
		return nil
	}

	planningLimit, implementationLimit := getProjectWIPLimits(project)
	planningCount := countTasksByStatus(allProjectTasks,
		types.TaskStatusQueuedSpecGeneration,
		types.TaskStatusSpecGeneration,
	)
	implementationCount := countTasksByStatus(allProjectTasks,
		types.TaskStatusQueuedImplementation,
		types.TaskStatusImplementationQueued,
		types.TaskStatusImplementation,
	)

	if latestTask.JustDoItMode {
		if implementationCount >= implementationLimit {
			log.Info().
				Str("task_id", latestTask.ID).
				Str("project_id", latestTask.ProjectID).
				Int("implementation_count", implementationCount).
				Int("wip_limit", implementationLimit).
				Msg("Skipping backlog task - implementation column at WIP limit")
			return nil
		}
	} else if planningCount >= planningLimit {
		log.Info().
			Str("task_id", latestTask.ID).
			Str("project_id", latestTask.ProjectID).
			Int("planning_count", planningCount).
			Int("wip_limit", planningLimit).
			Msg("Skipping backlog task - planning column at WIP limit")
		return nil
	}

	log.Info().
		Str("task_id", latestTask.ID).
		Str("helix_app_id", latestTask.HelixAppID).
		Msg("Auto-starting SpecTask planning phase")

	// Delegate to the canonical StartSpecGeneration implementation
	// This ensures both explicit start and auto-start use the same code path
	// Auto-start doesn't have user browser context, so pass empty options
	// o.specTaskService.StartSpecGeneration(ctx, latestTask)
	now := time.Now()
	if latestTask.JustDoItMode {
		latestTask.Status = types.TaskStatusQueuedImplementation
	} else {
		latestTask.Status = types.TaskStatusQueuedSpecGeneration
	}
	latestTask.StatusUpdatedAt = &now

	err = o.store.UpdateSpecTask(ctx, latestTask)
	if err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	return nil
}

func areBacklogDependenciesReady(dependencies []types.SpecTask) (bool, string) {
	for _, dependency := range dependencies {
		if dependency.ID == "" {
			return false, ""
		}
		if dependency.Archived {
			continue
		}
		if dependency.Status != types.TaskStatusDone {
			return false, dependency.ID
		}
	}

	return true, ""
}

func (o *SpecTaskOrchestrator) getBacklogProjectLock(projectID string) (*sync.Mutex, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}

	lock, _ := o.backlogProjectLocks.LoadOrStore(projectID, &sync.Mutex{})
	return lock.(*sync.Mutex), nil
}

func getProjectWIPLimits(project *types.Project) (int, int) {
	planningLimit := 3
	implementationLimit := 5

	if project.Metadata.BoardSettings != nil {
		if project.Metadata.BoardSettings.WIPLimits.Planning > 0 {
			planningLimit = project.Metadata.BoardSettings.WIPLimits.Planning
		}
		if project.Metadata.BoardSettings.WIPLimits.Implementation > 0 {
			implementationLimit = project.Metadata.BoardSettings.WIPLimits.Implementation
		}
	}

	return planningLimit, implementationLimit
}

func countTasksByStatus(tasks []*types.SpecTask, statuses ...types.SpecTaskStatus) int {
	statusMap := make(map[types.SpecTaskStatus]struct{}, len(statuses))
	for _, status := range statuses {
		statusMap[status] = struct{}{}
	}

	count := 0
	for _, task := range tasks {
		if _, ok := statusMap[task.Status]; ok {
			count++
		}
	}

	return count
}

// handleQueuedSpecGeneration handles tasks in queued spec generation
func (o *SpecTaskOrchestrator) handleQueuedSpecGeneration(ctx context.Context, task *types.SpecTask) error {
	dependenciesReady, blockingDependency := areBacklogDependenciesReady(task.DependsOn)
	if !dependenciesReady {
		log.Info().
			Str("task_id", task.ID).
			Str("project_id", task.ProjectID).
			Str("dependency_task_id", blockingDependency).
			Msg("Skipping queued spec generation task - waiting for dependency task to be done or archived")
		return nil
	}

	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		o.specTaskService.StartSpecGeneration(ctx, task)
	}()

	return nil
}

// handleQueuedImplementation handles tasks in queued implementation
func (o *SpecTaskOrchestrator) handleQueuedImplementation(ctx context.Context, task *types.SpecTask) error {
	dependenciesReady, blockingDependency := areBacklogDependenciesReady(task.DependsOn)
	if !dependenciesReady {
		log.Info().
			Str("task_id", task.ID).
			Str("project_id", task.ProjectID).
			Str("dependency_task_id", blockingDependency).
			Msg("Skipping queued implementation task - waiting for dependency task to be done or archived")
		return nil
	}

	// Check if implementation session is complete
	// This would integrate with existing SpecDrivenTaskService
	// For now, we'll assume implementation is ready when all implementation tasks exist
	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		o.specTaskService.StartJustDoItMode(ctx, task)
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

		now := time.Now()
		task.Status = types.TaskStatusSpecReview
		task.StatusUpdatedAt = &now
		task.UpdatedAt = now
		// Ensure DesignDocsPushedAt is set so the "Approve Spec" button appears.
		// For cloned tasks, specs may already exist on the record without being "pushed".
		if task.DesignDocsPushedAt == nil {
			task.DesignDocsPushedAt = &now
		}

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
	now := time.Now()
	task.Status = types.TaskStatusSpecGeneration
	task.StatusUpdatedAt = &now
	task.UpdatedAt = now

	return o.store.UpdateSpecTask(ctx, task)
}

// handleImplementationQueued handles tasks ready for implementation - reuses external agent
// This is a legacy state - new flow (design review approval) bypasses this entirely
func (o *SpecTaskOrchestrator) handleImplementationQueued(ctx context.Context, task *types.SpecTask) error {
	log.Info().
		Str("task_id", task.ID).
		Msg("Task in implementation_queued - moving directly to implementation")

	// Just move to implementation status - agent is already running from planning
	now := time.Now()
	task.Status = types.TaskStatusImplementation
	task.StatusUpdatedAt = &now
	task.UpdatedAt = now

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
	return nil
}

func (o *SpecTaskOrchestrator) handleSpecApproved(ctx context.Context, task *types.SpecTask) error {
	log.Info().
		Str("task_id", task.ID).
		Msg("Processing approved spec")

	// Task is approved, move to implementation
	err := o.specTaskService.ApproveSpecs(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to approve specs: %w", err)
	}
	return nil
}

// handlePullRequest polls external repo for PR merge status
// Called from the dedicated PR polling loop (runs every 1 minute)
func (o *SpecTaskOrchestrator) handlePullRequest(ctx context.Context, task *types.SpecTask) error {
	// Try to create PRs for repos that didn't have the branch ready when the
	// user first clicked "Open PR". This covers the case where the agent pushes
	// to a secondary repo after the initial PR creation.
	if o.ensurePRs != nil {
		project, err := o.store.GetProject(ctx, task.ProjectID)
		if err == nil && project.DefaultRepoID != "" {
			if err := o.ensurePRs(ctx, task, project.DefaultRepoID); err != nil {
				log.Debug().Err(err).Str("task_id", task.ID).Msg("Failed to ensure PRs for all repos (will retry)")
			}
		}
	}

	if !task.HasAnyPR() {
		log.Warn().
			Str("task_id", task.ID).
			Msg("Task in pull_request status but no PRs tracked in RepoPullRequests")
	}

	// Always call processExternalPullRequestStatus even with no tracked PRs —
	// it has a fallback that checks if the branch was merged to main directly
	// (e.g. PR was created and merged on GitHub before we could link it).
	return o.processExternalPullRequestStatus(ctx, task)
}

func (o *SpecTaskOrchestrator) processExternalPullRequestStatus(ctx context.Context, task *types.SpecTask) error {
	// Check each tracked PR across all repos.
	// Only move to done when ALL PRs are merged (stopping the agent prematurely
	// would prevent remaining PRs from getting review fixes pushed).
	// If ALL are closed (without merge), archive.
	anyOpen := false
	allMerged := true
	allClosed := true
	updated := false

	for i, repoPR := range task.RepoPullRequests {
		pr, err := o.gitService.GetPullRequest(ctx, repoPR.RepositoryID, repoPR.PRID)
		if err != nil {
			log.Warn().
				Err(err).
				Str("task_id", task.ID).
				Str("repo_id", repoPR.RepositoryID).
				Str("pr_id", repoPR.PRID).
				Msg("Failed to get pull request status, skipping")
			allClosed = false // Can't confirm it's closed
			continue
		}

		// Update state in RepoPullRequests
		newState := string(pr.State)
		if task.RepoPullRequests[i].PRState != newState {
			task.RepoPullRequests[i].PRState = newState
			updated = true
		}

		switch pr.State {
		case types.PullRequestStateOpen:
			anyOpen = true
			allMerged = false
			allClosed = false
			log.Trace().
				Str("task_id", task.ID).
				Str("repo_id", repoPR.RepositoryID).
				Str("pr_id", repoPR.PRID).
				Msg("PR still active, awaiting merge")
		case types.PullRequestStateMerged:
			allClosed = false
		case types.PullRequestStateClosed:
			allMerged = false
		case types.PullRequestStateUnknown:
			allMerged = false
			allClosed = false
			log.Warn().
				Str("task_id", task.ID).
				Str("repo_id", repoPR.RepositoryID).
				Str("pr_id", repoPR.PRID).
				Msg("PR state unknown, skipping")
		}
	}

	if allMerged && len(task.RepoPullRequests) > 0 {
		// ALL PRs merged - move to done
		now := time.Now()
		task.Status = types.TaskStatusDone
		task.StatusUpdatedAt = &now
		task.MergedToMain = true
		task.MergedAt = &now
		task.CompletedAt = &now
		task.UpdatedAt = now
		log.Info().
			Str("task_id", task.ID).
			Msg("PR merged! Moving task to done")

		// Trigger golden Docker cache build if enabled for this project
		if o.goldenBuildService != nil && task.ProjectID != "" {
			project, err := o.store.GetProject(ctx, task.ProjectID)
			if err == nil && project != nil {
				o.goldenBuildService.TriggerGoldenBuild(ctx, project)
			}
		}

		return o.store.UpdateSpecTask(ctx, task)
	}

	if allClosed && !anyOpen && len(task.RepoPullRequests) > 0 {
		// All PRs closed — log but don't auto-archive. Let the user decide.
		log.Info().
			Str("task_id", task.ID).
			Msg("All PRs closed, task remains in pull_request status")
	}

	// Persist any state updates
	if updated {
		task.UpdatedAt = time.Now()
		return o.store.UpdateSpecTask(ctx, task)
	}

	// If no PRs are tracked (or all PRs are closed), check if the branch has
	// been merged to main directly. This handles cases where the PR was created
	// and merged on GitHub before we could link it, or where the branch was
	// identical to main (no commits between them).
	if !anyOpen && task.BranchName != "" {
		project, err := o.store.GetProject(ctx, task.ProjectID)
		if err != nil {
			log.Debug().Err(err).Str("task_id", task.ID).Msg("Failed to get project for branch-merge check")
			return nil
		}
		if project.DefaultRepoID == "" {
			return nil
		}
		repo, err := o.store.GetGitRepository(ctx, project.DefaultRepoID)
		if err != nil {
			log.Debug().Err(err).Str("task_id", task.ID).Msg("Failed to get repo for branch-merge check")
			return nil
		}

		merged, mergeErr := o.gitService.IsBranchMerged(ctx, project.DefaultRepoID, task.BranchName, repo.DefaultBranch)
		if mergeErr != nil {
			if task.LastPushCommitHash != "" {
				merged, mergeErr = o.gitService.IsCommitInBranch(ctx, project.DefaultRepoID, task.LastPushCommitHash, repo.DefaultBranch)
				if mergeErr != nil {
					log.Debug().Err(mergeErr).Str("task_id", task.ID).Msg("Failed to check if commit is in main")
					return nil
				}
			} else {
				log.Debug().Err(mergeErr).Str("task_id", task.ID).Str("branch", task.BranchName).Msg("Failed to check if branch is merged")
				return nil
			}
		}

		if merged {
			log.Info().Str("task_id", task.ID).Str("branch", task.BranchName).Msg("Detected merged branch, moving task to done")
			now := time.Now()
			task.Status = types.TaskStatusDone
			task.MergedToMain = true
			task.MergedAt = &now
			task.CompletedAt = &now
			task.UpdatedAt = now
			return o.store.UpdateSpecTask(ctx, task)
		}
	}

	return nil
}

// prPollLoop polls external repos for PR merge status every minute
func (o *SpecTaskOrchestrator) prPollLoop(ctx context.Context) {
	defer o.wg.Done()

	// Use configured interval or default to 1 minute
	interval := o.prPollInterval
	if interval == 0 {
		interval = 30 * time.Second
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
			o.detectExternalPRActivity(ctx)
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

// detectExternalPRActivity checks tasks in spec_review or implementation status for:
// 1. Externally-opened PRs that should move the task to pull_request status
// 2. Branches that have been merged to main, moving the task to done status
// This handles cases where PRs are created/merged outside the normal Helix workflow.
func (o *SpecTaskOrchestrator) detectExternalPRActivity(ctx context.Context) {
	// Get tasks in spec_review or implementation status that have a branch but no PR tracked
	statuses := []types.SpecTaskStatus{
		types.TaskStatusSpecReview,
		types.TaskStatusImplementation,
	}

	var tasksToCheck []*types.SpecTask
	for _, status := range statuses {
		tasks, err := o.store.ListSpecTasks(ctx, &types.SpecTaskFilters{
			Status: status,
		})
		if err != nil {
			log.Error().Err(err).Str("status", string(status)).Msg("Failed to list tasks for external PR detection")
			continue
		}
		tasksToCheck = append(tasksToCheck, tasks...)
	}

	// Filter to tasks with a branch but no PRs tracked
	var eligibleTasks []*types.SpecTask
	for _, task := range tasksToCheck {
		if task.BranchName != "" && !task.HasAnyPR() {
			eligibleTasks = append(eligibleTasks, task)
		}
	}

	if len(eligibleTasks) == 0 {
		return
	}

	// Rate limit: process max 10 tasks per poll cycle
	maxTasks := 10
	if len(eligibleTasks) > maxTasks {
		log.Debug().Int("total", len(eligibleTasks)).Int("processing", maxTasks).Msg("Rate limiting external PR detection")
		eligibleTasks = eligibleTasks[:maxTasks]
	}

	log.Debug().Int("count", len(eligibleTasks)).Msg("Checking tasks for external PR activity")

	for _, task := range eligibleTasks {
		err := o.checkTaskForExternalPRActivity(ctx, task)
		if err != nil {
			// Don't spam logs for deleted projects
			if strings.Contains(err.Error(), "record not found") || strings.Contains(err.Error(), "not found") {
				log.Trace().
					Err(err).
					Str("task_id", task.ID).
					Msg("Task references deleted project - skipping external PR check")
			} else {
				log.Warn().
					Err(err).
					Str("task_id", task.ID).
					Msg("Failed to check task for external PR activity")
			}
		}
	}
}

// checkTaskForExternalPRActivity checks a single task for external PR activity
func (o *SpecTaskOrchestrator) checkTaskForExternalPRActivity(ctx context.Context, task *types.SpecTask) error {
	project, err := o.store.GetProject(ctx, task.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	// Check ALL project repos for PR activity, not just the default.
	// Tasks may only have PRs in secondary repos (e.g., Zed repo).
	allRepos, err := o.store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{
		ProjectID: project.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to list project repositories: %w", err)
	}

	branchRef := "refs/heads/" + task.BranchName

	for _, repo := range allRepos {
		if !repo.IsExternal || repo.ExternalURL == "" {
			continue
		}

		// Check for open or merged PRs on this branch
		prs, err := o.gitService.ListPullRequests(ctx, repo.ID)
		if err != nil {
			log.Debug().Err(err).Str("repo_id", repo.ID).Msg("Failed to list PRs, skipping repo")
			continue
		}

		for _, pr := range prs {
			branchMatches := pr.SourceBranch == branchRef || pr.SourceBranch == task.BranchName

			// Check if PR is open and matches our branch
			if pr.State == types.PullRequestStateOpen && branchMatches {
				log.Info().
					Str("task_id", task.ID).
					Str("pr_id", pr.ID).
					Str("pr_title", pr.Title).
					Str("branch", task.BranchName).
					Str("repo_name", repo.Name).
					Msg("Detected externally-opened PR, moving task to pull_request status")

				task.RepoPullRequests = append(task.RepoPullRequests, types.RepoPR{
					RepositoryID:   repo.ID,
					RepositoryName: repo.Name,
					PRID:           pr.ID,
					PRNumber:       pr.Number,
					PRURL:          pr.URL,
					PRState:        string(pr.State),
				})
				task.Status = types.TaskStatusPullRequest
				task.UpdatedAt = time.Now()
				if err := o.store.UpdateSpecTask(ctx, task); err != nil {
					return err
				}

				if o.attentionService != nil {
					go func(t *types.SpecTask, prID string, prURL string) {
						_, emitErr := o.attentionService.EmitEvent(
							context.Background(),
							types.AttentionEventPRReady,
							t,
							prID,
							map[string]interface{}{
								"pr_id":  prID,
								"pr_url": prURL,
							},
						)
						if emitErr != nil {
							log.Warn().Err(emitErr).
								Str("spec_task_id", t.ID).
								Msg("Failed to emit pr_ready attention event")
						}
					}(task, pr.ID, pr.URL)
				}
				return nil
			}

			// Check if PR is already merged
			if pr.State == types.PullRequestStateMerged && branchMatches {
				log.Info().
					Str("task_id", task.ID).
					Str("pr_id", pr.ID).
					Str("branch", task.BranchName).
					Str("repo_name", repo.Name).
					Msg("Detected merged PR, moving task to done status")

				now := time.Now()
				task.RepoPullRequests = append(task.RepoPullRequests, types.RepoPR{
					RepositoryID:   repo.ID,
					RepositoryName: repo.Name,
					PRID:           pr.ID,
					PRNumber:       pr.Number,
					PRURL:          pr.URL,
					PRState:        string(pr.State),
				})
				task.Status = types.TaskStatusDone
				task.MergedToMain = true
				task.MergedAt = &now
				task.CompletedAt = &now
				task.UpdatedAt = now
				return o.store.UpdateSpecTask(ctx, task)
			}
		}
	}

	// Fallback: check if branch has been merged to main in the primary repo
	// (handles squash-merges or branch deletion after merge)
	if project.DefaultRepoID == "" {
		return nil
	}
	repo, err := o.gitService.GetRepository(ctx, project.DefaultRepoID)
	if err != nil {
		return fmt.Errorf("failed to get default repository: %w", err)
	}
	if !repo.IsExternal {
		return nil
	}

	merged, err := o.gitService.IsBranchMerged(ctx, project.DefaultRepoID, task.BranchName, repo.DefaultBranch)
	if err != nil {
		if task.LastPushCommitHash != "" {
			merged, err = o.gitService.IsCommitInBranch(ctx, project.DefaultRepoID, task.LastPushCommitHash, repo.DefaultBranch)
			if err != nil {
				log.Debug().Err(err).Str("task_id", task.ID).Msg("Failed to check if commit is in main branch")
				return nil
			}
		} else {
			return nil
		}
	}

	if merged {
		log.Info().
			Str("task_id", task.ID).
			Str("branch", task.BranchName).
			Msg("Detected merged branch (no PR found), moving task to done status")

		now := time.Now()
		task.Status = types.TaskStatusDone
		task.MergedToMain = true
		task.MergedAt = &now
		task.CompletedAt = &now
		task.UpdatedAt = now
		return o.store.UpdateSpecTask(ctx, task)
	}

	return nil
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

func (o *SpecTaskOrchestrator) handleDone(ctx context.Context, task *types.SpecTask) error {
	// Terminate the desktop
	err := o.containerExecutor.StopDesktop(ctx, task.PlanningSessionID)
	if err != nil {
		return fmt.Errorf("failed to stop desktop: %w", err)
	}

	log.Info().
		Str("task_id", task.ID).
		Msg("Task in done status - stopping desktop")

	return nil
}
