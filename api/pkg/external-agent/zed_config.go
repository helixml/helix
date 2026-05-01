package external_agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// OAuthTokenGetter is a function that retrieves OAuth tokens for stdio MCPs
// Returns the access token and any error. If no token is available, returns empty string with no error.
type OAuthTokenGetter func(ctx context.Context, userID, providerName string) (string, error)

// ZedMCPConfig represents Zed's MCP configuration format
type ZedMCPConfig struct {
	ContextServers map[string]ContextServerConfig `json:"context_servers"`
	LanguageModels map[string]LanguageModelConfig `json:"language_models,omitempty"`
	Assistant      *AssistantSettings             `json:"assistant,omitempty"`
	ExternalSync   *ExternalSyncConfig            `json:"external_sync,omitempty"`
	Agent          *AgentConfig                   `json:"agent,omitempty"`
	Theme          string                         `json:"theme,omitempty"`

	// Misconfigured is set by GenerateZedMCPConfig when the agent's stored
	// provider/model is empty or references a provider that is not in the
	// supplied validProviders list. The fields are not serialized to clients
	// — handlers inspect them and return HTTP 422 so that session start fails
	// fast with a clear error in the spec-task UI rather than silently
	// spinning up a sandbox the user can't actually use.
	Misconfigured   bool   `json:"-"`
	MisconfigReason string `json:"-"`
}

type ExternalSyncConfig struct {
	Enabled       bool                 `json:"enabled"`
	WebsocketSync *WebsocketSyncConfig `json:"websocket_sync,omitempty"`
}

type WebsocketSyncConfig struct {
	Enabled     bool   `json:"enabled"`
	ExternalURL string `json:"external_url"`
}

type AgentConfig struct {
	DefaultModel           *ModelConfig `json:"default_model,omitempty"`
	InlineAssistantModel   *ModelConfig `json:"inline_assistant_model,omitempty"`
	CommitMessageModel     *ModelConfig `json:"commit_message_model,omitempty"`
	ThreadSummaryModel     *ModelConfig `json:"thread_summary_model,omitempty"`
	AlwaysAllowToolActions bool         `json:"always_allow_tool_actions"` // Deprecated: mapped to tool_permissions.default="allow" by handler
	ShowOnboarding         bool         `json:"show_onboarding"`
	AutoOpenPanel          bool         `json:"auto_open_panel"`
}

type LanguageModelConfig struct {
	APIURL string `json:"api_url"`           // Custom API URL (empty = use default provider URL)
	APIKey string `json:"api_key,omitempty"` // Deprecated: Zed reads from env vars (ANTHROPIC_API_KEY, OPENAI_API_KEY)
}

type AssistantSettings struct {
	Version      string       `json:"version"`
	DefaultModel *ModelConfig `json:"default_model,omitempty"`
}

