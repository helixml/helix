package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/types"
)

// getInteractiveEmbedModel godoc
// @Summary Get models available to an interactive task embed
// @Description Returns the current model and enabled models from the task's current provider. Available only to an interactive embed key bound to the task's active session.
// @Tags spec-driven-tasks
// @Produce json
// @Param taskId path string true "SpecTask ID"
// @Success 200 {object} types.InteractiveEmbedModelResponse
// @Failure 403 {object} types.APIError
// @Failure 409 {object} types.APIError
// @Router /api/v1/spec-tasks/{taskId}/interactive-embed-model [get]
// @Security BearerAuth
func (s *HelixAPIServer) getInteractiveEmbedModel(w http.ResponseWriter, r *http.Request) {
	user, task, config, ok := s.loadInteractiveEmbedModelTask(w, r, mux.Vars(r)["taskId"], types.ActionGet)
	if !ok {
		return
	}
	models, err := s.interactiveEmbedModels(r.Context(), user, task, config)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeResponse(w, interactiveEmbedModelResponse(config, models, false), http.StatusOK)
}

// updateInteractiveEmbedModel godoc
// @Summary Update the model for an interactive task embed
// @Description Changes only the model on the task's current code-agent configuration. Available only to an interactive embed key bound to the task's active session.
// @Tags spec-driven-tasks
// @Accept json
// @Produce json
// @Param taskId path string true "SpecTask ID"
// @Param request body types.InteractiveEmbedModelUpdateRequest true "Model selection"
// @Success 200 {object} types.InteractiveEmbedModelResponse
// @Failure 400 {object} types.APIError
// @Failure 403 {object} types.APIError
// @Failure 409 {object} types.APIError
// @Router /api/v1/spec-tasks/{taskId}/interactive-embed-model [patch]
// @Security BearerAuth
func (s *HelixAPIServer) updateInteractiveEmbedModel(w http.ResponseWriter, r *http.Request) {
	var req types.InteractiveEmbedModelUpdateRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Model) == "" {
		http.Error(w, "model is required", http.StatusBadRequest)
		return
	}

	taskID := mux.Vars(r)["taskId"]
	ctx, cancel := detachContext(r.Context(), 30*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	user, task, config, ok := s.loadInteractiveEmbedModelTask(w, r, taskID, types.ActionUpdate)
	if !ok {
		return
	}
	models, err := s.interactiveEmbedModels(ctx, user, task, config)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if !interactiveEmbedModelExists(models, req.Model) {
		http.Error(w, "model is not enabled for this provider", http.StatusBadRequest)
		return
	}

	next := *config
	next.Model = req.Model
	session, err := s.Store.GetSession(ctx, task.PlanningSessionID)
	if err != nil {
		http.Error(w, "task session not found", http.StatusConflict)
		return
	}
	_, restarted, httpErr := s.applySpecTaskExecutionConfig(
		ctx,
		user,
		task,
		session,
		types.SpecTaskPhaseForStatus(task.Status),
		&next,
		"The model configuration changed for this task.",
	)
	if httpErr != nil {
		http.Error(w, httpErr.Message, httpErr.StatusCode)
		return
	}
	writeResponse(w, interactiveEmbedModelResponse(&next, models, restarted), http.StatusOK)
}

