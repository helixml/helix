package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/gorilla/mux"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// getSpecTaskExecutionConfig godoc
// @Summary Get task execution configuration
// @Description Returns the task's current coding identity without exposing Agent secrets. Legacy tasks whose Agent was deleted fall back to their session and interaction snapshots.
// @Tags spec-driven-tasks
// @Produce json
// @Param taskId path string true "SpecTask ID"
// @Success 200 {object} types.SpecTaskExecutionConfig
// @Failure 404 {object} types.APIError
// @Router /api/v1/spec-tasks/{taskId}/execution-config [get]
// @Security BearerAuth
func (s *HelixAPIServer) getSpecTaskExecutionConfig(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ctx := r.Context()
	taskID := mux.Vars(r)["taskId"]
	task, err := s.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		http.Error(w, "SpecTask not found", http.StatusNotFound)
		return
	}
	if err := s.authorizeUserToProjectByID(ctx, user, task.ProjectID, types.ActionGet); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	config, err := s.resolveSpecTaskExecutionConfig(ctx, task)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeResponse(w, config, http.StatusOK)
}

func (s *HelixAPIServer) resolveSpecTaskExecutionConfig(ctx context.Context, task *types.SpecTask) (*types.SpecTaskExecutionConfig, error) {
	config := &types.SpecTaskExecutionConfig{AgentID: task.HelixAppID}

	if task.HelixAppID != "" {
		app, err := s.Store.GetApp(ctx, task.HelixAppID)
		switch {
		case err == nil:
			config.AgentAvailable = true
			config.AgentName = app.Config.Helix.Name
			effectiveApp := external_agent.ApplyCodeAgentOverrides(app, task.CodeAgentOverrides)
			if assistant := external_agent.FindZedExternalAssistant(effectiveApp); assistant != nil {
				config.Runtime = assistant.CodeAgentRuntime
				if config.Runtime == "" {
					config.Runtime = types.CodeAgentRuntimeZedAgent
				}
				config.CredentialType = assistant.CodeAgentCredentialType
				config.ProviderRef, config.Model = acpUsageProviderAndModel(assistant)
				if assistant.CodeAgentRuntime != types.CodeAgentRuntimeClaudeCode &&
					assistant.CodeAgentRuntime != types.CodeAgentRuntimeCodexCLI {
					if assistant.GenerationModelProvider != "" {
						config.ProviderRef = assistant.GenerationModelProvider
					}
					if assistant.GenerationModel != "" {
						config.Model = assistant.GenerationModel
					}
				}
				config.ReasoningEffort = assistant.ReasoningEffort
			}
		case errors.Is(err, store.ErrNotFound):
			// Continue with interaction/session snapshots below.
		default:
			return nil, fmt.Errorf("failed to load task agent: %w", err)
		}
	}

	if !config.AgentAvailable && task.PlanningSessionID != "" {
		interactions, _, err := s.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
			SessionID: task.PlanningSessionID,
			Page:      0,
			PerPage:   20,
			Order:     "created DESC",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to load task interactions: %w", err)
		}
		for _, interaction := range interactions {
			if interaction.CodeAgentConfigSnapshot == nil {
				continue
			}
			snapshot := interaction.CodeAgentConfigSnapshot
			config.Runtime = snapshot.Runtime
			config.CredentialType = snapshot.CredentialType
			config.ProviderRef = snapshot.Provider
			config.Model = snapshot.Model
			break
		}

		session, err := s.Store.GetSession(ctx, task.PlanningSessionID)
		switch {
		case err == nil:
			if config.Runtime == "" {
				config.Runtime = session.Metadata.CodeAgentRuntime
			}
			if config.AgentName == "" {
				config.AgentName = session.Metadata.ZedAgentName
			}
		case errors.Is(err, store.ErrNotFound):
		default:
			return nil, fmt.Errorf("failed to load task session: %w", err)
		}
	}

	if config.Runtime == "" {
		config.Runtime = types.CodeAgentRuntimeZedAgent
	}
	if config.AgentName == "" {
		config.AgentName = string(config.Runtime)
	}
	if task.CodeAgentOverrides != nil {
		if task.CodeAgentOverrides.ProviderRef != "" {
			config.ProviderRef = task.CodeAgentOverrides.ProviderRef
		}
		if task.CodeAgentOverrides.Model != "" {
			config.Model = task.CodeAgentOverrides.Model
		}
		if task.CodeAgentOverrides.ReasoningEffort != "" {
			config.ReasoningEffort = task.CodeAgentOverrides.ReasoningEffort
		}
		config.ServiceTier = task.CodeAgentOverrides.ServiceTier
	}

	return config, nil
}

