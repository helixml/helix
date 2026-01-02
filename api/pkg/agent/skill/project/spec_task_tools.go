package project

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
)

var listSpecTasksParameters = jsonschema.Definition{
	Type: jsonschema.Object,
	Properties: map[string]jsonschema.Definition{
		"status": {
			Type:        jsonschema.String,
			Description: "Filter by task status: backlog, spec_generation, spec_review, implementation, implementation_review, pull_request, done",
			Enum:        []string{"backlog", "spec_generation", "spec_review", "implementation", "implementation_review", "pull_request", "done"},
		},
		"priority": {
			Type:        jsonschema.String,
			Description: "Filter by task priority: low, medium, high, critical",
			Enum:        []string{"low", "medium", "high", "critical"},
		},
		"limit": {
			Type:        jsonschema.Integer,
			Description: "Maximum number of tasks to return (default: 20)",
		},
		"include_archived": {
			Type:        jsonschema.Boolean,
			Description: "Include archived tasks in results (default: false)",
		},
	},
	Required: []string{},
}

type ListSpecTasksTool struct {
	store     store.Store
	projectID string
}

func NewListSpecTasksTool(projectID string,store store.Store) *ListSpecTasksTool {
	return &ListSpecTasksTool{		
		store:     store,
		projectID: projectID,
	}
}

var _ agent.Tool = &ListSpecTasksTool{}

func (t *ListSpecTasksTool) Name() string {
	return "ListSpecTasks"
}

func (t *ListSpecTasksTool) Description() string {
	return "List spec-driven tasks in the project with optional filtering by status and priority"
}

func (t *ListSpecTasksTool) String() string {
	return "ListSpecTasks"
}

func (t *ListSpecTasksTool) StatusMessage() string {
	return "Listing spec tasks"
}

func (t *ListSpecTasksTool) Icon() string {
	return "ListIcon"
}

func (t *ListSpecTasksTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "ListSpecTasks",
				Description: "List spec-driven tasks in the project with optional filtering by status and priority",
				Parameters:  listSpecTasksParameters,
			},
		},
	}
}

type SpecTaskSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Priority    string `json:"priority"`
	Type        string `json:"type"`
	BranchName  string `json:"branch_name,omitempty"`
}

type ListSpecTasksResult struct {
	Tasks []SpecTaskSummary `json:"tasks"`
	Total int               `json:"total"`
}

