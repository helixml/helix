package server

// Session-scoped code-agent configuration. Chat surfaces that run an external
// coding agent without a SpecTask — org bot chat, project chat — own their
// model/reasoning choice on the session row itself. SpecTask sessions delegate
// to their task, which stays the single source of truth for those.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/types"
)

// sessionExecutionConfigSurface resolves who owns a session's coding identity.
// A SpecTask session's task is authoritative — getZedConfig reads the task
// first — so the session row never carries competing overrides.
type sessionExecutionConfigSurface struct {
	session   *types.Session
	task      *types.SpecTask
	config    *types.CodeAgentExecutionConfig
	agentID   string
	overrides *types.CodeAgentOverrides
}

func (s *HelixAPIServer) sessionExecutionConfigSurface(ctx context.Context, session *types.Session) (*sessionExecutionConfigSurface, error) {
	surface := &sessionExecutionConfigSurface{
		session:   session,
		config:    session.Metadata.CodeAgentConfig,
		agentID:   session.ParentApp,
		overrides: session.Metadata.CodeAgentOverrides,
	}
	if session.Metadata.SpecTaskID == "" {
		return surface, nil
	}
	task, err := s.Store.GetSpecTask(ctx, session.Metadata.SpecTaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to load session's spec task: %w", err)
	}
	surface.task = task
	surface.config = task.CodeAgentConfig
	surface.overrides = task.CodeAgentOverrides
	if task.HelixAppID != "" {
		surface.agentID = task.HelixAppID
	}
	return surface, nil
}

// getSessionExecutionConfig godoc
// @Summary Get session execution configuration
// @Description Returns the session's current coding identity without exposing Agent secrets. Sessions belonging to a SpecTask report the task's configuration.
// @Tags Sessions
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} types.AgentExecutionConfig
// @Failure 403 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Router /api/v1/sessions/{id}/execution-config [get]
// @Security BearerAuth
func (s *HelixAPIServer) getSessionExecutionConfig(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ctx := r.Context()
	session, err := s.Store.GetSession(ctx, mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if err := s.authorizeUserToSession(ctx, user, session, types.ActionGet); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	surface, err := s.sessionExecutionConfigSurface(ctx, session)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var config *types.AgentExecutionConfig
	if surface.config != nil {
		config = taskAgentExecutionConfig(surface.config)
		if surface.task == nil {
			config.AgentID = surface.agentID
		}
	} else {
		config, err = s.resolveExecutionConfig(ctx, surface.agentID, surface.overrides, session.ID)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeResponse(w, config, http.StatusOK)
}

// updateSessionExecutionConfig godoc
// @Summary Update session execution configuration
// @Description Replaces the complete coding execution config. SpecTask sessions write through to the task; general sessions keep their parent Agent for instructions and tools while storing runtime/model configuration on the session. Running sandboxes start a fresh ACP thread with the prior transcript.
// @Tags Sessions
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Param request body types.SessionExecutionConfigUpdateRequest true "Execution configuration"
// @Success 200 {object} types.SessionExecutionConfigUpdateResponse
// @Failure 400 {object} types.APIError
// @Failure 403 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 409 {object} types.APIError
// @Router /api/v1/sessions/{id}/execution-config [patch]
// @Security BearerAuth
func (s *HelixAPIServer) updateSessionExecutionConfig(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req types.SessionExecutionConfigUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Switching an agent outlives the browser request: it cancels the turn,
	// rewrites config, and seeds a new thread.
	ctx, cancel := detachContext(r.Context(), 30*time.Second)
	defer cancel()

	session, err := s.Store.GetSession(ctx, mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if err := s.authorizeUserToSession(ctx, user, session, types.ActionUpdate); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if session.Metadata.AgentType != string(types.AgentTypeZedExternal) {
		http.Error(w, "session does not run an external coding agent", http.StatusBadRequest)
		return
	}
	// A paused session is a frozen checkpoint — reconfigure its active
	// descendant instead, exactly as switch-agent requires.
	if session.Metadata.Paused {
		http.Error(w, fmt.Sprintf("session is paused (reason: %s); reconfigure its active descendant instead",
			session.Metadata.PausedReason), http.StatusConflict)
		return
	}

	surface, err := s.sessionExecutionConfigSurface(ctx, session)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if surface.task != nil {
		if req.AgentID != "" || req.CodeAgentOverrides != nil {
			http.Error(w, "agent_id and code_agent_overrides are not supported for SpecTask sessions; provide code_agent_config", http.StatusBadRequest)
			return
		}
		if req.CodeAgentConfig == nil {
			http.Error(w, "code_agent_config is required", http.StatusBadRequest)
			return
		}
		response := &types.SessionExecutionConfigUpdateResponse{
			SessionID:       session.ID,
			SpecTaskID:      surface.task.ID,
			CodeAgentConfig: surface.config,
		}
		_, restarted, httpErr := s.applySpecTaskExecutionConfig(
			ctx, user, surface.task, session, req.CodeAgentConfig,
			"The coding agent or model configuration changed for this session.",
		)
		if httpErr != nil {
			http.Error(w, httpErr.Message, httpErr.StatusCode)
			return
		}
		response.AgentThreadRestarted = restarted
		response.CodeAgentConfig = req.CodeAgentConfig
		writeResponse(w, response, http.StatusOK)
		return
	}
	if req.CodeAgentConfig != nil {
		if req.AgentID != "" || req.CodeAgentOverrides != nil {
			http.Error(w, "agent_id and code_agent_overrides cannot be combined with code_agent_config", http.StatusBadRequest)
			return
		}
		response := &types.SessionExecutionConfigUpdateResponse{
			SessionID:       session.ID,
			AgentID:         surface.agentID,
			CodeAgentConfig: surface.config,
		}
		_, restarted, httpErr := s.applySessionCodeAgentExecutionConfig(
			ctx, user, session, req.CodeAgentConfig,
			"The coding runtime or model configuration changed for this session.",
		)
		if httpErr != nil {
			http.Error(w, httpErr.Message, httpErr.StatusCode)
			return
		}
		response.AgentThreadRestarted = restarted
		response.CodeAgentConfig = req.CodeAgentConfig
		writeResponse(w, response, http.StatusOK)
		return
	}
	if surface.config != nil {
		http.Error(w, "code_agent_config is required for this session", http.StatusBadRequest)
		return
	}
	if req.CodeAgentOverrides == nil {
		http.Error(w, "code_agent_overrides is required", http.StatusBadRequest)
		return
	}
	if surface.agentID == "" {
		http.Error(w, "session has no agent to configure", http.StatusBadRequest)
		return
	}

	response := &types.SessionExecutionConfigUpdateResponse{
		SessionID:          session.ID,
		AgentID:            surface.agentID,
		CodeAgentOverrides: surface.overrides,
	}
	if surface.task != nil {
		response.SpecTaskID = surface.task.ID
	}

	persist := s.persistSessionCodeAgentConfig(session)

	_, restarted, httpErr := s.applyCodeAgentExecutionConfig(ctx, user, codeAgentConfigTarget{
		surface:       "coding sessions",
		session:       session,
		agentID:       surface.agentID,
		overrides:     surface.overrides,
		handoffReason: "The coding agent or model configuration changed for this session.",
		persist:       persist,
	}, req)
	if httpErr != nil {
		http.Error(w, httpErr.Message, httpErr.StatusCode)
		return
	}

	response.AgentThreadRestarted = restarted
	response.CodeAgentOverrides = req.CodeAgentOverrides
	if req.AgentID != "" {
		response.AgentID = req.AgentID
	}
	writeResponse(w, response, http.StatusOK)
}
