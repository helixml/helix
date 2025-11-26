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
		HelixAppID:     helixAppID,   // Helix agent used for entire workflow
		YoloMode:       req.YoloMode, // Set YOLO mode from request
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
	// Ensure HelixAppID is set (fallback for tasks created before this field existed)
	if task.HelixAppID == "" {
		task.HelixAppID = s.helixAgentID
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

	// Create Zed external agent session for spec generation
	// Planning agent needs git access to commit design docs to helix-specs branch
	// Build planning instructions as the message (not system prompt - agent has its own system prompt)
	planningPrompt := s.buildSpecGenerationPrompt(task)

	sessionMetadata := types.SessionMetadata{
		SystemPrompt: "",             // Don't override agent's system prompt
		AgentType:    "zed_external", // Use Zed agent for git access
		Stream:       false,
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
		OrganizationID: "",
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
	task.PlanningSessionID = session.ID
	err = s.store.UpdateSpecTask(ctx, task)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to update task with session ID")
		s.markTaskFailed(ctx, task, fmt.Sprintf("Failed to update task with session ID: %v", err))
		return
	}

	// Generate request_id for initial message and register the mapping
	// This allows the WebSocket handler to find and send the initial message to Zed
	requestID := system.GenerateRequestID()
	if s.RegisterRequestMapping != nil {
		s.RegisterRequestMapping(requestID, session.ID)
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

	_, err = s.controller.Options.Store.CreateInteraction(ctx, interaction)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to create initial interaction")
		s.markTaskFailed(ctx, task, fmt.Sprintf("Failed to create initial interaction: %v", err))
		return
	}

	// Launch the external agent (Zed) via Wolf executor to actually start working on the spec generation
	// Get parent project to access repository configuration
	project, err := s.store.GetProject(ctx, task.ProjectID)
	if err != nil {
		log.Error().Err(err).Str("project_id", task.ProjectID).Msg("Failed to get project for spec task")
		s.markTaskFailed(ctx, task, fmt.Sprintf("Failed to get project: %v", err))
		return
	}

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
	zedAgent := &types.ZedAgent{
		SessionID:           session.ID,
		UserID:              task.CreatedBy,
		Input:               "Initialize Zed development environment for spec generation",
		ProjectPath:         "workspace",   // Use relative path
		SpecTaskID:          task.ID,       // For task-scoped workspace
		PrimaryRepositoryID: primaryRepoID, // Primary repo to open in Zed
		RepositoryIDs:       repositoryIDs, // ALL project repos to checkout
		Env: []string{
			fmt.Sprintf("USER_API_TOKEN=%s", userAPIKey),
		},
	}

	// Start the Zed agent via Wolf executor (not NATS)
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
		branchName := fmt.Sprintf("feature/%s-%s", task.Name, task.ID[:8])
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

		err = s.store.UpdateSpecTask(ctx, task)
		if err != nil {
			return fmt.Errorf("failed to update task for revision: %w", err)
		}

		// TODO: Send revision request back to Helix agent
		log.Info().
			Str("task_id", req.TaskID).
			Str("comments", req.Comments).
			Msg("Specs require revision")
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

// startImplementation kicks off implementation with a Zed agent (legacy single-session)
func (s *SpecDrivenTaskService) startImplementation(ctx context.Context, task *types.SpecTask) {
	log.Info().
		Str("task_id", task.ID).
		Msg("Starting implementation with Zed agent")

	// Select available Zed agent
	zedAgent := s.selectZedAgent()
	if zedAgent == "" {
		log.Error().Str("task_id", task.ID).Msg("No Zed agents available")
		s.markTaskFailed(ctx, task, "Implementation failed - no Zed agents available")
		return
	}

	// Update task status (reuse planning agent, no separate implementation agent)
	task.Status = types.TaskStatusImplementationQueued
	task.UpdatedAt = time.Now()

	err := s.store.UpdateSpecTask(ctx, task)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to update task for implementation")
		return
	}

	// Create implementation prompt with approved specs
	implementationPrompt := s.buildImplementationPrompt(task)

	// Create Zed agent work item
	// GORM serializer handles JSON conversion
	workItem := &types.AgentWorkItem{
		ID:          fmt.Sprintf("impl_%s", task.ID),
		Name:        fmt.Sprintf("Implement: %s", task.Name),
		Description: implementationPrompt,
		Source:      "spec_driven_task",
		SourceID:    task.ID,
		SourceURL:   fmt.Sprintf("/tasks/%s", task.ID),
		Priority:    convertPriorityToInt(task.Priority),
		Status:      "pending",
		AgentType:   "zed",
		UserID:      task.CreatedBy,
		WorkData: map[string]interface{}{
			"task_id":             task.ID,
			"requirements_spec":   task.RequirementsSpec,
			"technical_design":    task.TechnicalDesign,
			"implementation_plan": task.ImplementationPlan,
			"original_prompt":     task.OriginalPrompt,
		},
		Config: map[string]interface{}{
			"workspace_dir": "/tmp/workspace",
			"project_path":  task.ProjectID,
		},
		Labels:    []string{"implementation", "spec-driven", task.Priority},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Store the work item in the database
	if s.controller == nil || s.controller.Options.Store == nil {
		log.Error().Str("task_id", task.ID).Msg("Controller or store not available for work item creation")
		s.markTaskFailed(ctx, task, "Implementation failed - no Zed agents available")
		return
	}

	err = s.controller.Options.Store.CreateAgentWorkItem(ctx, workItem)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to create work item")
		s.markTaskFailed(ctx, task, "Implementation failed - no Zed agents available")
		return
	}

	log.Info().
		Str("task_id", task.ID).
		Str("work_item_id", workItem.ID).
		Str("zed_agent", zedAgent).
		Msg("Implementation work item created and queued for Zed agent")
}

