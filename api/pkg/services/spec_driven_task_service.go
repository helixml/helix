package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/controller"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
)

// Spec-driven development: Specs worktree paths (relative to repository root)
const (
	SpecsWorktreeRelPath = "design"                // Relative path from repo root
	SpecsBranchName      = "helix-specs"           // Git branch name for spec-driven development
	SpecsTaskDirFormat   = "design/tasks/%s_%s_%s" // Format: tasks/DATE_NAME_ID
)

// RequestMappingRegistrar is a function type for registering request-to-session mappings
type RequestMappingRegistrar func(requestID, sessionID string)

// SpecDrivenTaskService manages the spec-driven development workflow:
// Specification: Helix agent generates specs from simple descriptions
// Implementation: Zed agent implements code from approved specs
type SpecDrivenTaskService struct {
	store                    store.Store
	controller               *controller.Controller
	externalAgentExecutor    external_agent.Executor      // Wolf executor for launching external agents
	gitRepositoryService     *GitRepositoryService        // Service for git repository operations
	RegisterRequestMapping   RequestMappingRegistrar      // Callback to register request-to-session mappings
	SendMessageToAgent       SpecTaskMessageSender        // Callback to send messages to agents via WebSocket
	helixAgentID             string                       // ID of Helix agent for spec generation
	zedAgentPool             []string                     // Pool of available Zed agents
	testMode                 bool                         // If true, skip async operations for testing
	MultiSessionManager      *SpecTaskMultiSessionManager // Manager for multi-session workflows
	ZedIntegrationService    *ZedIntegrationService       // Service for Zed instance and thread management
	DocumentHandoffService   *DocumentHandoffService      // Service for git-based document handoff
	SpecDocumentService      *SpecDocumentService         // Service for Kiro-style document generation
	ZedToHelixSessionService *ZedToHelixSessionService    // Service for Zed→Helix session creation
	SessionContextService    *SessionContextService       // Service for inter-session coordination
	auditLogService          *AuditLogService             // Service for audit logging
	wg                       sync.WaitGroup
}

// NewSpecDrivenTaskService creates a new service instance
func NewSpecDrivenTaskService(
	store store.Store,
	controller *controller.Controller,
	helixAgentID string,
	zedAgentPool []string,
	pubsub pubsub.PubSub,
	externalAgentExecutor external_agent.Executor,
	gitRepositoryService *GitRepositoryService,
	registerRequestMapping RequestMappingRegistrar,
) *SpecDrivenTaskService {
	service := &SpecDrivenTaskService{
		store:                  store,
		controller:             controller,
		externalAgentExecutor:  externalAgentExecutor,
		gitRepositoryService:   gitRepositoryService,
		RegisterRequestMapping: registerRequestMapping,
		helixAgentID:           helixAgentID,
		zedAgentPool:           zedAgentPool,
		testMode:               false,
		auditLogService:        NewAuditLogService(store),
	}

	// Initialize Zed integration service
	service.ZedIntegrationService = NewZedIntegrationService(
		store,
		controller,
		pubsub,
	)

	// Initialize document services
	service.SpecDocumentService = NewSpecDocumentService(
		store,
		"/workspace/git",  // Default git base path
		"Helix System",    // Default git user name
		"system@helix.ml", // Default git email
	)

	service.SessionContextService = NewSessionContextService(store)

	service.DocumentHandoffService = NewDocumentHandoffService(
		store,
		service.SpecDocumentService,
		nil, // Will be set after MultiSessionManager is created
		"/workspace/git",
		"Helix System",
		"system@helix.ml",
	)

	// Initialize multi-session manager
	var defaultImplementationApp string
	if len(zedAgentPool) > 0 {
		defaultImplementationApp = zedAgentPool[0]
	}

	service.MultiSessionManager = NewSpecTaskMultiSessionManager(
		store,
		controller,
		service,
		service.ZedIntegrationService,
		defaultImplementationApp,
	)

	// Set MultiSessionManager reference in DocumentHandoffService
	service.DocumentHandoffService.multiSessionManager = service.MultiSessionManager

	// Initialize Zed-to-Helix session service
	service.ZedToHelixSessionService = NewZedToHelixSessionService(
		store,
		service.MultiSessionManager,
		service.SessionContextService,
	)

	return service
}

// SetTestMode enables or disables test mode (prevents async operations)
func (s *SpecDrivenTaskService) SetTestMode(enabled bool) {
	s.testMode = enabled
	if s.MultiSessionManager != nil {
		s.MultiSessionManager.SetTestMode(enabled)
	}
	if s.ZedIntegrationService != nil {
		s.ZedIntegrationService.SetTestMode(enabled)
	}
	if s.DocumentHandoffService != nil {
		s.DocumentHandoffService.SetTestMode(enabled)
	}
	if s.SpecDocumentService != nil {
		s.SpecDocumentService.SetTestMode(enabled)
	}
	if s.ZedToHelixSessionService != nil {
		s.ZedToHelixSessionService.SetTestMode(enabled)
	}
	if s.SessionContextService != nil {
		s.SessionContextService.SetTestMode(enabled)
	}
}

