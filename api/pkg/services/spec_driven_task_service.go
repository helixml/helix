package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
)

// SpecDrivenTaskService manages the spec-driven development workflow:
// Specification: Helix agent generates specs from simple descriptions
// Implementation: Zed agent implements code from approved specs
type SpecDrivenTaskService struct {
	store                    store.Store
	controller               *controller.Controller
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
) *SpecDrivenTaskService {
	service := &SpecDrivenTaskService{
		store:        store,
		controller:   controller,
		helixAgentID: helixAgentID,
		zedAgentPool: zedAgentPool,
		testMode:     false,
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
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Store the task
	err := s.store.CreateSpecTask(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	// Immediately start spec generation (unless in test mode)
	if !s.testMode {
		go s.startSpecGeneration(context.Background(), task)
	}

	return task, nil
}

// startSpecGeneration kicks off spec generation with a Helix agent
func (s *SpecDrivenTaskService) startSpecGeneration(ctx context.Context, task *types.SpecTask) {
	log.Info().
		Str("task_id", task.ID).
		Str("original_prompt", task.OriginalPrompt).
		Msg("Starting spec generation")

	// Update task status
	task.Status = types.TaskStatusSpecGeneration
	task.SpecAgent = s.helixAgentID
	task.UpdatedAt = time.Now()

	err := s.store.UpdateSpecTask(ctx, task)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to update task status")
		return
	}

	// Create Zed external agent session for spec generation
	// Planning agent needs git access to commit design docs to helix-design-docs branch
	systemPrompt := s.buildSpecGenerationPrompt(task)

	sessionMetadata := types.SessionMetadata{
		SystemPrompt: systemPrompt,
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
		Provider:       "anthropic",          // Use Claude for spec generation
		ModelName:      "external_agent",     // Model name for external agents
		Owner:          task.CreatedBy,
		ParentApp:      "",
		OrganizationID: "",
		Metadata:       sessionMetadata,
		OwnerType:      types.OwnerTypeUser,
	}

	// Create the session in the database
	if s.controller == nil || s.controller.Options.Store == nil {
		log.Error().Str("task_id", task.ID).Msg("Controller or store not available for spec generation")
		s.markTaskFailed(ctx, task, types.TaskStatusSpecFailed)
		return
	}

	session, err = s.controller.Options.Store.CreateSession(ctx, *session)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to create spec generation session")
		s.markTaskFailed(ctx, task, types.TaskStatusSpecFailed)
		return
	}

	// Update task with session ID
	task.SpecSessionID = session.ID
	err = s.store.UpdateSpecTask(ctx, task)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to update task with session ID")
		s.markTaskFailed(ctx, task, types.TaskStatusSpecFailed)
		return
	}

	// Create initial interaction with the original prompt
	interaction := &types.Interaction{
		ID:            system.GenerateInteractionID(),
		Created:       time.Now(),
		Updated:       time.Now(),
		Scheduled:     time.Now(),
		SessionID:     session.ID,
		UserID:        task.CreatedBy,
		Mode:          types.SessionModeInference,
		SystemPrompt:  systemPrompt,
		PromptMessage: task.OriginalPrompt,
		State:         types.InteractionStateWaiting,
	}

	_, err = s.controller.Options.Store.CreateInteraction(ctx, interaction)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to create initial interaction")
		s.markTaskFailed(ctx, task, types.TaskStatusSpecFailed)
		return
	}

	log.Info().
		Str("task_id", task.ID).
		// Str("session_id", session.ID).
		Msg("Spec generation agent started")
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
			ID:    task.SpecSessionID,
			Owner: task.CreatedBy,
		}

		notificationPayload := &notification.Notification{
			Session: session,
			Event:   notification.EventCronTriggerComplete,
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
		// Specs approved - move to implementation
		task.Status = types.TaskStatusSpecApproved
		task.SpecApprovedBy = req.ApprovedBy
		task.SpecApprovedAt = &req.ApprovedAt

		err = s.store.UpdateSpecTask(ctx, task)
		if err != nil {
			return fmt.Errorf("failed to update task approval: %w", err)
		}

		// Start multi-session implementation (unless in test mode)
		if !s.testMode {
			go s.startMultiSessionImplementation(context.Background(), task)
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

// startMultiSessionImplementation kicks off multi-session implementation using the MultiSessionManager
func (s *SpecDrivenTaskService) startMultiSessionImplementation(ctx context.Context, task *types.SpecTask) {
	log.Info().
		Str("task_id", task.ID).
		Msg("Starting multi-session implementation")

	// Select available Zed agent for implementation
	zedAgent := s.selectZedAgent()
	if zedAgent == "" {
		log.Error().Str("task_id", task.ID).Msg("No Zed agents available")
		s.markTaskFailed(ctx, task, types.TaskStatusImplementationFailed)
		return
	}

	// Update task with implementation agent
	task.ImplementationAgent = zedAgent
	task.UpdatedAt = time.Now()

	err := s.store.UpdateSpecTask(ctx, task)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to update task with implementation agent")
		s.markTaskFailed(ctx, task, types.TaskStatusImplementationFailed)
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
		s.markTaskFailed(ctx, task, types.TaskStatusImplementationFailed)
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
		s.markTaskFailed(ctx, task, types.TaskStatusImplementationFailed)
		return
	}

	// Update task status
	task.Status = types.TaskStatusImplementationQueued
	task.ImplementationAgent = zedAgent
	task.UpdatedAt = time.Now()

	err := s.store.UpdateSpecTask(ctx, task)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to update task for implementation")
		return
	}

	// Create implementation prompt with approved specs
	implementationPrompt := s.buildImplementationPrompt(task)

	// Create Zed agent work item
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
		WorkData: mustMarshalJSON(map[string]interface{}{
			"task_id":             task.ID,
			"requirements_spec":   task.RequirementsSpec,
			"technical_design":    task.TechnicalDesign,
			"implementation_plan": task.ImplementationPlan,
			"original_prompt":     task.OriginalPrompt,
		}),
		Config: mustMarshalJSON(map[string]interface{}{
			"workspace_dir": "/tmp/workspace",
			"project_path":  task.ProjectID,
		}),
		Labels:    mustMarshalJSON([]string{"implementation", "spec-driven", task.Priority}),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Store the work item in the database
	if s.controller == nil || s.controller.Options.Store == nil {
		log.Error().Str("task_id", task.ID).Msg("Controller or store not available for work item creation")
		s.markTaskFailed(ctx, task, types.TaskStatusImplementationFailed)
		return
	}

	err = s.controller.Options.Store.CreateAgentWorkItem(ctx, workItem)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to create work item")
		s.markTaskFailed(ctx, task, types.TaskStatusImplementationFailed)
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
	return fmt.Sprintf(`You are a software specification expert working in a Zed editor with git access. Your job is to take a simple user request and generate comprehensive, implementable specifications.

**Project Context:**
- Project ID: %s
- Task Type: %s
- Priority: %s
- SpecTask ID: %s

**CRITICAL: Design Documents Location**
You have access to a git worktree for design documentation at:
.git-worktrees/helix-design-docs/

This is a forward-only branch specifically for design documents. All your design work MUST be saved there.

**DIRECTORY STRUCTURE - FOLLOW THIS EXACTLY:**
Your documents go in a task-specific directory:
.git-worktrees/helix-design-docs/tasks/%s_%s_%s/

Where the directory name is: {YYYY-MM-DD}_{branch-name}_{task_id}
(Date first for sorting, branch name for readability)

**Required Files in This Directory:**
1. requirements.md - Requirements specification
2. design.md - Technical design
3. progress.md - Implementation task checklist
4. sessions/ - Directory for session notes (optional)

**Git Workflow You Must Follow:**
` + "```bash" + `
# Navigate to design docs worktree
cd .git-worktrees/helix-design-docs

# Create your task directory (if not exists)
mkdir -p tasks/%s_%s_%s

# Work in your task directory
cd tasks/%s_%s_%s

# Create the three required documents:
# 1. requirements.md with user stories and acceptance criteria
# 2. design.md with architecture and technical design
# 3. progress.md with implementation task checklist in [ ] format

# Commit your work
git add .
git commit -m "Generated design documents for SpecTask %s"

# Push to helix-design-docs branch
git push origin helix-design-docs
` + "```" + `

**progress.md Task Checklist Format:**
` + "```markdown" + `
## Task Checklist

- [ ] Setup database schema
- [ ] Create API endpoints
- [ ] Implement authentication
- [ ] Add unit tests
- [ ] Update documentation
` + "```" + `

After committing, let the user know the design docs are ready for review.
They can continue chatting with you to refine the design before approval.

**Important Guidelines:**
- Be specific and actionable - avoid vague descriptions
- ALWAYS commit your work to the helix-design-docs git worktree
- The user can continue chatting with you to refine the design
- Make it easy for the implementation agent to work from your design
- Use the [ ] checklist format in progress.md for task tracking

Start by analyzing the user's request, then create comprehensive design documents in the worktree.`,
		task.ProjectID, task.Type, task.Priority, task.ID,
		time.Now().Format("2006-01-02"), sanitizeForBranchName(task.Name), task.ID,  // Directory name
		time.Now().Format("2006-01-02"), sanitizeForBranchName(task.Name), task.ID,  // mkdir command
		time.Now().Format("2006-01-02"), sanitizeForBranchName(task.Name), task.ID,  // cd command
		task.ID)                                                                       // Commit message
}

