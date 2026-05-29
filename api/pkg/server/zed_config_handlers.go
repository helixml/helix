package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/goose"
	modelPkg "github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/services"
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
	// remote sandboxes can reach the API. Only use http://api:8080 in
	// development where sandboxes run in the same Docker network.
	sandboxAPIURL := apiServer.Cfg.WebServer.SandboxAPIURL
	if sandboxAPIURL == "" {
		// Default to external URL so remote sandboxes work out of the box
		sandboxAPIURL = helixAPIURL
	}

	// Get API key for MCP and LLM authentication
	helixToken, err := apiServer.getAPIKeyForSession(ctx, session)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get API key for session")
		return nil, system.NewHTTPError500("failed to get API key for session")
	}

	// Determine if Kodit should be enabled for this session
	// 1. Must be globally enabled via Kodit.Enabled config
	// 2. Project must have KoditEnabled toggle on
	// 3. For SpecTask sessions, also check if project repos have Kodit indexing enabled
	koditEnabled := apiServer.Cfg.Kodit.Enabled
	if koditEnabled && session.Metadata.SpecTaskID != "" {
		// This is a SpecTask session - check if project repos have Kodit indexing
		koditEnabled = apiServer.checkSpecTaskKoditIndexing(ctx, session.Metadata.SpecTaskID)
	}

	// Get project skills if session has a project
	var projectSkills *types.AssistantSkills
	if session.ProjectID != "" {
		project, err := apiServer.Store.GetProject(ctx, session.ProjectID)
		if err != nil {
			log.Error().Err(err).Str("project_id", session.ProjectID).Msg("Failed to get project for skills config")
			return nil, system.NewHTTPError500("failed to get project for skills config")
		}
		projectSkills = project.Skills

		// Gate Kodit on the project-level toggle
		if koditEnabled && !project.KoditEnabled {
			koditEnabled = false
		}
	}

	// Use sandboxAPIURL for Zed config - this is the URL Zed uses to call the Helix API
	// In dev mode (SANDBOX_API_URL set): uses internal Docker network (http://api:8080)
	// In production (SANDBOX_API_URL not set): uses external URL (SERVER_URL)
	//
	// Create OAuth token getter for stdio MCPs that need OAuth tokens
	oauthTokenGetter := func(ctx context.Context, userID, providerName string) (string, error) {
		if apiServer.oauthManager == nil {
			return "", nil
		}
		// Use empty scopes - the token getter will use whatever scopes the user has
		return apiServer.oauthManager.GetTokenForTool(ctx, userID, providerName, nil)
	}
	providerSnapshot, err := apiServer.getProviderSnapshot(ctx, session.Owner, app)
	if err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("zed-config: failed to list providers; provider resolution will be skipped")
	}
	// Heal-on-read rewrites legacy name refs to immutable IDs. Skip the
	// persisted write for runner-token requests so two concurrent runner
	// pulls don't race UpdateApp and runner traffic doesn't bump
	// app.UpdatedAt — the in-memory rewrite still feeds Generate below.
	apiServer.healLegacyProviderRefs(ctx, app, providerSnapshot, user.TokenType != types.TokenTypeRunner)
	zedConfig, err := external_agent.GenerateZedMCPConfig(ctx, app, session.Owner, sessionID, sandboxAPIURL, helixToken, koditEnabled, projectSkills, oauthTokenGetter, providerSnapshot)
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate Zed config")
		return nil, system.NewHTTPError500("failed to generate Zed config")
	}

	// Hard-fail when the agent's stored model config is empty or references
	// an unknown provider. The settings-sync-daemon uses this endpoint as
	// its source of truth on session start; failing fast here surfaces the
	// real problem (broken agent config) in the spec-task UI rather than
	// silently spinning up a sandbox where Zed would fall back to its
	// built-in default model and confuse the user.
	if zedConfig.Misconfigured {
		return nil, system.NewHTTPError422(zedConfig.MisconfigReason)
	}

	// Convert to response format - include ALL fields from zedConfig
	contextServers := make(map[string]interface{})
	for name, server := range zedConfig.ContextServers {
		serverMap := make(map[string]interface{})

		// Upstream Zed uses untagged enum for context server config:
		// - Has "url" field → Http variant
		// - Has "command" field → Stdio variant
		// - Has "settings" field → Extension variant
		// The "source" field is no longer used (deprecated).
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
	// api_key is NOT included here — Zed reads ANTHROPIC_API_KEY / OPENAI_API_KEY from
	// container env vars (set by DesktopAgentAPIEnvVars). Only api_url is needed in settings.
	languageModels := make(map[string]interface{})
	for provider, config := range zedConfig.LanguageModels {
		modelConfig := map[string]interface{}{
			"api_url": config.APIURL,
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
			"show_onboarding": zedConfig.Agent.ShowOnboarding,
			"auto_open_panel": zedConfig.Agent.AutoOpenPanel,
		}
		// Use tool_permissions instead of deprecated always_allow_tool_actions
		if zedConfig.Agent.AlwaysAllowToolActions {
			agentConfig["tool_permissions"] = map[string]interface{}{
				"default": "allow",
			}
		}
		// Add default_model if configured
		if zedConfig.Agent.DefaultModel != nil {
			agentConfig["default_model"] = map[string]interface{}{
				"provider": zedConfig.Agent.DefaultModel.Provider,
				"model":    zedConfig.Agent.DefaultModel.Model,
			}
		}
		// Add feature-specific models to prevent Zed from using hardcoded gpt-4.1-mini defaults
		if zedConfig.Agent.InlineAssistantModel != nil {
			agentConfig["inline_assistant_model"] = map[string]interface{}{
				"provider": zedConfig.Agent.InlineAssistantModel.Provider,
				"model":    zedConfig.Agent.InlineAssistantModel.Model,
			}
		}
		if zedConfig.Agent.CommitMessageModel != nil {
			agentConfig["commit_message_model"] = map[string]interface{}{
				"provider": zedConfig.Agent.CommitMessageModel.Provider,
				"model":    zedConfig.Agent.CommitMessageModel.Model,
			}
		}
		if zedConfig.Agent.ThreadSummaryModel != nil {
			agentConfig["thread_summary_model"] = map[string]interface{}{
				"provider": zedConfig.Agent.ThreadSummaryModel.Provider,
				"model":    zedConfig.Agent.ThreadSummaryModel.Model,
			}
		}
	}

	// Use app.Updated for version, or current time if app is minimal
	version := app.Updated.Unix()
	if version == 0 {
		version = session.Updated.Unix()
	}

	// Build CodeAgentConfig from whichever app drives this session's
	// runtime. Mirrors getAgentNameForSession's source order: spec
	// task's HelixAppID first, then session.ParentApp — so any
	// zed_external session opened via /sessions/chat against a
	// claude_code (or other custom-runtime) agent ships the full
	// CodeAgentConfig, not just an "agent_name". Previously only the
	// spec-task path was covered.
	var codeAgentConfig *types.CodeAgentConfig
	var sessionProjectID = session.Metadata.ProjectID
	if session.Metadata.SpecTaskID != "" {
		if specTask, err := apiServer.Store.GetSpecTask(ctx, session.Metadata.SpecTaskID); err == nil {
			if specTask.ProjectID != "" {
				sessionProjectID = specTask.ProjectID
			}
			if specTask.HelixAppID != "" {
				if app, err := apiServer.Store.GetApp(ctx, specTask.HelixAppID); err == nil {
					codeAgentConfig = apiServer.buildCodeAgentConfig(ctx, app, sandboxAPIURL, sessionProjectID)
					apiServer.applySpecTaskGooseRecipe(ctx, specTask, codeAgentConfig)
				}
			}
		}
	}
	if codeAgentConfig == nil && session.ParentApp != "" {
		if app, err := apiServer.Store.GetApp(ctx, session.ParentApp); err == nil {
			codeAgentConfig = apiServer.buildCodeAgentConfig(ctx, app, sandboxAPIURL, sessionProjectID)
		}
	}

	// Check if user has an active Claude subscription (for credential sync in containers)
	var claudeSubAvailable bool
	if codeAgentConfig != nil && codeAgentConfig.Runtime == types.CodeAgentRuntimeClaudeCode {
		sub, err := apiServer.Store.GetEffectiveClaudeSubscription(ctx, session.Owner, session.OrganizationID)
		if err == nil && sub.Status == "active" {
			claudeSubAvailable = true
		}
	}

	// Note: Zed keybindings for system clipboard (Ctrl+C/V → editor::Copy/Paste)
	// are configured in keymap.json created by start-zed-helix.sh startup script

	// Resolve session owner's color scheme preference. The desktop follows the
	// owner — not whoever is currently watching — so two reviewers viewing the
	// same session can't fight over light/dark.
	ownerColorScheme := ""
	if ownerMeta, err := apiServer.Store.GetUserMeta(ctx, session.Owner); err == nil && ownerMeta != nil {
		ownerColorScheme = ownerMeta.Config.ColorScheme
	}
	zedTheme := zedConfig.Theme
	if ownerColorScheme == "light" {
		zedTheme = "One Light"
	} else if ownerColorScheme == "dark" {
		zedTheme = "Ayu Dark"
	}

	response := &types.ZedConfigResponse{
		ContextServers:              contextServers,
		LanguageModels:              languageModels,
		Assistant:                   assistant,
		ExternalSync:                externalSync,
		Agent:                       agentConfig,
		Theme:                       zedTheme,
		ColorScheme:                 ownerColorScheme,
		Version:                     version,
		CodeAgentConfig:             codeAgentConfig,
		ClaudeSubscriptionAvailable: claudeSubAvailable,
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

// @Summary Get merged Zed MCP context_servers for a session
// @Description Returns the union of helix-managed and user-side MCP context_servers,
// @Description for the session "MCP Tools" panel in the UI. Other Zed settings
// @Description (agent.*, language_models, theme) are owned by the daemon — anything
// @Description that needs the full Zed view goes through the settings-sync-daemon
// @Description on /zed-config + a local merge.
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

	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session")
		return nil, system.NewHTTPError404("session not found")
	}

	user := getRequestUser(req)
	if user == nil || session.Owner != user.ID {
		return nil, system.NewHTTPError403("access denied")
	}

	var app *types.App
	if session.ParentApp != "" {
		app, err = apiServer.Store.GetApp(ctx, session.ParentApp)
		if err != nil {
			log.Warn().Err(err).Str("app_id", session.ParentApp).Str("session_id", sessionID).Msg("Parent app not found - using default config")
			app = nil
		}
	}
	if app == nil {
		app = &types.App{ID: "default-agent", Config: types.AppConfig{}}
	}

	helixAPIURL := apiServer.Cfg.WebServer.SandboxAPIURL
	if helixAPIURL == "" {
		helixAPIURL = apiServer.Cfg.WebServer.URL
		if helixAPIURL == "" {
			helixAPIURL = "http://api:8080"
		}
	}

	helixToken, err := apiServer.getAPIKeyForSession(ctx, session)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get API key for session")
		return nil, system.NewHTTPError500("failed to get API key for session")
	}

	var projectSkills *types.AssistantSkills
	if session.ProjectID != "" {
		project, err := apiServer.Store.GetProject(ctx, session.ProjectID)
		if err != nil {
			log.Error().Err(err).Str("project_id", session.ProjectID).Msg("Failed to get project for skills config")
			return nil, system.NewHTTPError500("failed to get project for skills config")
		}
		projectSkills = project.Skills
	}

	oauthTokenGetter := func(ctx context.Context, userID, providerName string) (string, error) {
		if apiServer.oauthManager == nil {
			return "", nil
		}
		return apiServer.oauthManager.GetTokenForTool(ctx, userID, providerName, nil)
	}
	// providerSnapshot=nil here: this endpoint only exposes context_servers,
	// which don't depend on provider resolution or model validation. The
	// daemon hits /zed-config separately and handles those concerns there.
	zedConfig, err := external_agent.GenerateZedMCPConfig(ctx, app, session.Owner, sessionID, helixAPIURL, helixToken, apiServer.Cfg.Kodit.Enabled, projectSkills, oauthTokenGetter, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate Zed config")
		return nil, system.NewHTTPError500("failed to generate Zed config")
	}

	userOverrides, err := external_agent.GetUserZedOverrides(ctx, apiServer.Store, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get user overrides")
		return nil, system.NewHTTPError500("failed to get user overrides")
	}

	return map[string]interface{}{
		"context_servers": external_agent.MergeContextServers(zedConfig.ContextServers, userOverrides),
	}, nil
}

