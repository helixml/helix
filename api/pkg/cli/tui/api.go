package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
)

// APIClient wraps the helix client with TUI-specific methods.
type APIClient struct {
	client  *client.HelixClient
	baseURL string // HELIX_URL without /api/v1
}

func NewAPIClient(c *client.HelixClient) *APIClient {
	// Load base URL for web links
	cfg, _ := config.LoadCliConfig()
	baseURL := cfg.URL
	if baseURL == "" {
		baseURL = "https://app.helix.ml"
	}
	return &APIClient{client: c, baseURL: baseURL}
}

// WebURL returns the browser URL for a spec task.
func (a *APIClient) WebURL(projectID, taskID string) string {
	return fmt.Sprintf("%s/projects/%s/tasks/%s", a.baseURL, projectID, taskID)
}

func (a *APIClient) GetUserStatus(ctx context.Context) (*types.UserStatus, error) {
	var status types.UserStatus
	err := a.client.MakeRequest(ctx, http.MethodGet, "/status", nil, &status)
	if err != nil {
		return nil, err
	}
	return &status, nil
}

func (a *APIClient) ListOrganizations(ctx context.Context) ([]*types.Organization, error) {
	var orgs []*types.Organization
	err := a.client.MakeRequest(ctx, http.MethodGet, "/organizations", nil, &orgs)
	if err != nil {
		return nil, err
	}
	return orgs, nil
}

func (a *APIClient) PinProject(ctx context.Context, projectID string) error {
	return a.client.MakeRequest(ctx, http.MethodPost, "/projects/"+projectID+"/pin", nil, nil)
}

func (a *APIClient) UnpinProject(ctx context.Context, projectID string) error {
	return a.client.MakeRequest(ctx, http.MethodDelete, "/projects/"+projectID+"/pin", nil, nil)
}

func (a *APIClient) CreateProject(ctx context.Context, req *types.ProjectCreateRequest) (*types.Project, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	var project types.Project
	err = a.client.MakeRequest(ctx, http.MethodPost, "/projects", strings.NewReader(string(reqBody)), &project)
	if err != nil {
		return nil, err
	}
	return &project, nil
}

func (a *APIClient) GetProject(ctx context.Context, projectID string) (*types.Project, error) {
	var project types.Project
	err := a.client.MakeRequest(ctx, http.MethodGet, "/projects/"+projectID, nil, &project)
	if err != nil {
		return nil, err
	}
	return &project, nil
}

func (a *APIClient) ListProjects(ctx context.Context, orgID string) ([]*types.Project, error) {
	var projects []*types.Project
	path := "/projects"
	if orgID != "" {
		path += "?organization_id=" + orgID
	}
	err := a.client.MakeRequest(ctx, http.MethodGet, path, nil, &projects)
	if err != nil {
		return nil, err
	}
	return projects, nil
}

func (a *APIClient) ListSpecTasks(ctx context.Context, projectID string) ([]*types.SpecTask, error) {
	var tasks []*types.SpecTask
	path := "/spec-tasks?project_id=" + projectID
	err := a.client.MakeRequest(ctx, http.MethodGet, path, nil, &tasks)
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

func (a *APIClient) ListInteractions(ctx context.Context, sessionID string) ([]*types.Interaction, error) {
	var interactions []*types.Interaction
	err := a.client.MakeRequest(ctx, http.MethodGet, "/sessions/"+sessionID+"/interactions", nil, &interactions)
	if err != nil {
		return nil, err
	}
	return interactions, nil
}

func (a *APIClient) GetSpecTask(ctx context.Context, taskID string) (*types.SpecTask, error) {
	var task types.SpecTask
	err := a.client.MakeRequest(ctx, http.MethodGet, "/spec-tasks/"+taskID, nil, &task)
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (a *APIClient) ChatSession(ctx context.Context, req *types.SessionChatRequest) (string, error) {
	return a.client.ChatSession(ctx, req)
}

// SyncPromptHistory syncs prompt entries to the backend queue.
// This is the reliable prompt delivery mechanism — prompts are stored
// server-side and processed even if the TUI disconnects.
func (a *APIClient) SyncPromptHistory(ctx context.Context, req *types.PromptHistorySyncRequest) (*types.PromptHistorySyncResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	var resp types.PromptHistorySyncResponse
	err = a.client.MakeRequest(ctx, http.MethodPost, "/prompt-history/sync", strings.NewReader(string(reqBody)), &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (a *APIClient) CreateTaskFromPrompt(ctx context.Context, req *types.CreateTaskRequest) (*types.SpecTask, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	var task types.SpecTask
	err = a.client.MakeRequest(ctx, http.MethodPost, "/spec-tasks/from-prompt", strings.NewReader(string(reqBody)), &task)
	if err != nil {
		return nil, err
	}
	return &task, nil
}
