package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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
			log.Warn().Err(err).Str("app_id", session.ParentApp).Str("session_id", sessionID).Msg("Parent app not found - falling back to default config")
			// Fall back to default config if app doesn't exist
			app = &types.App{
				ID:     "external-agent-default",
				Config: types.AppConfig{},
			}
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
		// Add default_model if configured
		if zedConfig.Agent.DefaultModel != nil {
			agentConfig["default_model"] = map[string]interface{}{
				"provider": zedConfig.Agent.DefaultModel.Provider,
				"model":    zedConfig.Agent.DefaultModel.Model,
			}
		}
	}

	// Use app.Updated for version, or current time if app is minimal
	version := app.Updated.Unix()
	if version == 0 {
		version = session.Updated.Unix()
	}

	// Build CodeAgentConfig from the spec task's assistant configuration
	var codeAgentConfig *types.CodeAgentConfig
	if session.Metadata.SpecTaskID != "" {
		// Get the spec task to find the associated app
		specTask, err := apiServer.Store.GetSpecTask(ctx, session.Metadata.SpecTaskID)
		if err != nil {
			log.Error().Err(err).Str("spec_task_id", session.Metadata.SpecTaskID).Msg("Failed to get spec task for code agent config")
			return nil, system.NewHTTPError500(fmt.Sprintf("failed to get spec task: %v", err))
		}

		if specTask.HelixAppID == "" {
			log.Error().Str("spec_task_id", session.Metadata.SpecTaskID).Msg("Spec task has no HelixAppID configured")
			return nil, system.NewHTTPError500("spec task has no app configured")
		}

		// Get the app to find the code agent assistant
		specTaskApp, err := apiServer.Store.GetApp(ctx, specTask.HelixAppID)
		if err != nil {
			log.Error().Err(err).Str("app_id", specTask.HelixAppID).Msg("Failed to get app for code agent config")
			return nil, system.NewHTTPError500(fmt.Sprintf("failed to get app: %v", err))
		}

		codeAgentConfig = buildCodeAgentConfig(specTaskApp, helixAPIURL)
		if codeAgentConfig == nil {
			log.Error().
				Str("session_id", sessionID).
				Str("spec_task_id", session.Metadata.SpecTaskID).
				Str("app_id", specTask.HelixAppID).
				Msg("No zed_external assistant found in app")
			return nil, system.NewHTTPError500("no code agent (zed_external assistant) configured in app")
		}

		log.Debug().
			Str("session_id", sessionID).
			Str("spec_task_id", session.Metadata.SpecTaskID).
			Str("provider", codeAgentConfig.Provider).
			Str("model", codeAgentConfig.Model).
			Str("api_type", codeAgentConfig.APIType).
			Msg("Built code agent config from spec task")
	}

	// Note: Zed keybindings for system clipboard (Ctrl+C/V → editor::Copy/Paste)
	// are configured in keymap.json created by start-zed-helix.sh startup script

	response := &types.ZedConfigResponse{
		ContextServers:  contextServers,
		LanguageModels:  languageModels,
		Assistant:       assistant,
		ExternalSync:    externalSync,
		Agent:           agentConfig,
		Theme:           zedConfig.Theme,
		Version:         version,
		CodeAgentConfig: codeAgentConfig,
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

	var zedConfig *external_agent.ZedMCPConfig

	// If session has no parent app (e.g., exploratory sessions), return empty config
	if session.ParentApp == "" {
		log.Debug().Str("session_id", sessionID).Msg("Session has no parent app - returning empty Zed config")
		zedConfig = &external_agent.ZedMCPConfig{
			ContextServers: make(map[string]external_agent.ContextServerConfig),
		}
	} else {
		// Get Helix config for app-based sessions
		app, err := apiServer.Store.GetApp(ctx, session.ParentApp)
		if err != nil {
			log.Warn().Err(err).Str("app_id", session.ParentApp).Str("session_id", sessionID).Msg("Parent app not found - falling back to empty config")
			// Fall back to empty config if app doesn't exist
			zedConfig = &external_agent.ZedMCPConfig{
				ContextServers: make(map[string]external_agent.ContextServerConfig),
			}
		} else {
			helixAPIURL := apiServer.Cfg.WebServer.URL
			if helixAPIURL == "" {
				helixAPIURL = "http://api:8080"
			}

			helixToken := apiServer.Cfg.WebServer.RunnerToken
			if helixToken == "" {
				log.Warn().Msg("RUNNER_TOKEN not configured")
			}

			generatedConfig, err := external_agent.GenerateZedMCPConfig(app, session.Owner, sessionID, helixAPIURL, helixToken)
			if err != nil {
				log.Error().Err(err).Msg("Failed to generate Zed config")
				return nil, system.NewHTTPError500("failed to generate Zed config")
			}
			zedConfig = generatedConfig
		}
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

// buildCodeAgentConfig creates a CodeAgentConfig from the app's zed_external assistant configuration.
// Returns nil if no zed_external assistant is found.
func buildCodeAgentConfig(app *types.App, helixURL string) *types.CodeAgentConfig {
	// Find the assistant with AgentType = zed_external
	for _, assistant := range app.Config.Helix.Assistants {
		if assistant.AgentType == types.AgentTypeZedExternal {
			return buildCodeAgentConfigFromAssistant(&assistant, helixURL)
		}
	}
	return nil
}

// buildCodeAgentConfigFromAssistant creates a CodeAgentConfig from an assistant configuration.
func buildCodeAgentConfigFromAssistant(assistant *types.AssistantConfig, helixURL string) *types.CodeAgentConfig {
	provider := strings.ToLower(assistant.Provider)

	var baseURL, apiType, agentName string

	switch provider {
	case "anthropic":
		// Anthropic uses the /v1/messages endpoint
		baseURL = helixURL + "/v1"
		apiType = "anthropic"
		agentName = "claude-code"
	case "azure", "azure_openai":
		// Azure OpenAI uses the /openai/deployments/{model}/chat/completions endpoint
		baseURL = helixURL + "/openai"
		apiType = "azure_openai"
		agentName = "azure-agent"
	default:
		// OpenAI, OpenRouter, TogetherAI, Helix, etc. use the /v1/chat/completions endpoint
		baseURL = helixURL + "/v1"
		apiType = "openai"
		agentName = "qwen"
	}

	return &types.CodeAgentConfig{
		Provider:  assistant.Provider,
		Model:     assistant.Model,
		AgentName: agentName,
		BaseURL:   baseURL,
		APIType:   apiType,
	}
}