// getAgentNameForSession determines which code agent to use for a session.
// Priority: 1) stored ZedAgentName on the session (set when thread was created),
// 2) current app config (fallback for older sessions without stored agent name).
// This ensures we use the agent that actually created the thread, not whatever
// the app config happens to be now (which may have changed since the thread was created).
func (apiServer *HelixAPIServer) getAgentNameForSession(ctx context.Context, session *types.Session) string {
	// Use the stored agent name if available (set when the thread was first created)
	if session.Metadata.ZedAgentName != "" {
		return session.Metadata.ZedAgentName
	}

	agentName := "zed-agent" // Default to Zed's built-in agent

	// Resolve the app whose code_agent_runtime drives this session's
	// runtime choice. Two sources, in order:
	//   - spec task's HelixAppID, for spec-task-driven sessions
	//   - session.ParentApp, for any direct /sessions/chat caller that
	//     opens a session against an agent app (e.g. helix-org's
	//     embedded Spawner). Previously the function early-returned
	//     "zed-agent" for non-spec-task sessions, ignoring the parent
	//     app's CodeAgentRuntime entirely — so a claude_code agent
	//     opened via /sessions/chat got told "you're zed-agent" and
	//     fell through to the Anthropic proxy.
	var (
		runtimeApp *types.App
		source     string
	)
	if session.Metadata.SpecTaskID != "" {
		if specTask, err := apiServer.Store.GetSpecTask(ctx, session.Metadata.SpecTaskID); err == nil && specTask.HelixAppID != "" {
			if app, err := apiServer.Store.GetApp(ctx, specTask.HelixAppID); err == nil {
				runtimeApp = app
				source = "spec_task"
			}
		}
	}
	if runtimeApp == nil && session.ParentApp != "" {
		if app, err := apiServer.Store.GetApp(ctx, session.ParentApp); err == nil {
			runtimeApp = app
			source = "parent_app"
		}
	}
	if runtimeApp == nil {
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

	codeAgentConfig := apiServer.buildCodeAgentConfig(ctx, runtimeApp, sandboxAPIURL, "")
	if codeAgentConfig != nil {
		agentName = codeAgentConfig.AgentName
		log.Info().
			Str("session_id", session.ID).
			Str("source", source).
			Str("app_id", runtimeApp.ID).
			Str("agent_name", agentName).
			Str("runtime", string(codeAgentConfig.Runtime)).
			Msg("Using code agent config")
	}

	return agentName
}

// buildCodeAgentConfig creates a CodeAgentConfig from the app's zed_external assistant configuration.
// Returns nil if no zed_external assistant is found.
//
// projectID, when non-empty, is used by the Goose runtime to resolve recipe
// repo URLs to absolute container paths via the project's attached
// GitRepositories. Pass "" when the caller has no project context (e.g.
// non-spec-task sessions); recipes will simply not be wired up.
func (apiServer *HelixAPIServer) buildCodeAgentConfig(ctx context.Context, app *types.App, helixURL string, projectID string) *types.CodeAgentConfig {
	// Find the assistant with AgentType = zed_external
	for _, assistant := range app.Config.Helix.Assistants {
		if assistant.AgentType == types.AgentTypeZedExternal {
			// Resolve the agent's stored provider token (ID or legacy name) to
			// the provider's current canonical name so the model identifier
			// matches what GenerateZedMCPConfig writes into agent.default_model.
			// Without this resolution available_models would carry "pe_xxx/..."
			// while default_model carries "numpty/..." (the renamed provider's
			// current name) — Zed's model picker fails the lookup and falls
			// back to its built-in Claude default.
			//
			// app.Owner drives the actor identity here — buildCodeAgentConfig
			// has no session/request context. Org providers are still
			// resolved via the org bucket inside getProviderSnapshot.
			snapshot, err := apiServer.getProviderSnapshot(ctx, app.Owner, app)
			if err != nil {
				log.Warn().Err(err).Str("app_id", app.ID).Msg("buildCodeAgentConfig: provider snapshot unavailable; model prefix may not match agent.default_model")
			}
			cfg := apiServer.buildCodeAgentConfigFromAssistant(ctx, &assistant, helixURL, snapshot)
			if cfg != nil && cfg.Runtime == types.CodeAgentRuntimeGooseCode && projectID != "" && len(assistant.GooseRecipes) > 0 {
				if err := apiServer.resolveGooseRecipesIntoConfig(ctx, app, &assistant, projectID, cfg); err != nil {
					log.Warn().Err(err).Str("app_id", app.ID).Str("project_id", projectID).Msg("buildCodeAgentConfig: failed to resolve goose recipes; slash commands will be unavailable in this session")
				}
			}
			return cfg
		}
	}
	return nil
}

// buildCodeAgentConfigFromAssistant creates a CodeAgentConfig from an assistant configuration.
// For zed_external agents, use GenerationModelProvider/GenerationModel - that's where the UI
// stores the user's model selection for external agents.
// The CodeAgentRuntime determines how the LLM is configured in Zed (built-in agent vs qwen).
func (apiServer *HelixAPIServer) buildCodeAgentConfigFromAssistant(ctx context.Context, assistant *types.AssistantConfig, helixURL string, providerSnapshot []external_agent.ProviderRef) *types.CodeAgentConfig {
	// Get the code agent runtime, default to zed_agent
	runtime := assistant.CodeAgentRuntime
	if runtime == "" {
		runtime = types.CodeAgentRuntimeZedAgent
	}

	// Check if this agent uses subscription-based credentials (e.g., Claude OAuth)
	isSubscription := assistant.CodeAgentCredentialType.IsSubscription()

	// For zed_external agents, use generation model fields (that's where the UI sets the model)
	// Fall back to primary provider/model only if generation fields are empty.
	providerName := assistant.GenerationModelProvider
	if providerName == "" {
		providerName = assistant.Provider
	}
	modelName := assistant.GenerationModel
	if modelName == "" {
		modelName = assistant.Model
	}

	// Resolve the agent's stored provider token (ID or legacy name) to the
	// provider's current canonical name. Required so the model prefix here
	// matches what GenerateZedMCPConfig produces for agent.default_model —
	// see comment in buildCodeAgentConfig.
	if providerSnapshot != nil && providerName != "" {
		if resolved, _, ok := external_agent.ResolveProvider(providerName, providerSnapshot); ok {
			providerName = resolved.Name
		}
	}

	// Subscription agents don't need provider/model (they use OAuth credentials).
	// API key agents require both.
	if !isSubscription && (providerName == "" || modelName == "") {
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

	case types.CodeAgentRuntimeClaudeCode:
		agentName = "claude"
		if isSubscription {
			// Subscription mode: Claude Code talks directly to Anthropic
			// using OAuth credentials from ~/.claude/.credentials.json
			providerName = ""
			baseURL = ""
			apiType = ""
			model = ""
		} else {
			// API key mode: route through Helix proxy.
			// IMPORTANT: Use helixURL without "/v1" suffix because the Anthropic SDK
			// (used by Claude Code) appends "/v1/messages" to ANTHROPIC_BASE_URL.
			// Helix serves the Anthropic proxy at /v1/messages (registered in server.go).
			baseURL = helixURL
			apiType = "anthropic"
			model = modelName
		}

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

	// Look up model info to get token limits
	// Get token limits from model info if available (0 means use agent defaults)
	var maxTokens, maxOutputTokens int
	if apiServer.modelInfoProvider != nil {
		modelInfo, err := apiServer.modelInfoProvider.GetModelInfo(ctx, &modelPkg.ModelInfoRequest{
			Provider: providerName,
			Model:    modelName,
		})
		if err == nil {
			maxTokens = modelInfo.ContextLength
			maxOutputTokens = modelInfo.MaxCompletionTokens
		}
	}

	return &types.CodeAgentConfig{
		Provider:        providerName,
		Model:           model,
		AgentName:       agentName,
		BaseURL:         baseURL,
		APIType:         apiType,
		Runtime:         runtime,
		MaxTokens:       maxTokens,
		MaxOutputTokens: maxOutputTokens,
	}
}

// resolveGooseRecipesIntoConfig populates cfg.GooseRecipes (and
// GooseRecipeRootDir) by resolving the assistant's recipe repo URL against
// the project's attached GitRepositories. Recipes whose source files are
// missing on disk are skipped with a warn-level log — `goose acp` silently
// drops unparseable slash_commands entries, so leaving them in the config
// would produce a confusing partial state.
func (apiServer *HelixAPIServer) resolveGooseRecipesIntoConfig(ctx context.Context, app *types.App, assistant *types.AssistantConfig, _ string, cfg *types.CodeAgentConfig) error {
	if len(assistant.GooseRecipes) == 0 {
		return nil
	}
	if assistant.GooseRecipeRepoURL == "" {
		return fmt.Errorf("agent declares goose recipes but no recipe_repo_url is set")
	}
	repo, err := apiServer.Store.GetGitRepositoryByURL(ctx, app.OrganizationID, assistant.GooseRecipeRepoURL)
	if err != nil {
		return fmt.Errorf("recipe repo %s not found in this organization: %w", assistant.GooseRecipeRepoURL, err)
	}
	if repo.LocalPath == "" {
		// Repo hasn't been cloned yet — recipes will appear on the next
		// session start after the clone completes. Log and continue.
		return fmt.Errorf("recipe repo %s not yet cloned (LocalPath empty)", repo.ID)
	}

	cfg.GooseRecipeRootDir = repo.LocalPath
	resolved := make([]types.CodeAgentGooseRecipe, 0, len(assistant.GooseRecipes))
	for _, r := range assistant.GooseRecipes {
		// `filepath.Clean` defends against absolute paths or ".." escapes
		// even though applyProject already rejects them — defence in depth
		// for recipes added via direct DB/API writes that bypassed the
		// apply handler.
		clean := filepath.Clean(r.Path)
		if strings.HasPrefix(clean, "..") || strings.HasPrefix(clean, "/") {
			log.Warn().Str("recipe", r.Name).Str("path", r.Path).Msg("skipping goose recipe with unsafe path")
			continue
		}
		abs := filepath.Join(repo.LocalPath, clean)
		resolved = append(resolved, types.CodeAgentGooseRecipe{Name: r.Name, Path: abs})
	}
	cfg.GooseRecipes = resolved
	return nil
}

// applySpecTaskGooseRecipe loads the spec-task's selected recipe file, bakes
// parameter values into it, and writes the result onto cfg.GooseBakedRecipe.
// The daemon will write the baked YAML to ${XDG_CONFIG_HOME}/goose/baked-recipes/
// and register a single slash_command for it, so the agent's first instruction
// can be `/<recipe-name>` to fire it.
//
// No-op when the spec task has no recipe selected, when the agent isn't goose,
// or when the named recipe isn't part of the agent's GooseRecipes (e.g. the
// recipe was removed from the project YAML between task creation and now).
func (apiServer *HelixAPIServer) applySpecTaskGooseRecipe(ctx context.Context, specTask *types.SpecTask, cfg *types.CodeAgentConfig) {
	if specTask == nil || cfg == nil {
		return
	}
	if cfg.Runtime != types.CodeAgentRuntimeGooseCode {
		return
	}
	if specTask.GooseRecipeName == "" {
		return
	}

	var sourcePath string
	for _, r := range cfg.GooseRecipes {
		if r.Name == specTask.GooseRecipeName {
			sourcePath = r.Path
			break
		}
	}
	if sourcePath == "" {
		log.Warn().Str("spec_task_id", specTask.ID).Str("recipe", specTask.GooseRecipeName).Msg("spec-task references a goose recipe not declared on the agent; baking skipped")
		return
	}

	content, err := os.ReadFile(sourcePath)
	if err != nil {
		log.Warn().Err(err).Str("recipe_path", sourcePath).Msg("failed to read goose recipe source for baking")
		return
	}

	// Resolve file-typed parameters: the user supplied an attachment
	// filename on the spec-task; we need to substitute the absolute path
	// where that attachment lives inside the agent's workspace (committed
	// to the helix-specs branch at /home/retro/work/helix-specs/design/
	// tasks/<dir>/attachments/<filename>).
	params, err := apiServer.resolveGooseRecipeFileParams(ctx, specTask, content)
	if err != nil {
		log.Warn().Err(err).Str("spec_task_id", specTask.ID).Str("recipe", specTask.GooseRecipeName).Msg("failed to resolve file params for goose recipe; baking skipped")
		return
	}

	baked, err := goose.Bake(content, params)
	if err != nil {
		log.Warn().Err(err).Str("spec_task_id", specTask.ID).Str("recipe", specTask.GooseRecipeName).Msg("failed to bake goose recipe; falling back to unbaked")
		return
	}

	cfg.GooseBakedRecipe = &types.CodeAgentBakedRecipe{
		Name:    specTask.GooseRecipeName,
		Content: baked,
	}
}

// resolveGooseRecipeFileParams returns a copy of specTask.GooseRecipeParams
// with values for file-typed parameters rewritten from "<filename>" to the
// absolute path where the spec-task's attachment lives inside the agent's
// workspace. Non-file parameters pass through unchanged. The recipe is
// re-parsed (cheap, the file is already in memory) to discover which params
// are file-typed — we deliberately don't trust the frontend to tell us.
func (apiServer *HelixAPIServer) resolveGooseRecipeFileParams(ctx context.Context, specTask *types.SpecTask, recipeContent []byte) (map[string]string, error) {
	out := make(map[string]string, len(specTask.GooseRecipeParams))
	for k, v := range specTask.GooseRecipeParams {
		out[k] = v
	}

	recipe, err := goose.Parse(recipeContent)
	if err != nil {
		// Caller will surface the same parse error from Bake; just pass the
		// raw params through and let Bake fail with its own message.
		return out, nil
	}

	var fileParamKeys []string
	for _, p := range recipe.Parameters {
		if p.InputType == "file" {
			fileParamKeys = append(fileParamKeys, p.Key)
		}
	}
	if len(fileParamKeys) == 0 {
		return out, nil
	}

	attachments, err := apiServer.Store.ListSpecTaskAttachments(ctx, specTask.ID)
	if err != nil {
		return nil, fmt.Errorf("list spec-task attachments: %w", err)
	}
	byName := make(map[string]*types.SpecTaskAttachment, len(attachments))
	for _, a := range attachments {
		byName[a.Filename] = a
	}

	// Mirror services.spec_task_prompts.go: DesignDocPath is the canonical
	// task directory; ID is the legacy fallback for old tasks.
	taskDirName := specTask.DesignDocPath
	if taskDirName == "" {
		taskDirName = specTask.ID
	}

	for _, key := range fileParamKeys {
		filename := strings.TrimSpace(out[key])
		if filename == "" {
			// Optional file param, no value supplied — leave it for Bake's
			// required-param validation to handle.
			continue
		}
		if _, ok := byName[filename]; !ok {
			return nil, fmt.Errorf("recipe file parameter %q references attachment %q which is not uploaded on this spec task", key, filename)
		}
		out[key] = fmt.Sprintf("/home/retro/work/helix-specs/design/tasks/%s/attachments/%s", taskDirName, filename)
	}

	return out, nil
}

// getProviderSnapshot returns a ProviderRef snapshot of every provider
// visible to the actor for the given app — env-baked globals (ID="",
// Name=canonical) plus DB-backed user/org records (ID set, Name=current
// admin label). Used by zed-config code paths to resolve an agent's stored
// provider reference to its current canonical name.
//
// The app argument is what makes this org-aware: when app.OrganizationID
// is set, org-owned providers are listed first and the user's personal
// providers are merged in (so a user running an org agent that references
// their own personal provider still resolves). actorID may be "" when
// there's no user context (rare; e.g. system-driven paths) — in that case
// only the app-owner bucket is returned.
//
// Returning nil from this helper (e.g. when the manager isn't wired) tells
// GenerateZedMCPConfig to skip resolution.
func (apiServer *HelixAPIServer) getProviderSnapshot(ctx context.Context, actorID string, app *types.App) ([]external_agent.ProviderRef, error) {
	if apiServer.providerManager == nil {
		return nil, nil
	}
	endpoints, err := apiServer.listEndpointsForApp(ctx, actorID, app)
	if err != nil {
		return nil, err
	}
	refs := make([]external_agent.ProviderRef, 0, len(endpoints))
	for _, ep := range endpoints {
		refs = append(refs, external_agent.ProviderRef{ID: ep.ID, Name: ep.Name})
	}
	return refs, nil
}

// listEndpointsForApp returns ProviderEndpoint records visible to the actor
// for the given app, with the org-first + user-merge pattern that
// validateProvidersAndModels established. Centralising it here means every
// caller (substitution, validation, zed-config, spec-task pre-flight) sees
// the same view of "what providers can this agent legitimately reference".
//
// Without the merge, an org-owned agent that references the org member's
// personal provider would 422 at session start; with it, both buckets
// participate in resolution.
func (apiServer *HelixAPIServer) listEndpointsForApp(ctx context.Context, actorID string, app *types.App) ([]*types.ProviderEndpoint, error) {
	if apiServer.providerManager == nil {
		return nil, nil
	}
	owner := actorID
	if app != nil && app.OrganizationID != "" {
		owner = app.OrganizationID
	}
	endpoints, err := apiServer.providerManager.ListProviderEndpoints(ctx, owner)
	if err != nil {
		return nil, err
	}
	if app == nil || app.OrganizationID == "" || actorID == "" || actorID == app.OrganizationID {
		return endpoints, nil
	}
	userEndpoints, uerr := apiServer.providerManager.ListProviderEndpoints(ctx, actorID)
	if uerr != nil {
		// Best-effort merge: a personal-provider lookup failure shouldn't
		// hide the org bucket we already have. Log and return what we got.
		log.Warn().Err(uerr).Str("app_id", app.ID).Str("actor_id", actorID).Msg("listEndpointsForApp: failed to list personal providers; using org bucket only")
		return endpoints, nil
	}
	seen := make(map[string]struct{}, len(endpoints))
	for _, ep := range endpoints {
		seen[endpointKey(ep)] = struct{}{}
	}
	for _, ep := range userEndpoints {
		k := endpointKey(ep)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		endpoints = append(endpoints, ep)
	}
	return endpoints, nil
}

// endpointKey produces a stable de-dup key for a provider endpoint. ID wins
// when present; for env-baked globals (ID=="") the name namespace is used
// to keep "openai" from colliding with a DB-backed provider also named
// "openai".
func endpointKey(ep *types.ProviderEndpoint) string {
	if ep.ID != "" {
		return "id:" + ep.ID
	}
	return "name:" + ep.Name
}

// validateSpecTaskAgentConfig pre-flights the agent's provider/model snapshot
// against the registered providers visible to the actor. Returns a
// human-readable reason (suitable for HTTP 422) when the agent is
// misconfigured, or "" when usable. Used by spec-task entry handlers
// (start-planning, approve-specs) to refuse to queue a task whose agent
// would fail at session start. Without this, a stale agent record would
// spawn a desktop that boots but can't reach a routable model — the user
// has to dig through API logs to find the cause.
//
// Resolves the agent the same way the spec-driven task service does:
// task.HelixAppID first, falling back to project.DefaultHelixAppID. If
// neither is set, returns "" — the absent-agent failure is surfaced
// downstream with its own dedicated message.
func (apiServer *HelixAPIServer) validateSpecTaskAgentConfig(ctx context.Context, task *types.SpecTask, actorID string) (string, error) {
	appID := task.HelixAppID
	if appID == "" {
		project, err := apiServer.Store.GetProject(ctx, task.ProjectID)
		if err != nil {
			return "", fmt.Errorf("failed to load project: %w", err)
		}
		appID = project.DefaultHelixAppID
	}
	if appID == "" {
		return "", nil
	}
	app, err := apiServer.Store.GetApp(ctx, appID)
	if err != nil {
		return "", fmt.Errorf("failed to load agent app %s: %w", appID, err)
	}
	snapshot, err := apiServer.getProviderSnapshot(ctx, actorID, app)
	if err != nil {
		log.Warn().Err(err).Str("app_id", appID).Msg("spec-task: failed to list providers; skipping agent config validation")
		return "", nil
	}
	// Spec-task entry handlers always run with a real user token, so persist
	// the heal-on-read rewrite — runner-token entry into this code path is
	// not a thing for these handlers.
	apiServer.healLegacyProviderRefs(ctx, app, snapshot, true)
	return external_agent.ValidateAssistantModelConfig(app, snapshot), nil
}

// healLegacyProviderRefs rewrites name-based provider references on the app
// to the matching DB-backed provider's immutable ID, so future renames are
// silent. Best-effort — a write failure just logs and lets the next read
// retry. See external_agent.MigrateLegacyProviderRefs for the rules.
//
// persist=false skips the UpdateApp write (used on runner-token reads of
// /zed-config so two concurrent runner pulls don't race UpdateApp and
// runner traffic doesn't bump app.UpdatedAt). The in-memory rewrite still
// happens — that's what feeds the immediate Generate / Validate call.
func (apiServer *HelixAPIServer) healLegacyProviderRefs(ctx context.Context, app *types.App, snapshot []external_agent.ProviderRef, persist bool) {
	if !external_agent.MigrateLegacyProviderRefs(app, snapshot) {
		return
	}
	if !persist {
		log.Debug().Str("app_id", app.ID).Msg("agent legacy-name → ID migration: in-memory only (runner-token read)")
		return
	}
	if _, err := apiServer.Store.UpdateApp(ctx, app); err != nil {
		log.Warn().Err(err).Str("app_id", app.ID).Msg("agent legacy-name → ID migration: persist failed; will retry on next read")
		return
	}
	log.Info().Str("app_id", app.ID).Msg("agent legacy-name → ID migration: rewrote provider fields to immutable IDs")
}

// checkSpecTaskKoditIndexing checks if a SpecTask's project has any repositories with Kodit indexing enabled.
// Returns true if any repository in the project has KoditIndexing enabled, false otherwise.
func (apiServer *HelixAPIServer) checkSpecTaskKoditIndexing(ctx context.Context, specTaskID string) bool {
	// Get the SpecTask
	specTask, err := apiServer.Store.GetSpecTask(ctx, specTaskID)
	if err != nil {
		log.Warn().Err(err).Str("spec_task_id", specTaskID).Msg("Failed to get SpecTask for Kodit indexing check")
		return false
	}

	// Check if the SpecTask has a project
	if specTask.ProjectID == "" {
		log.Debug().Str("spec_task_id", specTaskID).Msg("SpecTask has no project, Kodit disabled")
		return false
	}

	// Get all repositories for the project
	repos, err := apiServer.Store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{ProjectID: specTask.ProjectID})
	if err != nil {
		log.Warn().Err(err).Str("project_id", specTask.ProjectID).Msg("Failed to list project repositories for Kodit indexing check")
		return false
	}

	// Check if any repository has Kodit indexing enabled
	for _, repo := range repos {
		if repo.KoditIndexing {
			log.Debug().
				Str("spec_task_id", specTaskID).
				Str("project_id", specTask.ProjectID).
				Str("repo_id", repo.ID).
				Msg("Found repository with Kodit indexing enabled")
			return true
		}
	}

	return false
}

// getAPIKeyForSession returns a session-scoped ephemeral API key.
// Keys are minted when the desktop starts and revoked when it shuts down.
//
// SECURITY: This is the single source of truth for API key selection.
// All code that needs to authenticate on behalf of a user/session should use this function.
// The key capabilities vary based on session type:
// - SpecTask sessions: git push rights to specific branch, LLM calls
// - Non-SpecTask sessions: LLM calls only
func (apiServer *HelixAPIServer) getAPIKeyForSession(ctx context.Context, session *types.Session) (string, error) {
	if session == nil || session.ID == "" {
		return "", fmt.Errorf("session is required for session-scoped API key")
	}

	apiKey, err := apiServer.specDrivenTaskService.GetOrCreateSessionAPIKey(ctx, &services.SessionAPIKeyRequest{
		UserID:         session.Owner,
		SessionID:      session.ID,
		OrganizationID: session.OrganizationID,
	})
	if err != nil {
		return "", fmt.Errorf("failed to get session API key for session %s: %w", session.ID, err)
	}
	return apiKey, nil
}
