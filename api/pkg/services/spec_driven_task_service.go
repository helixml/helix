package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
}

// NewSpecDrivenTaskService creates a new service instance
func NewSpecDrivenTaskService(
	store store.Store,
	controller *controller.Controller,
	helixAgentID string,
	zedAgentPool []string,
	pubsub pubsub.PubSub,
	externalAgentExecutor external_agent.Executor,
	registerRequestMapping RequestMappingRegistrar,
) *SpecDrivenTaskService {
	service := &SpecDrivenTaskService{
		store:                  store,
		controller:             controller,
		externalAgentExecutor:  externalAgentExecutor,
		RegisterRequestMapping: registerRequestMapping,
		helixAgentID:           helixAgentID,
		zedAgentPool:           zedAgentPool,
		testMode:               false,
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
func (s *SpecDrivenTaskService) CreateTaskFromPrompt(ctx context.Context, req *CreateTaskRequest) (*types.SpecTask, error) {
	// Determine which agent to use (single agent for entire workflow)
	helixAppID := s.helixAgentID
	if req.AppID != "" {
		helixAppID = req.AppID
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
		HelixAppID:     helixAppID,           // Helix agent used for entire workflow
		JustDoItMode:   req.JustDoItMode,   // Set Just Do It mode from request
		UseHostDocker:  req.UseHostDocker,  // Use host Docker socket (requires privileged sandbox)
		// Repositories inherited from parent project - no task-level repo configuration
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Store the task
	err := s.store.CreateSpecTask(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	// DO NOT auto-start spec generation
	// Tasks should start in backlog and wait for explicit user action to start planning
	// This allows WIP limits to be enforced on the planning column

	return task, nil
}

// StartSpecGeneration kicks off spec generation with a Helix agent
// This is now a public method that can be called explicitly to start planning
func (s *SpecDrivenTaskService) StartSpecGeneration(ctx context.Context, task *types.SpecTask) {
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

	// Build planning instructions as the message (not system prompt - agent has its own system prompt)
	planningPrompt := BuildPlanningPrompt(task, guidelines)

	sessionMetadata := types.SessionMetadata{
		SystemPrompt: "",             // Don't override agent's system prompt
		AgentType:    "zed_external", // Use Zed agent for git access
		Stream:       false,
		SpecTaskID:   task.ID, // CRITICAL: Set SpecTaskID so session restore uses correct workspace path
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
	userAPIKey, err := s.getOrCreatePersonalAPIKey(ctx, task.CreatedBy)
	if err != nil {
		log.Error().Err(err).Str("user_id", task.CreatedBy).Msg("Failed to get user API key for SpecTask")
		s.markTaskFailed(ctx, task, fmt.Sprintf("Failed to get user API key: %v", err))
		return
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
		Env: []string{
			fmt.Sprintf("USER_API_TOKEN=%s", userAPIKey),
		},
	}
	log.Debug().Str("task_id", task.ID).Str("session_id", session.ID).Str("helix_session_id", zedAgent.HelixSessionID).Msg("DEBUG: Created ZedAgent struct")

	// Check if executor is nil
	if s.externalAgentExecutor == nil {
		log.Error().Str("task_id", task.ID).Msg("ERROR: externalAgentExecutor is nil!")
		s.markTaskFailed(ctx, task, "externalAgentExecutor is nil")
		return
	}

	// Start the Zed agent via Wolf executor (not NATS)
	log.Debug().Str("task_id", task.ID).Str("session_id", session.ID).Msg("DEBUG: Calling StartZedAgent...")
	agentResp, err := s.externalAgentExecutor.StartZedAgent(ctx, zedAgent)
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
}

// StartJustDoItMode skips spec generation and goes straight to implementation with just the user's prompt
// This is for tasks that don't require planning code changes
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

	// Clear any previous error from metadata (in case this is a retry)
	if task.Metadata != nil {
		delete(task.Metadata, "error")
		delete(task.Metadata, "error_timestamp")
	}

	// Generate feature branch name (same logic as spec approval flow)
	// Use last 16 chars of task ID to get the random ULID portion (avoids timestamp collisions)
	taskIDSuffix := task.ID
	if len(taskIDSuffix) > 16 {
		taskIDSuffix = taskIDSuffix[len(taskIDSuffix)-16:]
	}
	branchName := fmt.Sprintf("feature/%s-%s", task.Name, taskIDSuffix)
	branchName = sanitizeBranchName(branchName)

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

	sessionMetadata := types.SessionMetadata{
		SystemPrompt: "",             // Don't override agent's system prompt
		AgentType:    "zed_external", // Use Zed agent for git access
		Stream:       false,
		SpecTaskID:   task.ID, // CRITICAL: Set SpecTaskID so session restore uses correct workspace path
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

	promptWithBranch := fmt.Sprintf(`%s
%s
---

**Working in ~/work/:** All code repositories are in ~/work/. That's where you make changes.

**If making code changes:**
1. git checkout -b %s
2. Make your changes
3. git push origin %s

**For persistent installs:** Add commands to .helix/startup.sh (runs at sandbox startup, must be idempotent).
`, task.OriginalPrompt, guidelinesSection, branchName, branchName)

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
	// Project already fetched earlier for agent inheritance

	// Get all project repositories
	projectRepos, err := s.store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{
		ProjectID: task.ProjectID,
	})
	if err != nil {
		log.Warn().Err(err).Str("project_id", task.ProjectID).Msg("Failed to get project repositories")
		projectRepos = nil
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

	// Get user's personal API token for git operations
	userAPIKey, err := s.getOrCreatePersonalAPIKey(ctx, task.CreatedBy)
	if err != nil {
		log.Error().Err(err).Str("user_id", task.CreatedBy).Msg("Failed to get user API key for Just Do It task")
		s.markTaskFailed(ctx, task, fmt.Sprintf("Failed to get user API key: %v", err))
		return
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
		Env: []string{
			fmt.Sprintf("USER_API_TOKEN=%s", userAPIKey),
		},
	}

	// Start the Zed agent via Wolf executor
	agentResp, err := s.externalAgentExecutor.StartZedAgent(ctx, zedAgent)
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

		var baseBranch string
		if project.DefaultRepoID != "" {
			repo, err := s.store.GetGitRepository(ctx, project.DefaultRepoID)
			if err == nil && repo != nil {
				baseBranch = repo.DefaultBranch
			}
		}
		if baseBranch == "" {
			baseBranch = "main"
		}

		// Generate feature branch name
		// Use last 16 chars of task ID to get the random ULID portion (avoids timestamp collisions)
		taskIDSuffix := task.ID
		if len(taskIDSuffix) > 16 {
			taskIDSuffix = taskIDSuffix[len(taskIDSuffix)-16:]
		}
		branchName := fmt.Sprintf("feature/%s-%s", task.Name, taskIDSuffix)
		// Sanitize branch name (replace spaces with hyphens, remove special chars)
		branchName = sanitizeBranchName(branchName)

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
			go func() {
				err := agentInstructionService.SendApprovalInstruction(
					context.Background(),
					sessionID,
					task.CreatedBy, // User who created the task
					task,
					branchName,
					baseBranch,
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

// getOrCreatePersonalAPIKey gets or creates a personal API key for the user
// IMPORTANT: Only uses personal API keys (not app-scoped keys) to ensure full access
func (s *SpecDrivenTaskService) getOrCreatePersonalAPIKey(ctx context.Context, userID string) (string, error) {
	// List user's existing API keys
	keys, err := s.store.ListAPIKeys(ctx, &store.ListAPIKeysQuery{
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

	createdKey, err := s.store.CreateAPIKey(ctx, apiKey)
	if err != nil {
		return "", fmt.Errorf("failed to create API key: %w", err)
	}

	log.Info().
		Str("user_id", userID).
		Str("key_name", createdKey.Name).
		Msg("✅ Created personal API key for agent access")

	return createdKey.Key, nil
}

// Request types
type CreateTaskRequest struct {
	ProjectID     string `json:"project_id"`
	Prompt        string `json:"prompt"`
	Type          string `json:"type"`
	Priority      string `json:"priority"`
	UserID        string `json:"user_id"`
	AppID         string `json:"app_id"`            // Optional: Helix agent to use for spec generation
	JustDoItMode  bool   `json:"just_do_it_mode"`   // Optional: Skip spec planning, go straight to implementation
	UseHostDocker bool   `json:"use_host_docker"`   // Optional: Use host Docker socket (requires privileged sandbox)
	// Git repositories are now managed at the project level - no task-level repo selection needed
}