// CreateTaskFromPrompt creates a new task in the backlog and kicks off spec generation
func (s *SpecDrivenTaskService) CreateTaskFromPrompt(ctx context.Context, req *types.CreateTaskRequest) (*types.SpecTask, error) {
	// Determine which agent to use (single agent for entire workflow)
	helixAppID := s.helixAgentID
	if req.AppID != "" {
		helixAppID = req.AppID
	}

	// Default branch mode to "new" if not specified
	branchMode := req.BranchMode
	if branchMode == "" {
		branchMode = types.BranchModeNew
	}

	task := &types.SpecTask{
		ID:             generateTaskID(),
		ProjectID:      req.ProjectID,
		Name:           generateTaskNameFromPrompt(req.Prompt),
		Description:    req.Prompt,
		Type:           req.Type,
		Priority:       req.Priority,
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: req.Prompt,
		CreatedBy:      req.UserID,
		HelixAppID:     helixAppID,        // Helix agent used for entire workflow
		JustDoItMode:   req.JustDoItMode,  // Set Just Do It mode from request
		UseHostDocker:  req.UseHostDocker, // Use host Docker socket (requires privileged sandbox)
		// Branch configuration
		BranchMode:   branchMode,
		BaseBranch:   req.BaseBranch,    // User-specified base branch (empty = use repo default)
		BranchPrefix: req.BranchPrefix,  // User-specified prefix for new branches
		BranchName:   req.WorkingBranch, // For existing mode, this is the branch to continue on
		// Repositories inherited from parent project - no task-level repo configuration
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Store the task
	err := s.store.CreateSpecTask(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
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
func (s *SpecDrivenTaskService) StartSpecGeneration(ctx context.Context, task *types.SpecTask, opts types.StartPlanningOptions) {
	// Add panic recovery for debugging
	defer func() {
		if r := recover(); r != nil {
			log.Error().Interface("panic", r).Str("task_id", task.ID).Msg("PANIC in StartSpecGeneration")
		}
	}()

	log.Debug().Str("task_id", task.ID).Str("helix_app_id", task.HelixAppID).Msg("DEBUG: StartSpecGeneration entered")

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

	// Ensure HelixAppID is set - inherit from project default, then fall back to system default
	helixAppIDChanged := false
	if task.HelixAppID == "" {
		// First try project's default agent
		if project != nil && project.DefaultHelixAppID != "" {
			task.HelixAppID = project.DefaultHelixAppID
			helixAppIDChanged = true
			log.Info().
				Str("task_id", task.ID).
				Str("helix_app_id", project.DefaultHelixAppID).
				Msg("Inherited HelixAppID from project default")
		} else {
			// Fall back to system default
			task.HelixAppID = s.helixAgentID
			helixAppIDChanged = true
			log.Debug().Str("task_id", task.ID).Str("helix_app_id", s.helixAgentID).Msg("Set system default HelixAppID")
		}
	}

	log.Info().
		Str("task_id", task.ID).
		Str("original_prompt", task.OriginalPrompt).
		Str("helix_app_id", task.HelixAppID).
		Msg("Starting spec generation")

	// Assign task number and design doc path if not already set
	if task.TaskNumber == 0 && task.ProjectID != "" {
		taskNumber, err := s.store.IncrementProjectTaskNumber(ctx, task.ProjectID)
		if err != nil {
			log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to get task number, using fallback")
			// Fallback: use a hash of task ID for uniqueness
			taskNumber = 1
		}
		task.TaskNumber = taskNumber
		// Generate unique design doc path (checks for collisions across all projects)
		designDocPath, err := GenerateUniqueDesignDocPath(ctx, s.store, task, taskNumber)
		if err != nil {
			log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to generate unique design doc path, using fallback")
			designDocPath = GenerateDesignDocPath(task, taskNumber)
		}
		task.DesignDocPath = designDocPath
		log.Info().
			Str("task_id", task.ID).
			Int("task_number", taskNumber).
			Str("design_doc_path", task.DesignDocPath).
			Msg("Assigned task number and design doc path")
	}

	// Clear any previous error from metadata (in case this is a retry)
	if task.Metadata != nil {
		delete(task.Metadata, "error")
		delete(task.Metadata, "error_timestamp")
	}

	// Update task status (SpecAgent already set in CreateTaskFromPrompt)
	task.Status = types.TaskStatusSpecGeneration
	task.UpdatedAt = time.Now()

	err := s.store.UpdateSpecTask(ctx, task)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to update task status")
		return
	}

	// If we inherited the agent ID, it's now persisted via the UpdateSpecTask above
	if helixAppIDChanged {
		log.Debug().Str("task_id", task.ID).Str("helix_app_id", task.HelixAppID).Msg("HelixAppID persisted to task")
	}

	// Get primary repo name for the planning prompt (to reference in Code-Ref commits)
	primaryRepoName := ""
	if project != nil && project.DefaultRepoID != "" {
		repo, err := s.store.GetGitRepository(ctx, project.DefaultRepoID)
		if err == nil && repo != nil {
			primaryRepoName = repo.Name
		}
	}

	// Build planning instructions as the message (not system prompt - agent has its own system prompt)
	planningPrompt := BuildPlanningPrompt(task, guidelines, primaryRepoName)

	// Get CodeAgentRuntime from the app config (needed for session resume to select correct agent)
	codeAgentRuntime := s.getCodeAgentRuntimeForTask(ctx, task)

	sessionMetadata := types.SessionMetadata{
		SystemPrompt:     "",             // Don't override agent's system prompt
		AgentType:        "zed_external", // Use Zed agent for git access
		Stream:           false,
		SpecTaskID:       task.ID,           // CRITICAL: Set SpecTaskID so session restore uses correct workspace path
		CodeAgentRuntime: codeAgentRuntime, // For open_thread on resume
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
		ParentApp:      task.HelixAppID, // Use the Helix agent for entire workflow
		OrganizationID: orgID,
		Metadata:       sessionMetadata,
		OwnerType:      types.OwnerTypeUser,
	}

	// Create the session in the database
	if s.controller == nil || s.controller.Options.Store == nil {
		log.Error().Str("task_id", task.ID).Msg("Controller or store not available for spec generation")
		s.markTaskFailed(ctx, task, "Controller or store not available for spec generation")
		return
	}

	session, err = s.controller.Options.Store.CreateSession(ctx, *session)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to create spec generation session")
		s.markTaskFailed(ctx, task, fmt.Sprintf("Failed to create spec generation session: %v", err))
		return
	}

	// Update task with session ID
	log.Debug().Str("task_id", task.ID).Str("session_id", session.ID).Msg("DEBUG: About to update task with session ID")
	task.PlanningSessionID = session.ID
	err = s.store.UpdateSpecTask(ctx, task)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to update task with session ID")
		s.markTaskFailed(ctx, task, fmt.Sprintf("Failed to update task with session ID: %v", err))
		return
	}
	log.Debug().Str("task_id", task.ID).Msg("DEBUG: Task updated with session ID")

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
	fullMessage := planningPrompt + "\n\n**User Request:**\n" + task.OriginalPrompt

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
	_, err = s.controller.Options.Store.CreateInteraction(ctx, interaction)
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

	// Build list of all repository IDs to clone from project
	repositoryIDs := []string{}
	for _, repo := range projectRepos {
		if repo.ID != "" {
			repositoryIDs = append(repositoryIDs, repo.ID)
		}
	}

	// Determine primary repository from project configuration
	primaryRepoID := project.DefaultRepoID
	if primaryRepoID == "" && len(projectRepos) > 0 {
		// Use first project repo as fallback if no default set
		primaryRepoID = projectRepos[0].ID
	}

	// Get user's personal API token for git operations (not app-scoped keys)
	userAPIKey, err := s.GetOrCreateSandboxAPIKey(ctx, &SandboxAPIKeyRequest{
		UserID:     task.CreatedBy,
		ProjectID:  task.ProjectID,
		SpecTaskID: task.ID,
	})
	if err != nil {
		log.Error().Err(err).Str("user_id", task.CreatedBy).Msg("Failed to get user API key for SpecTask")
		s.markTaskFailed(ctx, task, fmt.Sprintf("Failed to get user API key: %v", err))
		return
	}

	// Get display settings from app's ExternalAgentConfig (or use defaults)
	displayWidth := 1920
	displayHeight := 1080
	displayRefreshRate := 60
	resolution := ""
	zoomLevel := 0
	desktopType := ""
	if task.HelixAppID != "" {
		app, err := s.store.GetApp(ctx, task.HelixAppID)
		if err == nil && app != nil && app.Config.Helix.ExternalAgentConfig != nil {
			width, height := app.Config.Helix.ExternalAgentConfig.GetEffectiveResolution()
			displayWidth = width
			displayHeight = height
			if app.Config.Helix.ExternalAgentConfig.DisplayRefreshRate > 0 {
				displayRefreshRate = app.Config.Helix.ExternalAgentConfig.DisplayRefreshRate
			}
			// CRITICAL: Also get resolution preset, zoom level, and desktop type for proper HiDPI scaling
			resolution = app.Config.Helix.ExternalAgentConfig.Resolution
			zoomLevel = app.Config.Helix.ExternalAgentConfig.GetEffectiveZoomLevel()
			desktopType = app.Config.Helix.ExternalAgentConfig.GetEffectiveDesktopType()
			log.Debug().
				Str("task_id", task.ID).
				Int("display_width", displayWidth).
				Int("display_height", displayHeight).
				Int("display_refresh_rate", displayRefreshRate).
				Str("resolution", resolution).
				Int("zoom_level", zoomLevel).
				Str("desktop_type", desktopType).
				Msg("Using display settings from app's ExternalAgentConfig")
		}
	}

	// Create ZedAgent struct with session info for Wolf executor
	log.Debug().Str("task_id", task.ID).Msg("DEBUG: About to create ZedAgent struct")
	zedAgent := &types.ZedAgent{
		SessionID:           session.ID,
		HelixSessionID:      session.ID, // CRITICAL: Use planning session for settings-sync-daemon to fetch correct CodeAgentConfig
		UserID:              task.CreatedBy,
		Input:               "Initialize Zed development environment for spec generation",
		ProjectPath:         "workspace",        // Use relative path
		SpecTaskID:          task.ID,            // For task-scoped workspace
		PrimaryRepositoryID: primaryRepoID,      // Primary repo to open in Zed
		RepositoryIDs:       repositoryIDs,      // ALL project repos to checkout
		UseHostDocker:       task.UseHostDocker, // Use host Docker socket if requested
		DisplayWidth:        displayWidth,
		DisplayHeight:       displayHeight,
		DisplayRefreshRate:  displayRefreshRate,
		Resolution:          resolution,
		ZoomLevel:           zoomLevel,
		DesktopType:         desktopType,
		Env:                 buildEnvWithLocale(userAPIKey, opts),
	}
	log.Debug().Str("task_id", task.ID).Str("session_id", session.ID).Str("helix_session_id", zedAgent.HelixSessionID).Msg("DEBUG: Created ZedAgent struct")

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
		s.markTaskFailed(ctx, task, err.Error())
		return
	}

	log.Info().
		Str("task_id", task.ID).
		Str("session_id", session.ID).
		Str("planning_session_id", task.PlanningSessionID).
		Str("wolf_lobby_id", agentResp.WolfLobbyID).
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
func (s *SpecDrivenTaskService) StartJustDoItMode(ctx context.Context, task *types.SpecTask, opts types.StartPlanningOptions) {
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

	// Ensure HelixAppID is set - inherit from project default, then fall back to system default
	if task.HelixAppID == "" {
		// First try project's default agent
		if project != nil && project.DefaultHelixAppID != "" {
			task.HelixAppID = project.DefaultHelixAppID
			log.Info().
				Str("task_id", task.ID).
				Str("helix_app_id", project.DefaultHelixAppID).
				Msg("Inherited HelixAppID from project default")
		} else {
			// Fall back to system default
			task.HelixAppID = s.helixAgentID
			log.Debug().Str("task_id", task.ID).Str("helix_app_id", s.helixAgentID).Msg("Set system default HelixAppID")
		}
	}

	log.Info().
		Str("task_id", task.ID).
		Str("original_prompt", task.OriginalPrompt).
		Str("helix_app_id", task.HelixAppID).
		Msg("Starting Just Do It mode - skipping spec generation")

	// Assign task number and design doc path if not already set
	if task.TaskNumber == 0 && task.ProjectID != "" {
		taskNumber, err := s.store.IncrementProjectTaskNumber(ctx, task.ProjectID)
		if err != nil {
			log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to get task number, using fallback")
			taskNumber = 1
		}
		task.TaskNumber = taskNumber
		// Generate unique design doc path (checks for collisions across all projects)
		designDocPath, err := GenerateUniqueDesignDocPath(ctx, s.store, task, taskNumber)
		if err != nil {
			log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to generate unique design doc path, using fallback")
			designDocPath = GenerateDesignDocPath(task, taskNumber)
		}
		task.DesignDocPath = designDocPath
		log.Info().
			Str("task_id", task.ID).
			Int("task_number", taskNumber).
			Str("design_doc_path", task.DesignDocPath).
			Msg("Assigned task number and design doc path")
	}

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
	// NOTE: If HelixAppID was inherited from project, it will be persisted here
	task.Status = types.TaskStatusImplementation
	task.BranchName = branchName
	task.UpdatedAt = time.Now()
	now := time.Now()
	task.StartedAt = &now

	err := s.store.UpdateSpecTask(ctx, task)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to update task status for Just Do It mode")
		return
	}

	// Get CodeAgentRuntime from the app config (needed for session resume to select correct agent)
	codeAgentRuntimeJDI := s.getCodeAgentRuntimeForTask(ctx, task)

	sessionMetadata := types.SessionMetadata{
		SystemPrompt:     "",             // Don't override agent's system prompt
		AgentType:        "zed_external", // Use Zed agent for git access
		Stream:           false,
		SpecTaskID:       task.ID,              // CRITICAL: Set SpecTaskID so session restore uses correct workspace path
		CodeAgentRuntime: codeAgentRuntimeJDI, // For open_thread on resume
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
		ParentApp:      task.HelixAppID, // Use the Helix agent for workflow
		OrganizationID: orgID,
		Metadata:       sessionMetadata,
		OwnerType:      types.OwnerTypeUser,
	}

	// Create the session in the database
	if s.controller == nil || s.controller.Options.Store == nil {
		log.Error().Str("task_id", task.ID).Msg("Controller or store not available for Just Do It mode")
		s.markTaskFailed(ctx, task, "Controller or store not available")
		return
	}

	session, err = s.controller.Options.Store.CreateSession(ctx, *session)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to create Just Do It session")
		s.markTaskFailed(ctx, task, fmt.Sprintf("Failed to create session: %v", err))
		return
	}

	// Update task with session ID (use PlanningSessionID since it's the primary session)
	task.PlanningSessionID = session.ID
	err = s.store.UpdateSpecTask(ctx, task)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to update task with session ID")
		s.markTaskFailed(ctx, task, fmt.Sprintf("Failed to update task with session ID: %v", err))
		return
	}

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

	// Build git instructions based on branch mode
	var gitInstructions string
	if task.BranchMode == types.BranchModeExisting {
		// Continuing work on existing branch
		gitInstructions = fmt.Sprintf(`**If making code changes:**
1. git checkout %s  (continue on existing branch)
2. Make your changes
3. git push origin %s`, branchName, branchName)
	} else {
		// Creating new branch from base
		baseBranch := task.BaseBranch
		if baseBranch == "" {
			// Use the primary repo's default branch as fallback
			for _, repo := range projectRepos {
				if repo.ID == primaryRepoID && repo.DefaultBranch != "" {
					baseBranch = repo.DefaultBranch
					break
				}
			}
		}
		if baseBranch == "" {
			// Last resort: use "main" but log a warning
			baseBranch = "main"
			log.Warn().
				Str("task_id", task.ID).
				Str("project_id", task.ProjectID).
				Msg("No base branch or default branch configured, falling back to 'main'")
		}
		gitInstructions = fmt.Sprintf(`**If making code changes:**
1. git fetch origin && git checkout %s && git pull origin %s
2. git checkout -b %s  (create new branch from %s)
3. Make your changes
4. git push origin %s`, baseBranch, baseBranch, branchName, baseBranch, branchName)
	}

	promptWithBranch := fmt.Sprintf(`%s
%s
---

**Working in /home/retro/work/:** All code repositories are in /home/retro/work/. That's where you make changes.

**Primary Project Directory:** /home/retro/work/%s/

**Shell commands:** Specify is_background (true or false) on all shell commands - it's required. Use true for long-running operations (builds, servers, installs).

%s

**For persistent installs:** Add commands to /home/retro/work/helix-specs/.helix/startup.sh (runs at sandbox startup, must be idempotent). Push directly to helix-specs branch.
`, task.OriginalPrompt, guidelinesSection, primaryRepoName, gitInstructions)

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

	_, err = s.controller.Options.Store.CreateInteraction(ctx, interaction)
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

	// Get user's personal API token for git operations
	userAPIKey, err := s.GetOrCreateSandboxAPIKey(ctx, &SandboxAPIKeyRequest{
		UserID:     task.CreatedBy,
		ProjectID:  task.ProjectID,
		SpecTaskID: task.ID,
	})
	if err != nil {
		log.Error().Err(err).Str("user_id", task.CreatedBy).Msg("Failed to get user API key for Just Do It task")
		s.markTaskFailed(ctx, task, fmt.Sprintf("Failed to get user API key: %v", err))
		return
	}

	// Get display settings from app's ExternalAgentConfig (or use defaults)
	displayWidthJDI := 1920
	displayHeightJDI := 1080
	displayRefreshRateJDI := 60
	resolutionJDI := ""
	zoomLevelJDI := 0
	desktopTypeJDI := ""
	if task.HelixAppID != "" {
		app, err := s.store.GetApp(ctx, task.HelixAppID)
		if err == nil && app != nil && app.Config.Helix.ExternalAgentConfig != nil {
			width, height := app.Config.Helix.ExternalAgentConfig.GetEffectiveResolution()
			displayWidthJDI = width
			displayHeightJDI = height
			if app.Config.Helix.ExternalAgentConfig.DisplayRefreshRate > 0 {
				displayRefreshRateJDI = app.Config.Helix.ExternalAgentConfig.DisplayRefreshRate
			}
			// CRITICAL: Also get resolution preset, zoom level, and desktop type for proper HiDPI scaling
			resolutionJDI = app.Config.Helix.ExternalAgentConfig.Resolution
			zoomLevelJDI = app.Config.Helix.ExternalAgentConfig.GetEffectiveZoomLevel()
			desktopTypeJDI = app.Config.Helix.ExternalAgentConfig.GetEffectiveDesktopType()
			log.Debug().
				Str("task_id", task.ID).
				Int("display_width", displayWidthJDI).
				Int("display_height", displayHeightJDI).
				Int("display_refresh_rate", displayRefreshRateJDI).
				Str("resolution", resolutionJDI).
				Int("zoom_level", zoomLevelJDI).
				Str("desktop_type", desktopTypeJDI).
				Msg("Just Do It: Using display settings from app's ExternalAgentConfig")
		}
	}

	// Create ZedAgent struct with session info for Wolf executor
	zedAgent := &types.ZedAgent{
		SessionID:           session.ID,
		HelixSessionID:      session.ID, // CRITICAL: Use planning session for settings-sync-daemon to fetch correct CodeAgentConfig
		UserID:              task.CreatedBy,
		Input:               "Initialize Zed development environment",
		ProjectPath:         "workspace",        // Use relative path
		SpecTaskID:          task.ID,            // For task-scoped workspace
		PrimaryRepositoryID: primaryRepoID,      // Primary repo to open in Zed
		RepositoryIDs:       repositoryIDs,      // ALL project repos to checkout
		UseHostDocker:       task.UseHostDocker, // Use host Docker socket if requested
		DisplayWidth:        displayWidthJDI,
		DisplayHeight:       displayHeightJDI,
		DisplayRefreshRate:  displayRefreshRateJDI,
		Resolution:          resolutionJDI,
		ZoomLevel:           zoomLevelJDI,
		DesktopType:         desktopTypeJDI,
		Env:                 buildEnvWithLocale(userAPIKey, opts),
	}

	// Start the Zed agent via Wolf executor
	agentResp, err := s.externalAgentExecutor.StartDesktop(ctx, zedAgent)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Str("session_id", session.ID).Msg("Failed to launch external agent for Just Do It mode")
		s.markTaskFailed(ctx, task, err.Error())
		return
	}

	log.Info().
		Str("task_id", task.ID).
		Str("session_id", session.ID).
		Str("branch_name", branchName).
		Str("wolf_lobby_id", agentResp.WolfLobbyID).
		Str("container_name", agentResp.ContainerName).
		Msg("Just Do It mode: Zed agent launched with branch instructions")

	// Log audit event for agent started (now that session is created)
	if s.auditLogService != nil {
		s.auditLogService.LogAgentStarted(ctx, task, session.ID, task.CreatedBy, "")
	}
}

