package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// Spec-driven development: Specs worktree paths (relative to repository root)
const (
	SpecsWorktreeRelPath = "design"                // Relative path from repo root
	SpecsBranchName      = "helix-specs"           // Git branch name for spec-driven development
	SpecsTaskDirFormat   = "design/tasks/%s_%s_%s" // Format: tasks/DATE_NAME_ID
)

// RequestMappingRegistrar is a function type for registering request-to-session mappings
type RequestMappingRegistrar func(requestID, sessionID string)

// DesktopExecFunc executes a command inside a running desktop container via RevDial.
type DesktopExecFunc func(ctx context.Context, sessionID string, command []string) error

// AttachmentBlobReader reads the bytes of a SpecTask attachment from the filestore.
// Injected by the server so this service doesn't need to import the controller package.
type AttachmentBlobReader func(ctx context.Context, absolutePath string) ([]byte, error)

// SpecDrivenTaskService manages the spec-driven development workflow:
// Specification: Helix agent generates specs from simple descriptions
// Implementation: Zed agent implements code from approved specs
type SpecDrivenTaskService struct {
	store    store.Store
	notifier notification.Notifier
	// controller               *controller.Controller
	externalAgentExecutor    external_agent.Executor   // Wolf executor for launching external agents
	gitRepositoryService     *GitRepositoryService     // Service for git repository operations
	RegisterRequestMapping   RequestMappingRegistrar   // Callback to register request-to-session mappings
	EnqueueMessageToAgent    SpecTaskMessageEnqueuer   // Callback to enqueue messages onto the session-scoped prompt queue (the single sender path)
	helixAgentID             string                    // ID of Helix agent for spec generation
	zedAgentPool             []string                  // Pool of available Zed agents
	testMode                 bool                      // If true, skip async operations for testing
	ZedIntegrationService    *ZedIntegrationService    // Service for Zed instance and thread management
	ZedToHelixSessionService *ZedToHelixSessionService // Service for Zed→Helix session creation
	SessionContextService    *SessionContextService    // Service for inter-session coordination
	auditLogService          *AuditLogService          // Service for audit logging
	koditService             KoditServicer             // Kodit code intelligence (for MCP documentation in prompts)
	ExecInDesktop            DesktopExecFunc           // Callback to exec commands in running desktop containers
	ReadAttachmentBlob       AttachmentBlobReader      // Callback to load attachment bytes from filestore
	wg                       sync.WaitGroup
}

// NewSpecDrivenTaskService creates a new service instance
func NewSpecDrivenTaskService(
	store store.Store,
	notifier notification.Notifier,
	helixAgentID string,
	zedAgentPool []string,
	pubsub pubsub.PubSub,
	externalAgentExecutor external_agent.Executor,
	gitRepositoryService *GitRepositoryService,
	registerRequestMapping RequestMappingRegistrar,
	koditService KoditServicer,
) *SpecDrivenTaskService {
	service := &SpecDrivenTaskService{
		store:                  store,
		externalAgentExecutor:  externalAgentExecutor,
		gitRepositoryService:   gitRepositoryService,
		RegisterRequestMapping: registerRequestMapping,
		helixAgentID:           helixAgentID,
		zedAgentPool:           zedAgentPool,
		koditService:           koditService,
		testMode:               false,
		auditLogService:        NewAuditLogService(store),
	}

	// Initialize Zed integration service
	service.ZedIntegrationService = NewZedIntegrationService(
		store,
		pubsub,
	)

	service.SessionContextService = NewSessionContextService(store)

	// Initialize Zed-to-Helix session service
	service.ZedToHelixSessionService = NewZedToHelixSessionService(
		store,
		// service.MultiSessionManager,
		service.SessionContextService,
	)

	return service
}

// SetKoditService replaces the kodit service after late initialization.
func (s *SpecDrivenTaskService) SetKoditService(svc KoditServicer) {
	s.koditService = svc
}

// SetTestMode enables or disables test mode (prevents async operations)
func (s *SpecDrivenTaskService) SetTestMode(enabled bool) {
	s.testMode = enabled

	if s.ZedIntegrationService != nil {
		s.ZedIntegrationService.SetTestMode(enabled)
	}
	if s.ZedToHelixSessionService != nil {
		s.ZedToHelixSessionService.SetTestMode(enabled)
	}
	if s.SessionContextService != nil {
		s.SessionContextService.SetTestMode(enabled)
	}
	if s.auditLogService != nil {
		s.auditLogService.SetTestMode(enabled)
	}
}

// SetAuditLogWaitGroup sets a WaitGroup for tracking async audit log operations (used in tests)
func (s *SpecDrivenTaskService) SetAuditLogWaitGroup(wg *sync.WaitGroup) {
	if s.auditLogService != nil {
		s.auditLogService.SetWaitGroup(wg)
	}
}

