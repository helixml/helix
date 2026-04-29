package services

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// CreateSpecTaskRequest is the cross-call-site request for creating a SpecTask
// outside of the user-driven REST API. Used by:
//   - The Optimus agent's CreateSpecTask MCP tool (chat-driven)
//   - The proposal-decision handler when an agent's spec_task proposal is approved
type CreateSpecTaskRequest struct {
	ProjectID      string
	UserID         string
	Name           string
	Description    string
	Type           string                 // "feature", "bug", "refactor"; defaults to "feature"
	Priority       types.SpecTaskPriority // defaults to medium
	OriginalPrompt string
	JustDoItMode   bool
	DependsOn      []types.SpecTask
	ParentTaskID   string // optional; set when this task was spawned from a proposal
}

// CreateSpecTaskFromProposal creates a SpecTask, assigning a globally-unique task
// number and design-doc directory path. This is the shared core extracted from
// the Optimus agent's CreateSpecTask tool so the proposal-decision handler can
// reuse the same logic without duplication.
func CreateSpecTaskFromProposal(ctx context.Context, s store.Store, req CreateSpecTaskRequest) (*types.SpecTask, error) {
	if req.ProjectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if req.Description == "" {
		return nil, fmt.Errorf("description is required")
	}

	taskType := req.Type
	if taskType == "" {
		taskType = "feature"
	}

	priority := req.Priority
	if priority == "" {
		priority = types.SpecTaskPriorityMedium
	}

	project, err := s.GetProject(ctx, req.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	now := time.Now()
	task := &types.SpecTask{
		ID:             system.GenerateSpecTaskID(),
		ProjectID:      req.ProjectID,
		UserID:         req.UserID,
		OrganizationID: project.OrganizationID,
		Name:           req.Name,
		Description:    req.Description,
		Type:           taskType,
		Priority:       priority,
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: req.OriginalPrompt,
		DependsOn:      req.DependsOn,
		JustDoItMode:   req.JustDoItMode,
		ParentTaskID:   req.ParentTaskID,
		CreatedBy:      req.UserID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	taskNumber, err := s.IncrementGlobalTaskNumber(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to get global task number, using fallback")
		taskNumber = 1
	}
	task.TaskNumber = taskNumber
	task.DesignDocPath = GenerateDesignDocPath(task, taskNumber)

	if err := s.CreateSpecTask(ctx, task); err != nil {
		return nil, fmt.Errorf("failed to create spec task: %w", err)
	}

	log.Info().
		Str("task_id", task.ID).
		Str("project_id", task.ProjectID).
		Int("task_number", task.TaskNumber).
		Str("parent_task_id", task.ParentTaskID).
		Msg("Created spec task via proposal/tool helper")

	return task, nil
}