func (s *HelixAPIServer) loadInteractiveEmbedModelTask(
	w http.ResponseWriter,
	r *http.Request,
	taskID string,
	action types.Action,
) (*types.User, *types.SpecTask, *types.CodeAgentExecutionConfig, bool) {
	user := getRequestUser(r)
	if user == nil || user.APIKeyType != types.APIkeytypeEmbed || !user.EmbedInteractive ||
		user.SpecTaskID != taskID || user.SessionID == "" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil, nil, nil, false
	}
	task, err := s.Store.GetSpecTask(r.Context(), taskID)
	if err != nil {
		http.Error(w, "SpecTask not found", http.StatusNotFound)
		return nil, nil, nil, false
	}
	if err := s.authorizeUserToProjectByID(r.Context(), user, task.ProjectID, action); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return nil, nil, nil, false
	}
	if task.PlanningSessionID == "" || task.PlanningSessionID != user.SessionID {
		http.Error(w, "embed key is not bound to the task's active session", http.StatusConflict)
		return nil, nil, nil, false
	}
	config, err := s.resolveInteractiveEmbedCodeAgentConfig(r.Context(), task)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return nil, nil, nil, false
	}
	if config == nil || config.CredentialType != types.CodeAgentCredentialTypeAPIKey || config.ProviderRef == "" {
		http.Error(w, "task does not use a provider-backed model", http.StatusConflict)
		return nil, nil, nil, false
	}
	return user, task, config, true
}

func (s *HelixAPIServer) resolveInteractiveEmbedCodeAgentConfig(
	ctx context.Context,
	task *types.SpecTask,
) (*types.CodeAgentExecutionConfig, error) {
	if config := task.ActiveCodeAgentConfig(); config != nil {
		return config, nil
	}
	if task.HelixAppID == "" {
		return nil, fmt.Errorf("task does not have a code-agent configuration")
	}
	app, err := s.Store.GetApp(ctx, task.HelixAppID)
	if err != nil {
		return nil, fmt.Errorf("failed to load task coding agent: %w", err)
	}
	config, err := external_agent.MaterializeCodeAgentConfig(app, task.CodeAgentOverrides)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve task coding agent: %w", err)
	}
	return config, nil
}

func (s *HelixAPIServer) interactiveEmbedModels(
	ctx context.Context,
	user *types.User,
	task *types.SpecTask,
	config *types.CodeAgentExecutionConfig,
) ([]types.OpenAIModel, error) {
	app := external_agent.AppFromCodeAgentConfig(config, task.UserID, task.OrganizationID)
	endpoints, err := s.listEndpointsForApp(ctx, user.ID, app)
	if err != nil {
		return nil, fmt.Errorf("failed to load providers: %w", err)
	}
	if task.OrganizationID != "" {
		harness, err := s.loadOrgCodeAgentHarnessPolicy(ctx, task.OrganizationID, config.Runtime)
		if err != nil {
			return nil, fmt.Errorf("failed to load code-agent policy: %w", err)
		}
		endpoints = filterProviderEndpointsForHarness(endpoints, harness, config.Runtime)
	}
	endpoint := resolveProviderEndpointRef(endpoints, config.ProviderRef)
	if endpoint == nil {
		return nil, fmt.Errorf("task provider is not available")
	}
	result, err := s.getProviderModels(ctx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to load provider models: %w", err)
	}
	models := make([]types.OpenAIModel, 0, len(result.Models))
	for _, model := range result.Models {
		if model.ID != "" && model.Enabled && (model.Type == "" || model.Type == "chat" || model.Type == "text") {
			models = append(models, model)
		}
	}
	return models, nil
}

func interactiveEmbedModelExists(models []types.OpenAIModel, modelID string) bool {
	for _, model := range models {
		if model.ID == modelID {
			return true
		}
	}
	return false
}

func interactiveEmbedModelResponse(config *types.CodeAgentExecutionConfig, models []types.OpenAIModel, restarted bool) types.InteractiveEmbedModelResponse {
	response := types.InteractiveEmbedModelResponse{
		Model:                config.Model,
		Runtime:              config.Runtime,
		Models:               make([]types.InteractiveEmbedModel, 0, len(models)),
		AgentThreadRestarted: restarted,
	}
	for _, item := range models {
		response.Models = append(response.Models, types.InteractiveEmbedModel{
			ID:               item.ID,
			Name:             item.Name,
			Description:      item.Description,
			ContextLength:    item.ContextLength,
			InputModalities:  item.InputModalities,
			ReasoningEfforts: item.ReasoningEfforts,
		})
	}
	return response
}