// buildImplementationPrompt creates the prompt for implementation Zed agent
func (s *SpecDrivenTaskService) buildImplementationPrompt(task *types.SpecTask) string {
	return fmt.Sprintf(`You are a senior software engineer working in a Zed editor with git access. You're implementing a feature based on approved specifications.

**Task: %s**
**SpecTask ID: %s**

**CRITICAL: Design Documents Location**
The approved design documents are in a task-specific directory in the helix-design-docs worktree:
.git-worktrees/helix-design-docs/tasks/%s_%s_%s/

Where the directory name is: {YYYY-MM-DD}_{branch-name}_{task_id}

**DIRECTORY STRUCTURE:**
` + "```" + `
.git-worktrees/helix-design-docs/tasks/%s_%s_%s/
├── requirements.md      (approved requirements)
├── design.md           (approved technical design)
├── progress.md         (YOUR TASK CHECKLIST - track here!)
└── sessions/           (session notes)
` + "```" + `

**CRITICAL: Task Progress Tracking**
The progress.md file contains your task checklist in this format:
- [ ] Task description (pending)
- [~] Task description (in progress - YOU mark this)
- [x] Task description (completed - YOU mark this)

**Your Workflow:**
` + "```bash" + `
# Navigate to your task directory
cd .git-worktrees/helix-design-docs/tasks/%s_%s_%s

# Read your design documents
cat requirements.md
cat design.md
cat progress.md

# Find the next [ ] pending task
# Mark it in progress
sed -i 's/- \[ \] Task name/- \[~\] Task name/' progress.md
git add progress.md
git commit -m "🤖 Started: Task name"
git push origin helix-design-docs

# Implement that specific task in the main codebase (cd back to repo root)
cd /workspace/repos/{repo}
# ... do the coding work ...

# When done, mark complete
cd .git-worktrees/helix-design-docs/tasks/%s_%s_%s
sed -i 's/- \[~\] Task name/- \[x\] Task name/' progress.md
git add progress.md
git commit -m "🤖 Completed: Task name"
git push origin helix-design-docs

# Move to next [ ] task
# Repeat until all tasks are [x]
` + "```" + `

**Original User Request:**
%s

**Your Mission:**
1. Read design docs from .git-worktrees/helix-design-docs/
2. Read progress.md to see your task checklist
3. Work through tasks one by one
4. Mark each task [~] when starting, [x] when done
5. Commit progress updates to helix-design-docs branch
6. Implement code in the main repository
7. Create feature branch and push when all tasks complete
8. Open pull request with summary

**Guidelines:**
- ALWAYS mark your progress in progress.md with [~] and [x]
- ALWAYS commit progress updates to helix-design-docs
- Follow the technical design exactly
- Implement all acceptance criteria
- Write tests for everything
- Handle all edge cases
- The user and orchestrator are watching your progress via git commits

Start by reading the design documents from the worktree, then work through the task list systematically.`,
		task.Name, task.ID,
		time.Now().Format("2006-01-02"), sanitizeForBranchName(task.Name), task.ID,  // Directory structure 1
		time.Now().Format("2006-01-02"), sanitizeForBranchName(task.Name), task.ID,  // Directory structure 2
		time.Now().Format("2006-01-02"), sanitizeForBranchName(task.Name), task.ID,  // cd command 1
		time.Now().Format("2006-01-02"), sanitizeForBranchName(task.Name), task.ID,  // cd command 2
		task.OriginalPrompt)
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

func (s *SpecDrivenTaskService) markTaskFailed(ctx context.Context, task *types.SpecTask, status string) {
	task.Status = status
	task.UpdatedAt = time.Now()
	err := s.store.UpdateSpecTask(ctx, task)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Str("status", status).Msg("Failed to mark task as failed")
	}
}

func generateTaskID() string {
	return fmt.Sprintf("task_%d", time.Now().UnixNano())
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

// Request types
type CreateTaskRequest struct {
	ProjectID string `json:"project_id"`
	Prompt    string `json:"prompt"`
	Type      string `json:"type"`
	Priority  string `json:"priority"`
	UserID    string `json:"user_id"`
}
