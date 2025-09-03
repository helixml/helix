package services

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// SpecDrivenTaskService manages the spec-driven development workflow:
// Specification: Helix agent generates specs from simple descriptions
// Implementation: Zed agent implements code from approved specs
type SpecDrivenTaskService struct {
	store        types.Store
	controller   types.Controller
	helixAgentID string   // ID of Helix agent for spec generation
	zedAgentPool []string // Pool of available Zed agents
}

// NewSpecDrivenTaskService creates a new service instance
func NewSpecDrivenTaskService(
	store types.Store,
	controller types.Controller,
	helixAgentID string,
	zedAgentPool []string,
) *SpecDrivenTaskService {
	return &SpecDrivenTaskService{
		store:        store,
		controller:   controller,
		helixAgentID: helixAgentID,
		zedAgentPool: zedAgentPool,
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

	// Immediately start spec generation
	go s.startSpecGeneration(context.Background(), task)

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

	// Create system prompt for spec generation
	systemPrompt := s.buildSpecGenerationPrompt(task)

	// Create Helix session for spec generation
	sessionReq := &types.CreateSessionRequest{
		UserID:       task.CreatedBy,
		ProjectID:    task.ProjectID,
		SessionMode:  types.SessionModeInference,
		SystemPrompt: systemPrompt,
		Metadata: map[string]interface{}{
			"task_id":    task.ID,
			"phase":      "spec_generation",
			"agent_type": types.AgentTypeSpecGeneration,
		},
	}

	session, err := s.controller.CreateSession(ctx, sessionReq)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to create spec generation session")
		s.markTaskFailed(ctx, task, types.TaskStatusSpecFailed)
		return
	}

	// Update task with session ID
	task.SpecSessionID = session.ID
	s.store.UpdateSpecTask(ctx, task)

	// Send the original prompt to start spec generation
	messageReq := &types.CreateMessageRequest{
		SessionID: session.ID,
		UserID:    task.CreatedBy,
		Content:   task.OriginalPrompt,
	}

	_, err = s.controller.CreateMessage(ctx, messageReq)
	if err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to send prompt to spec generation agent")
		s.markTaskFailed(ctx, task, types.TaskStatusSpecFailed)
		return
	}

	log.Info().
		Str("task_id", task.ID).
		Str("session_id", session.ID).
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

	// TODO: Send notification to user for spec review
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

		// Start implementation
		go s.startImplementation(context.Background(), task)

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

// startImplementation kicks off implementation with a Zed agent
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

	// TODO: Create Zed agent session/work item
	// This will integrate with the Zed agent system
	workItem := &types.AgentWorkItem{
		ID:          fmt.Sprintf("impl_%s", task.ID),
		Name:        fmt.Sprintf("Implement: %s", task.Name),
		Description: implementationPrompt,
		Source:      "two_phase_task",
		SourceURL:   fmt.Sprintf("/tasks/%s", task.ID),
		Priority:    convertPriorityToInt(task.Priority),
		UserID:      task.CreatedBy,
		// TODO: Add project/repo context
	}

	log.Info().
		Str("task_id", task.ID).
		Str("zed_agent", zedAgent).
		Msg("Implementation work item created for Zed agent")

	// TODO: Submit work item to Zed agent queue
}

// buildSpecGenerationPrompt creates the system prompt for Helix spec generation
func (s *SpecDrivenTaskService) buildSpecGenerationPrompt(task *types.SpecTask) string {
	return fmt.Sprintf(`You are a software specification expert. Your job is to take a simple user request and generate comprehensive, implementable specifications.

**Project Context:**
- Project ID: %s
- Task Type: %s
- Priority: %s

**Your Task:**
Convert the user's simple request into three detailed documents:

1. **Requirements Specification** (markdown format):
   - Clear user stories in "As a [user], I want [goal] so that [benefit]" format
   - EARS acceptance criteria (Event, Action, Response, Success criteria)
   - Functional and non-functional requirements
   - Edge cases and error handling

2. **Technical Design** (markdown format):
   - Architecture overview and component diagram
   - Data model changes (database schema, API contracts)
   - UI/UX design if applicable
   - Security and performance considerations
   - Integration points and dependencies

3. **Implementation Plan** (markdown format):
   - Discrete, measurable tasks broken down by component
   - Estimated complexity for each task
   - Dependencies between tasks
   - Testing strategy and criteria
   - Deployment considerations

**Important Guidelines:**
- Be specific and actionable - avoid vague descriptions
- Consider the full software development lifecycle
- Think about maintainability and scalability
- Include error handling and edge cases
- Make it easy for a coding agent to implement

Please analyze the user request and generate these three documents. Format your response clearly with markdown headers.`,
		task.ProjectID, task.Type, task.Priority)
}

// buildImplementationPrompt creates the prompt for Zed implementation agent
func (s *SpecDrivenTaskService) buildImplementationPrompt(task *types.SpecTask) string {
	return fmt.Sprintf(`You are a senior software engineer tasked with implementing a feature based on approved specifications.

**Task: %s**

**Original User Request:**
%s

**APPROVED SPECIFICATIONS:**

## Requirements Specification
%s

## Technical Design
%s

## Implementation Plan
%s

**Your Mission:**
1. Carefully read and understand all specifications above
2. Navigate the codebase to understand current architecture
3. Implement the feature following the approved technical design
4. Write comprehensive tests based on the acceptance criteria
5. Ensure code follows project conventions and best practices
6. Create a feature branch and push your changes
7. Open a pull request with a detailed description

**Guidelines:**
- Follow the technical design exactly - don't deviate without good reason
- Implement all acceptance criteria from the requirements spec
- Write clean, maintainable, well-documented code
- Include unit tests and integration tests as appropriate
- Handle all edge cases mentioned in the specifications
- Use the existing codebase patterns and conventions

Start by exploring the codebase to understand the current structure, then implement step by step following the implementation plan.`,
		task.Name, task.OriginalPrompt, task.RequirementsSpec, task.TechnicalDesign, task.ImplementationPlan)
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

func (s *SpecDrivenTaskService) markTaskFailed(ctx context.Context, task *types.SpecTask, status string) {
	task.Status = status
	task.UpdatedAt = time.Now()
	s.store.UpdateSpecTask(ctx, task)
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