// CreateTaskFromPrompt creates a new task in the backlog and kicks off spec generation
func (s *SpecDrivenTaskService) CreateTaskFromPrompt(ctx context.Context, req *types.CreateTaskRequest) (*types.SpecTask, error) {
	if req.AppID != "" {
		return nil, fmt.Errorf("app_id is no longer supported; provide code_agent_config")
	}
	if req.CodeAgentOverrides != nil {
		return nil, fmt.Errorf("code_agent_overrides is no longer supported; provide code_agent_config")
	}
	if req.SandboxResourceOverrides != nil && !req.SandboxResourceOverrides.ValidPreset() {
		return nil, fmt.Errorf("invalid sandbox resource preset")
	}
	if !types.ValidSpecTaskSandboxRuntime(req.SandboxRuntime) {
		return nil, fmt.Errorf("invalid spec task sandbox runtime %q", req.SandboxRuntime)
	}
	// Fetch project to get organization ID and task defaults.
	var project *types.Project
	if req.ProjectID != "" {
		var err error
		project, err = s.store.GetProject(ctx, req.ProjectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get project: %w", err)
		}
	}
	// Deliberately left nil when neither the request nor the project chose a size:
	// a stored override means "someone chose this", not "this was the default the
	// day the row was written". Materializing it here froze every task created
	// after 1eff4e801 at 4 vCPU / 8 GB, so a raised default could never reach
	// them. The default is resolved at container-create time instead — see
	// HydraExecutor.resolveSpecTaskLaunchConfig.
	sandboxResources := req.SandboxResourceOverrides
	if sandboxResources == nil && project != nil && project.DefaultSandboxResourceOverrides != nil {
		projectResources := *project.DefaultSandboxResourceOverrides
		sandboxResources = &projectResources
	}
	sandboxRuntime := req.SandboxRuntime
	if sandboxRuntime == "" && project != nil {
		sandboxRuntime = project.DefaultSandboxRuntime
	}
	sandboxRuntime = types.EffectiveSpecTaskSandboxRuntime(sandboxRuntime)

	codeAgentConfig := cloneCodeAgentExecutionConfig(req.CodeAgentConfig)
	if codeAgentConfig == nil && project != nil {
		codeAgentConfig = cloneCodeAgentExecutionConfig(project.CodeAgentConfig)
	}
	// A legacy project is allowed to defer materialization until task start.
	// This is the only remaining read path for DefaultHelixAppID.
	if codeAgentConfig == nil && (project == nil || project.DefaultHelixAppID == "") {
		return nil, fmt.Errorf("project has no code-agent configuration")
	}

	// Default branch mode to "new" if not specified
	branchMode := req.BranchMode
	if branchMode == "" {
		branchMode = types.BranchModeNew
	}
	if branchMode == types.BranchModeExisting && strings.TrimSpace(req.WorkingBranch) == "" {
		return nil, fmt.Errorf("working branch is required for existing branch mode")
	}

	// An explicit name wins; otherwise derive one from the prompt.
	taskName := strings.TrimSpace(req.Name)
	if taskName == "" {
		taskName = GenerateTaskNameFromPrompt(req.Prompt)
	}

	// Determine organization ID from project if it belongs to an org
	organizationID := ""
	if project != nil {
		organizationID = project.OrganizationID
	}
	credentialOwnerID := req.CredentialOwnerID
	if credentialOwnerID == "" && organizationID != "" && codeAgentConfig != nil &&
		codeAgentConfig.Runtime == types.CodeAgentRuntimeClaudeCode && codeAgentConfig.CredentialType.IsSubscription() {
		delegated, err := s.store.GetDelegatedClaudeSubscriptionForOrg(ctx, organizationID)
		if err == nil {
			credentialOwnerID = delegated.OwnerID
		} else if !errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("resolve organization Claude subscription: %w", err)
		}
	}

	// Determine initial status. If AutoStart is requested, skip backlog and queue
	// immediately (mirrors cloneTaskToProject behaviour). This bypasses the project's
	// auto_start_backlog_tasks setting so the task starts even when project auto-start is off.
	initialStatus := types.TaskStatusBacklog
	assigneeID := req.AssigneeID
	planningStartedBy := ""
	if req.AutoStart {
		if req.JustDoItMode {
			initialStatus = types.TaskStatusQueuedImplementation
		} else {
			initialStatus = types.TaskStatusQueuedSpecGeneration
		}
		assigneeID = req.UserID
		planningStartedBy = req.UserID
	}

	task := &types.SpecTask{
		ID:                       generateTaskID(),
		ProjectID:                req.ProjectID,
		UserID:                   req.UserID,
		OrganizationID:           organizationID,
		AssigneeID:               assigneeID, // Auto-started work is always assigned to the user who started it
		Name:                     taskName,
		Description:              req.Prompt,
		Type:                     req.Type,
		Priority:                 req.Priority,
		Status:                   initialStatus,
		OriginalPrompt:           req.Prompt,
		CreatedBy:                req.UserID,
		PlanningStartedBy:        planningStartedBy,
		CodeAgentConfig:          codeAgentConfig,
		SandboxResourceOverrides: sandboxResources,
		SandboxRuntime:           sandboxRuntime,
		JustDoItMode:             req.JustDoItMode, // Set Just Do It mode from request
		// Credential-only override: whose Claude subscription authenticates this
		// task's agent. Enforced at resolution time against the named user's
		// delegation grant; missing or revoked grants fail closed.
		CredentialOwnerID: credentialOwnerID,
		// Branch configuration
		BranchMode:   branchMode,
		BaseBranch:   req.BaseBranch,    // User-specified base branch (empty = use repo default)
		BranchPrefix: req.BranchPrefix,  // User-specified prefix for new branches
		BranchName:   req.WorkingBranch, // For existing mode, this is the branch to continue on
		// Goose recipe selection — bakes parameter values into the agent's
		// recipe at session start. Skipped silently if the agent isn't goose.
		GooseRecipeName:   req.GooseRecipeName,
		GooseRecipeParams: req.GooseRecipeParams,
		// Repositories inherited from parent project - no task-level repo configuration
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if req.DependsOn != nil {
		task.DependsOn = make([]types.SpecTask, 0, len(req.DependsOn))
		for _, dependsOnID := range req.DependsOn {
			if dependsOnID == "" {
				continue
			}
			task.DependsOn = append(task.DependsOn, types.SpecTask{ID: dependsOnID})
		}
	}

	// Assign task number immediately at creation time so it's always visible in UI
	// Task numbers are globally unique across the entire deployment
	taskNumber, err := s.store.IncrementGlobalTaskNumber(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get global task number, using fallback")
		taskNumber = 1
	}
	task.TaskNumber = taskNumber
	// Generate design doc path based on task name and number
	task.DesignDocPath = GenerateDesignDocPath(task, taskNumber)
	log.Info().
		Str("task_id", task.ID).
		Int("task_number", taskNumber).
		Str("design_doc_path", task.DesignDocPath).
		Msg("Assigned task number and design doc path at creation")

	// Store the task
	err = s.store.CreateSpecTask(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	// PR DETECTION: Check if the existing branch has an open PR
	// If so, update the task to start in pull_request status
	if branchMode == types.BranchModeExisting && req.WorkingBranch != "" && s.gitRepositoryService != nil {
		prDetected := s.detectAndLinkExistingPR(ctx, task, req.ProjectID, req.WorkingBranch)
		if prDetected {
			log.Info().
				Str("task_id", task.ID).
				Str("branch", req.WorkingBranch).
				Bool("has_pr", task.HasAnyPR()).
				Msg("Detected existing PR for branch, task starts in pull_request column")
		}
	}

	// Log audit event for task creation
	if s.auditLogService != nil {
		s.auditLogService.LogTaskCreated(ctx, task, req.UserID, req.UserEmail)
	}

	// DO NOT auto-start spec generation
	// Tasks should start in backlog and wait for explicit user action to start planning
	// This allows WIP limits to be enforced on the planning column

	return task, nil
}

// StartSpecGeneration kicks off spec generation with a Helix agent
// This is now a public method that can be called explicitly to start planning
// opts contains optional settings like keyboard layout from browser locale detection
func (s *SpecDrivenTaskService) StartSpecGeneration(ctx context.Context, task *types.SpecTask) {
	// Add panic recovery for debugging
	defer func() {
		if r := recover(); r != nil {
			log.Error().Interface("panic", r).Str("task_id", task.ID).Msg("PANIC in StartSpecGeneration")
		}
	}()

	log.Debug().Str("task_id", task.ID).Msg("StartSpecGeneration entered")

	// Get project first - needed for agent inheritance and guidelines
	var project *types.Project
	orgID := ""
	guidelines := ""
	if task.ProjectID != "" {
		var err error
		project, err = s.store.GetProject(ctx, task.ProjectID)
		if err != nil {
			log.Warn().Err(err).Str("project_id", task.ProjectID).Msg("Failed to get project")
		} else if project != nil {
			orgID = project.OrganizationID
			// Get organization guidelines
			if orgID != "" {
				org, orgErr := s.store.GetOrganization(ctx, &store.GetOrganizationQuery{ID: orgID})
				if orgErr == nil && org != nil && org.Guidelines != "" {
					guidelines = org.Guidelines
				}
			}
			// Append project guidelines
			if project.Guidelines != "" {
				if guidelines != "" {
					guidelines += "\n\n---\n\n"
				}
				guidelines += project.Guidelines
			}
		}
	}

	if err := s.migrateSpecTaskCodeAgentConfig(ctx, task, project); err != nil {
		s.markTaskFailed(ctx, task, fmt.Sprintf("failed to prepare code-agent configuration: %v", err))
		return
	}

	log.Info().
		Str("task_id", task.ID).
		Str("original_prompt", task.OriginalPrompt).
		Str("code_agent_runtime", string(task.CodeAgentConfig.Runtime)).
		Msg("Starting spec generation")

	// Note: Task number and design doc path are now assigned at creation time
	// in CreateTaskFromPrompt, so they should always be set by this point.

	// For cloned tasks, pre-populate the design docs in helix-specs so the agent can read them
	if task.ClonedFromID != "" && project != nil && project.DefaultRepoID != "" {
		if err := s.prepopulateClonedSpecs(ctx, task, project); err != nil {
			log.Warn().Err(err).Str("task_id", task.ID).Msg("Failed to pre-populate cloned specs (agent will need to create from scratch)")
		}
	}

	// Clear any previous error from metadata (in case this is a retry)
	if task.Metadata != nil {
		delete(task.Metadata, "error")
		delete(task.Metadata, "error_timestamp")
	}

	// Update task status (SpecAgent already set in CreateTaskFromPrompt)
	now := time.Now()
	task.Status = types.TaskStatusSpecGeneration
	task.StatusUpdatedAt = &now
	task.PlanningStartedAt = &now
	task.UpdatedAt = now

	err := s.store.UpdateSpecTask(ctx, task)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to update task status")
		return
	}

	// Build planning instructions as the message (not system prompt - agent has its own system prompt)
	koditDoc := ""
	if project != nil && project.KoditEnabled {
		koditDoc = s.koditService.MCPDocumentation()
	}

	// Build repository section listing local + Kodit repos for the agent
	repoSection := s.buildRepositorySectionForTask(ctx, task, project)

	// Stage user-uploaded attachments into the helix-specs branch so they appear in
	// the agent's workspace, and build the prompt section that points to them.
	attachmentsSection, attachErr := s.stageAttachmentsAndBuildPromptSection(ctx, task, project)
	if attachErr != nil {
		log.Warn().Err(attachErr).Str("task_id", task.ID).Msg("Failed to stage attachments — continuing without them")
	}
	agentToolsSection := BuildAgentToolsSection(project.AgentTools, task.AgentTools)
	planningPrompt := BuildPlanningPrompt(task, guidelines, koditDoc, repoSection, attachmentsSection, agentToolsSection)

	// Get CodeAgentRuntime from the app config (needed for session resume to select correct agent)
	codeAgentRuntime := codeAgentRuntimeForSpecTask(task)

	sessionMetadata := types.SessionMetadata{
		SystemPrompt:     "",             // Don't override agent's system prompt
		AgentType:        "zed_external", // Use Zed agent for git access
		Stream:           false,
		SpecTaskID:       task.ID,          // CRITICAL: Set SpecTaskID so session restore uses correct workspace path
		CodeAgentRuntime: codeAgentRuntime, // For open_thread on resume
		// Whose Claude subscription authenticates this session's agent, when the
		// dispatcher is a service account acting for a human. Credential-only —
		// the session is still owned by task.CreatedBy.
		CredentialOwnerID: task.CredentialOwnerID,
		// Autonomous surface: no human watches a planning run, so recover the
		// agent automatically on crash rather than stalling errored+idle.
		AutoRestartOnCrash: true,
	}

	session := &types.Session{
		ID:             system.GenerateSessionID(),
		Name:           fmt.Sprintf("Spec Generation: %s", task.Name),
		Created:        time.Now(),
		Updated:        time.Now(),
		Mode:           types.SessionModeInference,
		Type:           types.SessionTypeText,
		Provider:       "anthropic",      // Use Claude for spec generation
		ModelName:      "external_agent", // Model name for external agents
		Owner:          task.CreatedBy,
		ParentApp:      "",
		OrganizationID: orgID,
		ProjectID:      task.ProjectID, // For project-level skills
		Metadata:       sessionMetadata,
		OwnerType:      types.OwnerTypeUser,
	}

	// Create the session first so we have a real session ID to claim with. If
	// we lose the atomic claim below, we delete this orphan and return — the
	// winning caller's session is the one that ends up driving the desktop.
	//
	// This is more wasteful than a pre-claim (we burn a session row when we
	// lose) but avoids the rollback footgun: if we claimed first and
	// CreateSession failed, planning_session_id would point at a non-existent
	// session and future retries would silently noop on the read-then-write
	// guard. With this ordering, the claim is the single source of truth and
	// no retry path can be left in a half-claimed state.
	session, err = s.store.CreateSession(ctx, *session)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to create spec generation session")
		s.markTaskFailed(ctx, task, fmt.Sprintf("Failed to create spec generation session: %v", err))
		return
	}

	// Atomically claim the planning_session_id slot. Two concurrent callers
	// (orchestrator ticker + status-change subscription firing on the same
	// task within milliseconds) each get to here with their own session, but
	// only one wins this single-statement UPDATE. The loser deletes its
	// orphan session and returns BEFORE reaching StartDesktop — preventing
	// the workspace-volume race that corrupts the spec-task's git clone.
	// Replaces the read-then-write TOCTOU guard that issue #10 of the
	// 2026-03-18 ZFS deployment design doc attempted to close.
	claimed, err := s.store.SetPlanningSessionIDIfEmpty(ctx, task.ID, session.ID)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Str("session_id", session.ID).Msg("Failed to claim planning_session_id; rolling back session")
		if _, delErr := s.store.DeleteSession(ctx, session.ID); delErr != nil {
			log.Warn().Err(delErr).Str("session_id", session.ID).Msg("Failed to delete orphan session after claim error")
		}
		s.markTaskFailed(ctx, task, fmt.Sprintf("Failed to claim planning_session_id: %v", err))
		return
	}
	if !claimed {
		log.Info().
			Str("task_id", task.ID).
			Str("orphan_session_id", session.ID).
			Msg("Lost race to claim planning_session_id; deleting orphan session and bailing before StartDesktop")
		if _, delErr := s.store.DeleteSession(ctx, session.ID); delErr != nil {
			log.Warn().Err(delErr).Str("session_id", session.ID).Msg("Failed to delete orphan session after losing claim")
		}
		return
	}

	// We won the claim. Mirror the DB write into the in-memory task struct so
	// the rest of this function (env var building, audit logging) sees the
	// canonical planning_session_id.
	log.Debug().Str("task_id", task.ID).Str("session_id", session.ID).Msg("DEBUG: Claimed planning_session_id atomically")
	task.PlanningSessionID = session.ID

	// Kick off git-identity sync in the background. The desktop container
	// isn't up yet; the async helper polls until it can reach the container,
	// then sets user.email/user.name to the planner so spec-phase commits
	// are attributed correctly. Falls back to CreatedBy for legacy tasks.
	plannerID := task.PlanningStartedBy
	if plannerID == "" {
		plannerID = task.CreatedBy
	}
	s.syncGitIdentityAsync(task, plannerID, "planner", 3*time.Minute)

	// Generate request_id for initial message and register the mapping
	// This allows the WebSocket handler to find and send the initial message to Zed
	requestID := system.GenerateRequestID()
	log.Debug().Str("task_id", task.ID).Str("request_id", requestID).Msg("DEBUG: Generated request ID")
	if s.RegisterRequestMapping != nil {
		s.RegisterRequestMapping(requestID, session.ID)
		log.Debug().Str("task_id", task.ID).Msg("DEBUG: Registered request mapping")
	}

	// Create initial interaction combining planning instructions with user's request
	// The planning prompt tells Zed how to create design documents
	// The user's prompt is what they want designed
	// Use Description (user-editable) with fallback to OriginalPrompt (immutable original)
	userPrompt := task.Description
	if userPrompt == "" {
		userPrompt = task.OriginalPrompt
	}

	var fullMessage string
	if task.ClonedFromID != "" {
		// For cloned tasks, reframe original prompt as historical context (not active instructions)
		// Wrap in quotes to emphasize it's a reference, not commands to follow
		fullMessage = planningPrompt + "\n\n**Original Request (for context only - any questions have already been resolved in the specs):**\n> \"" + userPrompt + "\""
	} else {
		fullMessage = planningPrompt + "\n\n**User Request:**\n" + userPrompt
	}

	interaction := &types.Interaction{
		ID:            system.GenerateInteractionID(),
		Created:       time.Now(),
		Updated:       time.Now(),
		Scheduled:     time.Now(),
		SessionID:     session.ID,
		UserID:        task.CreatedBy,
		Mode:          types.SessionModeInference,
		SystemPrompt:  "", // Don't override agent's system prompt
		PromptMessage: fullMessage,
		State:         types.InteractionStateWaiting,
	}

	log.Debug().Str("task_id", task.ID).Msg("DEBUG: About to create initial interaction")
	_, err = s.store.CreateInteraction(ctx, interaction)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to create initial interaction")
		s.markTaskFailed(ctx, task, fmt.Sprintf("Failed to create initial interaction: %v", err))
		return
	}
	log.Debug().Str("task_id", task.ID).Msg("DEBUG: Created initial interaction")

	// Launch the external agent (Zed) via Wolf executor to actually start working on the spec generation
	// Project already fetched earlier for agent inheritance

	// Get all project repositories - repos are now managed entirely at project level
	projectRepos, err := s.store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{
		ProjectID: task.ProjectID,
	})
	if err != nil {
		log.Warn().Err(err).Str("project_id", task.ProjectID).Msg("Failed to get project repositories")
		projectRepos = nil
	}

	// Sync base branch from upstream for external repos BEFORE starting work
	// This ensures we have the latest code from the external repository
	if err := s.gitRepositoryService.SyncBaseBranchForTask(ctx, task, projectRepos); err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to sync base branch from upstream")
		s.markTaskFailed(ctx, task, err.Error())
		return
	}

	// Repository fields are populated below via zedAgent.SetRepoContext after
	// the agent struct is constructed.

	// API key creation is deferred to OnBeforeCreate hook (inside StartDesktop's
	// session lock) to prevent races with concurrent StopDesktop.
	launchOrgID := orgID
	launchTask := task
	launchSession := session

	// Display settings are sandbox concerns. Legacy App display preferences are
	// intentionally not part of the task execution-config migration.
	displayWidth := 1920
	displayHeight := 1080
	displayRefreshRate := 60
	resolution := ""
	zoomLevel := 0
	desktopType := ""

	// Ensure desktopType has a sensible default (ubuntu) when not set by app config
	// This is critical for video_source_mode: ubuntu uses "pipewire", sway uses "wayland"
	if desktopType == "" {
		desktopType = "ubuntu"
		log.Debug().Str("task_id", task.ID).Msg("Using default desktop type: ubuntu")
	}

	// Create ZedAgent struct with session info for Wolf executor
	log.Debug().Str("task_id", task.ID).Msg("DEBUG: About to create ZedAgent struct")
	// Build env vars (locale; API key added in OnBeforeCreate hook,
	// project secrets injected by HydraExecutor.StartDesktop)
	envVars := buildEnvWithLocale("", task.PlanningOptions)

	// Inject startup script from project YAML (stored in database).
	// helix-workspace-setup.sh uses this as a fallback when no .helix/startup.sh
	// exists in the helix-specs branch (typical for externally-applied projects).
	if project.StartupScriptYAML != "" {
		envVars = append(envVars, "HELIX_STARTUP_SCRIPT="+project.StartupScriptYAML)
	}
	// Legacy: also pass install/start for backward compatibility
	if project.StartupInstall != "" {
		envVars = append(envVars, "HELIX_STARTUP_INSTALL="+project.StartupInstall)
	}
	if project.StartupStart != "" {
		envVars = append(envVars, "HELIX_STARTUP_START="+project.StartupStart)
	}

	zedAgent := &types.DesktopAgent{
		OrganizationID: orgID,
		SessionID:      session.ID,
		UserID:         task.CreatedBy,
		Input:          "Initialize Zed development environment for spec generation",
		ProjectID:      task.ProjectID, // For golden Docker cache overlay
		ProjectPath:    "workspace",    // Use relative path
		SpecTaskID:     task.ID,        // For task-scoped workspace
		VCPUs:          sandboxVCPUs(task),
		MemoryMB:       sandboxMemoryMB(task),
		// RepositoryIDs / PrimaryRepositoryID set by SetRepoContext below.
		DisplayWidth:       displayWidth,
		DisplayHeight:      displayHeight,
		DisplayRefreshRate: displayRefreshRate,
		Resolution:         resolution,
		ZoomLevel:          zoomLevel,
		DesktopType:        desktopType,
		Env:                envVars,
		OnBeforeCreate: func(hookCtx context.Context, a *types.DesktopAgent) error {
			apiKey, err := s.GetOrCreateSessionAPIKey(hookCtx, &SessionAPIKeyRequest{
				OrganizationID: launchOrgID,
				UserID:         launchTask.CreatedBy,
				SessionID:      launchSession.ID,
			})
			if err != nil {
				return fmt.Errorf("failed to get session API key: %w", err)
			}
			a.Env = append(a.Env, types.DesktopAgentAPIEnvVars(apiKey)...)
			return nil
		},
		// Branch configuration - startup script will checkout correct branch
		BranchMode:    string(task.BranchMode),
		BaseBranch:    task.BaseBranch,
		WorkingBranch: task.BranchName, // For existing mode: checkout this; for new mode: create this
	}
	zedAgent.SetRepoContext(projectRepos, project.DefaultRepoID)
	log.Debug().Str("task_id", task.ID).Str("session_id", session.ID).Msg("DEBUG: Created ZedAgent struct")

	// Check if executor is nil
	if s.externalAgentExecutor == nil {
		log.Error().Str("task_id", task.ID).Msg("ERROR: externalAgentExecutor is nil!")
		s.markTaskFailed(ctx, task, "externalAgentExecutor is nil")
		return
	}

	// Start the Zed agent via Wolf executor (not NATS)
	log.Debug().Str("task_id", task.ID).Str("session_id", session.ID).Msg("DEBUG: Calling StartDesktop...")
	agentResp, err := s.externalAgentExecutor.StartDesktop(ctx, zedAgent)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Str("session_id", session.ID).Msg("Failed to launch external agent for spec generation")
		s.markTaskFailedErr(ctx, task, err)
		return
	}

	log.Info().
		Str("task_id", task.ID).
		Str("session_id", session.ID).
		Str("planning_session_id", task.PlanningSessionID).
		Str("dev_container_id", agentResp.DevContainerID).
		Str("container_name", agentResp.ContainerName).
		Msg("Spec generation agent session created and Zed agent launched via Wolf executor")

	// Log audit event for agent started (now that session is created)
	if s.auditLogService != nil {
		s.auditLogService.LogAgentStarted(ctx, task, session.ID, task.CreatedBy, "")
	}
}

