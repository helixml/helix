package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
)

// APIClient wraps the helix client with TUI-specific methods.
type APIClient struct {
	client *client.HelixClient
}

func NewAPIClient(c *client.HelixClient) *APIClient {
	return &APIClient{client: c}
}

func (a *APIClient) ListProjects(ctx context.Context) ([]*types.Project, error) {
	var projects []*types.Project
	err := a.client.MakeRequest(ctx, http.MethodGet, "/projects", nil, &projects)
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

func (a *APIClient) ChatSession(ctx context.Context, req *types.SessionChatRequest) (string, error) {
	return a.client.ChatSession(ctx, req)
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
