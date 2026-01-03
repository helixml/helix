package project

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
)

type UpdateSpecTaskTool struct {
	store     store.Store
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

func (r *UpdateSpecTaskResult) ToString() string {
	updatedFields := "-"
	if len(r.Updated) > 0 {
		updatedFields = fmt.Sprintf("%v", r.Updated)
	}
	return fmt.Sprintf("ID: %s\nTask: %s\nDescription: %s\nStatus: %s\nPriority: %s\nType: %s\nUpdated Fields: %s\nMessage: %s",
		r.ID, r.Name, r.Description, r.Status, r.Priority, r.Type, updatedFields, r.Message)
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

	return result.ToString(), nil
}