type ModelConfig struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type ContextServerConfig struct {
	// Stdio-based MCP server (command execution)
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`

	// HTTP-based MCP server (direct connection)
	// Upstream Zed uses untagged enum — presence of "url" field indicates Http variant.
	// The "source" field is no longer used (deprecated in upstream Zed).
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// GenerateZedMCPConfig creates Zed MCP configuration from Helix app config.
// projectSkills are optional project-level skills that overlay on top of agent skills.
// oauthTokenGetter is optional - if provided, OAuth tokens will be injected into stdio MCPs.
// providerSnapshot is an optional list of provider records visible to the owner
// (env-baked globals + DB-backed). When non-nil, the agent's stored provider
// reference (ID for DB-backed, canonical name for globals) is resolved
// against it; the resolved provider's current name is used in the model
// prefix written to settings.json. A missing provider is treated as
// misconfiguration and Agent.DefaultModel is left unset. Pass nil to skip
// resolution (e.g. on the runner-side path where the manager isn't
// reachable) — the stored token is then used verbatim.
func GenerateZedMCPConfig(
	ctx context.Context,
	app *types.App,
	userID string,
	sessionID string,
	helixAPIURL string,
	helixToken string,
	koditEnabled bool,
	projectSkills *types.AssistantSkills,
	oauthTokenGetter OAuthTokenGetter,
	providerSnapshot []ProviderRef,
) (*ZedMCPConfig, error) {
	config := &ZedMCPConfig{
		ContextServers: make(map[string]ContextServerConfig),
	}

	// Set base Helix integration settings (always required)
	config.ExternalSync = &ExternalSyncConfig{
		Enabled: true,
		WebsocketSync: &WebsocketSyncConfig{
			Enabled:     true,
			ExternalURL: fmt.Sprintf("%s/api/v1/external-agents/sync?session_id=%s", helixAPIURL, sessionID),
		},
	}
	// Find the zed_external assistant configuration
	var assistant *types.AssistantConfig
	for i := range app.Config.Helix.Assistants {
		if app.Config.Helix.Assistants[i].AgentType == types.AgentTypeZedExternal {
			assistant = &app.Config.Helix.Assistants[i]
			break
		}
	}

	// Fallback to first assistant if no zed_external found
	if assistant == nil && len(app.Config.Helix.Assistants) > 0 {
		assistant = &app.Config.Helix.Assistants[0]
	}

	// For zed_external agents, prefer GenerationModel fields (where UI stores the selection)
	var provider, model string
	if assistant != nil {
		provider = assistant.GenerationModelProvider
		if provider == "" {
			provider = assistant.Provider
		}
		model = assistant.GenerationModel
		if model == "" {
			model = assistant.Model
		}
	}

	// Decide whether the agent's stored model fields are usable. There are
	// two failure modes we MUST NOT paper over:
	//
	//   1. Empty fields. Previously the code silently substituted
	//      anthropic/claude-sonnet-4-5-latest, which made misconfigured
	//      external agents look like working Claude agents in Zed (Deviqon
	//      bug, 2026-04-28).
	//   2. Provider deleted. The agent record stores the provider's
	//      immutable ID (DB-backed) or canonical name (env-baked globals).
	//      If the DB-backed provider is deleted, ID resolution fails and
	//      we report misconfig instead of feeding an unroutable string into
	//      the model prefix. Renames are no-ops because the ID survives.
	//
	// Spec-task entry handlers run the same validator
	// (ValidateAssistantModelConfig) before transitioning a task to a queued
	// state, so misconfigured agents should not reach session start. This
	// block is the defense-in-depth for any caller that bypasses those entry
	// handlers (default-app session, legacy code paths). When misconfigured,
	// we leave Agent.DefaultModel unset and log loudly; Zed falls back to
	// its built-in default.
	useAgentModel := true
	if assistant == nil {
		// No assistant means this is the default-app path used for sessions
		// without a parent app. Keep the legacy SaaS-friendly default so
		// those sessions still come up.
		provider = "anthropic"
		model = "claude-sonnet-4-5-latest"
	} else if reason := ValidateAssistantModelConfig(app, providerSnapshot); reason != "" {
		log.Error().
			Str("app_id", app.ID).
			Str("provider", provider).
			Str("model", model).
			Msg("zed-config: " + reason + " — refusing to write agent.default_model")
		useAgentModel = false
		config.Misconfigured = true
		config.MisconfigReason = reason
	} else if providerSnapshot != nil {
		// Resolve the stored token (ID or legacy name) to the provider's
		// current canonical name. settings.json carries the current name;
		// the agent record carries the immutable ID. Renames flow into
		// running sessions on the next 30s daemon poll.
		resolved, byLegacy, _ := ResolveProvider(provider, providerSnapshot)
		if byLegacy {
			log.Warn().
				Str("app_id", app.ID).
				Str("stored_provider", provider).
				Str("resolved_name", resolved.Name).
				Str("resolved_id", resolved.ID).
				Msg("zed-config: agent stores provider by name (legacy); re-save the agent so it stores the immutable provider ID")
		}
		provider = resolved.Name
	}

	// Configure agent. AlwaysAllowToolActions / ShowOnboarding / AutoOpenPanel
	// are always set; default_model and the feature-specific model overrides
	// are set only when we trust the agent's configuration.
	config.Agent = &AgentConfig{
		AlwaysAllowToolActions: true,
		ShowOnboarding:         false,
		AutoOpenPanel:          true,
	}
	if useAgentModel {
		// Map Helix provider to Zed's provider type and format model name
		// Zed only knows: anthropic, openai, google, ollama, copilot, lmstudio, deepseek
		// All other providers (nebius, together, openrouter, etc.) use OpenAI-compatible API
		zedProvider, zedModel := mapHelixToZedProvider(provider, model)
		// Set feature-specific models to prevent Zed from using its hardcoded
		// gpt-4.1-mini default for "fast" operations (see
		// zed-industries/zed#31420). If not set, these fall back to
		// default_model, but we set them explicitly to ensure all LLM calls
		// route through Helix.
		config.Agent.DefaultModel = &ModelConfig{Provider: zedProvider, Model: zedModel}
		config.Agent.InlineAssistantModel = &ModelConfig{Provider: zedProvider, Model: zedModel}
		config.Agent.CommitMessageModel = &ModelConfig{Provider: zedProvider, Model: zedModel}
		config.Agent.ThreadSummaryModel = &ModelConfig{Provider: zedProvider, Model: zedModel}
	}
	config.Theme = "Ayu Dark"

	// Configure language_models to route API calls through Helix proxy
	// CRITICAL: Zed reads api_url from settings.json, NOT from environment variables!
	// We must explicitly set api_url in language_models for each provider.
	//
	// IMPORTANT: Anthropic and OpenAI have different URL conventions in Zed:
	// - Anthropic: base URL only (Zed appends /v1/messages)
	// - OpenAI: base URL + /v1 (Zed appends /chat/completions)
	// api_key is NOT set here — Zed reads ANTHROPIC_API_KEY / OPENAI_API_KEY from
	// container env vars (set by DesktopAgentAPIEnvVars). Only api_url routing is needed.
	config.LanguageModels = map[string]LanguageModelConfig{
		"anthropic": {
			APIURL: helixAPIURL, // Zed appends /v1/messages
		},
		"openai": {
			APIURL: helixAPIURL + "/v1", // Zed appends /chat/completions
		},
	}

	// 1. Add Helix native tools via HTTP MCP gateway (APIs, Knowledge, Zapier)
	// Uses the unified MCP gateway at /api/v1/mcp/helix instead of helix-cli
	// This allows external agents in sandboxes to access Helix tools without needing helix-cli installed
	if assistant != nil && hasNativeTools(*assistant) {
		helixMCPURL := fmt.Sprintf("%s/api/v1/mcp/helix?app_id=%s&session_id=%s", helixAPIURL, app.ID, sessionID)
		config.ContextServers["helix-native"] = ContextServerConfig{
			URL: helixMCPURL,
			Headers: map[string]string{
				"Authorization": fmt.Sprintf("Bearer %s", helixToken),
			},
		}
	}

	// 2. Add Kodit MCP server for code intelligence (via unified MCP gateway)
	// Only add if Kodit is enabled - otherwise Zed will get 501 errors
	if koditEnabled {
		// The Helix MCP gateway at /api/v1/mcp/kodit authenticates users and forwards to Kodit
		koditMCPURL := fmt.Sprintf("%s/api/v1/mcp/kodit?session_id=%s", helixAPIURL, sessionID)
		config.ContextServers["kodit"] = ContextServerConfig{
			URL: koditMCPURL,
			Headers: map[string]string{
				"Authorization": fmt.Sprintf("Bearer %s", helixToken),
			},
		}
	}

	// 3. Add desktop MCP server (screenshot, clipboard, input, window management tools)
	// Proxied through the Helix API gateway so it works in both local dev and SaaS (app.helix.ml).
	// The gateway authenticates the request and forwards it via RevDial to the desktop HTTP
	// server's /mcp route inside the sandbox container.
	// Provides take_screenshot, save_screenshot, type_text, mouse_click, get_clipboard, set_clipboard,
	// list_windows, focus_window, maximize_window, tile_window, move_to_workspace, switch_to_workspace, get_workspaces
	desktopMCPURL := fmt.Sprintf("%s/api/v1/mcp/desktop?session_id=%s", helixAPIURL, sessionID)
	config.ContextServers["helix-desktop"] = ContextServerConfig{
		URL: desktopMCPURL,
		Headers: map[string]string{
			"Authorization": fmt.Sprintf("Bearer %s", helixToken),
		},
	}

	// 4. Add session MCP server (session navigation and context tools)
	// This runs on the Helix API server (needs database access for session data)
	// Provides current_session, session_toc, session_title_history, search_session,
	// search_all_sessions, list_sessions, get_turn, get_turns, get_interaction
	sessionMCPURL := fmt.Sprintf("%s/api/v1/mcp/session?session_id=%s", helixAPIURL, sessionID)
	config.ContextServers["helix-session"] = ContextServerConfig{
		URL: sessionMCPURL,
		Headers: map[string]string{
			"Authorization": fmt.Sprintf("Bearer %s", helixToken),
		},
	}

	// 5. Add Chrome DevTools MCP server for browser automation and debugging
	// Provides 26 tools for browser control: navigation, DOM/CSS inspection, performance tracing,
	// console access, network analysis, and input automation.
	// Uses Puppeteer internally to control Chrome via CDP (Chrome DevTools Protocol).
	// See: https://developer.chrome.com/blog/chrome-devtools-mcp
	config.ContextServers["chrome-devtools"] = ContextServerConfig{
		Command: "npx",
		// --viewport sets the rendered page size (Chrome window ends up viewport + ~80px
		// of decorations). 1280x800 sits at the canonical desktop-vs-mobile breakpoint
		// so sites still render in desktop mode, and the resulting Chrome window leaves
		// a wide margin on a 1920x1080 monitor — staying below Mutter's auto-maximize
		// threshold (the previous 1600x1080 value tripped it).
		// Stealth flags: make Chrome less detectable as automation.
		// Disables navigator.webdriver, suppresses "Chrome is being controlled" infobar,
		// and prevents extension probing (e.g. LinkedIn bot detection).
		Args: []string{
			"chrome-devtools-mcp@latest",
			"--viewport", "1280x800",
			"--chrome-arg=--disable-blink-features=AutomationControlled",
			"--chrome-arg=--no-first-run",
			"--chrome-arg=--disable-infobars",
			"--chrome-arg=--disable-extensions",
		},
		Env: map[string]string{
			// Point to the actual browser binary (Chromium on ARM64, Chrome on amd64).
			// google-chrome-stable symlink also exists, but CHROME_PATH is the
			// documented way to configure the MCP server for non-Chrome browsers.
			"CHROME_PATH": "/usr/bin/google-chrome-stable",
		},
	}

	// 6. Route external MCP servers through Helix proxy
	// HTTP/HTTPS MCPs go through /api/v1/mcp/external/{mcp_name} for:
	// - SSE endpoint URL rewriting (external server's endpoint isn't reachable from sandbox)
	// - Transport adaptation (can convert between SSE and Streamable HTTP if needed)
	// - Centralized authentication and authorization
	// Stdio MCPs are passed through directly (they run locally in the sandbox)
	// OAuth tokens are injected for stdio MCPs with oauth_provider set
	if assistant != nil {
		for _, mcp := range assistant.MCPs {
			serverName := sanitizeName(mcp.Name)
			config.ContextServers[serverName] = mcpToContextServerWithProxy(ctx, mcp, userID, helixAPIURL, helixToken, oauthTokenGetter)
		}
	}

	// Add project-level MCPs (these overlay on top of agent MCPs)
	// Project MCPs with the same name will override agent MCPs
	if projectSkills != nil {
		for _, mcp := range projectSkills.MCPs {
			serverName := sanitizeName(mcp.Name)
			config.ContextServers[serverName] = mcpToContextServerWithProxy(ctx, mcp, userID, helixAPIURL, helixToken, oauthTokenGetter)
		}
	}

	return config, nil
}

// hasNativeTools checks if assistant has Helix native tools
func hasNativeTools(assistant types.AssistantConfig) bool {
	// Check if any native tools are configured
	hasAPIs := len(assistant.APIs) > 0
	hasRAG := assistant.RAGSourceID != ""
	hasKnowledge := len(assistant.Knowledge) > 0

	// Check tool configs for native tools
	hasNativeToolConfigs := false
	for _, tool := range assistant.Tools {
		switch tool.ToolType {
		case types.ToolTypeAPI, types.ToolTypeZapier:
			hasNativeToolConfigs = true
		}
	}

	return hasAPIs || hasRAG || hasKnowledge || hasNativeToolConfigs
}

// mcpToContextServerWithProxy converts Helix MCP config to Zed context server config,
// routing HTTP MCPs through the Helix proxy for proper SSE endpoint handling.
// Stdio MCPs run directly inside the dev container.
// If oauthTokenGetter is provided and the MCP has oauth_provider set, the token will be injected.
func mcpToContextServerWithProxy(ctx context.Context, mcp types.AssistantMCP, userID, helixAPIURL, helixToken string, oauthTokenGetter OAuthTokenGetter) ContextServerConfig {
	// Check for explicit stdio transport (new format with Command/Args/Env)
	// This is used for MCPs that run inside the dev container via npx or other commands
	if mcp.Transport == "stdio" || mcp.Command != "" {
		env := mcp.Env
		if env == nil {
			env = make(map[string]string)
		} else {
			// Make a copy to avoid modifying the original
			envCopy := make(map[string]string)
			for k, v := range env {
				envCopy[k] = v
			}
			env = envCopy
		}

		// Inject OAuth token if provider is configured and tokenGetter is available
		if mcp.OAuthProvider != "" && oauthTokenGetter != nil {
			token, err := oauthTokenGetter(ctx, userID, mcp.OAuthProvider)
			if err != nil {
				log.Warn().Err(err).Str("provider", mcp.OAuthProvider).Msg("Failed to get OAuth token for stdio MCP")
			} else if token != "" {
				// Map provider names to their expected environment variable names
				switch strings.ToLower(mcp.OAuthProvider) {
				case "github":
					env["GITHUB_PERSONAL_ACCESS_TOKEN"] = token
				default:
					// Generic fallback using provider name
					envKey := fmt.Sprintf("%s_ACCESS_TOKEN", strings.ToUpper(mcp.OAuthProvider))
					env[envKey] = token
				}
				log.Debug().Str("provider", mcp.OAuthProvider).Msg("Injected OAuth token into stdio MCP environment")
			}
		}

		// Ensure args is an empty slice, not nil (Zed doesn't accept null for args)
		args := mcp.Args
		if args == nil {
			args = []string{}
		}

		return ContextServerConfig{
			Command: mcp.Command,
			Args:    args,
			Env:     env,
		}
	}

	// For HTTP/HTTPS MCPs, route through Helix proxy
	// This is necessary because:
	// 1. SSE protocol sends an endpoint URL that would point to the unreachable external server
	// 2. The sandbox can't reach external MCP servers directly
	// 3. We want centralized auth and transport adaptation
	if strings.HasPrefix(mcp.URL, "http://") || strings.HasPrefix(mcp.URL, "https://") {
		// Route through Helix external MCP proxy
		// The proxy will connect to the actual MCP server and forward requests
		proxyURL := fmt.Sprintf("%s/api/v1/mcp/external/%s", helixAPIURL, sanitizeName(mcp.Name))

		// The proxy always exposes as Streamable HTTP (the modern protocol)
		// It handles SSE transport internally when connecting to legacy servers
		return ContextServerConfig{
			URL: proxyURL,
			Headers: map[string]string{
				"Authorization": fmt.Sprintf("Bearer %s", helixToken),
			},
		}
	}

	// Legacy stdio transport - parse command from URL (e.g., "stdio://npx @modelcontextprotocol/server-filesystem /tmp")
	// Kept for backward compatibility
	cmd, args := parseStdioURL(mcp.URL)
	return ContextServerConfig{
		Command: cmd,
		Args:    args,
		Env:     buildMCPEnv(mcp),
	}
}

func buildMCPEnv(mcp types.AssistantMCP) map[string]string {
	// Use the explicit Env field if set
	if len(mcp.Env) > 0 {
		return mcp.Env
	}
	// Legacy: convert Headers to env vars
	env := make(map[string]string)
	for k, v := range mcp.Headers {
		env[fmt.Sprintf("MCP_HEADER_%s", strings.ToUpper(k))] = v
	}
	return env
}

func parseStdioURL(url string) (string, []string) {
	// Remove "stdio://" prefix
	url = strings.TrimPrefix(url, "stdio://")

	// Split into command and args
	parts := strings.Fields(url)
	if len(parts) == 0 {
		return "", nil
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	return parts[0], parts[1:]
}

func sanitizeName(name string) string {
	// MCP tool names: alphanumeric, hyphens, underscores only
	name = strings.ToLower(name)
	// Replace invalid characters with hyphens
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			result.WriteRune(r)
		} else {
			result.WriteRune('-')
		}
	}
	return strings.Trim(result.String(), "-")
}

// getAPIKeyForProvider retrieves the API key for a given provider from environment
func getAPIKeyForProvider(provider string) string {
	switch strings.ToLower(provider) {
	case "anthropic":
		return os.Getenv("ANTHROPIC_API_KEY")
	case "openai":
		return os.Getenv("OPENAI_API_KEY")
	case "together":
		return os.Getenv("TOGETHER_API_KEY")
	default:
		return ""
	}
}

// normalizeModelIDForZed converts model IDs to the format Zed expects.
// Zed's serde config only recognizes specific model aliases (e.g., "claude-3-5-haiku-latest"),
// not dated versions (e.g., "claude-3-5-haiku-20241022"). This function strips dates
// and converts to the -latest format.
func normalizeModelIDForZed(modelID string) string {
	// Already has -latest suffix, return as-is
	if strings.HasSuffix(modelID, "-latest") {
		return modelID
	}

	// Claude 4.6 models
	if strings.HasPrefix(modelID, "claude-opus-4-6") {
		return "claude-opus-4-6-latest"
	}
	if strings.HasPrefix(modelID, "claude-sonnet-4-6") {
		return "claude-sonnet-4-6-latest"
	}

	// Claude 4.5 models
	if strings.HasPrefix(modelID, "claude-opus-4-5") {
		return "claude-opus-4-5-latest"
	}
	if strings.HasPrefix(modelID, "claude-sonnet-4-5") {
		return "claude-sonnet-4-5-latest"
	}
	if strings.HasPrefix(modelID, "claude-haiku-4-5") {
		return "claude-haiku-4-5-latest"
	}

	// Claude 4.1 models (must come before generic claude-opus-4 / claude-sonnet-4)
	if strings.HasPrefix(modelID, "claude-opus-4-1") {
		return "claude-opus-4-1-latest"
	}

	// Claude 4.0 models (generic — catches claude-opus-4-20250514, claude-sonnet-4-20250514, etc.)
	if strings.HasPrefix(modelID, "claude-opus-4") {
		return "claude-opus-4-latest"
	}
	if strings.HasPrefix(modelID, "claude-sonnet-4") {
		return "claude-sonnet-4-latest"
	}

	// Claude 3.x models (old naming: claude-3-5-sonnet, claude-3-5-haiku, etc.)
	if strings.HasPrefix(modelID, "claude-3-7-sonnet") {
		return "claude-3-7-sonnet-latest"
	}
	if strings.HasPrefix(modelID, "claude-3-5-sonnet") {
		return "claude-3-5-sonnet-latest"
	}
	if strings.HasPrefix(modelID, "claude-3-5-haiku") {
		return "claude-3-5-haiku-latest"
	}
	if strings.HasPrefix(modelID, "claude-3-opus") {
		return "claude-3-opus-latest"
	}
	if strings.HasPrefix(modelID, "claude-3-sonnet") {
		return "claude-3-sonnet-latest"
	}
	if strings.HasPrefix(modelID, "claude-3-haiku") {
		return "claude-3-haiku-latest"
	}

	// OpenAI models - these typically don't have date suffixes in settings
	// but normalize common patterns just in case
	if strings.HasPrefix(modelID, "gpt-4o-") && !strings.HasPrefix(modelID, "gpt-4o-mini") {
		return "gpt-4o"
	}
	if strings.HasPrefix(modelID, "gpt-4o-mini-") {
		return "gpt-4o-mini"
	}

	// Return unchanged for other models (Gemini, Qwen, etc. - these go through OpenAI provider)
	return modelID
}

// ProviderRef is the minimal projection of a provider endpoint that the
// agent-config code path needs: a stable ID (empty for env-baked globals,
// which have no DB row) and the current canonical name. Callers build the
// snapshot from the provider manager's live state (globals + DB-backed user
// providers visible to the owner).
//
// We deliberately store both ID and Name so resolution can be ID-first with
// a name fallback for legacy agent records that were saved before agents
// stored IDs. After such a fallback the agent should be re-saved so it picks
// up the immutable reference.
type ProviderRef struct {
	ID   string // empty for env-baked global providers (openai, anthropic, ...)
	Name string // current canonical name; for DB-backed providers this is the admin-set label
}

// ResolveProvider matches a stored agent token (an ID for DB-backed providers,
// the canonical name for globals) against the provider snapshot. ID match
// wins; if no ID matches, falls back to a case-insensitive name match
// (legacy agents). The bool return distinguishes a successful resolve
// (ok=true, byLegacyName=false), a legacy resolve that should be flagged for
// rewriting (ok=true, byLegacyName=true), and a failed resolve (ok=false).
//
// snapshot==nil means "no manager handle" — the runner-side path opts out of
// resolution this way; callers should treat this as "skip validation, trust
// the stored value as a name".
func ResolveProvider(token string, snapshot []ProviderRef) (ref ProviderRef, byLegacyName bool, ok bool) {
	if snapshot == nil {
		return ProviderRef{Name: token}, false, true
	}
	for _, p := range snapshot {
		if p.ID != "" && p.ID == token {
			return p, false, true
		}
	}
	want := strings.ToLower(token)
	for _, p := range snapshot {
		if strings.EqualFold(p.Name, want) || strings.ToLower(p.Name) == want {
			byLegacy := p.ID != "" // global with no ID is a normal match, not legacy
			return p, byLegacy, true
		}
	}
	return ProviderRef{}, false, false
}

// ValidateAssistantModelConfig checks whether the app's zed_external assistant
// has a usable provider/model combination given the currently registered
// providers. Returns the empty string when the configuration is usable;
// otherwise an operator-friendly message suitable for surfacing in the
// spec-task UI / API 422 response.
//
// snapshot is the list of provider records visible to the owner (env-baked
// globals + DB-backed). Pass nil to skip provider-existence validation
// (used by the runner-side path that has no manager handle).
//
// Default-app sessions (no parent app, len(Assistants)==0) skip validation —
// they fall through to the legacy SaaS-friendly default in GenerateZedMCPConfig.
//
// Renames: agent records store the provider's immutable ID, so renaming a
// provider in admin is a no-op for resolution.
// Deletes: ID lookup fails and we report misconfig.
func ValidateAssistantModelConfig(app *types.App, snapshot []ProviderRef) string {
	if app == nil || len(app.Config.Helix.Assistants) == 0 {
		return ""
	}
	var assistant *types.AssistantConfig
	for i := range app.Config.Helix.Assistants {
		if app.Config.Helix.Assistants[i].AgentType == types.AgentTypeZedExternal {
			assistant = &app.Config.Helix.Assistants[i]
			break
		}
	}
	if assistant == nil {
		assistant = &app.Config.Helix.Assistants[0]
	}
	provider := assistant.GenerationModelProvider
	if provider == "" {
		provider = assistant.Provider
	}
	model := assistant.GenerationModel
	if model == "" {
		model = assistant.Model
	}
	if provider == "" || model == "" {
		return fmt.Sprintf("agent %q is missing a provider or model selection — open the agent settings and pick a provider and model", app.ID)
	}
	if snapshot == nil {
		return ""
	}
	if _, _, ok := ResolveProvider(provider, snapshot); !ok {
		return fmt.Sprintf("agent %q references provider %q which does not match any current provider — the provider may have been renamed or deleted. Open the agent settings and re-pick a provider, or restore/rename the provider in admin.", app.ID, provider)
	}
	return ""
}

// mapHelixToZedProvider maps a Helix provider name to a Zed provider type and formats the model name.
// Zed only recognizes a fixed set of providers: anthropic, openai, google, ollama, copilot, lmstudio, deepseek.
// All other Helix providers (nebius, together, openrouter, etc.) are OpenAI-compatible and should use "openai".
//
// For the model name:
// - Anthropic models: normalize to -latest format (e.g., claude-sonnet-4-5-latest)
// - OpenAI-native models: use as-is (e.g., gpt-4o)
// - All other providers: prefix with "provider/" so Helix's router can route to the correct backend
//
// Examples:
//
//	helixProvider="anthropic", model="claude-sonnet-4-5" → zedProvider="anthropic", zedModel="claude-sonnet-4-5-latest"
//	helixProvider="openai", model="gpt-4o" → zedProvider="openai", zedModel="openai/gpt-4o"
//	helixProvider="nebius", model="Qwen/Qwen3-Coder" → zedProvider="openai", zedModel="nebius/Qwen/Qwen3-Coder"
func mapHelixToZedProvider(helixProvider, model string) (zedProvider, zedModel string) {
	provider := strings.ToLower(helixProvider)

	switch provider {
	case "anthropic":
		// Anthropic uses Zed's native Anthropic provider which routes to Helix's Anthropic proxy.
		// Model name is normalized to -latest format (required by Zed's serde config).
		// No provider prefix needed since Anthropic API is separate from OpenAI API.
		return "anthropic", normalizeModelIDForZed(model)

	default:
		// All other providers (openai, nebius, together, openrouter, azure, google, etc.)
		// route through Zed's OpenAI provider → Helix's OpenAI-compatible proxy.
		// Model is prefixed with provider name so Helix can route to the correct backend.
		return "openai", fmt.Sprintf("%s/%s", helixProvider, model)
	}
}

// GetZedConfigForSession retrieves Zed MCP config for a session
func GetZedConfigForSession(ctx context.Context, s store.Store, sessionID string) (*ZedMCPConfig, error) {
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	app, err := s.GetApp(ctx, session.ParentApp)
	if err != nil {
		return nil, fmt.Errorf("failed to get app: %w", err)
	}

	// Get project if session has one (for project-level skill overlays)
	var projectSkills *types.AssistantSkills
	if session.ProjectID != "" {
		project, err := s.GetProject(ctx, session.ProjectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get project %s for skills config: %w", session.ProjectID, err)
		}
		projectSkills = project.Skills
	}

	// Get Helix API URL from environment
	// For production, use SANDBOX_API_URL if set, else SERVER_URL, else fallback
	helixAPIURL := os.Getenv("SANDBOX_API_URL")
	if helixAPIURL == "" {
		helixAPIURL = os.Getenv("SERVER_URL")
	}
	if helixAPIURL == "" {
		helixAPIURL = os.Getenv("HELIX_API_URL")
	}
	if helixAPIURL == "" {
		helixAPIURL = "http://api:8080"
	}
	// Use "outer-api" instead of "api" for the Zed inference URL. Both resolve
	// to the same IP in the desktop's /etc/hosts, but "outer-api" survives
	// Helix-in-Helix scenarios where an inner compose stack shadows "api".
	if strings.Contains(helixAPIURL, "://api:") {
		if _, err := net.LookupHost("outer-api"); err == nil {
			helixAPIURL = strings.Replace(helixAPIURL, "://api:", "://outer-api:", 1)
			log.Info().Str("url", helixAPIURL).Msg("Rewrote API URL to outer-api for Zed config")
		}
	}

	// Generate runner token for this session
	helixToken := os.Getenv("RUNNER_TOKEN")
	if helixToken == "" {
		log.Warn().Msg("RUNNER_TOKEN not set, Zed MCP tools may not work")
	}

	// Check if Kodit is enabled (defaults to false)
	koditEnabled := os.Getenv("KODIT_ENABLED") == "true"

	// Create OAuth token getter that looks up tokens from the store
	oauthTokenGetter := func(ctx context.Context, userID, providerName string) (string, error) {
		// First find the provider by name
		providers, err := s.ListOAuthProviders(ctx, &store.ListOAuthProvidersQuery{})
		if err != nil {
			return "", fmt.Errorf("failed to list OAuth providers: %w", err)
		}

		var providerID string
		for _, p := range providers {
			if strings.EqualFold(p.Name, providerName) || strings.EqualFold(string(p.Type), providerName) {
				providerID = p.ID
				break
			}
		}
		if providerID == "" {
			return "", nil // No provider found, not an error
		}

		// Get the user's connection to this provider
		conn, err := s.GetOAuthConnectionByUserAndProvider(ctx, userID, providerID)
		if err != nil {
			if err == store.ErrNotFound {
				return "", nil // No connection, not an error
			}
			return "", fmt.Errorf("failed to get OAuth connection: %w", err)
		}

		return conn.AccessToken, nil
	}

	// Runner-side path has no provider-manager handle, so we skip provider
	// validation here. The handler-side callers (getZedConfig,
	// getMergedZedSettings) do pass the live provider list.
	config, err := GenerateZedMCPConfig(ctx, app, session.Owner, sessionID, helixAPIURL, helixToken, koditEnabled, projectSkills, oauthTokenGetter, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Zed config: %w", err)
	}

	return config, nil
}

// MergeZedConfigWithUserOverrides merges Helix config with user overrides
func MergeZedConfigWithUserOverrides(helixConfig *ZedMCPConfig, userOverrides map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Start with Helix context servers
	result["context_servers"] = helixConfig.ContextServers

	// Apply user overrides
	if userServers, ok := userOverrides["context_servers"].(map[string]interface{}); ok {
		// Deep merge: user additions and modifications
		if helixServers, ok := result["context_servers"].(map[string]ContextServerConfig); ok {
			merged := make(map[string]interface{})
			// Convert Helix servers to map[string]interface{}
			for name, server := range helixServers {
				serverMap := map[string]interface{}{
					"command": server.Command,
					"args":    server.Args,
				}
				if len(server.Env) > 0 {
					serverMap["env"] = server.Env
				}
				merged[name] = serverMap
			}
			// Apply user overrides
			for name, server := range userServers {
				merged[name] = server
			}
			result["context_servers"] = merged
		}
	}

	// Apply other user settings (non-MCP)
	for k, v := range userOverrides {
		if k != "context_servers" {
			result[k] = v
		}
	}

	return result
}

// SaveUserZedOverrides saves user's Zed settings overrides
func SaveUserZedOverrides(ctx context.Context, s store.Store, sessionID string, overrides map[string]interface{}) error {
	overridesJSON, err := json.Marshal(overrides)
	if err != nil {
		return fmt.Errorf("failed to marshal overrides: %w", err)
	}

	override := &types.ZedSettingsOverride{
		SessionID: sessionID,
		Overrides: overridesJSON,
	}

	return s.UpsertZedSettingsOverride(ctx, override)
}

// GetUserZedOverrides retrieves user's Zed settings overrides
func GetUserZedOverrides(ctx context.Context, s store.Store, sessionID string) (map[string]interface{}, error) {
	override, err := s.GetZedSettingsOverride(ctx, sessionID)
	if err != nil {
		// No overrides yet, return empty
		return make(map[string]interface{}), nil
	}

	var overrides map[string]interface{}
	if err := json.Unmarshal(override.Overrides, &overrides); err != nil {
		return nil, fmt.Errorf("failed to unmarshal overrides: %w", err)
	}

	return overrides, nil
}