// buildSpecGenerationPrompt creates the system prompt for planning Zed agent
func (s *SpecDrivenTaskService) buildSpecGenerationPrompt(task *types.SpecTask) string {
	return fmt.Sprintf(`You are a software specification expert working in a Zed editor with git access. Your job is to take a user request and generate SHORT, SIMPLE, implementable specifications.

**CRITICAL: Planning phase needs to run quickly - be concise!**
- Match document complexity to task complexity
- Simple tasks = minimal docs (1-2 paragraphs per section)
- Complex tasks = add necessary detail (architecture diagrams, sequence flows, etc.)
- Only essential information, no fluff
- Focus on actionable items, not explanations

**Project Context:**
- Project ID: %s
- Task Type: %s
- Priority: %s
- SpecTask ID: %s

**CRITICAL: Specification Documents Location (Spec-Driven Development)**
The helix-specs git branch is ALREADY CHECKED OUT at:
~/work/helix-specs/

⚠️  IMPORTANT:
- This directory ALREADY EXISTS - DO NOT create a "helix-specs" directory
- You are ALREADY in the helix-specs git worktree when you cd to ~/work/helix-specs/
- DO NOT run "mkdir helix-specs" or create nested helix-specs folders

**DIRECTORY STRUCTURE - FOLLOW THIS EXACTLY:**
Your documents go in a task-specific directory:
~/work/helix-specs/design/tasks/%s_%s_%s/

Where the directory name is: {YYYY-MM-DD}_{branch-name}_{task_id}
(Date first for sorting, branch name for readability)

The design/ and design/tasks/ directories might not exist yet - create them if needed.
But ~/work/helix-specs/ itself ALREADY EXISTS - never create it.

**Required Files in This Directory (spec-driven development format):**
1. requirements.md - User stories + EARS acceptance criteria
2. design.md - Architecture + sequence diagrams + implementation considerations
3. tasks.md - Discrete, trackable implementation tasks
4. sessions/ - Directory for session notes (optional)

**Git Workflow You Must Follow:**
`+"```bash"+`
# The helix-specs worktree is ALREADY checked out at ~/work/helix-specs/
# DO NOT create a helix-specs directory - it already exists!

# Navigate to helix-specs worktree (this directory ALREADY EXISTS)
cd ~/work/helix-specs

# Create your task directory structure (if it doesn't exist)
# IMPORTANT: design/tasks is relative to ~/work/helix-specs/, NOT nested inside another helix-specs folder
mkdir -p design/tasks/%s_%s_%s

# Work in your task directory
cd design/tasks/%s_%s_%s

# Create the three required documents (spec-driven development format):
# 1. requirements.md with user stories and EARS acceptance criteria
# 2. design.md with architecture, sequence diagrams, implementation considerations
# 3. tasks.md with discrete, trackable implementation tasks in [ ] format

# CRITICAL: Commit and push IMMEDIATELY after creating docs
# Go back to worktree root to commit
cd ~/work/helix-specs
git add design/tasks/%s_%s_%s/
git commit -m "Generated design documents for SpecTask %s"

# ⚠️  ABSOLUTELY REQUIRED: PUSH NOW
# This push triggers the backend to move your task to review
# Without this push, your task will be STUCK in planning forever
git push origin helix-specs
`+"```"+`

**⚠️  CRITICAL: You ABSOLUTELY MUST push design docs immediately after creating them**
- The backend watches for pushes to helix-specs and moves your task to review
- WITHOUT pushing, the task stays STUCK in planning and review CANNOT begin
- If you make ANY changes to design docs, commit and push immediately
- PUSH IS MANDATORY - not optional

**tasks.md Format (spec-driven development approach):**
`+"```markdown"+`
# Implementation Tasks

## Discrete, Trackable Tasks

- [ ] Setup database schema
- [ ] Create API endpoints
- [ ] Implement authentication
- [ ] Add unit tests
- [ ] Update documentation
`+"```"+`

**After Pushing:**
- Inform the user that design docs are ready for review
- Continue the conversation to discuss and refine the design
- Your comments and questions will appear as regular chat messages
- When the user requests changes, update the docs and push again immediately

**Important Guidelines:**
- **MATCH COMPLEXITY TO TASK** - Simple tasks = simple docs, complex tasks = add detail
- **BE CONCISE** - Keep everything brief, but include necessary detail
- **NO FLUFF** - Only actionable information, skip lengthy explanations
- Be specific and actionable - avoid vague descriptions
- ALWAYS commit your work to the helix-specs git worktree
- Use the [ ] checklist format in tasks.md for task tracking

**Scaling Complexity:**
- Simple task (e.g., "fix a bug"): Minimal docs, just essentials
- Medium task (e.g., "add a feature"): Core sections, key decisions
- Complex task (e.g., "build authentication system"): Add architecture diagrams, sequence flows, data models

**Document Guidelines:**
- requirements.md: Core user stories + key acceptance criteria (as many as needed)
- design.md: Essential architecture + key decisions (add sections for complex tasks)
- tasks.md: Discrete, implementable tasks (could be 3 tasks or 20+ depending on scope)

Start by analyzing the user's request complexity, then create SHORT, SIMPLE spec documents in the worktree.`,
		task.ProjectID, task.Type, task.Priority, task.ID, // Project context (lines 677-680)
		time.Now().Format("2006-01-02"), sanitizeForBranchName(task.Name), task.ID, // Directory name (line 693)
		time.Now().Format("2006-01-02"), sanitizeForBranchName(task.Name), task.ID, // mkdir command (line 717)
		time.Now().Format("2006-01-02"), sanitizeForBranchName(task.Name), task.ID, // cd command (line 720)
		time.Now().Format("2006-01-02"), sanitizeForBranchName(task.Name), task.ID, // git add command (line 730)
		task.ID) // Commit message (line 731)
}

