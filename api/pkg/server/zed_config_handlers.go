package server

import (
	"context"
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
	// Use SERVER_URL for external-facing URLs (browser access)
	helixAPIURL := apiServer.Cfg.WebServer.URL
	if helixAPIURL == "" {
		helixAPIURL = "http://api:8080"
	}

	// Use SANDBOX_API_URL for sandbox containers
	// This is the URL that Zed inside the sandbox uses to call the Helix API
	// If not explicitly set, use the external-facing URL (SERVER_URL) so that
	// remote Wolf sandboxes can reach the API. Only use http://api:8080 in
	// development where sandboxes run in the same Docker network.
	sandboxAPIURL := apiServer.Cfg.WebServer.SandboxAPIURL
	if sandboxAPIURL == "" {
		// Default to external URL so remote sandboxes work out of the box
		sandboxAPIURL = helixAPIURL
	}

	helixToken := apiServer.Cfg.WebServer.RunnerToken
	if helixToken == "" {
		log.Warn().Msg("RUNNER_TOKEN not configured")
	}

	// Use sandboxAPIURL for Zed config - this is the URL Zed uses to call the Helix API
	// In dev mode (SANDBOX_API_URL set): uses internal Docker network (http://api:8080)
	// In production (SANDBOX_API_URL not set): uses external URL (SERVER_URL)
	zedConfig, err := external_agent.GenerateZedMCPConfig(app, session.Owner, sessionID, sandboxAPIURL, helixToken, apiServer.Cfg.Kodit.Enabled)
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate Zed config")
		return nil, system.NewHTTPError500("failed to generate Zed config")
	}

	// Convert to response format - include ALL fields from zedConfig
	contextServers := make(map[string]interface{})
	for name, server := range zedConfig.ContextServers {
		serverMap := make(map[string]interface{})

		// HTTP-based MCP server
		// Zed expects "url" field for HTTP context_servers (untagged union)
		if server.URL != "" {
			serverMap["url"] = server.URL
			if len(server.Headers) > 0 {
				serverMap["headers"] = server.Headers
			}
		} else {
			// Stdio-based MCP server
			serverMap["command"] = server.Command
			serverMap["args"] = server.Args
			if len(server.Env) > 0 {
				serverMap["env"] = server.Env
			}
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

		codeAgentConfig = buildCodeAgentConfig(specTaskApp, sandboxAPIURL)
		if codeAgentConfig == nil {
			// No zed_external assistant configured - Zed will use built-in agent with defaults
			// from GenerateZedMCPConfig (language_models routing through Helix proxy)
			log.Debug().
				Str("session_id", sessionID).
				Str("spec_task_id", session.Metadata.SpecTaskID).
				Str("app_id", specTask.HelixAppID).
				Msg("No zed_external assistant found - using default Zed agent with Helix proxy")
		}
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

	// Get app config - fall back to default if not found or not set
	var app *types.App
	if session.ParentApp != "" {
		var err error
		app, err = apiServer.Store.GetApp(ctx, session.ParentApp)
		if err != nil {
			log.Warn().Err(err).Str("app_id", session.ParentApp).Str("session_id", sessionID).Msg("Parent app not found - using default config")
			app = nil
		}
	}

	// If no app found, use a default app with sensible defaults
	if app == nil {
		log.Debug().Str("session_id", sessionID).Msg("No parent app - using default Zed config with claude-sonnet")
		app = &types.App{
			ID:     "default-agent",
			Config: types.AppConfig{},
		}
	}

	// Use SANDBOX_API_URL for what Zed inside sandbox uses
	// If not explicitly set, default to external-facing URL (SERVER_URL)
	helixAPIURL := apiServer.Cfg.WebServer.SandboxAPIURL
	if helixAPIURL == "" {
		helixAPIURL = apiServer.Cfg.WebServer.URL
		if helixAPIURL == "" {
			helixAPIURL = "http://api:8080"
		}
	}

	helixToken := apiServer.Cfg.WebServer.RunnerToken
	if helixToken == "" {
		log.Warn().Msg("RUNNER_TOKEN not configured")
	}

	// Always generate config - GenerateZedMCPConfig has sensible defaults
	// (anthropic/claude-sonnet-4-5-latest, theme, language_models routing, etc.)
	zedConfig, err := external_agent.GenerateZedMCPConfig(app, session.Owner, sessionID, helixAPIURL, helixToken, apiServer.Cfg.Kodit.Enabled)
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

// getAgentNameForSession determines which code agent to use based on the session's spec task configuration.
// Returns "zed-agent" as default, or the configured agent name (e.g., "qwen") if a code agent is configured.
func (apiServer *HelixAPIServer) getAgentNameForSession(ctx context.Context, session *types.Session) string {
	agentName := "zed-agent" // Default to Zed's built-in agent

	if session.Metadata.SpecTaskID == "" {
		return agentName
	}

	specTask, err := apiServer.Store.GetSpecTask(ctx, session.Metadata.SpecTaskID)
	if err != nil || specTask.HelixAppID == "" {
		return agentName
	}

	specTaskApp, err := apiServer.Store.GetApp(ctx, specTask.HelixAppID)
	if err != nil {
		return agentName
	}

	// Use SANDBOX_API_URL for sandbox containers
	// If not explicitly set, default to external-facing URL (SERVER_URL)
	sandboxAPIURL := apiServer.Cfg.WebServer.SandboxAPIURL
	if sandboxAPIURL == "" {
		sandboxAPIURL = apiServer.Cfg.WebServer.URL
		if sandboxAPIURL == "" {
			sandboxAPIURL = "http://api:8080"
		}
	}

	codeAgentConfig := buildCodeAgentConfig(specTaskApp, sandboxAPIURL)
	if codeAgentConfig != nil {
		agentName = codeAgentConfig.AgentName
		log.Info().
			Str("session_id", session.ID).
			Str("spec_task_id", session.Metadata.SpecTaskID).
			Str("agent_name", agentName).
			Str("runtime", string(codeAgentConfig.Runtime)).
			Msg("Using code agent config from spec task")
	}

	return agentName
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
// For zed_external agents, use GenerationModelProvider/GenerationModel - that's where the UI
// stores the user's model selection for external agents.
// The CodeAgentRuntime determines how the LLM is configured in Zed (built-in agent vs qwen).
func buildCodeAgentConfigFromAssistant(assistant *types.AssistantConfig, helixURL string) *types.CodeAgentConfig {
	// Get the code agent runtime, default to zed_agent
	runtime := assistant.CodeAgentRuntime
	if runtime == "" {
		runtime = types.CodeAgentRuntimeZedAgent
	}

	// For zed_external agents, use generation model fields (that's where the UI sets the model)
	// Fall back to primary provider/model only if generation fields are empty
	providerName := assistant.GenerationModelProvider
	if providerName == "" {
		providerName = assistant.Provider
	}
	modelName := assistant.GenerationModel
	if modelName == "" {
		modelName = assistant.Model
	}

	// If still no provider/model, return nil (can't configure code agent without these)
	if providerName == "" || modelName == "" {
		return nil
	}

	provider := strings.ToLower(providerName)
	var baseURL, apiType, agentName, model string

	// The runtime choice determines how the LLM is configured in Zed
	switch runtime {
	case types.CodeAgentRuntimeQwenCode:
		// Qwen Code: Uses the qwen command as a custom agent_server
		// All providers go through OpenAI-compatible API with provider prefix
		baseURL = helixURL + "/v1"
		apiType = "openai"
		agentName = "qwen"
		model = fmt.Sprintf("%s/%s", providerName, modelName)

	default: // CodeAgentRuntimeZedAgent
		// Zed Agent: Uses Zed's built-in agent panel with env vars
		// The API type depends on the provider
		switch provider {
		case "anthropic":
			baseURL = helixURL + "/v1"
			apiType = "anthropic"
			agentName = "zed-agent"
			model = modelName
		case "azure", "azure_openai":
			baseURL = helixURL + "/openai"
			apiType = "azure_openai"
			agentName = "zed-agent"
			model = modelName
		default:
			// For other providers (OpenAI, OpenRouter, etc.), use OpenAI-compatible API
			baseURL = helixURL + "/v1"
			apiType = "openai"
			agentName = "zed-agent"
			model = fmt.Sprintf("%s/%s", providerName, modelName)
		}
	}

	return &types.CodeAgentConfig{
		Provider:  providerName,
		Model:     model,
		AgentName: agentName,
		BaseURL:   baseURL,
		APIType:   apiType,
		Runtime:   runtime,
	}
}