// StartJustDoItMode skips spec generation and goes straight to implementation with just the user's prompt
// This is for tasks that don't require planning code changes
// opts contains optional settings like keyboard layout from browser locale detection
func (s *SpecDrivenTaskService) StartJustDoItMode(ctx context.Context, task *types.SpecTask) {
	// Add panic recovery for debugging (match StartSpecGeneration pattern)
	defer func() {
		if r := recover(); r != nil {
			log.Error().Interface("panic", r).Str("task_id", task.ID).Msg("PANIC in StartJustDoItMode")
		}
	}()

	// Get project first - needed for agent inheritance and guidelines
	var project *types.Project
	orgID := ""
	guidelines := ""
	if task.ProjectID != "" {
		var err error
		project, err = s.store.GetProject(ctx, task.ProjectID)
		if err != nil {
			log.Warn().Err(err).Str("project_id", task.ProjectID).Msg("Failed to get project")
		} else if project != nil {
			orgID = project.OrganizationID
			// Get organization guidelines
			if orgID != "" {
				org, orgErr := s.store.GetOrganization(ctx, &store.GetOrganizationQuery{ID: orgID})
				if orgErr == nil && org != nil && org.Guidelines != "" {
					guidelines = org.Guidelines
				}
			}
			// Append project guidelines
			if project.Guidelines != "" {
				if guidelines != "" {
					guidelines += "\n\n---\n\n"
				}
				guidelines += project.Guidelines
			}
		}
	}

	if err := s.migrateSpecTaskCodeAgentConfig(ctx, task, project); err != nil {
		s.markTaskFailed(ctx, task, fmt.Sprintf("failed to prepare code-agent configuration: %v", err))
		return
	}

	// Use Description (user-editable) with fallback to OriginalPrompt (immutable original)
	userPrompt := task.Description
	if userPrompt == "" {
		userPrompt = task.OriginalPrompt
	}

	log.Info().
		Str("task_id", task.ID).
		Str("user_prompt", userPrompt).
		Str("code_agent_runtime", string(task.CodeAgentConfig.Runtime)).
		Msg("Starting Just Do It mode - skipping spec generation")

	// Note: Task number and design doc path are now assigned at creation time
	// in CreateTaskFromPrompt, so they should always be set by this point.

	// Clear any previous error from metadata (in case this is a retry)
	if task.Metadata != nil {
		delete(task.Metadata, "error")
		delete(task.Metadata, "error_timestamp")
	}

	// Handle branch configuration based on mode
	var branchName string
	if task.BranchMode == types.BranchModeExisting && task.BranchName != "" {
		// Existing mode: use the branch name that was set during task creation
		branchName = task.BranchName
		log.Info().
			Str("task_id", task.ID).
			Str("branch_name", branchName).
			Msg("Continuing work on existing branch")
	} else {
		// New mode: generate unique feature branch name (checks for collisions across all projects)
		var err error
		branchName, err = GenerateUniqueBranchName(ctx, s.store, task)
		if err != nil {
			log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to generate unique branch name, using fallback")
			branchName = GenerateFeatureBranchName(task)
		}

		// Set base branch if not already set (defaults to repo default, handled in agent prompt)
		if task.BaseBranch == "" && project != nil && project.DefaultRepoID != "" {
			repo, err := s.store.GetGitRepository(ctx, project.DefaultRepoID)
			if err == nil && repo != nil && repo.DefaultBranch != "" {
				task.BaseBranch = repo.DefaultBranch
			}
		}
	}

	// Update task status directly to implementation (skip all spec phases)
	now := time.Now()
	task.Status = types.TaskStatusImplementation
	task.StatusUpdatedAt = &now
	task.BranchName = branchName
	task.UpdatedAt = now
	task.StartedAt = &now

	err := s.store.UpdateSpecTask(ctx, task)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to update task status for Just Do It mode")
		return
	}

	// Get CodeAgentRuntime from the app config (needed for session resume to select correct agent)
	codeAgentRuntimeJDI := codeAgentRuntimeForSpecTask(task)

	sessionMetadata := types.SessionMetadata{
		SystemPrompt:     "",             // Don't override agent's system prompt
		AgentType:        "zed_external", // Use Zed agent for git access
		Stream:           false,
		SpecTaskID:       task.ID,             // CRITICAL: Set SpecTaskID so session restore uses correct workspace path
		CodeAgentRuntime: codeAgentRuntimeJDI, // For open_thread on resume
		// Whose Claude subscription authenticates this session's agent, when the
		// dispatcher is a service account acting for a human. Credential-only —
		// the session is still owned by task.CreatedBy. This is the path HelixOS
		// bots take (just-do-it), so it is the one that routes a bot to its
		// owner's Claude account instead of the orchestrator's.
		CredentialOwnerID: task.CredentialOwnerID,
		// Autonomous surface: no human watches a just-do-it run, so recover the
		// agent automatically on crash rather than stalling errored+idle.
		AutoRestartOnCrash: true,
	}

	session := &types.Session{
		ID:             system.GenerateSessionID(),
		Name:           fmt.Sprintf("Just Do It: %s", task.Name),
		Created:        time.Now(),
		Updated:        time.Now(),
		Mode:           types.SessionModeInference,
		Type:           types.SessionTypeText,
		Provider:       "anthropic",      // Use Claude
		ModelName:      "external_agent", // Model name for external agents
		Owner:          task.CreatedBy,
		ParentApp:      "",
		OrganizationID: orgID,
		ProjectID:      task.ProjectID, // For project-level skills
		Metadata:       sessionMetadata,
		OwnerType:      types.OwnerTypeUser,
	}

	session, err = s.store.CreateSession(ctx, *session)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to create Just Do It session")
		s.markTaskFailed(ctx, task, fmt.Sprintf("Failed to create session: %v", err))
		return
	}

	claimed, err := s.store.SetPlanningSessionIDIfEmpty(ctx, task.ID, session.ID)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Str("session_id", session.ID).Msg("Failed to claim planning_session_id; rolling back Just Do It session")
		if _, delErr := s.store.DeleteSession(ctx, session.ID); delErr != nil {
			log.Warn().Err(delErr).Str("session_id", session.ID).Msg("Failed to delete orphan Just Do It session after claim error")
		}
		s.markTaskFailed(ctx, task, fmt.Sprintf("Failed to claim planning_session_id: %v", err))
		return
	}
	if !claimed {
		log.Info().
			Str("task_id", task.ID).
			Str("orphan_session_id", session.ID).
			Msg("Lost race to claim planning_session_id; deleting orphan Just Do It session")
		if _, delErr := s.store.DeleteSession(ctx, session.ID); delErr != nil {
			log.Warn().Err(delErr).Str("session_id", session.ID).Msg("Failed to delete orphan Just Do It session after losing claim")
		}
		return
	}
	task.PlanningSessionID = session.ID

	// Just-Do-It skips the spec phase but still pushes commits to a feature
	// branch, so the git identity must match the user who started the task.
	// Mirror the planning-phase flow: async sync so we don't block on the
	// container being reachable.
	jdiActorID := task.PlanningStartedBy
	if jdiActorID == "" {
		jdiActorID = task.CreatedBy
	}
	s.syncGitIdentityAsync(task, jdiActorID, "just-do-it", 3*time.Minute)

	// Generate request_id for initial message and register the mapping
	requestID := system.GenerateRequestID()
	if s.RegisterRequestMapping != nil {
		s.RegisterRequestMapping(requestID, session.ID)
	}

	// In Just Do It mode, send the user's prompt with brief branch instructions
	// Keep it minimal - no detailed spec generation instructions, just branch info
	guidelinesSection := ""
	if guidelines != "" {
		guidelinesSection = fmt.Sprintf(`
## Guidelines

Follow these guidelines when making changes:

%s

---
`, guidelines)
	}

	// Get all project repositories early (needed for prompt)
	projectRepos, err := s.store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{
		ProjectID: task.ProjectID,
	})
	if err != nil {
		log.Warn().Err(err).Str("project_id", task.ProjectID).Msg("Failed to get project repositories")
		projectRepos = nil
	}

	// Sync base branch from upstream for external repos BEFORE starting work
	// This ensures we have the latest code from the external repository
	if err := s.gitRepositoryService.SyncBaseBranchForTask(ctx, task, projectRepos); err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to sync base branch from upstream")
		s.markTaskFailed(ctx, task, err.Error())
		return
	}

	// Determine primary repository from project configuration
	primaryRepoID := project.DefaultRepoID
	if primaryRepoID == "" && len(projectRepos) > 0 {
		// Use first project repo as fallback if no default set
		primaryRepoID = projectRepos[0].ID
	}

	// Get primary repo name for the prompt
	var primaryRepoName string
	if primaryRepoID != "" {
		for _, repo := range projectRepos {
			if repo.ID == primaryRepoID {
				primaryRepoName = repo.Name
				break
			}
		}
	}

	// Build git instructions - branch is already checked out by startup script (start-zed-helix.sh)
	// Just tell agent to verify and push when done
	gitInstructions := fmt.Sprintf(`**Branch already checked out:**
- Verify: `+"`git branch --show-current`"+` should show %s
- Make your changes
- Push: `+"`git push origin %s`", branchName, branchName)

	// Build repository section listing local + Kodit repos for the agent
	repoSection := s.buildRepositorySectionForTask(ctx, task, project)

	// Just-Do-It skips the planning prompt, which normally tells the agent where
	// task attachments were staged. Build the same attachment section here so an
	// implementation-only task can actually inspect its uploaded files.
	attachmentsSection, attachErr := s.stageAttachmentsAndBuildPromptSection(ctx, task, project)
	if attachErr != nil {
		log.Warn().Err(attachErr).Str("task_id", task.ID).Msg("Failed to stage attachments — continuing without them")
	}

	// qwen-code's Shell tool requires `is_background` on every call (see
	// qwen-code/packages/core/src/tools/shell.test.ts). Other runtimes use a
	// different parameter name (Claude Code: `run_in_background`, Codex: none),
	// and forcing `is_background` on those tools triggers
	// `InputValidationError: An unexpected parameter "is_background" was provided`
	// on the agent's first Bash call, burning a tool slot before any real work.
	shellCommandsGuidance := ""
	if codeAgentRuntimeJDI == types.CodeAgentRuntimeQwenCode {
		shellCommandsGuidance = "**Shell commands:** Specify is_background (true or false) on all shell commands - it's required. Use true for long-running operations (builds, servers, installs).\n\n"
	}

	promptWithBranch := buildJustDoItPrompt(
		userPrompt,
		guidelinesSection,
		primaryRepoName,
		repoSection,
		attachmentsSection,
		shellCommandsGuidance,
		gitInstructions,
	)

	interaction := &types.Interaction{
		ID:            system.GenerateInteractionID(),
		Created:       time.Now(),
		Updated:       time.Now(),
		Scheduled:     time.Now(),
		SessionID:     session.ID,
		UserID:        task.CreatedBy,
		Mode:          types.SessionModeInference,
		SystemPrompt:  "", // Don't override agent's system prompt
		PromptMessage: promptWithBranch,
		State:         types.InteractionStateWaiting,
	}

	_, err = s.store.CreateInteraction(ctx, interaction)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to create initial interaction")
		s.markTaskFailed(ctx, task, fmt.Sprintf("Failed to create initial interaction: %v", err))
		return
	}

	// Launch the external agent (Zed) via Wolf executor
	// Project and projectRepos already fetched earlier

	// Build list of all repository IDs to clone from project
	repositoryIDs := []string{}
	for _, repo := range projectRepos {
		if repo.ID != "" {
			repositoryIDs = append(repositoryIDs, repo.ID)
		}
	}

	// Get session-scoped ephemeral API key for this dev container
	// Key is minted now and will be revoked when the desktop shuts down
	userAPIKey, err := s.GetOrCreateSessionAPIKey(ctx, &SessionAPIKeyRequest{
		OrganizationID: orgID,
		UserID:         task.CreatedBy,
		SessionID:      session.ID,
	})
	if err != nil {
		log.Error().Err(err).Str("user_id", task.CreatedBy).Str("session_id", session.ID).Msg("Failed to get session API key for Just Do It task")
		s.markTaskFailed(ctx, task, fmt.Sprintf("Failed to get session API key: %v", err))
		return
	}

	// Use task/session sandbox defaults; execution config no longer references
	// an App-owned display profile.
	displayWidthJDI := 1920
	displayHeightJDI := 1080
	displayRefreshRateJDI := 60
	resolutionJDI := ""
	zoomLevelJDI := 0
	desktopTypeJDI := ""

	// Build env vars (base + locale; project secrets injected by HydraExecutor.StartDesktop)
	envVarsJDI := buildEnvWithLocale(userAPIKey, task.PlanningOptions)

	// Inject startup script from project YAML (same as planning phase)
	if project.StartupScriptYAML != "" {
		envVarsJDI = append(envVarsJDI, "HELIX_STARTUP_SCRIPT="+project.StartupScriptYAML)
	}
	// Legacy: also pass install/start for backward compatibility
	if project.StartupInstall != "" {
		envVarsJDI = append(envVarsJDI, "HELIX_STARTUP_INSTALL="+project.StartupInstall)
	}
	if project.StartupStart != "" {
		envVarsJDI = append(envVarsJDI, "HELIX_STARTUP_START="+project.StartupStart)
	}

	// Create ZedAgent struct with session info for Wolf executor
	zedAgent := &types.DesktopAgent{
		OrganizationID:      orgID,
		SessionID:           session.ID,
		UserID:              task.CreatedBy,
		Input:               "Initialize Zed development environment",
		ProjectID:           task.ProjectID, // For golden Docker cache overlay
		ProjectPath:         "workspace",    // Use relative path
		SpecTaskID:          task.ID,        // For task-scoped workspace
		VCPUs:               sandboxVCPUs(task),
		MemoryMB:            sandboxMemoryMB(task),
		PrimaryRepositoryID: primaryRepoID, // Primary repo to open in Zed
		RepositoryIDs:       repositoryIDs, // ALL project repos to checkout
		DisplayWidth:        displayWidthJDI,
		DisplayHeight:       displayHeightJDI,
		DisplayRefreshRate:  displayRefreshRateJDI,
		Resolution:          resolutionJDI,
		ZoomLevel:           zoomLevelJDI,
		DesktopType:         desktopTypeJDI,
		Env:                 envVarsJDI,
		// Branch configuration - startup script will checkout correct branch
		BranchMode:    string(task.BranchMode),
		BaseBranch:    task.BaseBranch,
		WorkingBranch: task.BranchName, // For existing mode: checkout this; for new mode: create this
	}

	// Start the Zed agent via Wolf executor
	agentResp, err := s.externalAgentExecutor.StartDesktop(ctx, zedAgent)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Str("session_id", session.ID).Msg("Failed to launch external agent for Just Do It mode")
		s.markTaskFailedErr(ctx, task, err)
		return
	}

	log.Info().
		Str("task_id", task.ID).
		Str("session_id", session.ID).
		Str("branch_name", branchName).
		Str("dev_container_id", agentResp.DevContainerID).
		Str("container_name", agentResp.ContainerName).
		Msg("Just Do It mode: Zed agent launched with branch instructions")

	// Log audit event for agent started (now that session is created)
	if s.auditLogService != nil {
		s.auditLogService.LogAgentStarted(ctx, task, session.ID, task.CreatedBy, "")
	}
}