// buildImplementationPrompt creates the prompt for implementation Zed agent
func (s *SpecDrivenTaskService) buildImplementationPrompt(task *types.SpecTask) string {
	return fmt.Sprintf(`You are a senior software engineer working in a Zed editor with git access. You're implementing a feature based on approved specifications.

**Task: %s**
**SpecTask ID: %s**

**CRITICAL: Design Documents Location**
The helix-specs git branch is ALREADY CHECKED OUT at:
~/work/helix-specs/

⚠️  IMPORTANT:
- This directory ALREADY EXISTS - DO NOT create a "helix-specs" directory
- You are ALREADY in the helix-specs git worktree when you cd to ~/work/helix-specs/
- DO NOT run "mkdir helix-specs" or create nested helix-specs folders

The approved design documents are at:
~/work/helix-specs/design/tasks/%s_%s_%s/

Where the directory name is: {YYYY-MM-DD}_{branch-name}_{task_id}

**DIRECTORY STRUCTURE (spec-driven development format):**
`+"```"+`
~/work/helix-specs/           ← ALREADY EXISTS, already checked out
└── design/
    └── tasks/
        └── %s_%s_%s/
            ├── requirements.md      (user stories + EARS acceptance criteria)
            ├── design.md           (architecture + sequence diagrams + considerations)
            ├── tasks.md            (YOUR TASK CHECKLIST - track here!)
            └── sessions/           (session notes)
`+"```"+`

**CRITICAL: Task Progress Tracking**
The tasks.md file contains discrete, trackable tasks in this format:
- [ ] Task description (pending)
- [~] Task description (in progress - YOU mark this)
- [x] Task description (completed - YOU mark this)

**Your Workflow:**
`+"```bash"+`
# The helix-specs worktree is ALREADY checked out at ~/work/helix-specs/
# DO NOT create a helix-specs directory!

# Navigate to your task directory
cd ~/work/helix-specs/design/tasks/%s_%s_%s

# Read your design documents (spec-driven development format)
cat requirements.md    # User stories + EARS criteria
cat design.md         # Architecture + sequence diagrams
cat tasks.md          # Your task checklist

# Find the next [ ] pending task
# Mark it in progress
sed -i 's/- \[ \] Task name/- \[~\] Task name/' tasks.md
git add tasks.md
git commit -m "🤖 Started: Task name"

# ⚠️  REQUIRED: Push immediately after marking progress
git push origin helix-specs

# Implement that specific task in the main codebase (cd back to repo root)
cd /workspace/repos/{repo}
# ... do the coding work ...

# When done, mark complete
cd ~/work/helix-specs/design/tasks/%s_%s_%s
sed -i 's/- \[~\] Task name/- \[x\] Task name/' tasks.md
git add tasks.md
git commit -m "🤖 Completed: Task name"

# ⚠️  REQUIRED: Push immediately after marking progress
git push origin helix-specs

# Move to next [ ] task
# Repeat until all tasks are [x]
`+"```"+`

**Original User Request:**
%s

**Your Mission:**
1. Read design docs from ~/work/helix-specs/design/tasks/{date}_{name}_{taskid}/
2. Read tasks.md to see your task checklist
3. Work through tasks one by one (discrete, trackable)
4. Mark each task [~] when starting, [x] when done
5. **CRITICAL: Push progress updates to helix-specs after EACH task**
6. Implement code in the main repository
7. Create feature branch and push when all tasks complete
8. Open pull request with summary

**Guidelines:**
- ALWAYS mark your progress in tasks.md with [~] and [x]
- **CRITICAL: After ANY change to design docs, you MUST commit and push to helix-specs immediately**
- The backend tracks your progress by monitoring pushes to helix-specs
- Follow the technical design and sequence diagrams exactly
- Implement all EARS acceptance criteria from requirements.md
- Write tests for everything
- Handle all edge cases

**⚠️  PUSH REQUIREMENTS:**
- After completing each task: commit and push to helix-specs
- After modifying requirements.md: commit and push to helix-specs
- After modifying design.md: commit and push to helix-specs
- After modifying tasks.md: commit and push to helix-specs
- The orchestrator monitors these pushes to track your progress

Start by reading the spec documents from the worktree, then work through the task list systematically.`,
		task.Name, task.ID, // Task context (line 794-795)
		time.Now().Format("2006-01-02"), sanitizeForBranchName(task.Name), task.ID, // Task dir (line 807)
		time.Now().Format("2006-01-02"), sanitizeForBranchName(task.Name), task.ID, // Tree structure (line 816)
		time.Now().Format("2006-01-02"), sanitizeForBranchName(task.Name), task.ID, // cd command (line 835)
		time.Now().Format("2006-01-02"), sanitizeForBranchName(task.Name), task.ID, // cd command (line 856)
		task.OriginalPrompt) // Original request (line 868)
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
	ProjectID string `json:"project_id"`
	Prompt    string `json:"prompt"`
	Type      string `json:"type"`
	Priority  string `json:"priority"`
	UserID    string `json:"user_id"`
	AppID     string `json:"app_id"`    // Optional: Helix agent to use for spec generation
	YoloMode  bool   `json:"yolo_mode"` // Optional: Skip human review and auto-approve specs
	// Git repositories are now managed at the project level - no task-level repo selection needed
}
