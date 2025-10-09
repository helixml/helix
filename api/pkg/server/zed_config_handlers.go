package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// @Summary Get Zed configuration
// @Description Get Helix-managed Zed MCP configuration for a session
// @Tags Zed
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} types.ZedConfigResponse
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/sessions/{id}/zed-config [get]
func (apiServer *HelixAPIServer) getZedConfig(_ http.ResponseWriter, req *http.Request) (*types.ZedConfigResponse, *system.HTTPError) {
	ctx := req.Context()
	vars := mux.Vars(req)
	sessionID := vars["id"]

	// Get session to verify access
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session")
		return nil, system.NewHTTPError404("session not found")
	}

	// Verify access: either user owns this session OR request is using runner token
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError403("access denied")
	}

	// Allow runner token OR session owner
	if user.TokenType != types.TokenTypeRunner && session.Owner != user.ID {
		return nil, system.NewHTTPError403("access denied")
	}

	// Get app (for external agents, parent_app may be empty)
	var app *types.App
	if session.ParentApp != "" {
		app, err = apiServer.Store.GetApp(ctx, session.ParentApp)
		if err != nil {
			log.Error().Err(err).Str("app_id", session.ParentApp).Msg("Failed to get app")
			return nil, system.NewHTTPError500("failed to get app")
		}
	} else {
		// External agent sessions don't have a parent app - create minimal app config
		log.Debug().Str("session_id", sessionID).Msg("Session has no parent_app (likely external agent), using default config")
		app = &types.App{
			ID:     "external-agent-default",
			Config: types.AppConfig{},
		}
	}

	// Generate Zed MCP config
	helixAPIURL := apiServer.Cfg.WebServer.URL
	if helixAPIURL == "" {
		helixAPIURL = "http://api:8080"
	}

	helixToken := apiServer.Cfg.WebServer.RunnerToken
	if helixToken == "" {
		log.Warn().Msg("RUNNER_TOKEN not configured")
	}

	zedConfig, err := external_agent.GenerateZedMCPConfig(app, session.Owner, sessionID, helixAPIURL, helixToken)
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate Zed config")
		return nil, system.NewHTTPError500("failed to generate Zed config")
	}

	// Convert to response format - include ALL fields from zedConfig
	contextServers := make(map[string]interface{})
	for name, server := range zedConfig.ContextServers {
		serverMap := map[string]interface{}{
			"command": server.Command,
			"args":    server.Args,
		}
		if len(server.Env) > 0 {
			serverMap["env"] = server.Env
		}
		contextServers[name] = serverMap
	}

	// Build language models config
	// Note: API keys come from environment variables, not settings.json
	languageModels := make(map[string]interface{})
	for provider, config := range zedConfig.LanguageModels {
		modelConfig := map[string]interface{}{
			"api_url": config.APIURL, // Empty string = use default provider URL
		}
		if len(config.AvailableModels) > 0 {
			modelConfig["available_models"] = config.AvailableModels
		}
		languageModels[provider] = modelConfig
	}

	// Build assistant config
	var assistant map[string]interface{}
	if zedConfig.Assistant != nil {
		assistant = map[string]interface{}{
			"version": zedConfig.Assistant.Version,
		}
		if zedConfig.Assistant.DefaultModel != nil {
			assistant["default_model"] = map[string]interface{}{
				"provider": zedConfig.Assistant.DefaultModel.Provider,
				"model":    zedConfig.Assistant.DefaultModel.Model,
			}
		}
	}

	// Build external_sync config
	var externalSync map[string]interface{}
	if zedConfig.ExternalSync != nil {
		externalSync = map[string]interface{}{
			"enabled": zedConfig.ExternalSync.Enabled,
		}
		if zedConfig.ExternalSync.WebsocketSync != nil {
			externalSync["websocket_sync"] = map[string]interface{}{
				"enabled":      zedConfig.ExternalSync.WebsocketSync.Enabled,
				"external_url": zedConfig.ExternalSync.WebsocketSync.ExternalURL,
			}
		}
	}

	// Build agent config
	var agentConfig map[string]interface{}
	if zedConfig.Agent != nil {
		agentConfig = map[string]interface{}{
			"always_allow_tool_actions": zedConfig.Agent.AlwaysAllowToolActions,
			"show_onboarding":           zedConfig.Agent.ShowOnboarding,
			"auto_open_panel":           zedConfig.Agent.AutoOpenPanel,
		}
	}

	// Use app.Updated for version, or current time if app is minimal
	version := app.Updated.Unix()
	if version == 0 {
		version = session.Updated.Unix()
	}

	response := &types.ZedConfigResponse{
		ContextServers: contextServers,
		LanguageModels: languageModels,
		Assistant:      assistant,
		ExternalSync:   externalSync,
		Agent:          agentConfig,
		Theme:          zedConfig.Theme,
		Version:        version,
	}

	return response, nil
}