func sandboxVCPUs(task *types.SpecTask) int {
	if task == nil {
		return 0
	}
	return types.EffectiveSpecTaskSandboxResources(task.SandboxResourceOverrides).VCPUs
}

func sandboxMemoryMB(task *types.SpecTask) int {
	if task == nil {
		return 0
	}
	return types.EffectiveSpecTaskSandboxResources(task.SandboxResourceOverrides).MemoryMB
}

// buildEnvWithLocale constructs the environment variable array for desktop containers
// API token is added separately via OnBeforeCreate hook (inside the session lock)
func buildEnvWithLocale(userAPIKey string, opts types.StartPlanningOptions) []string {
	var env []string
	// Only add API env vars if key is provided (legacy callers).
	// New callers pass "" and use OnBeforeCreate hook for race-safe key injection.
	if userAPIKey != "" {
		env = types.DesktopAgentAPIEnvVars(userAPIKey)
	}

	log.Info().
		Int("token_env_vars_count", len(env)).
		Bool("user_api_key_set", userAPIKey != "").
		Msg("buildEnvWithLocale: env vars prepared")

	// Add keyboard layout if specified (from browser locale detection)
	if opts.KeyboardLayout != "" {
		env = append(env, fmt.Sprintf("XKB_DEFAULT_LAYOUT=%s", opts.KeyboardLayout))
		log.Debug().Str("keyboard", opts.KeyboardLayout).Msg("Adding keyboard layout to container env")
	}

	// Add timezone if specified
	if opts.Timezone != "" {
		env = append(env, fmt.Sprintf("TZ=%s", opts.Timezone))
		log.Debug().Str("timezone", opts.Timezone).Msg("Adding timezone to container env")
	}

	return env
}

