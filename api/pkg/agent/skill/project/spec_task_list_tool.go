package project

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/store"
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