// buildEnvWithLocale constructs the environment variable array for desktop containers
// Includes the API token and optional locale settings (keyboard layout, timezone)
func buildEnvWithLocale(userAPIKey string, opts types.StartPlanningOptions) []string {
	env := []string{
		fmt.Sprintf("USER_API_TOKEN=%s", userAPIKey),
	}

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
	task.RequirementsSpec = specs.RequirementsSpec
	task.TechnicalDesign = specs.TechnicalDesign
	task.ImplementationPlan = specs.ImplementationPlan
	task.Status = types.TaskStatusSpecReview
	task.UpdatedAt = time.Now()

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
	if s.controller != nil && s.controller.Options.Notifier != nil {
		// Note: The notification system expects a session, but for task notifications we'll create a minimal one
		session := &types.Session{
			ID:    task.PlanningSessionID,
			Owner: task.CreatedBy,
		}

		notificationPayload := &types.Notification{
			Session: session,
			Event:   types.EventCronTriggerComplete,
		}

		if err := s.controller.Options.Notifier.Notify(ctx, notificationPayload); err != nil {
			log.Error().Err(err).Str("task_id", taskID).Msg("Failed to send spec review notification")
			// Don't fail the whole operation if notification fails
		}
	}

	return nil
}

// ApproveSpecs handles human approval of generated specs
func (s *SpecDrivenTaskService) ApproveSpecs(ctx context.Context, req *types.SpecApprovalResponse) error {
	task, err := s.store.GetSpecTask(ctx, req.TaskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	if req.Approved {
		// Get project and repository info
		project, err := s.store.GetProject(ctx, task.ProjectID)
		if err != nil {
			return fmt.Errorf("failed to get project: %w", err)
		}

		// Ensure HelixAppID is set - inherit from project default for old tasks
		if task.HelixAppID == "" && project.DefaultHelixAppID != "" {
			task.HelixAppID = project.DefaultHelixAppID
			log.Info().
				Str("task_id", task.ID).
				Str("helix_app_id", project.DefaultHelixAppID).
				Msg("[ApproveSpecs] Inherited HelixAppID from project default")

			// Also update the planning session's ParentApp if it was empty
			if task.PlanningSessionID != "" {
				session, sessionErr := s.store.GetSession(ctx, task.PlanningSessionID)
				if sessionErr == nil && session != nil && session.ParentApp == "" {
					session.ParentApp = task.HelixAppID
					if _, updateErr := s.store.UpdateSession(ctx, *session); updateErr != nil {
						log.Warn().Err(updateErr).Str("session_id", session.ID).Msg("Failed to update session ParentApp (continuing)")
					} else {
						log.Info().Str("session_id", session.ID).Str("parent_app", task.HelixAppID).Msg("[ApproveSpecs] Updated session ParentApp")
					}
				}
			}
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
					return fmt.Errorf(FormatDivergenceErrorForUser(divergeErr, repo.Name))
				}
				log.Error().Err(err).Str("repo_id", repo.ID).Str("branch", repo.DefaultBranch).Msg("Failed to sync from remote")
				return fmt.Errorf("failed to sync base branch from external repository '%s': %w", repo.ExternalURL, err)
			}
		}

		// Handle branch configuration based on mode
		var branchName string
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
		}

		// Specs approved - move to implementation
		task.Status = types.TaskStatusImplementation
		task.BranchName = branchName
		task.SpecApprovedBy = req.ApprovedBy
		task.SpecApprovedAt = &req.ApprovedAt
		now := time.Now()
		task.StartedAt = &now

		err = s.store.UpdateSpecTask(ctx, task)
		if err != nil {
			return fmt.Errorf("failed to update task approval: %w", err)
		}

		// Send instruction to existing agent session (reuse planning session)
		sessionID := task.PlanningSessionID

		if sessionID != "" && !s.testMode {
			// Create agent instruction service
			agentInstructionService := NewAgentInstructionService(s.store)

			// Send approval instruction asynchronously (don't block the response)
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()

				err := agentInstructionService.SendApprovalInstruction(
					context.Background(),
					sessionID,
					task.CreatedBy, // User who created the task
					task,
					branchName,
					repo.DefaultBranch,
					repo.Name,
				)
				if err != nil {
					log.Error().
						Err(err).
						Str("task_id", task.ID).
						Str("session_id", sessionID).
						Msg("Failed to send approval instruction to agent")
				}
			}()

			log.Info().
				Str("task_id", task.ID).
				Str("session_id", sessionID).
				Str("branch_name", branchName).
				Msg("Specs approved - sent implementation instruction to existing agent")
		} else {
			log.Warn().
				Str("task_id", task.ID).
				Msg("No planning session ID found - agent will not receive implementation instruction")
		}

	} else {
		// Specs need revision
		task.Status = types.TaskStatusSpecRevision
		task.SpecRevisionCount++

		// Ensure HelixAppID is set - inherit from project default for old tasks
		if task.HelixAppID == "" {
			project, projErr := s.store.GetProject(ctx, task.ProjectID)
			if projErr == nil && project != nil && project.DefaultHelixAppID != "" {
				task.HelixAppID = project.DefaultHelixAppID
				log.Info().
					Str("task_id", task.ID).
					Str("helix_app_id", project.DefaultHelixAppID).
					Msg("[RequestRevision] Inherited HelixAppID from project default")

				// Also update the planning session's ParentApp if it was empty
				if task.PlanningSessionID != "" {
					session, sessionErr := s.store.GetSession(ctx, task.PlanningSessionID)
					if sessionErr == nil && session != nil && session.ParentApp == "" {
						session.ParentApp = task.HelixAppID
						if _, updateErr := s.store.UpdateSession(ctx, *session); updateErr != nil {
							log.Warn().Err(updateErr).Str("session_id", session.ID).Msg("Failed to update session ParentApp (continuing)")
						} else {
							log.Info().Str("session_id", session.ID).Str("parent_app", task.HelixAppID).Msg("[RequestRevision] Updated session ParentApp")
						}
					}
				}
			}
		}

		err = s.store.UpdateSpecTask(ctx, task)
		if err != nil {
			return fmt.Errorf("failed to update task for revision: %w", err)
		}

		// Send revision instruction to existing agent session via WebSocket
		if s.SendMessageToAgent != nil && !s.testMode {
			go func(t *types.SpecTask, comments string) {
				message := BuildRevisionInstructionPrompt(t, comments)
				_, err := s.SendMessageToAgent(context.Background(), t, message, "")
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
			}(task, req.Comments)
		} else if !s.testMode {
			log.Warn().
				Str("task_id", task.ID).
				Msg("No message sender configured - agent will not receive revision instruction")
		}

		// Log audit event for review comment (revision request)
		if s.auditLogService != nil && req.Comments != "" {
			// reviewID=planningSessionID, commentID=empty (revision not a specific comment), commentText, userID, userEmail
			s.auditLogService.LogReviewComment(ctx, task, task.PlanningSessionID, "", req.Comments, req.ApprovedBy, "")
		}
	}

	return nil
}

