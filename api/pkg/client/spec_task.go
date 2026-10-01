package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/helixml/helix/api/pkg/types"
)

// SpecTaskFilter narrows ListSpecTasks. Exactly one of ProjectID or
// OrganizationID is required by the API.
type SpecTaskFilter struct {
	ProjectID       string
	OrganizationID  string
	Status          types.SpecTaskStatus
	Limit           int    // 0 = server default (all)
	Offset          int    // pagination offset
	Sort            string // "created", "updated", or "last_message"
	IncludeArchived bool
	ArchivedOnly    bool
}

func (f *SpecTaskFilter) query() string {
	if f == nil {
		return ""
	}
	q := url.Values{}
	if f.ProjectID != "" {
		q.Set("project_id", f.ProjectID)
	}
	if f.OrganizationID != "" {
		q.Set("organization_id", f.OrganizationID)
	}
	if f.Status != "" {
		q.Set("status", string(f.Status))
	}
	if f.Limit > 0 {
		q.Set("limit", strconv.Itoa(f.Limit))
	}
	if f.Offset > 0 {
		q.Set("offset", strconv.Itoa(f.Offset))
	}
	if f.Sort != "" {
		q.Set("sort", f.Sort)
	}
	if f.IncludeArchived {
		q.Set("include_archived", "true")
	}
	if f.ArchivedOnly {
		q.Set("archived_only", "true")
	}
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

// ListSpecTasks lists spec tasks for a project or organization.
func (c *HelixClient) ListSpecTasks(ctx context.Context, f *SpecTaskFilter) ([]*types.SpecTask, error) {
	if f == nil || (f.ProjectID == "" && f.OrganizationID == "") {
		return nil, fmt.Errorf("project_id or organization_id is required")
	}
	var tasks []*types.SpecTask
	if err := c.makeRequest(ctx, http.MethodGet, "/spec-tasks"+f.query(), nil, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

// GetSpecTask returns one spec task by ID.
func (c *HelixClient) GetSpecTask(ctx context.Context, taskID string) (*types.SpecTask, error) {
	if taskID == "" {
		return nil, fmt.Errorf("task id is required")
	}
	var task types.SpecTask
	if err := c.makeRequest(ctx, http.MethodGet, "/spec-tasks/"+url.PathEscape(taskID), nil, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// CreateSpecTaskFromPrompt creates a spec task from a natural-language prompt.
// The server derives the owner from the API key; UserID/UserEmail in req are
// ignored. Set JustDoItMode + AutoStart for an immediately-running headless
// task.
func (c *HelixClient) CreateSpecTaskFromPrompt(ctx context.Context, req *types.CreateTaskRequest) (*types.SpecTask, error) {
	if req == nil || req.ProjectID == "" || req.Prompt == "" {
		return nil, fmt.Errorf("project_id and prompt are required")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	var task types.SpecTask
	if err := c.makeRequest(ctx, http.MethodPost, "/spec-tasks/from-prompt", bytes.NewReader(body), &task); err != nil {
		return nil, err
	}
	if task.ID == "" {
		return nil, fmt.Errorf("create spec task returned no id")
	}
	return &task, nil
}

// FindSpecTaskByName returns the project's task with exactly this name, or
// nil when none exists. Archived tasks are included so a deterministic name
// stays unique across retries. It is the idempotency/recovery primitive for
// orchestrators that name tasks deterministically (e.g. "retest <id>") and
// may lose the create response.
func (c *HelixClient) FindSpecTaskByName(ctx context.Context, projectID, name string) (*types.SpecTask, error) {
	if projectID == "" || name == "" {
		return nil, fmt.Errorf("project id and name are required")
	}
	const pageSize = 200
	const maxScan = 10000
	for offset := 0; offset < maxScan; offset += pageSize {
		tasks, err := c.ListSpecTasks(ctx, &SpecTaskFilter{
			ProjectID: projectID, Limit: pageSize, Offset: offset, Sort: "created", IncludeArchived: true,
		})
		if err != nil {
			return nil, err
		}
		for _, t := range tasks {
			if t.ProjectID == projectID && t.Name == name {
				return t, nil
			}
		}
		if len(tasks) < pageSize {
			return nil, nil
		}
	}
	return nil, fmt.Errorf("spec task lookup exceeded %d tasks for project %s", maxScan, projectID)
}
