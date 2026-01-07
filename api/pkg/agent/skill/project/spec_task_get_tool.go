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
	store     store.Store
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

func (r *GetSpecTaskResult) ToString() string {
	branchName := r.BranchName
	if branchName == "" {
		branchName = "-"
	}
	originalPrompt := r.OriginalPrompt
	if originalPrompt == "" {
		originalPrompt = "-"
	}
	requirementsSpec := r.RequirementsSpec
	if requirementsSpec == "" {
		requirementsSpec = "-"
	}
	technicalDesign := r.TechnicalDesign
	if technicalDesign == "" {
		technicalDesign = "-"
	}
	implementationPlan := r.ImplementationPlan
	if implementationPlan == "" {
		implementationPlan = "-"
	}
	return fmt.Sprintf("ID: %s\nTask: %s\nDescription: %s\nStatus: %s\nPriority: %s\nType: %s\nBranchName: %s\nOriginalPrompt: %s\nRequirementsSpec: %s\nTechnicalDesign: %s\nImplementationPlan: %s\nCreatedAt: %s\nUpdatedAt: %s",
		r.ID, r.Name, r.Description, r.Status, r.Priority, r.Type, branchName, originalPrompt, requirementsSpec, technicalDesign, implementationPlan, r.CreatedAt, r.UpdatedAt)
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

	return result.ToString(), nil
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
