package project

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
)

// generateDesignDocPath creates a human-readable directory path for design docs
// Format: "NNNNNN_shortname" e.g., "000001_install-cowsay"
// This is a local copy to avoid import cycles with the services package
func generateDesignDocPath(taskName string, taskNumber int) string {
	// Sanitize task name for use in path
	name := strings.ToLower(taskName)
	reg := regexp.MustCompile(`\s+`)
	name = reg.ReplaceAllString(name, " ")
	reg = regexp.MustCompile(`[^a-z0-9- ]`)
	name = reg.ReplaceAllString(name, "")
	name = strings.ReplaceAll(name, " ", "-")
	reg = regexp.MustCompile(`-+`)
	name = reg.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if len(name) > 25 {
		truncated := name[:25]
		lastHyphen := strings.LastIndex(truncated, "-")
		if lastHyphen > 10 {
			name = truncated[:lastHyphen]
		} else {
			name = truncated
		}
	}
	name = strings.TrimRight(name, "-")
	return fmt.Sprintf("%06d_%s", taskNumber, name)
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
		"skip_planning": {
			Type:        jsonschema.Boolean,
			Description: "Skip planning and go straight to implementation. Useful when the task is clear and well defined.",
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

func (r *CreateSpecTaskResult) ToString() string {
	return fmt.Sprintf("ID: %s\nTask: %s\nDescription: %s\nStatus: %s\nPriority: %s\nType: %s\nMessage: %s",
		r.ID, r.Name, r.Description, r.Status, r.Priority, r.Type, r.Message)
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

	skipPlanning := false
	if sp, ok := args["skip_planning"].(bool); ok {
		skipPlanning = sp
	}

	priority := types.SpecTaskPriorityMedium
	if p, ok := args["priority"].(string); ok && p != "" {
		priority = types.SpecTaskPriority(p)
	}

	originalPrompt := ""
	if op, ok := args["original_prompt"].(string); ok {
		originalPrompt = op
	}

	// Get project to determine organization ID
	project, err := t.store.GetProject(ctx, projectID)
	if err != nil {
		log.Error().Err(err).Str("project_id", projectID).Msg("Failed to get project for spec task creation")
		return "", fmt.Errorf("failed to get project: %w", err)
	}

	task := &types.SpecTask{
		ID:             system.GenerateSpecTaskID(),
		ProjectID:      projectID,
		UserID:         meta.UserID,
		OrganizationID: project.OrganizationID,
		Name:           name,
		Description:    description,
		Type:           taskType,
		Priority:       priority,
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: originalPrompt,
		JustDoItMode:   skipPlanning,
		CreatedBy:      meta.UserID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Assign task number immediately at creation time so it's always visible in UI
	// Task numbers are globally unique across the entire deployment
	taskNumber, err := t.store.IncrementGlobalTaskNumber(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to get global task number for agent-created task, using fallback")
		taskNumber = 1
	}
	task.TaskNumber = taskNumber
	task.DesignDocPath = generateDesignDocPath(task.Name, taskNumber)
	log.Info().
		Str("task_id", task.ID).
		Int("task_number", taskNumber).
		Str("design_doc_path", task.DesignDocPath).
		Msg("Assigned task number and design doc path to agent-created task")

	err = t.store.CreateSpecTask(ctx, task)
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

	return result.ToString(), nil
}