// sanitizeBranchName makes a branch name git-safe
func sanitizeBranchName(name string) string {
	// Replace spaces with hyphens
	name = strings.ReplaceAll(name, " ", "-")
	// Remove special characters except hyphens and underscores
	result := strings.Builder{}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '/' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// startMultiSessionImplementation kicks off multi-session implementation using the MultiSessionManager
func (s *SpecDrivenTaskService) startMultiSessionImplementation(ctx context.Context, task *types.SpecTask) {
	log.Info().
		Str("task_id", task.ID).
		Msg("Starting multi-session implementation")

	// Select available Zed agent for implementation
	zedAgent := s.selectZedAgent()
	if zedAgent == "" {
		log.Error().Str("task_id", task.ID).Msg("No Zed agents available")
		s.markTaskFailed(ctx, task, "Implementation failed - no Zed agents available")
		return
	}

	// No need to update task - we're reusing the planning agent and session
	task.UpdatedAt = time.Now()

	err := s.store.UpdateSpecTask(ctx, task)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to update task with implementation agent")
		s.markTaskFailed(ctx, task, "Implementation failed - no Zed agents available")
		return
	}

	// Create implementation sessions configuration
	config := &types.SpecTaskImplementationSessionsCreateRequest{
		SpecTaskID:         task.ID,
		ProjectPath:        "/workspace/" + task.ID, // Default project path
		AutoCreateSessions: true,
		WorkspaceConfig: map[string]interface{}{
			"TASK_ID":    task.ID,
			"TASK_NAME":  task.Name,
			"AGENT_TYPE": zedAgent,
		},
	}

	// Create implementation sessions via MultiSessionManager
	_, err = s.MultiSessionManager.CreateImplementationSessions(ctx, task.ID, config)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to create implementation sessions")
		s.markTaskFailed(ctx, task, "Implementation failed - no Zed agents available")
		return
	}

	log.Info().
		Str("task_id", task.ID).
		Str("implementation_agent", zedAgent).
		Msg("Multi-session implementation started successfully")
}