// HandleSpecGenerationComplete processes completed spec generation from Helix agent
func (s *SpecDrivenTaskService) HandleSpecGenerationComplete(ctx context.Context, taskID string, specs *types.SpecGeneration) error {
	task, err := s.store.GetSpecTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	// Update task with generated specs
	now := time.Now()
	task.RequirementsSpec = specs.RequirementsSpec
	task.TechnicalDesign = specs.TechnicalDesign
	task.ImplementationPlan = specs.ImplementationPlan
	task.Status = types.TaskStatusSpecReview
	task.StatusUpdatedAt = &now
	task.UpdatedAt = now

	err = s.store.UpdateSpecTask(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to update task with specs: %w", err)
	}

	log.Info().
		Str("task_id", taskID).
		Msg("Spec generation completed, awaiting human review")

	// Log audit event for spec generated
	if s.auditLogService != nil {
		s.auditLogService.LogSpecGenerated(ctx, task, task.CreatedBy, "")
	}

	// Send notification to user for spec review
	if s.notifier != nil {
		// Note: The notification system expects a session, but for task notifications we'll create a minimal one
		session := &types.Session{
			ID:    task.PlanningSessionID,
			Owner: task.CreatedBy,
		}

		notificationPayload := &types.Notification{
			Session: session,
			Event:   types.EventCronTriggerComplete,
		}

		if err := s.notifier.Notify(ctx, notificationPayload); err != nil {
			log.Error().Err(err).Str("task_id", taskID).Msg("Failed to send spec review notification")
			// Don't fail the whole operation if notification fails
		}
	}

	return nil
}

