package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// getSpecTaskExecutionConfig godoc
// @Summary Get task execution configuration
// @Description Returns the task-owned code-agent configuration. Unmigrated historical tasks are resolved through their legacy App until task start materializes the configuration.
// @Tags spec-driven-tasks
// @Produce json
// @Param taskId path string true "SpecTask ID"
// @Success 200 {object} types.AgentExecutionConfig
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

func (s *HelixAPIServer) resolveSpecTaskExecutionConfig(ctx context.Context, task *types.SpecTask) (*types.AgentExecutionConfig, error) {
	if task.CodeAgentConfig != nil {
		return taskAgentExecutionConfig(task.CodeAgentConfig), nil
	}
	return s.resolveExecutionConfig(ctx, task.HelixAppID, task.CodeAgentOverrides, task.PlanningSessionID)
}

// updateSpecTaskExecutionConfig godoc
// @Summary Update task execution configuration
// @Description Replaces a task's complete code-agent configuration or sandbox resource preset. Running sandboxes are resized in place and code-agent changes start a fresh ACP thread; stopped sandboxes record code-agent changes for the next start.
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
	if req.AgentID != "" || req.CodeAgentOverrides != nil {
		http.Error(w, "agent_id and code_agent_overrides are no longer supported; provide code_agent_config", http.StatusBadRequest)
		return
	}
	codeChange := req.CodeAgentConfig != nil
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

	// A task that has already started owns a live session; the switch has to
	// land on it as well as on the task row.
	var session *types.Session
	if task.PlanningSessionID != "" {
		session, err = s.Store.GetSession(ctx, task.PlanningSessionID)
		if err != nil {
			http.Error(w, "task session not found", http.StatusConflict)
			return
		}
	}

	changed, restarted, httpErr := s.applySpecTaskExecutionConfig(
		ctx, user, task, session, req.CodeAgentConfig,
		"The coding agent or model configuration changed for this task.",
	)
	if httpErr != nil {
		http.Error(w, httpErr.Message, httpErr.StatusCode)
		return
	}
	if !changed {
		writeResponse(w, response, http.StatusOK)
		return
	}
	response.AgentThreadRestarted = restarted
	response.Task = task
	writeResponse(w, response, http.StatusOK)
}