func (t *ListSpecTasksTool) Execute(ctx context.Context, meta agent.Meta, args map[string]interface{}) (string, error) {
	projectID := t.projectID
	if projectID == "" {
		projectContext, ok := types.GetHelixProjectContext(ctx)
		if !ok {
			return "", fmt.Errorf("helix project context not found")
		}
		projectID = projectContext.ProjectID
	}

	log.Info().
		Str("project_id", projectID).
		Str("user_id", meta.UserID).
		Str("session_id", meta.SessionID).
		Interface("args", args).
		Msg("Executing ListSpecTasks tool")

	filters := &types.SpecTaskFilters{
		ProjectID: projectID,
	}

	if status, ok := args["status"].(string); ok && status != "" {
		filters.Status = types.SpecTaskStatus(status)
	}

	if priority, ok := args["priority"].(string); ok && priority != "" {
		filters.Priority = priority
	}

	if limit, ok := args["limit"].(float64); ok && limit > 0 {
		filters.Limit = int(limit)
	} else {
		filters.Limit = 20
	}

	if includeArchived, ok := args["include_archived"].(bool); ok {
		filters.IncludeArchived = includeArchived
	}

	tasks, err := t.store.ListSpecTasks(ctx, filters)
	if err != nil {
		log.Error().Err(err).Str("project_id", projectID).Msg("Failed to list spec tasks")
		return "", fmt.Errorf("failed to list spec tasks: %w", err)
	}

	summaries := make([]SpecTaskSummary, 0, len(tasks))
	for _, task := range tasks {
		summaries = append(summaries, SpecTaskSummary{
			ID:          task.ID,
			Name:        task.Name,
			Description: task.Description,
			Status:      string(task.Status),
			Priority:    string(task.Priority),
			Type:        task.Type,
			BranchName:  task.BranchName,
		})
	}

	result := ListSpecTasksResult{
		Tasks: summaries,
		Total: len(summaries),
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	return string(resultJSON), nil
}

// CreateSpecTaskTool - creates a new spec task

var createSpecTaskParameters = jsonschema.Definition{
	Type: jsonschema.Object,
	Properties: map[string]jsonschema.Definition{
		"name": {
			Type:        jsonschema.String,
			Description: "Short, descriptive name for the task",
		},
		"description": {
			Type:        jsonschema.String,
			Description: "Detailed description of what needs to be done",
		},
		"type": {
			Type:        jsonschema.String,
			Description: "Task type: feature, bug, or refactor",
			Enum:        []string{"feature", "bug", "refactor"},
		},
		"priority": {
			Type:        jsonschema.String,
			Description: "Task priority: low, medium, high, or critical",
			Enum:        []string{"low", "medium", "high", "critical"},
		},
		"original_prompt": {
			Type:        jsonschema.String,
			Description: "The original user request or prompt that led to this task",
		},
	},
	Required: []string{"name", "description"},
}

type CreateSpecTaskTool struct {
	store     store.Store
	projectID string
}

func NewCreateSpecTaskTool(projectID string, store store.Store) *CreateSpecTaskTool {
	return &CreateSpecTaskTool{
		store:     store,
		projectID: projectID,
	}
}

var _ agent.Tool = &CreateSpecTaskTool{}

func (t *CreateSpecTaskTool) Name() string {
	return "CreateSpecTask"
}

func (t *CreateSpecTaskTool) Description() string {
	return "Create a new spec-driven task in the project"
}

func (t *CreateSpecTaskTool) String() string {
	return "CreateSpecTask"
}

func (t *CreateSpecTaskTool) StatusMessage() string {
	return "Creating spec task"
}

func (t *CreateSpecTaskTool) Icon() string {
	return "AddIcon"
}

func (t *CreateSpecTaskTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "CreateSpecTask",
				Description: "Create a new spec-driven task in the project",
				Parameters:  createSpecTaskParameters,
			},
		},
	}
}

type CreateSpecTaskResult struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Priority    string `json:"priority"`
	Type        string `json:"type"`
	Message     string `json:"message"`
}

func (t *CreateSpecTaskTool) Execute(ctx context.Context, meta agent.Meta, args map[string]interface{}) (string, error) {
	projectID := t.projectID
	if projectID == "" {
		projectContext, ok := types.GetHelixProjectContext(ctx)
		if !ok {
			return "", fmt.Errorf("helix project context not found")
		}
		projectID = projectContext.ProjectID
	}

	log.Info().
		Str("project_id", projectID).
		Str("user_id", meta.UserID).
		Str("session_id", meta.SessionID).
		Interface("args", args).
		Msg("Executing CreateSpecTask tool")

	name, ok := args["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("name is required")
	}

	description, ok := args["description"].(string)
	if !ok || description == "" {
		return "", fmt.Errorf("description is required")
	}

	taskType := "feature"
	if t, ok := args["type"].(string); ok && t != "" {
		taskType = t
	}

	priority := types.SpecTaskPriorityMedium
	if p, ok := args["priority"].(string); ok && p != "" {
		priority = types.SpecTaskPriority(p)
	}

	originalPrompt := ""
	if op, ok := args["original_prompt"].(string); ok {
		originalPrompt = op
	}

	task := &types.SpecTask{
		ID:             system.GenerateSpecTaskID(),
		ProjectID:      projectID,
		Name:           name,
		Description:    description,
		Type:           taskType,
		Priority:       priority,
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: originalPrompt,
		CreatedBy:      meta.UserID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	err := t.store.CreateSpecTask(ctx, task)
	if err != nil {
		log.Error().Err(err).Str("project_id", projectID).Msg("Failed to create spec task")
		return "", fmt.Errorf("failed to create spec task: %w", err)
	}

	result := CreateSpecTaskResult{
		ID:          task.ID,
		Name:        task.Name,
		Description: task.Description,
		Status:      string(task.Status),
		Priority:    string(task.Priority),
		Type:        task.Type,
		Message:     "Task created successfully",
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	return string(resultJSON), nil
}

// GetSpecTaskTool - retrieves a single spec task by ID

var getSpecTaskParameters = jsonschema.Definition{
	Type: jsonschema.Object,
	Properties: map[string]jsonschema.Definition{
		"task_id": {
			Type:        jsonschema.String,
			Description: "The ID of the task to retrieve",
		},
	},
	Required: []string{"task_id"},
}

type GetSpecTaskTool struct {
	store store.Store
	projectID string
}

func NewGetSpecTaskTool(projectID string, store store.Store) *GetSpecTaskTool {
	return &GetSpecTaskTool{
		store:     store,
		projectID: projectID,
	}
}

var _ agent.Tool = &GetSpecTaskTool{}

func (t *GetSpecTaskTool) Name() string {
	return "GetSpecTask"
}

func (t *GetSpecTaskTool) Description() string {
	return "Get detailed information about a specific spec task by ID"
}

func (t *GetSpecTaskTool) String() string {
	return "GetSpecTask"
}

func (t *GetSpecTaskTool) StatusMessage() string {
	return "Getting spec task details"
}

func (t *GetSpecTaskTool) Icon() string {
	return "InfoIcon"
}

func (t *GetSpecTaskTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "GetSpecTask",
				Description: "Get detailed information about a specific spec task by ID",
				Parameters:  getSpecTaskParameters,
			},
		},
	}
}