// NOTE: Planning prompt is now in spec_task_prompts.go:BuildPlanningPrompt

// Helper functions
func (s *SpecDrivenTaskService) selectZedAgent() string {
	// Simple round-robin for now
	// TODO: Implement proper load balancing
	if len(s.zedAgentPool) == 0 {
		return ""
	}
	return s.zedAgentPool[0]
}

// mustMarshalJSON marshals data to JSON, panicking on error (for static data)
func mustMarshalJSON(data interface{}) datatypes.JSON {
	jsonData, err := json.Marshal(data)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal JSON: %v", err))
	}
	return datatypes.JSON(jsonData)
}

func (s *SpecDrivenTaskService) markTaskFailed(ctx context.Context, task *types.SpecTask, errorMessage string) {
	// Keep task in backlog status but set error metadata
	task.Status = types.TaskStatusBacklog
	task.UpdatedAt = time.Now()

	// Store error in metadata
	if task.Metadata == nil {
		task.Metadata = make(map[string]interface{})
	}
	task.Metadata["error"] = errorMessage
	task.Metadata["error_timestamp"] = time.Now().Format(time.RFC3339)

	err := s.store.UpdateSpecTask(ctx, task)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Str("error", errorMessage).Msg("Failed to mark task with error")
	}
}

func generateTaskID() string {
	return system.GenerateSpecTaskID()
}