// ApproveSpecs handles human approval of generated specs
func (s *SpecDrivenTaskService) ApproveSpecs(ctx context.Context, task *types.SpecTask) error {
	task, err := s.store.GetSpecTask(ctx, task.ID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	// Early-exit if the task is already past the spec phase. This is a fast-path
	// for the common case; the authoritative race guard is the atomic
	// TransitionSpecTaskStatus call below, which is the only thing that prevents
	// two concurrent callers from both sending the implementation instruction.
	if task.Status == types.TaskStatusImplementation ||
		task.Status == types.TaskStatusImplementationQueued ||
		task.Status == types.TaskStatusQueuedImplementation {
		log.Info().
			Str("task_id", task.ID).
			Str("status", string(task.Status)).
			Msg("[ApproveSpecs] Task already past approval — skipping")
		return nil
	}

	if task.SpecApproval == nil {
		approvedAt := time.Now()
		if task.SpecApprovedAt != nil {
			approvedAt = *task.SpecApprovedAt
		}
		task.SpecApproval = &types.SpecApprovalResponse{
			TaskID:     task.ID,
			Approved:   true,
			ApprovedBy: task.SpecApprovedBy,
			ApprovedAt: approvedAt,
		}
	}

	var project *types.Project
	if task.CodeAgentConfig == nil || task.HelixAppID != "" || task.CodeAgentOverrides != nil {
		project, err = s.store.GetProject(ctx, task.ProjectID)
		if err != nil {
			return fmt.Errorf("failed to get project for code-agent migration: %w", err)
		}
		if err := s.migrateSpecTaskCodeAgentConfig(ctx, task, project); err != nil {
			return fmt.Errorf("failed to migrate task code-agent configuration: %w", err)
		}
	}

	if task.SpecApproval.Approved {
		// Get project and repository info
		if project == nil {
			project, err = s.store.GetProject(ctx, task.ProjectID)
			if err != nil {
				return fmt.Errorf("failed to get project: %w", err)
			}
		}

		if task.CodeAgentConfig == nil {
			return fmt.Errorf("task has no code-agent configuration")
		}

		if project.DefaultRepoID == "" {
			return fmt.Errorf("default repository not set for project")
		}

		repo, err := s.store.GetGitRepository(ctx, project.DefaultRepoID)
		if err != nil {
			return fmt.Errorf("failed to get default repository: %w", err)
		}

		if repo.DefaultBranch == "" {
			return fmt.Errorf("default branch not set for repository, please set it")
		}

		if repo.ExternalURL != "" {
			log.Info().Str("repo_id", repo.ID).Str("branch", repo.DefaultBranch).Msg("ApproveSpecs: syncing base branch from remote")

			// Use SyncBaseBranch which handles divergence detection
			err = s.gitRepositoryService.SyncBaseBranch(ctx, repo.ID, repo.DefaultBranch)
			if err != nil {
				// Check for divergence error and format a user-friendly message
				if divergeErr := GetBranchDivergenceError(err); divergeErr != nil {
					return fmt.Errorf("%s", FormatDivergenceErrorForUser(divergeErr, repo.Name))
				}
				log.Error().Err(err).Str("repo_id", repo.ID).Str("branch", repo.DefaultBranch).Msg("Failed to sync from remote")
				return fmt.Errorf("failed to sync base branch from external repository '%s': %w", repo.ExternalURL, err)
			}
		}

		// Handle branch configuration based on mode
		var branchName string
		effectiveBaseBranch := repo.DefaultBranch
		if task.BranchMode == types.BranchModeExisting && task.BranchName != "" {
			// Existing mode: use the branch name that was set during task creation
			branchName = task.BranchName
			log.Info().
				Str("task_id", task.ID).
				Str("branch_name", branchName).
				Msg("[ApproveSpecs] Continuing work on existing branch")
		} else {
			// New mode: generate unique feature branch name (checks for collisions across all projects)
			var err error
			branchName, err = GenerateUniqueBranchName(ctx, s.store, task)
			if err != nil {
				log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to generate unique branch name, using fallback")
				branchName = GenerateFeatureBranchName(task)
			}

			// Set base branch if not already set
			if task.BaseBranch == "" {
				task.BaseBranch = repo.DefaultBranch
			}
			if task.BranchMode == types.BranchModeNew {
				effectiveBaseBranch = task.BaseBranch
			}
		}

		// Atomically transition the status from a spec-phase state to
		// implementation. Only one caller's UPDATE can match the WHERE clause,
		// so only one caller proceeds to send the implementation instruction.
		// This is the authoritative race guard — the read-then-check pattern at
		// the top of this function is just a fast path for the common case.
		now := time.Now()
		extraFields := map[string]any{
			"branch_name": branchName,
			"started_at":  now,
			"base_branch": task.BaseBranch,
		}
		// Persist the synthesized SpecApproval struct (only set when the
		// caller arrived with task.SpecApproval == nil). The pre-PR2260
		// implementation persisted this implicitly via UpdateSpecTask saving
		// the whole struct; the atomic-update path only writes columns we
		// list here.
		if task.SpecApproval != nil {
			specApprovalJSON, marshalErr := json.Marshal(task.SpecApproval)
			if marshalErr != nil {
				return fmt.Errorf("failed to marshal SpecApproval: %w", marshalErr)
			}
			extraFields["spec_approval"] = string(specApprovalJSON)
		}
		transitioned, err := s.store.TransitionSpecTaskStatus(
			ctx,
			task.ID,
			[]types.SpecTaskStatus{
				types.TaskStatusSpecApproved,
				types.TaskStatusSpecReview,
				types.TaskStatusSpecRevision,
				types.TaskStatusSpecGeneration,
			},
			types.TaskStatusImplementation,
			extraFields,
		)
		if err != nil {
			return fmt.Errorf("failed to transition task to implementation: %w", err)
		}
		if !transitioned {
			log.Info().
				Str("task_id", task.ID).
				Msg("[ApproveSpecs] Another caller won the race — skipping")
			return nil
		}

		// Reflect the transition in the in-memory task so downstream code
		// (logging, the message sender) sees the new state.
		task.Status = types.TaskStatusImplementation
		task.StatusUpdatedAt = &now
		task.BranchName = branchName
		task.StartedAt = &now

		// Update git identity in the running container to match the approver,
		// so implementation commits are attributed to the user who approved specs.
		sessionID := task.PlanningSessionID

		if err := s.syncGitIdentityToUser(ctx, task, task.SpecApprovedBy, "approver"); err != nil {
			// Don't fail the whole approval — push-time credentials already
			// fall back to SpecApprovedBy (see git_http_server.go), so commit
			// attribution in the container is the only thing that reverts to
			// the creator. Log loudly so operators notice.
			log.Error().Err(err).Str("task_id", task.ID).Str("session_id", sessionID).
				Msg("Failed to update container git identity at approval; commits may be attributed to previous actor")
		}

		// For BranchMode=new tasks the feature branch name is only generated
		// here (line ~1268 above), not when the planning container started.
		// helix-workspace-setup.sh therefore couldn't create the branch at
		// container boot (HELIX_WORKING_BRANCH was empty), so the working
		// tree is still on the base branch (typically `main`). The
		// implementation-phase prompt tells the agent to "verify branch" but
		// never says "check out the feature branch" — agents that find
		// themselves on `main` commit there and try to push to `main`. The
		// server's pre-receive hook restricts pushes to helix-specs +
		// the feature branch, so the push is rejected — but we've already
		// wasted a turn and the agent is in a confused state.
		//
		// Make the branch state correct *before* the implementation prompt
		// arrives. Idempotent: `checkout -B` works whether the branch exists
		// locally, remotely, or not at all; `push -u` is a no-op if the
		// remote already has it. Errors are logged but don't block the
		// transition: the existing pre-receive hook stops genuinely-bad
		// pushes, and the agent prompt still names the right branch.
		if task.BranchMode == types.BranchModeNew {
			if err := s.ensureFeatureBranchInContainer(ctx, sessionID, repo.Name, branchName, effectiveBaseBranch); err != nil {
				log.Error().Err(err).
					Str("task_id", task.ID).Str("session_id", sessionID).
					Str("repo", repo.Name).Str("branch", branchName).Str("base", effectiveBaseBranch).
					Msg("Failed to check out feature branch in container at approval; agent may start on base branch")
			}
		}

		// Send instruction to existing agent session (reuse planning session)
		if sessionID != "" && !s.testMode {
			// Create agent instruction service
			agentInstructionService := NewAgentInstructionService(s.store, s.EnqueueMessageToAgent, s.koditService)

			err := agentInstructionService.SendApprovalInstruction(
				context.Background(),
				sessionID,
				task.CreatedBy, // User who created the task
				task,
				branchName,
				effectiveBaseBranch,
				repo.Name,
			)
			if err != nil {
				log.Error().
					Err(err).
					Str("task_id", task.ID).
					Str("session_id", sessionID).
					Msg("Failed to send approval instruction to agent")
				return err
			}

			log.Info().
				Str("task_id", task.ID).
				Str("session_id", sessionID).
				Str("branch_name", branchName).
				Str("base_branch", effectiveBaseBranch).
				Msg("Specs approved - sent implementation instruction to existing agent")
		} else {
			log.Warn().
				Str("task_id", task.ID).
				Msg("No planning session ID found - agent will not receive implementation instruction")
		}

	} else {
		// Specs need revision
		now := time.Now()
		task.Status = types.TaskStatusSpecRevision
		task.StatusUpdatedAt = &now
		task.SpecRevisionCount++

		if task.CodeAgentConfig == nil {
			return fmt.Errorf("task has no code-agent configuration")
		}

		err = s.store.UpdateSpecTask(ctx, task)
		if err != nil {
			return fmt.Errorf("failed to update task for revision: %w", err)
		}

		// Send revision instruction to existing agent session via the queue
		if s.EnqueueMessageToAgent != nil && !s.testMode {
			go func(t *types.SpecTask, comments string) {
				message := BuildRevisionInstructionPrompt(t, comments)
				// interrupt=true: revision instruction is reviewer-driven feedback delivered via the
				// task-state machine — same semantic as a comment, should preempt in-flight work.
				err := s.EnqueueMessageToAgent(context.Background(), t, message, true, "")
				if err != nil {
					log.Error().
						Err(err).
						Str("task_id", t.ID).
						Str("planning_session_id", t.PlanningSessionID).
						Msg("Failed to send revision instruction to agent via WebSocket")
				} else {
					log.Info().
						Str("task_id", t.ID).
						Str("comments", comments).
						Msg("Specs require revision - sent revision instruction to agent via WebSocket")
				}
			}(task, task.SpecApproval.Comments)
		} else if !s.testMode {
			log.Warn().
				Str("task_id", task.ID).
				Msg("No message sender configured - agent will not receive revision instruction")
		}

		// Log audit event for review comment (revision request)
		if s.auditLogService != nil && task.SpecApproval.Comments != "" {
			// reviewID=planningSessionID, commentID=empty (revision not a specific comment), commentText, userID, userEmail
			s.auditLogService.LogReviewComment(ctx, task, task.PlanningSessionID, "", task.SpecApproval.Comments, task.SpecApproval.ApprovedBy, "")
		}
	}

	return nil
}

// syncGitIdentityToUser updates the container's global git user.name and
// user.email to match the given userID, so commits authored after this call
// are attributed correctly. phaseLabel ("planner", "approver", …) is purely
// for logs so we can tell which transition triggered the sync.
//
// Behaviour:
//   - Returns nil (no-op) when there is no session, no exec callback, we are
//     in test mode, or userID is empty.
//   - Returns an error when we can identify who the actor should be but
//     can't set their identity (DB lookup fails, email missing, or exec
//     fails — e.g. the container isn't up yet).
//   - Sets user.email first because email is what Git uses for authorship
//     attribution. If setting name fails after email succeeded, we log the
//     partial state but return success, since the attribution-critical field
//     is correct.
func (s *SpecDrivenTaskService) syncGitIdentityToUser(ctx context.Context, task *types.SpecTask, userID, phaseLabel string) error {
	if s.testMode || s.ExecInDesktop == nil {
		return nil
	}
	if task.PlanningSessionID == "" {
		return nil
	}
	if userID == "" {
		// No actor recorded for this phase — nothing to sync against.
		// Expected for legacy tasks predating the PlanningStartedBy field,
		// or for auto-approval paths that don't carry an identity.
		return nil
	}

	actor, err := s.store.GetUser(ctx, &store.GetUserQuery{ID: userID})
	if err != nil {
		return fmt.Errorf("failed to look up %s %s: %w", phaseLabel, userID, err)
	}
	if actor == nil {
		return fmt.Errorf("%w: %s %s", errIdentityUserNotFound, phaseLabel, userID)
	}

	actorEmail := actor.GitAuthorEmail()
	if actorEmail == "" {
		return fmt.Errorf("%w: %s %s", errIdentityNoEmail, phaseLabel, actor.ID)
	}

	actorName := actor.GitAuthorName()

	sessionID := task.PlanningSessionID

	// Email first — this is the attribution-critical field. If it fails we
	// don't touch user.name at all, so we don't leave a mixed identity.
	if err := s.ExecInDesktop(ctx, sessionID, []string{"git", "config", "--global", "user.email", actorEmail}); err != nil {
		return fmt.Errorf("failed to set git user.email: %w", err)
	}
	if err := s.ExecInDesktop(ctx, sessionID, []string{"git", "config", "--global", "user.name", actorName}); err != nil {
		// user.email succeeded so commits will attribute correctly by
		// email; the display name may carry over from whoever was set
		// previously.
		log.Warn().Err(err).
			Str("task_id", task.ID).Str("session_id", sessionID).Str("phase", phaseLabel).
			Msg("Set user.email but failed to set user.name; commit attribution correct but display name may lag")
		return nil
	}

	log.Info().
		Str("task_id", task.ID).Str("session_id", sessionID).Str("phase", phaseLabel).
		Str("actor_name", actorName).Str("actor_email", actorEmail).
		Msg("Updated git identity in container")
	return nil
}

// ensureFeatureBranchInContainer checks out the spec task's feature branch
// in the dev container's primary repo working tree at the moment we
// transition the task into the implementation phase.
//
// Why this is needed: for BranchMode=new tasks the feature branch name
// is only generated in ApproveSpecs (above) — well after the planning
// container started. helix-workspace-setup.sh runs once at container
// boot and only creates a feature branch when HELIX_WORKING_BRANCH is
// already set in the container env. For planning-phase containers that
// env var is empty (task.BranchName is empty until specs land), so the
// working tree is left on the base branch. The implementation-phase
// prompt assumes the right branch is already checked out, so agents
// just commit to base (typically `main`). The pre-receive hook
// rejects the push, the agent gets confused, and the user loses a turn.
//
// The fix runs the same git plumbing the workspace-setup script would
// have run, but at the point we actually know the branch name. Safe to
// re-run: `checkout -B` works whether the branch exists locally,
// remotely, or not at all; `push -u` is a no-op if the remote already
// has it.
//
// Failures are non-fatal: this is best-effort and the existing
// pre-receive hook still stops genuinely-bad pushes to base. We log
// loudly so operators see when it falls back to the broken default.
func (s *SpecDrivenTaskService) ensureFeatureBranchInContainer(ctx context.Context, sessionID, repoName, branchName, baseBranch string) error {
	if s.testMode || s.ExecInDesktop == nil {
		return nil
	}
	if sessionID == "" || repoName == "" || branchName == "" || baseBranch == "" {
		return fmt.Errorf("missing required arg: sessionID=%q repoName=%q branchName=%q baseBranch=%q",
			sessionID, repoName, branchName, baseBranch)
	}

	// Single bash command so we get atomic chained semantics and one
	// docker-exec round-trip. -B is idempotent. -u sets upstream once;
	// subsequent runs are no-ops.
	script := fmt.Sprintf(
		"cd /home/retro/work/%s && git fetch origin %s && git checkout -B %s origin/%s && git push -u origin %s",
		shellQuoteArg(repoName), shellQuoteArg(baseBranch),
		shellQuoteArg(branchName), shellQuoteArg(baseBranch),
		shellQuoteArg(branchName),
	)
	if err := s.ExecInDesktop(ctx, sessionID, []string{"bash", "-c", script}); err != nil {
		return fmt.Errorf("git checkout/push feature branch: %w", err)
	}

	log.Info().
		Str("session_id", sessionID).Str("repo", repoName).
		Str("branch", branchName).Str("base", baseBranch).
		Msg("Feature branch checked out and pushed in container")
	return nil
}

// shellQuoteArg wraps an argument in single quotes and escapes any
// embedded single quotes. Repository / branch names are validated
// elsewhere but we don't want this helper to be the place an unexpected
// metacharacter causes an injection — defence in depth.
func shellQuoteArg(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// syncGitIdentityAsync runs syncGitIdentityToUser in the background, retrying
// until it succeeds or maxWait elapses. Used at planning-boot time where the
// desktop container is still coming up — we can't block the caller on the
// container being reachable, and we can't count on a "ready" signal.
//
// Errors that won't be fixed by retrying (missing email, user not found) are
// detected early and returned via the first attempt without further retries.
func (s *SpecDrivenTaskService) syncGitIdentityAsync(task *types.SpecTask, userID, phaseLabel string, maxWait time.Duration) {
	if s.testMode || s.ExecInDesktop == nil || task.PlanningSessionID == "" || userID == "" {
		return
	}

	s.wg.Add(1)
	go func(taskID, sessionID string) {
		defer s.wg.Done()

		// Defensive panic recovery — this runs detached from the request.
		defer func() {
			if r := recover(); r != nil {
				log.Error().Interface("panic", r).Str("task_id", taskID).
					Msg("PANIC in syncGitIdentityAsync")
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), maxWait)
		defer cancel()

		backoff := 2 * time.Second
		attempts := 0
		for {
			attempts++
			err := s.syncGitIdentityToUser(ctx, task, userID, phaseLabel)
			if err == nil {
				if attempts > 1 {
					log.Info().Str("task_id", taskID).Int("attempts", attempts).Str("phase", phaseLabel).
						Msg("Git identity synced after container-ready retries")
				}
				return
			}

			// Non-retryable errors: user missing / email missing / lookup
			// failed — retrying won't fix them. Give up immediately so we
			// don't spam the logs for the full maxWait window.
			if isNonRetryableIdentityError(err) {
				log.Error().Err(err).Str("task_id", taskID).Str("phase", phaseLabel).
					Msg("Git identity sync failed with non-retryable error")
				return
			}

			select {
			case <-ctx.Done():
				log.Warn().Err(err).Str("task_id", taskID).Int("attempts", attempts).Str("phase", phaseLabel).
					Msg("Gave up syncing git identity after timeout; commits may not be attributed correctly")
				return
			case <-time.After(backoff):
			}

			// Gentle backoff, capped so we keep trying throughout the window.
			if backoff < 10*time.Second {
				backoff += 2 * time.Second
			}
		}
	}(task.ID, task.PlanningSessionID)
}

// Sentinel errors returned by syncGitIdentityToUser for conditions that won't
// change on retry. Classified via errors.Is so wrapping layers don't break the
// distinction.
var (
	errIdentityUserNotFound = errors.New("identity actor not found")
	errIdentityNoEmail      = errors.New("identity actor has no email")
)

// isNonRetryableIdentityError returns true for errors from syncGitIdentityToUser
// that won't change on retry: lookup-not-found and missing-email conditions.
// Container-not-ready (exec) errors return false so the async wrapper keeps
// retrying until the container comes up.
func isNonRetryableIdentityError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, errIdentityUserNotFound) || errors.Is(err, errIdentityNoEmail)
}

