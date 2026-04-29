package project

import (
	"context"
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
)

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
		"depends_on": {
			Type:        jsonschema.Array,
			Description: "Optional list of task IDs that this task depends on",
			Items: &jsonschema.Definition{
				Type: jsonschema.String,
			},
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

	var dependsOn []types.SpecTask
	if rawDependsOn, hasDependsOn := args["depends_on"]; hasDependsOn {
		parsedDependsOn, err := parseSpecTaskDependsOnArg(rawDependsOn)
		if err != nil {
			return "", err
		}
		dependsOn = parsedDependsOn
	}

	task, err := services.CreateSpecTaskFromProposal(ctx, t.store, services.CreateSpecTaskRequest{
		ProjectID:      projectID,
		UserID:         meta.UserID,
		Name:           name,
		Description:    description,
		Type:           taskType,
		Priority:       priority,
		OriginalPrompt: originalPrompt,
		JustDoItMode:   skipPlanning,
		DependsOn:      dependsOn,
	})
	if err != nil {
		log.Error().Err(err).Str("project_id", projectID).Msg("Failed to create spec task")
		return "", err
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

func parseSpecTaskDependsOnArg(raw interface{}) ([]types.SpecTask, error) {
	if raw == nil {
		return []types.SpecTask{}, nil
	}

	var dependencyIDs []string
	switch typed := raw.(type) {
	case []string:
		dependencyIDs = typed
	case []interface{}:
		dependencyIDs = make([]string, 0, len(typed))
		for _, item := range typed {
			dependencyID, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("depends_on must contain only task IDs (strings)")
			}
			dependencyIDs = append(dependencyIDs, dependencyID)
		}
	default:
		return nil, fmt.Errorf("depends_on must be an array of task IDs")
	}

	seen := make(map[string]struct{}, len(dependencyIDs))
	dependsOn := make([]types.SpecTask, 0, len(dependencyIDs))
	for _, dependencyID := range dependencyIDs {
		dependencyID = strings.TrimSpace(dependencyID)
		if dependencyID == "" {
			return nil, fmt.Errorf("depends_on cannot contain empty task IDs")
		}
		if _, exists := seen[dependencyID]; exists {
			continue
		}
		seen[dependencyID] = struct{}{}
		dependsOn = append(dependsOn, types.SpecTask{ID: dependencyID})
	}

	return dependsOn, nil
}