func generateTaskNameFromPrompt(prompt string) string {
	if len(prompt) > 60 {
		return prompt[:57] + "..."
	}
	return prompt
}

func convertPriorityToInt(priority string) int {
	switch priority {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 2
	}
}

type SandboxAPIKeyRequest struct {
	UserID     string
	ProjectID  string
	SpecTaskID string
}

// getOrCreatePersonalAPIKey gets or creates a personal API key for the user
// IMPORTANT: Only uses personal API keys (not app-scoped keys) to ensure full access
func (s *SpecDrivenTaskService) GetOrCreateSandboxAPIKey(ctx context.Context, req *SandboxAPIKeyRequest) (string, error) {
	existing, err := s.store.GetAPIKey(ctx, &types.ApiKey{
		Owner:      req.UserID,
		OwnerType:  types.OwnerTypeUser,
		ProjectID:  req.ProjectID,
		SpecTaskID: req.SpecTaskID,
	})
	if err != nil && err != store.ErrNotFound {
		return "", fmt.Errorf("failed to get existing API key: %w", err)
	}

	if existing != nil {
		return existing.Key, nil
	}

	newKey, err := system.GenerateAPIKey()
	if err != nil {
		return "", fmt.Errorf("failed to generate API key: %w", err)
	}

	apiKey := &types.ApiKey{
		Owner:      req.UserID,
		OwnerType:  types.OwnerTypeUser,
		Key:        newKey,
		Name:       "Auto-generated for sandbox agent access - " + req.ProjectID + " - " + req.SpecTaskID,
		Type:       types.APIkeytypeAPI,
		ProjectID:  req.ProjectID,
		SpecTaskID: req.SpecTaskID,
	}

	createdKey, err := s.store.CreateAPIKey(ctx, apiKey)
	if err != nil {
		return "", fmt.Errorf("failed to create API key: %w", err)
	}

	log.Info().
		Str("user_id", req.UserID).
		Str("project_id", req.ProjectID).
		Str("spec_task_id", req.SpecTaskID).
		Str("key_name", createdKey.Name).
		Msg("✅ Created personal API key for agent access")

	return createdKey.Key, nil
}