// updateSpecTaskExecutionConfig godoc
// @Summary Update task execution configuration
// @Description Replaces a task's code-agent overrides or sandbox resource preset. Running sandboxes are resized in place; code-agent changes start a new ACP thread with normalized prior context.
// @Tags spec-driven-tasks
// @Accept json
// @Produce json
// @Param taskId path string true "SpecTask ID"
// @Param request body types.SpecTaskExecutionConfigUpdateRequest true "Execution configuration"
// @Success 200 {object} types.SpecTaskExecutionConfigUpdateResponse
// @Failure 400 {object} types.APIError
// @Failure 409 {object} types.APIError
// @Router /api/v1/spec-tasks/{taskId}/execution-config [patch]
// @Security BearerAuth
func (s *HelixAPIServer) updateSpecTaskExecutionConfig(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	taskID := mux.Vars(r)["taskId"]
	var req types.SpecTaskExecutionConfigUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	codeChange := req.CodeAgentOverrides != nil || req.AgentID != ""
	if codeChange == (req.SandboxResourceOverrides != nil) {
		http.Error(w, "provide exactly one execution configuration change", http.StatusBadRequest)
		return
	}

	ctx, cancel := detachContext(r.Context(), 30*time.Second)
	defer cancel()
	task, err := s.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		http.Error(w, "SpecTask not found", http.StatusNotFound)
		return
	}
	if err := s.authorizeUserToProjectByID(ctx, user, task.ProjectID, types.ActionUpdate); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	response := &types.SpecTaskExecutionConfigUpdateResponse{Task: task}
	if req.SandboxResourceOverrides != nil {
		if !req.SandboxResourceOverrides.ValidPreset() {
			http.Error(w, "sandbox size must be 1 CPU/2 GB, 4 CPU/8 GB, or 8 CPU/16 GB", http.StatusBadRequest)
			return
		}
		if reflect.DeepEqual(task.SandboxResourceOverrides, req.SandboxResourceOverrides) {
			writeResponse(w, response, http.StatusOK)
			return
		}
		oldResources := task.SandboxResourceOverrides
		if task.PlanningSessionID != "" && s.externalAgentExecutor != nil && s.externalAgentExecutor.HasRunningContainer(ctx, task.PlanningSessionID) {
			if err := s.externalAgentExecutor.UpdateDesktopResources(ctx, task.PlanningSessionID, req.SandboxResourceOverrides); err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			response.SandboxResourcesApplied = true
		}
		task.SandboxResourceOverrides = req.SandboxResourceOverrides
		if err := s.Store.UpdateSpecTask(ctx, task); err != nil {
			if response.SandboxResourcesApplied {
				rollback := oldResources
				if rollback == nil {
					rollback = types.DefaultSpecTaskSandboxResources()
				}
				if rollbackErr := s.externalAgentExecutor.UpdateDesktopResources(context.Background(), task.PlanningSessionID, rollback); rollbackErr != nil {
					log.Error().Err(rollbackErr).Str("task_id", task.ID).Msg("Failed to roll back sandbox resources after task update failure")
				}
			}
			http.Error(w, fmt.Sprintf("failed to save sandbox resources: %v", err), http.StatusInternalServerError)
			return
		}
		response.Task = task
		writeResponse(w, response, http.StatusOK)
		return
	}

	targetAgentID := task.HelixAppID
	if req.AgentID != "" {
		targetAgentID = req.AgentID
		targetApp, appErr := s.Store.GetApp(ctx, targetAgentID)
		if appErr != nil {
			http.Error(w, "selected agent not found", http.StatusBadRequest)
			return
		}
		if authErr := s.authorizeUserToApp(ctx, user, targetApp, types.ActionGet); authErr != nil {
			http.Error(w, authErr.Error(), http.StatusForbidden)
			return
		}
		if kindErr := requireAgentKind(targetApp, types.AgentKindCoding, "spec tasks"); kindErr != nil {
			http.Error(w, kindErr.Error(), http.StatusBadRequest)
			return
		}
	}
	candidate := *task
	candidate.HelixAppID = targetAgentID
	if err := s.validateTaskCodeAgentOverrides(ctx, &candidate, req.CodeAgentOverrides, user.ID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if task.HelixAppID == targetAgentID && reflect.DeepEqual(task.CodeAgentOverrides, req.CodeAgentOverrides) {
		writeResponse(w, response, http.StatusOK)
		return
	}
	var session *types.Session
	if task.PlanningSessionID != "" {
		session, err = s.Store.GetSession(ctx, task.PlanningSessionID)
		if err != nil {
			http.Error(w, "task session not found", http.StatusConflict)
			return
		}
		if cancelErr := s.cancelSpecTaskTurnsForSwitch(ctx, session.ID); cancelErr != nil {
			http.Error(w, fmt.Sprintf("failed to stop the current agent turn before switching: %v", cancelErr), http.StatusConflict)
			return
		}
	}

	oldOverrides := task.CodeAgentOverrides
	oldAgentID := task.HelixAppID
	task.HelixAppID = targetAgentID
	task.CodeAgentOverrides = req.CodeAgentOverrides
	if err := s.Store.UpdateSpecTask(ctx, task); err != nil {
		http.Error(w, fmt.Sprintf("failed to save code-agent overrides: %v", err), http.StatusInternalServerError)
		return
	}
	if task.PlanningSessionID != "" {
		runtime, err := s.codeAgentRuntimeForTask(ctx, task)
		if err != nil {
			task.HelixAppID = oldAgentID
			task.CodeAgentOverrides = oldOverrides
			_ = s.Store.UpdateSpecTask(ctx, task)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if switchErr := s.switchAgentInPlaceForNextTurn(ctx, session, runtime, targetAgentID, true, "The coding agent or model configuration changed for this task."); switchErr != nil {
			task.HelixAppID = oldAgentID
			task.CodeAgentOverrides = oldOverrides
			_ = s.Store.UpdateSpecTask(ctx, task)
			http.Error(w, switchErr.Error(), switchErr.StatusCode)
			return
		}
		response.AgentThreadRestarted = true
	}
	response.Task = task
	writeResponse(w, response, http.StatusOK)
}

func (s *HelixAPIServer) cancelSpecTaskTurnsForSwitch(ctx context.Context, sessionID string) error {
	const maxTurns = 100
	for i := 0; i < maxTurns; i++ {
		status, err := s.cancelActiveTurn(ctx, sessionID)
		if err != nil {
			return err
		}
		if status == "noop" {
			return nil
		}
	}
	return fmt.Errorf("more than %d active turns remain", maxTurns)
}

func (s *HelixAPIServer) validateTaskCodeAgentOverrides(ctx context.Context, task *types.SpecTask, overrides *types.CodeAgentOverrides, actorID string) error {
	if overrides == nil {
		return fmt.Errorf("code-agent overrides are required")
	}
	if overrides.ProviderRef != "" && overrides.Model == "" {
		return fmt.Errorf("model is required when provider_ref is set")
	}
	effort := strings.ToLower(strings.TrimSpace(overrides.ReasoningEffort))
	switch effort {
	case "", "none", "low", "medium", "high", "xhigh", "max", "ultra":
	default:
		return fmt.Errorf("unsupported reasoning effort %q", overrides.ReasoningEffort)
	}
	if overrides.ServiceTier != "" && overrides.ServiceTier != "fast" {
		return fmt.Errorf("unsupported service tier %q", overrides.ServiceTier)
	}
	app, err := s.Store.GetApp(ctx, task.HelixAppID)
	if err != nil {
		return fmt.Errorf("failed to load task agent: %w", err)
	}
	effectiveApp := external_agent.ApplyCodeAgentOverrides(app, overrides)
	assistant := external_agent.FindZedExternalAssistant(effectiveApp)
	if assistant == nil {
		return fmt.Errorf("task agent has no coding assistant")
	}
	if overrides.ServiceTier != "" && assistant.CodeAgentRuntime != types.CodeAgentRuntimeCodexCLI {
		return fmt.Errorf("service tier is only supported by Codex")
	}
	if assistant.CodeAgentCredentialType.IsSubscription() && overrides.ProviderRef != "" {
		return fmt.Errorf("subscription agents cannot override provider_ref")
	}
	snapshot, err := s.getProviderSnapshot(ctx, actorID, app)
	if err != nil {
		return fmt.Errorf("failed to load providers: %w", err)
	}
	if reason := external_agent.ValidateAssistantModelConfig(effectiveApp, snapshot); reason != "" {
		return fmt.Errorf("%s", reason)
	}
	return nil
}

func (s *HelixAPIServer) codeAgentRuntimeForTask(ctx context.Context, task *types.SpecTask) (types.CodeAgentRuntime, error) {
	app, err := s.Store.GetApp(ctx, task.HelixAppID)
	if err != nil {
		return "", fmt.Errorf("failed to load task agent: %w", err)
	}
	assistant := external_agent.FindZedExternalAssistant(app)
	if assistant == nil {
		return "", fmt.Errorf("task agent has no coding assistant")
	}
	if assistant.CodeAgentRuntime == "" {
		return types.CodeAgentRuntimeZedAgent, nil
	}
	return assistant.CodeAgentRuntime, nil
}