type GetSpecTaskResult struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Description        string `json:"description"`
	Status             string `json:"status"`
	Priority           string `json:"priority"`
	Type               string `json:"type"`
	BranchName         string `json:"branch_name,omitempty"`
	OriginalPrompt     string `json:"original_prompt,omitempty"`
	RequirementsSpec   string `json:"requirements_spec,omitempty"`
	TechnicalDesign    string `json:"technical_design,omitempty"`
	ImplementationPlan string `json:"implementation_plan,omitempty"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

func (t *GetSpecTaskTool) Execute(ctx context.Context, meta agent.Meta, args map[string]interface{}) (string, error) {
	projectID := t.projectID
	if projectID == "" {
		projectContext, ok := types.GetHelixProjectContext(ctx)
		if !ok {
			return "", fmt.Errorf("helix project context not found")
		}
		projectID = projectContext.ProjectID
	}

	log.Info().
		Str("project_id", projectID).
		Str("user_id", meta.UserID).
		Str("session_id", meta.SessionID).
		Interface("args", args).
		Msg("Executing GetSpecTask tool")

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("task_id is required")
	}

	task, err := t.store.GetSpecTask(ctx, taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to get spec task")
		return "", fmt.Errorf("failed to get spec task: %w", err)
	}

	if task.ProjectID != projectID {
		return "", fmt.Errorf("task does not belong to this project")
	}

	result := GetSpecTaskResult{
		ID:                 task.ID,
		Name:               task.Name,
		Description:        task.Description,
		Status:             string(task.Status),
		Priority:           string(task.Priority),
		Type:               task.Type,
		BranchName:         task.BranchName,
		OriginalPrompt:     task.OriginalPrompt,
		RequirementsSpec:   task.RequirementsSpec,
		TechnicalDesign:    task.TechnicalDesign,
		ImplementationPlan: task.ImplementationPlan,
		CreatedAt:          task.CreatedAt.Format(time.RFC3339),
		UpdatedAt:          task.UpdatedAt.Format(time.RFC3339),
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	return string(resultJSON), nil
}

// UpdateSpecTaskTool - updates an existing spec task

var updateSpecTaskParameters = jsonschema.Definition{
	Type: jsonschema.Object,
	Properties: map[string]jsonschema.Definition{
		"task_id": {
			Type:        jsonschema.String,
			Description: "The ID of the task to update",
		},
		"name": {
			Type:        jsonschema.String,
			Description: "New name for the task (optional)",
		},
		"description": {
			Type:        jsonschema.String,
			Description: "New description for the task (optional)",
		},
		"status": {
			Type:        jsonschema.String,
			Description: "New status: backlog, spec_generation, spec_review, spec_revision, spec_approved, implementation_queued, implementation, implementation_review, pull_request, done",
			Enum:        []string{"backlog", "spec_generation", "spec_review", "spec_revision", "spec_approved", "implementation_queued", "implementation", "implementation_review", "pull_request", "done"},
		},
		"priority": {
			Type:        jsonschema.String,
			Description: "New priority: low, medium, high, critical (optional)",
			Enum:        []string{"low", "medium", "high", "critical"},
		},
	},
	Required: []string{"task_id"},
}

type UpdateSpecTaskTool struct {
	store store.Store
	projectID string
}

func NewUpdateSpecTaskTool(projectID string, store store.Store) *UpdateSpecTaskTool {
	return &UpdateSpecTaskTool{
		store:     store,
		projectID: projectID,
	}
}

var _ agent.Tool = &UpdateSpecTaskTool{}

func (t *UpdateSpecTaskTool) Name() string {
	return "UpdateSpecTask"
}

func (t *UpdateSpecTaskTool) Description() string {
	return "Update an existing spec task's properties like status, priority, name, or description"
}

func (t *UpdateSpecTaskTool) String() string {
	return "UpdateSpecTask"
}

func (t *UpdateSpecTaskTool) StatusMessage() string {
	return "Updating spec task"
}

func (t *UpdateSpecTaskTool) Icon() string {
	return "EditIcon"
}

func (t *UpdateSpecTaskTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "UpdateSpecTask",
				Description: "Update an existing spec task's properties like status, priority, name, or description",
				Parameters:  updateSpecTaskParameters,
			},
		},
	}
}

type UpdateSpecTaskResult struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Priority    string   `json:"priority"`
	Type        string   `json:"type"`
	Updated     []string `json:"updated_fields"`
	Message     string   `json:"message"`
}

func (t *UpdateSpecTaskTool) Execute(ctx context.Context, meta agent.Meta, args map[string]interface{}) (string, error) {
	projectID := t.projectID
	if projectID == "" {
		projectContext, ok := types.GetHelixProjectContext(ctx)
		if !ok {
			return "", fmt.Errorf("helix project context not found")
		}
		projectID = projectContext.ProjectID
	}

	log.Info().
		Str("project_id", projectID).
		Str("user_id", meta.UserID).
		Str("session_id", meta.SessionID).
		Interface("args", args).
		Msg("Executing UpdateSpecTask tool")

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("task_id is required")
	}

	task, err := t.store.GetSpecTask(ctx, taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to get spec task for update")
		return "", fmt.Errorf("failed to get spec task: %w", err)
	}

	if task.ProjectID != projectID {
		return "", fmt.Errorf("task does not belong to this project")
	}

	var updatedFields []string

	if name, ok := args["name"].(string); ok && name != "" {
		task.Name = name
		updatedFields = append(updatedFields, "name")
	}

	if description, ok := args["description"].(string); ok && description != "" {
		task.Description = description
		updatedFields = append(updatedFields, "description")
	}

	if status, ok := args["status"].(string); ok && status != "" {
		task.Status = types.SpecTaskStatus(status)
		updatedFields = append(updatedFields, "status")
	}

	if priority, ok := args["priority"].(string); ok && priority != "" {
		task.Priority = types.SpecTaskPriority(priority)
		updatedFields = append(updatedFields, "priority")
	}

	if len(updatedFields) == 0 {
		return "", fmt.Errorf("no fields to update")
	}

	task.UpdatedAt = time.Now()

	err = t.store.UpdateSpecTask(ctx, task)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to update spec task")
		return "", fmt.Errorf("failed to update spec task: %w", err)
	}

	result := UpdateSpecTaskResult{
		ID:          task.ID,
		Name:        task.Name,
		Description: task.Description,
		Status:      string(task.Status),
		Priority:    string(task.Priority),
		Type:        task.Type,
		Updated:     updatedFields,
		Message:     "Task updated successfully",
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	return string(resultJSON), nil
}