// buildRepositorySectionForTask fetches project and org repos, then builds the repository section
func (s *SpecDrivenTaskService) buildRepositorySectionForTask(ctx context.Context, task *types.SpecTask, project *types.Project) string {
	if task.ProjectID == "" {
		return ""
	}

	// Fetch project repos
	projectRepos, err := s.store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{
		ProjectID: task.ProjectID,
	})
	if err != nil {
		return ""
	}

	// Fetch Kodit org repos if enabled
	var koditOrgRepos []*types.GitRepository
	if project != nil && project.KoditEnabled && project.OrganizationID != "" {
		orgRepos, err := s.store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{
			OrganizationID: project.OrganizationID,
		})
		if err == nil {
			for _, repo := range orgRepos {
				if repo.KoditIndexing {
					koditOrgRepos = append(koditOrgRepos, repo)
				}
			}
		}
	}

	primaryRepoID := ""
	if project != nil {
		primaryRepoID = project.DefaultRepoID
	}

	return BuildRepositorySection(projectRepos, koditOrgRepos, primaryRepoID)
}

// Helper functions
func (s *SpecDrivenTaskService) selectZedAgent() string {
	// Simple round-robin for now
	// TODO: Implement proper load balancing
	if len(s.zedAgentPool) == 0 {
		return ""
	}
	return s.zedAgentPool[0]
}

func (s *SpecDrivenTaskService) markTaskFailed(ctx context.Context, task *types.SpecTask, errorMessage string) {
	s.markTaskFailedWithCause(ctx, task, errorMessage, nil)
}

// markTaskFailedErr records a failure whose cause the browser may need to act
// on. A missing subscription is not something a retry can fix — the user has to
// connect the provider first — so the reason is persisted in a machine-readable
// form alongside the message.
func (s *SpecDrivenTaskService) markTaskFailedErr(ctx context.Context, task *types.SpecTask, err error) {
	s.markTaskFailedWithCause(ctx, task, err.Error(), err)
}

func (s *SpecDrivenTaskService) markTaskFailedWithCause(ctx context.Context, task *types.SpecTask, errorMessage string, cause error) {
	// Keep task in backlog status but set error metadata
	now := time.Now()
	task.Status = types.TaskStatusBacklog
	task.StatusUpdatedAt = &now
	task.UpdatedAt = now

	// Store error in metadata
	if task.Metadata == nil {
		task.Metadata = make(map[string]interface{})
	}
	task.Metadata["error"] = errorMessage
	task.Metadata["error_timestamp"] = time.Now().Format(time.RFC3339)
	delete(task.Metadata, types.TaskErrorCodeKey)
	delete(task.Metadata, types.TaskErrorProviderKey)
	var missingSubscription *types.MissingSubscriptionError
	if errors.As(cause, &missingSubscription) {
		task.Metadata[types.TaskErrorCodeKey] = types.TaskErrorSubscriptionRequired
		task.Metadata[types.TaskErrorProviderKey] = missingSubscription.Provider
	}

	err := s.store.UpdateSpecTask(ctx, task)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Str("error", errorMessage).Msg("Failed to mark task with error")
	}
}

func generateTaskID() string {
	return system.GenerateSpecTaskID()
}

func GenerateTaskNameFromPrompt(prompt string) string {
	// Replace newlines and other whitespace with spaces to create clean task names
	// (prompts can contain newlines from multi-line input)
	name := strings.ReplaceAll(prompt, "\n", " ")
	name = strings.ReplaceAll(name, "\r", " ")
	name = strings.ReplaceAll(name, "\t", " ")
	// Collapse multiple spaces into one
	for strings.Contains(name, "  ") {
		name = strings.ReplaceAll(name, "  ", " ")
	}
	name = strings.TrimSpace(name)

	runes := []rune(name)
	if len(runes) > 60 {
		return string(runes[:57]) + "..."
	}
	return name
}

// isTaskInactive returns true if the task is in a terminal/inactive state
// (completed, failed, or archived) and should not block creating new tasks on the same branch
func isTaskInactive(task *types.SpecTask) bool {
	if task.Archived {
		return true
	}
	switch task.Status {
	case types.TaskStatusDone, types.TaskStatusSpecFailed, types.TaskStatusImplementationFailed:
		return true
	default:
		return false
	}
}

// detectAndLinkExistingPR checks if the branch has an open pull request and links it to the task
// Returns true if a PR was found and linked, false otherwise
// The task is updated in-place and saved to the database
func (s *SpecDrivenTaskService) detectAndLinkExistingPR(ctx context.Context, task *types.SpecTask, projectID, branchName string) bool {
	// Get project to find the default repository
	project, err := s.store.GetProject(ctx, projectID)
	if err != nil || project == nil {
		log.Warn().Err(err).Str("project_id", projectID).Msg("Failed to get project for PR detection")
		return false
	}

	if project.DefaultRepoID == "" {
		log.Debug().Str("project_id", projectID).Msg("Project has no default repo, skipping PR detection")
		return false
	}

	// List PRs from the repository
	prs, err := s.gitRepositoryService.ListPullRequests(ctx, project.DefaultRepoID)
	if err != nil {
		log.Warn().Err(err).Str("repo_id", project.DefaultRepoID).Msg("Failed to list PRs for detection")
		return false
	}

	// Find an open PR with matching source branch
	// ADO branch refs are like "refs/heads/branch-name"
	branchRef := "refs/heads/" + branchName
	for _, pr := range prs {
		// Check if PR is open (ADO uses "active" status)
		if pr.State != "active" {
			continue
		}

		// Check if source branch matches
		if pr.SourceBranch == branchRef || pr.SourceBranch == branchName {
			log.Info().
				Str("pr_id", pr.ID).
				Str("pr_title", pr.Title).
				Str("source_branch", pr.SourceBranch).
				Str("target_branch", pr.TargetBranch).
				Msg("Found existing PR for branch")

			// Get repo details for RepoPullRequests
			repo, repoErr := s.store.GetGitRepository(ctx, project.DefaultRepoID)
			repoName := ""
			if repoErr == nil && repo != nil {
				repoName = repo.Name
			}

			// Update task with PR info via RepoPullRequests
			now := time.Now()
			task.RepoPullRequests = append(task.RepoPullRequests, types.RepoPR{
				RepositoryID:   project.DefaultRepoID,
				RepositoryName: repoName,
				PRID:           pr.ID,
				PRNumber:       pr.Number,
				PRURL:          pr.URL,
				PRState:        string(pr.State),
			})
			task.Status = types.TaskStatusPullRequest
			task.StatusUpdatedAt = &now

			// Save updated task
			err = s.store.UpdateSpecTask(ctx, task)
			if err != nil {
				log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to update task with PR info")
				return false
			}

			return true
		}
	}

	return false
}

// SessionAPIKeyRequest specifies the scope for a session-scoped ephemeral API key.
// Keys are minted when a desktop starts and revoked when it shuts down.
// The SessionID is used for key lifecycle management (creation/revocation).
type SessionAPIKeyRequest struct {
	OrganizationID string
	UserID         string
	SessionID      string
}

// GetOrCreateSessionAPIKey gets or creates an ephemeral API key for a session.
// Keys are scoped to the session for lifecycle management (minted on start, revoked on stop).
// The key capabilities vary based on session type:
// - SpecTask sessions: git push rights to specific branch, LLM calls
// - Non-SpecTask sessions: LLM calls only
func (s *SpecDrivenTaskService) GetOrCreateSessionAPIKey(ctx context.Context, req *SessionAPIKeyRequest) (string, error) {
	if req.SessionID == "" {
		return "", fmt.Errorf("session ID is required for session-scoped API key")
	}

	// Check for existing session-scoped key
	existing, err := s.store.GetAPIKey(ctx, &types.ApiKey{
		OrganizationID: req.OrganizationID,
		Owner:          req.UserID,
		OwnerType:      types.OwnerTypeUser,
		SessionID:      req.SessionID,
	})
	if err != nil && err != store.ErrNotFound {
		return "", fmt.Errorf("failed to get existing API key: %w", err)
	}

	if existing != nil {
		return existing.Key, nil
	}

	// Look up session to derive scope for attribution
	session, err := s.store.GetSession(ctx, req.SessionID)
	if err != nil {
		return "", fmt.Errorf("failed to get session '%s': %w", req.SessionID, err)
	}

	// Derive project ID and spec task ID from session metadata
	var projectID, specTaskID string
	if session.Metadata.SpecTaskID != "" {
		specTaskID = session.Metadata.SpecTaskID
		specTask, err := s.store.GetSpecTask(ctx, specTaskID)
		if err != nil {
			log.Warn().Err(err).Str("spec_task_id", specTaskID).Msg("Failed to get spec task for API key attribution")
		} else {
			projectID = specTask.ProjectID
		}
	}

	newKey, err := system.GenerateAPIKey()
	if err != nil {
		return "", fmt.Errorf("failed to generate API key: %w", err)
	}

	// Create session-scoped ephemeral key
	keyName := fmt.Sprintf("Session key - %s", req.SessionID)
	if specTaskID != "" {
		keyName = fmt.Sprintf("Session key - %s (task: %s)", req.SessionID, specTaskID)
	}

	apiKey := &types.ApiKey{
		OrganizationID: req.OrganizationID,
		Owner:          req.UserID,
		OwnerType:      types.OwnerTypeUser,
		Key:            newKey,
		Name:           keyName,
		Type:           types.APIkeytypeAPI,
		SessionID:      req.SessionID,
		ProjectID:      projectID,  // For metrics/attribution
		SpecTaskID:     specTaskID, // For metrics/attribution
	}

	createdKey, err := s.store.CreateAPIKey(ctx, apiKey)
	if err != nil {
		return "", fmt.Errorf("failed to create API key: %w", err)
	}

	log.Info().
		Str("user_id", req.UserID).
		Str("session_id", req.SessionID).
		Str("project_id", projectID).
		Str("spec_task_id", specTaskID).
		Bool("is_spec_task", specTaskID != "").
		Msg("✅ Created ephemeral session API key")

	return createdKey.Key, nil
}

