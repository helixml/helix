package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
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

	var planningLimit int = 3

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
	// CRITICAL: Use /filestore/ prefix (not /opt/helix/filestore/) so translateToHostPath works
	// wolf_executor.go:translateToHostPath expects paths starting with /filestore/
	// Also use "spec-tasks" (with hyphen) for consistency with wolf_executor.go
	workspaceDir := fmt.Sprintf("/filestore/workspaces/spec-tasks/%s", task.ID)

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
		ProjectPath:         "backend",          // Default primary repo path
		RepositoryIDs:       repositoryIDs,      // Repositories to clone
		PrimaryRepositoryID: primaryRepoID,      // Primary repository for design docs
		SpecTaskID:          task.ID,            // Link to SpecTask
		UseHostDocker:       task.UseHostDocker, // Use host Docker socket if requested
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
			repoInstructions += fmt.Sprintf("- `%s` at `~/work/%s`\n", repo.Name, repo.Name)
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

// getOrCreateUserAPIKey gets user's existing personal API key for git operations
// IMPORTANT: Only uses personal API keys (not app-scoped keys) to ensure full access
func (o *SpecTaskOrchestrator) getOrCreateUserAPIKey(ctx context.Context, userID string) (string, error) {
	// List user's existing API keys
	keys, err := o.store.ListAPIKeys(ctx, &store.ListAPIKeysQuery{
		Owner:     userID,
		OwnerType: types.OwnerTypeUser,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list user API keys: %w", err)
	}

	// Filter out app-scoped API keys - we need a personal API key for full access
	// App-scoped keys have restricted path access and can't be used for RevDial
	var personalKeys []*types.ApiKey
	for _, key := range keys {
		if key.AppID == nil || !key.AppID.Valid || key.AppID.String == "" {
			personalKeys = append(personalKeys, key)
		}
	}

	// Use user's first personal API key if available
	if len(personalKeys) > 0 {
		log.Debug().
			Str("user_id", userID).
			Str("api_key_name", personalKeys[0].Name).
			Bool("key_is_personal", true).
			Msg("Using user's existing personal API key for git operations")
		return personalKeys[0].Key, nil
	}

	// No personal API keys exist - create one automatically
	log.Info().Str("user_id", userID).Msg("No personal API keys found, creating one for agent access")

	newKey, err := system.GenerateAPIKey()
	if err != nil {
		return "", fmt.Errorf("failed to generate API key: %w", err)
	}

	apiKey := &types.ApiKey{
		Owner:     userID,
		OwnerType: types.OwnerTypeUser,
		Key:       newKey,
		Name:      "Auto-generated for agent access",
		Type:      types.APIkeytypeAPI,
		// AppID is nil - this is a personal key
	}

	createdKey, err := o.store.CreateAPIKey(ctx, apiKey)
	if err != nil {
		return "", fmt.Errorf("failed to create API key: %w", err)
	}

	log.Info().
		Str("user_id", userID).
		Str("key_name", createdKey.Name).
		Msg("✅ Created personal API key for agent access")

	return createdKey.Key, nil
}