// @Summary Update Zed user settings
// @Description Update user's custom Zed settings overrides
// @Tags Zed
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Param overrides body map[string]interface{} true "User settings overrides"
// @Success 200 {object} map[string]string
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/sessions/{id}/zed-config/user [post]
func (apiServer *HelixAPIServer) updateZedUserSettings(_ http.ResponseWriter, req *http.Request) (map[string]string, *system.HTTPError) {
	ctx := req.Context()
	vars := mux.Vars(req)
	sessionID := vars["id"]

	// Get session to verify access
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session")
		return nil, system.NewHTTPError404("session not found")
	}

	// Verify access: either user owns this session OR request is using runner token
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError403("access denied")
	}

	// Allow runner token OR session owner
	if user.TokenType != types.TokenTypeRunner && session.Owner != user.ID {
		return nil, system.NewHTTPError403("access denied")
	}

	// Parse user overrides
	var overrides map[string]interface{}
	if err := json.NewDecoder(req.Body).Decode(&overrides); err != nil {
		return nil, system.NewHTTPError400("invalid request body")
	}

	// Save to database
	if err := external_agent.SaveUserZedOverrides(ctx, apiServer.Store, sessionID, overrides); err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to save Zed user settings")
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to save settings: %v", err))
	}

	log.Info().
		Str("session_id", sessionID).
		Str("user_id", user.ID).
		Msg("Updated Zed user settings")

	return map[string]string{"status": "ok"}, nil
}

// @Summary Get merged Zed settings
// @Description Get merged Helix + user Zed settings for a session
// @Tags Zed
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/sessions/{id}/zed-settings [get]
func (apiServer *HelixAPIServer) getMergedZedSettings(_ http.ResponseWriter, req *http.Request) (map[string]interface{}, *system.HTTPError) {
	ctx := req.Context()
	vars := mux.Vars(req)
	sessionID := vars["id"]

	// Get session to verify access
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session")
		return nil, system.NewHTTPError404("session not found")
	}

	// Verify user owns this session
	user := getRequestUser(req)
	if user == nil || session.Owner != user.ID {
		return nil, system.NewHTTPError403("access denied")
	}

	// Get Helix config
	app, err := apiServer.Store.GetApp(ctx, session.ParentApp)
	if err != nil {
		log.Error().Err(err).Str("app_id", session.ParentApp).Msg("Failed to get app")
		return nil, system.NewHTTPError500("failed to get app")
	}

	helixAPIURL := apiServer.Cfg.WebServer.URL
	if helixAPIURL == "" {
		helixAPIURL = "http://api:8080"
	}

	helixToken := apiServer.Cfg.WebServer.RunnerToken
	if helixToken == "" {
		log.Warn().Msg("RUNNER_TOKEN not configured")
	}

	zedConfig, err := external_agent.GenerateZedMCPConfig(app, session.Owner, sessionID, helixAPIURL, helixToken)
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate Zed config")
		return nil, system.NewHTTPError500("failed to generate Zed config")
	}

	// Get user overrides
	userOverrides, err := external_agent.GetUserZedOverrides(ctx, apiServer.Store, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get user overrides")
		return nil, system.NewHTTPError500("failed to get user overrides")
	}

	// Merge
	merged := external_agent.MergeZedConfigWithUserOverrides(zedConfig, userOverrides)

	return merged, nil
}
