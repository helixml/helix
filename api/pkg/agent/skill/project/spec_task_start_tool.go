package project

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"

	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
)

var startSpecTaskParameters = jsonschema.Definition{
	Type: jsonschema.Object,
	Properties: map[string]jsonschema.Definition{
		"task_id": {
			Type:        jsonschema.String,
			Description: "The ID of the task to start",
		},
		"skip_planning": {
			Type:        jsonschema.Boolean,
			Description: "If true, skip spec planning and go straight to implementation. If false, start with spec generation.",
		},
	},
	Required: []string{"task_id", "skip_planning"},
}

type StartSpecTaskTool struct {
	store     store.Store
	projectID string
}

func NewStartSpecTaskTool(projectID string, store store.Store) *StartSpecTaskTool {
	return &StartSpecTaskTool{
		store:     store,
		projectID: projectID,
	}
}

var _ agent.Tool = &StartSpecTaskTool{}

func (t *StartSpecTaskTool) Name() string {
	return "StartSpecTask"
}

func (t *StartSpecTaskTool) Description() string {
	return "Start a spec task - either with spec planning or skip directly to implementation"
}

func (t *StartSpecTaskTool) String() string {
	return "StartSpecTask"
}

func (t *StartSpecTaskTool) StatusMessage() string {
	return "Starting spec task"
}

func (t *StartSpecTaskTool) Icon() string {
	return "PlayIcon"
}

func (t *StartSpecTaskTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "StartSpecTask",
				Description: "Start a spec task - either with spec planning or skip directly to implementation",
				Parameters:  startSpecTaskParameters,
			},
		},
	}
}

type StartSpecTaskResult struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (r *StartSpecTaskResult) ToString() string {
	return fmt.Sprintf("ID: %s\nTask: %s\nStatus: %s\nMessage: %s",
		r.ID, r.Name, r.Status, r.Message)
}

func (t *StartSpecTaskTool) Execute(ctx context.Context, meta agent.Meta, args map[string]interface{}) (string, error) {
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
		Msg("Executing StartSpecTask tool")

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("task_id is required")
	}

	skipPlanning, _ := args["skip_planning"].(bool)

	task, err := t.store.GetSpecTask(ctx, taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to get spec task for start")
		return "", fmt.Errorf("failed to get spec task: %w", err)
	}

	if task.ProjectID != projectID {
		return "", fmt.Errorf("task does not belong to this project")
	}

	var newStatus types.SpecTaskStatus
	var message string

	if skipPlanning {
		newStatus = types.TaskStatusQueuedImplementation
		task.JustDoItMode = true
		message = "Task started - queued for implementation (skipping planning)"
	} else {
		newStatus = types.TaskStatusQueuedSpecGeneration
		task.JustDoItMode = false
		message = "Task started - queued for spec generation"
	}

	task.Status = newStatus
	task.UpdatedAt = time.Now()

	err = t.store.UpdateSpecTask(ctx, task)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to start spec task")
		return "", fmt.Errorf("failed to start spec task: %w", err)
	}

	result := StartSpecTaskResult{
		ID:      task.ID,
		Name:    task.Name,
		Status:  string(task.Status),
		Message: message,
	}

	return result.ToString(), nil
}
