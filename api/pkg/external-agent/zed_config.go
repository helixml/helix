package external_agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
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
	DefaultModel             *ModelConfig `json:"default_model,omitempty"`
	InlineAssistantModel     *ModelConfig `json:"inline_assistant_model,omitempty"`
	CommitMessageModel       *ModelConfig `json:"commit_message_model,omitempty"`
	ThreadSummaryModel       *ModelConfig `json:"thread_summary_model,omitempty"`
	AllowUnsandboxedCommands bool         `json:"-"`                         // Mapped to sandbox_permissions.allow_unsandboxed by handler
	AlwaysAllowToolActions   bool         `json:"always_allow_tool_actions"` // Deprecated: mapped to tool_permissions.default="allow" by handler
	ShowOnboarding           bool         `json:"show_onboarding"`
	AutoOpenPanel            bool         `json:"auto_open_panel"`
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
// reference (stable ID, or a legacy canonical name) is resolved
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
	orgWorkerID string,
	specTaskTools []string,
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
	assistant := FindZedExternalAssistant(app)

	provider, model := AssistantModelSelection(assistant)
	providerName := provider

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
		providerName = provider
		model = "claude-sonnet-4-6"
	} else if usesUpstreamSubscription(assistant) {
		// Subscription credentials only make sense for runtimes that handle
		// inference upstream via OAuth, not through a Helix provider. Don't
		// write a Helix-routed default into settings.json — Zed falls back to
		// its built-in defaults for inline assistant / commit messages / thread
		// summaries, and Claude Code routes via its own OAuth path. The pre-
		// flight ValidateAssistantModelConfig already applies the same bypass
		// so spec-task start handlers don't 422.
		//
		// We deliberately scope this branch to Claude Code and Codex CLI. Other
		// runtimes (zed_agent, qwen_code, gemini_cli, goose_code) cannot use a
		// "subscription" credential — there's no OAuth path for it. Treating
		// such an assistant as subscription-credentialed leaves agent.default_model
		// unset, which trips start-zed-helix.sh's wait_for_zed_config (it greps
		// for the literal "default_model" key) and the dev container times out
		// with a generic "Agent never connected" banner. The misconfigured
		// credential_type=subscription is almost always a stale UI default; we
		// log it and fall through to the api_key path so the agent can boot.
		useAgentModel = false
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
		providerName = resolved.Name
		if resolved.ID != "" {
			provider = resolved.ID
		}
	}

	// Configure agent. Permissions / ShowOnboarding / AutoOpenPanel are always
	// set; model overrides are set only when we trust the agent's configuration.
	config.Agent = &AgentConfig{
		AllowUnsandboxedCommands: true,
		AlwaysAllowToolActions:   true,
		ShowOnboarding:           false,
		AutoOpenPanel:            true,
	}
	if useAgentModel {
		// Map Helix provider to Zed's provider type and format model name
		// Zed only knows: anthropic, openai, google, ollama, copilot, lmstudio, deepseek
		// All other providers (nebius, together, openrouter, etc.) use OpenAI-compatible API
		zedProvider, zedModel := mapHelixToZedProviderToken(providerName, provider, model)
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

	// Configure language_models to route API calls through Helix proxy.
	// CRITICAL: Zed reads api_url from settings.json, NOT from environment variables!
	// We must explicitly set api_url in language_models for each provider.
	//
	// IMPORTANT: Anthropic and OpenAI have different URL conventions in Zed:
	// - Anthropic: base URL only (Zed appends /v1/messages)
	// - OpenAI: base URL + /v1 (Zed appends /chat/completions)
	// api_key is NOT set here — Zed reads ANTHROPIC_API_KEY / OPENAI_API_KEY from
	// container env vars (set by DesktopAgentAPIEnvVars). Only api_url routing is needed.
	//
	// We only inject the entries the user actually has Helix providers for.
	// If we unconditionally inject "anthropic", Zed's bottom-left model picker
	// shows Claude Sonnet as a default option even when the user has no
	// Anthropic provider configured — picking it then fails with
	// "provider \"openai\" not found" from /v1/messages because Zed sends
	// the wrong provider on the Anthropic-only endpoint.
	config.LanguageModels = buildLanguageModels(providerSnapshot, helixAPIURL)

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
	//
	// Invoke the globally-installed binary directly (Dockerfile.ubuntu-helix
	// pins `chrome-devtools-mcp` via `npm install -g`). Going through
	// `npx chrome-devtools-mcp@latest` instead causes npm's `_npx/<hash>`
	// cache to do a "reify mark retired" rename dance every spawn; when Zed
	// and Claude Code spawn in parallel the renames race and the JSON-RPC
	// `initialize` never returns — Zed surfaces this as
	// `chrome-devtools context server failed to start: Context server
	// request timeout` (180s).
	config.ContextServers["chrome-devtools"] = ContextServerConfig{
		Command: "/usr/bin/chrome-devtools-mcp",
		// --viewport sets the rendered page size (Chrome window ends up viewport + ~80px
		// of decorations). 1280x800 sits at the canonical desktop-vs-mobile breakpoint
		// so sites still render in desktop mode, and the resulting Chrome window leaves
		// a wide margin on a 1920x1080 monitor — staying below Mutter's auto-maximize
		// threshold (the previous 1600x1080 value tripped it).
		// Stealth flags: make Chrome less detectable as automation.
		// Disables navigator.webdriver, suppresses "Chrome is being controlled" infobar,
		// and prevents extension probing (e.g. LinkedIn bot detection).
		Args: []string{
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

	if orgWorkerID != "" {
		config.ContextServers["helix"] = ContextServerConfig{
			URL: strings.TrimRight(helixAPIURL, "/") + "/api/v1/mcp/helix-org",
			Headers: map[string]string{
				"Authorization": fmt.Sprintf("Bearer %s", helixToken),
			},
		}
	}

	// Spec tasks get the project-scoped slice of the same tool registry, so a
	// task can create and steer other tasks as sub-agents. The rev is what
	// makes an edit land mid-session: Zed caches tools/list from initialize,
	// so the URL has to change for it to restart the context server.
	if len(specTaskTools) > 0 {
		config.ContextServers["helix-tasks"] = ContextServerConfig{
			URL: strings.TrimRight(helixAPIURL, "/") + "/api/v1/mcp/helix-tasks?rev=" + AgentToolsRev(specTaskTools),
			Headers: map[string]string{
				"Authorization": fmt.Sprintf("Bearer %s", helixToken),
			},
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
	ID           string
	Name         string
	EndpointType types.ProviderEndpointType
}

// FindZedExternalAssistant returns the assistant config that owns the
// zed_external agent for the given app, falling back to the first
// assistant when no zed_external entry is present (legacy/migration path).
// Returns nil when the app has no assistants at all.
//
// Centralised here so GenerateZedMCPConfig, ValidateAssistantModelConfig,
// and buildCodeAgentConfig agree on which assistant they're talking about
// — a previous version had three independent walks, which would silently
// diverge if anyone changed the matching rule.
func FindZedExternalAssistant(app *types.App) *types.AssistantConfig {
	if app == nil || len(app.Config.Helix.Assistants) == 0 {
		return nil
	}
	for i := range app.Config.Helix.Assistants {
		if app.Config.Helix.Assistants[i].AgentType == types.AgentTypeZedExternal {
			return &app.Config.Helix.Assistants[i]
		}
	}
	return &app.Config.Helix.Assistants[0]
}

// AssistantModelSelection returns the provider/model pair persisted by the
// active code-agent settings UI. Explicit Claude Code API-key mode uses the
// generation fields; other external runtimes and legacy empty credential types
// use the top-level fields with the generation fallback.
func AssistantModelSelection(assistant *types.AssistantConfig) (string, string) {
	if assistant == nil {
		return "", ""
	}
	if assistant.CodeAgentRuntime == types.CodeAgentRuntimeClaudeCode &&
		assistant.CodeAgentCredentialType == types.CodeAgentCredentialTypeAPIKey &&
		(assistant.GenerationModelProvider != "" || assistant.GenerationModel != "") {
		return assistant.GenerationModelProvider, assistant.GenerationModel
	}
	provider := assistant.Provider
	if provider == "" {
		provider = assistant.GenerationModelProvider
	}
	model := assistant.Model
	if model == "" {
		model = assistant.GenerationModel
	}
	return provider, model
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
	if types.IsGlobalProviderID(token) || strings.HasPrefix(token, "pe_") {
		return ProviderRef{}, false, false
	}
	var selected *ProviderRef
	for _, p := range snapshot {
		if types.CanonicalProviderName(p.Name) == types.CanonicalProviderName(token) {
			if selected == nil || providerRefPrecedence(p) > providerRefPrecedence(*selected) {
				candidate := p
				selected = &candidate
			}
		}
	}
	if selected != nil {
		return *selected, selected.ID != token, true
	}
	return ProviderRef{}, false, false
}

func providerRefPrecedence(ref ProviderRef) int {
	switch {
	case ref.EndpointType == types.ProviderEndpointTypeOrg || ref.EndpointType == types.ProviderEndpointTypeUser:
		return 3
	case strings.HasPrefix(ref.ID, "pe_"):
		return 2
	case types.IsGlobalProviderID(ref.ID):
		return 1
	default:
		return 3
	}
}

// CodeAgentRuntimeAllowsProvider keeps native vendor CLIs on the API shape
// they actually implement. Claude Code speaks Anthropic Messages; Codex speaks
// OpenAI Responses. The general-purpose harnesses continue to accept any
// provider exposed through Helix's OpenAI-compatible proxy.
func CodeAgentRuntimeAllowsProvider(runtime types.CodeAgentRuntime, providerName string) bool {
	providerName = types.CanonicalProviderName(providerName)
	switch runtime {
	case types.CodeAgentRuntimeClaudeCode:
		return strings.EqualFold(providerName, string(types.ProviderAnthropic))
	case types.CodeAgentRuntimeCodexCLI:
		return strings.EqualFold(providerName, string(types.ProviderOpenAI))
	default:
		return true
	}
}

func requiredProviderForCodeAgentRuntime(runtime types.CodeAgentRuntime) string {
	switch runtime {
	case types.CodeAgentRuntimeClaudeCode:
		return string(types.ProviderAnthropic)
	case types.CodeAgentRuntimeCodexCLI:
		return string(types.ProviderOpenAI)
	default:
		return ""
	}
}

// MigrateLegacyProviderRefs walks every provider-bearing field on every
// assistant in app.Config and, where the stored value is a legacy name that
// resolves (case-insensitively) to a current DB-backed provider, rewrites
// the field in-place to that provider's immutable ID. Returns true if any
// field changed — the caller is responsible for persisting via
// Store.UpdateApp.
//
// Globals (ref.ID == "") are left alone: their canonical name is itself
// immutable, so there's nothing to migrate. Misconfigured fields (resolver
// miss) are also left alone — we have no ID to write.
//
// Called from session-start and spec-task pre-flight handlers so legacy
// agents heal themselves on first use after the ID-based refactor lands,
// without the operator having to re-save anything.
func MigrateLegacyProviderRefs(app *types.App, snapshot []ProviderRef) bool {
	if app == nil || snapshot == nil || len(app.Config.Helix.Assistants) == 0 {
		return false
	}
	changed := false
	rewrite := func(field *string) {
		if field == nil || *field == "" {
			return
		}
		ref, byLegacy, ok := ResolveProvider(*field, snapshot)
		if !ok || !byLegacy || ref.ID == "" {
			return
		}
		*field = ref.ID
		changed = true
	}
	for i := range app.Config.Helix.Assistants {
		a := &app.Config.Helix.Assistants[i]
		rewrite(&a.Provider)
		rewrite(&a.GenerationModelProvider)
		rewrite(&a.SmallGenerationModelProvider)
		rewrite(&a.ReasoningModelProvider)
		rewrite(&a.SmallReasoningModelProvider)
	}
	return changed
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
	assistant := FindZedExternalAssistant(app)
	if assistant == nil {
		return ""
	}
	// Subscription-credential agents (Claude Code with OAuth) auth directly
	// with the upstream provider and do not route inference through a Helix
	// provider, so empty provider/model is the documented shape
	// (see types.CodeAgentCredentialTypeSubscription). Validating against a
	// Helix provider snapshot would be meaningless and incorrectly 422s any
	// task started on such an agent.
	//
	// Scoped to claude_code runtime: a zed_agent / qwen_code / gemini_cli /
	// codex_cli assistant cannot use OAuth, so credential_type=subscription
	// on those is misconfig (almost always a stale UI default) — we let it
	// fall through to the normal provider/model check rather than silently
	// bypassing it. See the matching condition in GenerateZedMCPConfig.
	if usesUpstreamSubscription(assistant) {
		return ""
	}
	provider, model := AssistantModelSelection(assistant)
	if provider == "" || model == "" {
		return fmt.Sprintf("agent %q is missing a provider or model selection — open the agent settings and pick a provider and model", app.ID)
	}
	if snapshot == nil {
		return ""
	}
	resolved, _, ok := ResolveProvider(provider, snapshot)
	if !ok {
		if app.OrganizationID != "" {
			return types.OrganizationProviderUnavailableMessage
		}
		return fmt.Sprintf("agent %q references provider %q which does not match any current provider — the provider may have been renamed or deleted. Open the agent settings and re-pick a provider, or restore/rename the provider in admin.", app.ID, provider)
	}
	if required := requiredProviderForCodeAgentRuntime(assistant.CodeAgentRuntime); required != "" &&
		!CodeAgentRuntimeAllowsProvider(assistant.CodeAgentRuntime, resolved.Name) {
		return fmt.Sprintf("coding-agent harness %q API-key mode requires the %s provider; provider %q is not compatible",
			assistant.CodeAgentRuntime, required, resolved.Name)
	}
	return ""
}

func usesUpstreamSubscription(assistant *types.AssistantConfig) bool {
	if assistant == nil || !assistant.CodeAgentCredentialType.IsSubscription() {
		return false
	}
	return assistant.CodeAgentRuntime == types.CodeAgentRuntimeClaudeCode ||
		assistant.CodeAgentRuntime == types.CodeAgentRuntimeCodexCLI
}

// buildLanguageModels returns the language_models block for Zed's settings.json,
// containing only the provider entries the user actually has Helix providers
// for. The mapping mirrors mapHelixToZedProvider:
//
//   - A Helix "anthropic" provider unlocks Zed's "anthropic" entry (routes to
//     Helix's /v1/messages).
//   - Any other Helix provider (openai, nebius, together, openrouter, ...)
//     unlocks Zed's "openai" entry (routes to Helix's /v1/chat/completions).
//
// snapshot==nil is the runner-side opt-out path; we preserve historical
// behaviour there by injecting both entries.
func buildLanguageModels(snapshot []ProviderRef, helixAPIURL string) map[string]LanguageModelConfig {
	if snapshot == nil {
		return map[string]LanguageModelConfig{
			"anthropic": {APIURL: helixAPIURL},
			"openai":    {APIURL: helixAPIURL + "/v1"},
		}
	}

	hasAnthropic := false
	hasOpenAICompat := false
	for _, p := range snapshot {
		if types.CanonicalProviderName(p.Name) == string(types.ProviderAnthropic) {
			hasAnthropic = true
		} else {
			hasOpenAICompat = true
		}
	}

	out := map[string]LanguageModelConfig{}
	if hasAnthropic {
		out["anthropic"] = LanguageModelConfig{APIURL: helixAPIURL}
	}
	if hasOpenAICompat {
		out["openai"] = LanguageModelConfig{APIURL: helixAPIURL + "/v1"}
	}
	return out
}

// mapHelixToZedProvider maps a Helix provider name to a Zed provider type and formats the model name.
// Zed only recognizes a fixed set of providers: anthropic, openai, google, ollama, copilot, lmstudio, deepseek.
// All other Helix providers (nebius, together, openrouter, etc.) are OpenAI-compatible and should use "openai".
//
// For the model name:
// - Anthropic models: passed through verbatim (see the anthropic case below)
// - OpenAI-native models: use as-is (e.g., gpt-4o)
// - All other providers: prefix with "provider/" so Helix's router can route to the correct backend
//
// Examples:
//
//	helixProvider="anthropic", model="claude-opus-4-8" → zedProvider="anthropic", zedModel="claude-opus-4-8"
//	helixProvider="openai", model="gpt-4o" → zedProvider="openai", zedModel="openai/gpt-4o"
//	helixProvider="nebius", model="Qwen/Qwen3-Coder" → zedProvider="openai", zedModel="nebius/Qwen/Qwen3-Coder"
func mapHelixToZedProvider(helixProvider, model string) (zedProvider, zedModel string) {
	return mapHelixToZedProviderToken(helixProvider, helixProvider, model)
}

func mapHelixToZedProviderToken(providerName, routingToken, model string) (zedProvider, zedModel string) {
	provider := types.CanonicalProviderName(providerName)

	switch provider {
	case "anthropic":
		// Zed discovers Anthropic models from the provider's /v1/models listing
		// (Helix's proxy) and resolves agent.default_model by exact id. The stored
		// model id already comes from that same listing (the picker sources it), so
		// pass it through verbatim. Do NOT rewrite to a "-latest" alias: Helix's
		// listing returns bare/dated ids (e.g. "claude-opus-4-8"), the "-latest"
		// form matches nothing, and Zed silently falls back to its built-in default
		// (gpt-5-mini) — which has no Helix route, so the agent returns empty.
		return "anthropic", model

	default:
		// All other providers (openai, nebius, together, openrouter, azure, google, etc.)
		// route through Zed's OpenAI provider → Helix's OpenAI-compatible proxy.
		// Model is prefixed with provider name so Helix can route to the correct backend.
		return "openai", fmt.Sprintf("%s/%s", routingToken, model)
	}
}

// MergeContextServers returns the union of helix-managed MCP context servers
// and the user-side ones uploaded via /zed-config/user. Used by the
// /zed-settings endpoint to render the per-session "MCP Tools" UI panel.
//
// Note: this is the only piece of the old MergeZedConfigWithUserOverrides
// that had any live consumer. The agent / language_models / theme merge
// previously here was dead code — the desktop hot path runs through the
// settings-sync-daemon, which polls /zed-config and merges client-side.
// Keeping the duplication invited drift (see P1-5).
func MergeContextServers(helixServers map[string]ContextServerConfig, userOverrides map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{}, len(helixServers))
	for name, server := range helixServers {
		entry := make(map[string]interface{})
		if server.URL != "" {
			entry["url"] = server.URL
			if len(server.Headers) > 0 {
				entry["headers"] = server.Headers
			}
		} else {
			entry["command"] = server.Command
			entry["args"] = server.Args
			if len(server.Env) > 0 {
				entry["env"] = server.Env
			}
		}
		merged[name] = entry
	}

	if userServers, ok := userOverrides["context_servers"].(map[string]interface{}); ok {
		for name, server := range userServers {
			merged[name] = server
		}
	}

	return merged
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

// AgentToolsRev is a short, stable fingerprint of an effective tool list. It
// rides the spec-task MCP URL so a tool edit changes settings.json, which is
// what makes Zed restart that context server and pick up the new tools.
func AgentToolsRev(tools []string) string {
	sorted := append([]string(nil), tools...)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, ",")))
	return hex.EncodeToString(sum[:4])
}