// getCodeAgentRuntimeForTask gets the CodeAgentRuntime from the task's associated app configuration.
// This is used to send the correct agent_name in open_thread commands when resuming sessions.
func (s *SpecDrivenTaskService) getCodeAgentRuntimeForTask(ctx context.Context, task *types.SpecTask) types.CodeAgentRuntime {
	if task.HelixAppID == "" {
		log.Debug().Str("spec_task_id", task.ID).Msg("Spec task has no HelixAppID, defaulting to zed_agent runtime")
		return types.CodeAgentRuntimeZedAgent
	}

	app, err := s.store.GetApp(ctx, task.HelixAppID)
	if err != nil {
		log.Warn().Err(err).
			Str("spec_task_id", task.ID).
			Str("helix_app_id", task.HelixAppID).
			Msg("Failed to get app for code agent runtime, defaulting to zed_agent")
		return types.CodeAgentRuntimeZedAgent
	}

	// Find the zed_external assistant in the app config
	for _, assistant := range app.Config.Helix.Assistants {
		if assistant.AgentType == types.AgentTypeZedExternal {
			if assistant.CodeAgentRuntime != "" {
				log.Debug().
					Str("spec_task_id", task.ID).
					Str("helix_app_id", task.HelixAppID).
					Str("code_agent_runtime", string(assistant.CodeAgentRuntime)).
					Msg("Found code agent runtime from app config")
				return assistant.CodeAgentRuntime
			}
			break
		}
	}

	log.Debug().
		Str("spec_task_id", task.ID).
		Str("helix_app_id", task.HelixAppID).
		Msg("No code agent runtime configured in app, defaulting to zed_agent")
	return types.CodeAgentRuntimeZedAgent
}