// RevokeSessionAPIKeys revokes all API keys associated with a session.
// This should be called when a desktop shuts down to clean up ephemeral keys.
func (s *SpecDrivenTaskService) RevokeSessionAPIKeys(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session ID is required to revoke session keys")
	}

	// Find all keys for this session
	keys, err := s.store.ListAPIKeys(ctx, &store.ListAPIKeysQuery{})
	if err != nil {
		return fmt.Errorf("failed to list API keys: %w", err)
	}

	var revokedCount int
	for _, key := range keys {
		if key.SessionID == sessionID {
			if err := s.store.DeleteAPIKey(ctx, key.Key); err != nil {
				log.Warn().Err(err).Str("key", key.Key[:8]+"...").Str("session_id", sessionID).Msg("Failed to revoke session API key")
				continue
			}
			revokedCount++
		}
	}

	if revokedCount > 0 {
		log.Info().
			Str("session_id", sessionID).
			Int("revoked_count", revokedCount).
			Msg("🔒 Revoked ephemeral session API keys")
	}

	return nil
}

// ResumeSession restarts a desktop container for an existing session
// Used by the reconciler to restart sessions after Wolf crash or sandbox restart
func (s *SpecDrivenTaskService) ResumeSession(ctx context.Context, task *types.SpecTask, session *types.Session) error {
	log.Info().
		Str("task_id", task.ID).
		Str("session_id", session.ID).
		Msg("Resuming session after container loss")

	// Get project for repository IDs and migrate legacy App-backed execution
	// configuration before recreating the agent container.
	var repositoryIDs []string
	var primaryRepoID string
	var project *types.Project
	if task.ProjectID != "" {
		var err error
		project, err = s.store.GetProject(ctx, task.ProjectID)
		if err != nil {
			log.Warn().Err(err).Str("project_id", task.ProjectID).Msg("Failed to get project for resume")
		} else if project != nil {
			repoIDs, err := s.store.GetRepositoriesForProject(ctx, project.ID)
			if err != nil {
				log.Warn().Err(err).Str("project_id", project.ID).Msg("Failed to get project repositories")
			} else {
				repositoryIDs = repoIDs
				// Set primary repo ID
				if project.DefaultRepoID != "" {
					primaryRepoID = project.DefaultRepoID
				} else if len(repositoryIDs) > 0 {
					primaryRepoID = repositoryIDs[0]
				}
			}
		}
	}
	if task.CodeAgentConfig == nil || task.HelixAppID != "" || task.CodeAgentOverrides != nil || session.ParentApp != "" || session.Metadata.CodeAgentOverrides != nil {
		if project == nil {
			return fmt.Errorf("project is required to migrate task code-agent configuration")
		}
		if err := s.migrateSpecTaskCodeAgentConfig(ctx, task, project); err != nil {
			return fmt.Errorf("failed to migrate task code-agent configuration before resume: %w", err)
		}
	}

	// API key creation is deferred to OnBeforeCreate hook (inside StartDesktop's
	// session lock) to prevent a race where StopDesktop revokes the key between
	// creation and container start.

	// Use display settings from session metadata or defaults
	displayWidth := session.Metadata.AgentVideoWidth
	displayHeight := session.Metadata.AgentVideoHeight
	displayRefreshRate := session.Metadata.AgentVideoRefreshRate
	if displayWidth == 0 {
		displayWidth = 2560
	}
	if displayHeight == 0 {
		displayHeight = 1600
	}
	if displayRefreshRate == 0 {
		displayRefreshRate = 60
	}

	desktopType := "ubuntu" // Default

	// Build the ZedAgent for restart
	zedAgent := &types.DesktopAgent{
		OrganizationID:      task.OrganizationID,
		SessionID:           session.ID,
		UserID:              task.UserID,
		Input:               "Resuming Zed development environment after container restart",
		ProjectPath:         "workspace",
		SpecTaskID:          task.ID,
		VCPUs:               sandboxVCPUs(task),
		MemoryMB:            sandboxMemoryMB(task),
		PrimaryRepositoryID: primaryRepoID,
		RepositoryIDs:       repositoryIDs,
		DisplayWidth:        displayWidth,
		DisplayHeight:       displayHeight,
		DisplayRefreshRate:  displayRefreshRate,
		Resolution:          fmt.Sprintf("%dx%d", displayWidth, displayHeight),
		ZoomLevel:           1.0,
		DesktopType:         desktopType,
		OnBeforeCreate: func(hookCtx context.Context, a *types.DesktopAgent) error {
			apiKey, err := s.GetOrCreateSessionAPIKey(hookCtx, &SessionAPIKeyRequest{
				OrganizationID: task.OrganizationID,
				UserID:         task.CreatedBy,
				SessionID:      session.ID,
			})
			if err != nil {
				return fmt.Errorf("failed to get session API key for resume: %w", err)
			}
			a.Env = append(a.Env, types.DesktopAgentAPIEnvVars(apiKey)...)
			return nil
		},
		BranchMode:    string(task.BranchMode),
		BaseBranch:    task.BaseBranch,
		WorkingBranch: task.BranchName,
	}

	// Start the desktop container
	agentResp, err := s.externalAgentExecutor.StartDesktop(ctx, zedAgent)
	if err != nil {
		return fmt.Errorf("failed to start desktop for resume: %w", err)
	}

	log.Info().
		Str("task_id", task.ID).
		Str("session_id", session.ID).
		Str("dev_container_id", agentResp.DevContainerID).
		Str("container_name", agentResp.ContainerName).
		Msg("Successfully resumed session with new container")

	return nil
}

// prepopulateClonedSpecs writes the cloned specs from DB to the helix-specs branch
// so the agent can read and adapt them. This is needed because cloned tasks have
// specs in DB fields but the target project's helix-specs doesn't have them yet.
func (s *SpecDrivenTaskService) prepopulateClonedSpecs(ctx context.Context, task *types.SpecTask, project *types.Project) error {
	// Get the repo to find the local path
	repo, err := s.store.GetGitRepository(ctx, project.DefaultRepoID)
	if err != nil {
		return fmt.Errorf("failed to get default repository: %w", err)
	}

	if repo.LocalPath == "" {
		return fmt.Errorf("repository has no local path")
	}

	// Get user info for the commit
	user, err := s.store.GetUser(ctx, &store.GetUserQuery{ID: task.CreatedBy})
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	authorName := user.GitAuthorName()
	if authorName == "" {
		authorName = "Helix"
	}
	authorEmail := user.GitAuthorEmail()
	if authorEmail == "" {
		authorEmail = "helix@helix.ml"
	}

	// Base path for design docs
	basePath := fmt.Sprintf("design/tasks/%s", task.DesignDocPath)

	// Use WithExternalRepoWrite to handle pre-sync, writes, post-push, and rollback
	// For task cloning, we use lenient options - don't fail task start if push fails
	if s.gitRepositoryService == nil {
		return fmt.Errorf("gitRepositoryService is required for prepopulateClonedSpecs")
	}

	writeErr := s.gitRepositoryService.WithExternalRepoWrite(
		ctx,
		repo,
		ExternalRepoWriteOptions{
			Branch:          SpecsBranchName,
			FailOnSyncError: true,  // Fail if we can't sync - prevents divergence
			FailOnPushError: false, // Don't fail task start on push error (but still rollback)
		},
		func() error {
			// Write requirements.md
			if task.RequirementsSpec != "" {
				filePath := basePath + "/requirements.md"
				_, _, err := CommitFileToBareBranch(
					ctx,
					repo.LocalPath,
					SpecsBranchName,
					filePath,
					[]byte(task.RequirementsSpec),
					authorName,
					authorEmail,
					fmt.Sprintf("Pre-populate cloned specs: requirements.md for %s", task.Name),
				)
				if err != nil {
					log.Warn().Err(err).Str("file", filePath).Msg("Failed to write cloned requirements.md")
				}
			}

			// Write design.md
			if task.TechnicalDesign != "" {
				filePath := basePath + "/design.md"
				_, _, err := CommitFileToBareBranch(
					ctx,
					repo.LocalPath,
					SpecsBranchName,
					filePath,
					[]byte(task.TechnicalDesign),
					authorName,
					authorEmail,
					fmt.Sprintf("Pre-populate cloned specs: design.md for %s", task.Name),
				)
				if err != nil {
					log.Warn().Err(err).Str("file", filePath).Msg("Failed to write cloned design.md")
				}
			}

			// Write tasks.md
			if task.ImplementationPlan != "" {
				filePath := basePath + "/tasks.md"
				_, _, err := CommitFileToBareBranch(
					ctx,
					repo.LocalPath,
					SpecsBranchName,
					filePath,
					[]byte(task.ImplementationPlan),
					authorName,
					authorEmail,
					fmt.Sprintf("Pre-populate cloned specs: tasks.md for %s", task.Name),
				)
				if err != nil {
					log.Warn().Err(err).Str("file", filePath).Msg("Failed to write cloned tasks.md")
				}
			}

			// Copy attachments from the source task's directory into the cloned task's
			// directory. Files live at design/tasks/<src-design-doc>/attachments/* and
			// we re-create them under <new-design-doc>/attachments/* + a matching
			// SpecTaskAttachment row so the cloned task lists them in its prompt too.
			if err := s.cloneAttachmentsInRepo(ctx, task, repo, authorName, authorEmail); err != nil {
				log.Warn().Err(err).Str("task_id", task.ID).Msg("Failed to clone attachments — cloned task will lack them")
			}

			log.Info().
				Str("task_id", task.ID).
				Str("cloned_from", task.ClonedFromID).
				Str("design_doc_path", task.DesignDocPath).
				Msg("Pre-populated cloned specs in helix-specs branch")

			return nil
		},
	)
	if writeErr != nil {
		return writeErr
	}

	return nil
}
