package external_agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// ZedMCPConfig represents Zed's MCP configuration format
type ZedMCPConfig struct {
	ContextServers map[string]ContextServerConfig `json:"context_servers"`
	LanguageModels map[string]LanguageModelConfig `json:"language_models,omitempty"`
	Assistant      *AssistantSettings             `json:"assistant,omitempty"`
	ExternalSync   *ExternalSyncConfig            `json:"external_sync,omitempty"`
	Agent          *AgentConfig                   `json:"agent,omitempty"`
	Theme          string                         `json:"theme,omitempty"`
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
	AlwaysAllowToolActions bool         `json:"always_allow_tool_actions"`
	ShowOnboarding         bool         `json:"show_onboarding"`
	AutoOpenPanel          bool         `json:"auto_open_panel"`
}

type LanguageModelConfig struct {
	APIURL string `json:"api_url"` // Custom API URL (empty = use default provider URL)
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
	// Zed expects "url" field for HTTP context_servers (untagged union)
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// GenerateZedMCPConfig creates Zed MCP configuration from Helix app config
func GenerateZedMCPConfig(
	app *types.App,
	userID string,
	sessionID string,
	helixAPIURL string,
	helixToken string,
	koditEnabled bool,
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

	// Default to anthropic/claude-sonnet if nothing is configured
	if provider == "" {
		provider = "anthropic"
	}
	if model == "" {
		model = "claude-sonnet-4-5-latest"
	}

	// Map Helix provider to Zed's provider type and format model name
	// Zed only knows: anthropic, openai, google, ollama, copilot, lmstudio, deepseek
	// All other providers (nebius, together, openrouter, etc.) use OpenAI-compatible API
	zedProvider, zedModel := mapHelixToZedProvider(provider, model)

	// Configure agent with default model (CRITICAL: default_model goes in agent, not assistant!)
	config.Agent = &AgentConfig{
		DefaultModel: &ModelConfig{
			Provider: zedProvider,
			Model:    zedModel,
		},
		AlwaysAllowToolActions: true,
		ShowOnboarding:         false,
		AutoOpenPanel:          true,
	}
	config.Theme = "One Dark"

	// Configure language_models to route API calls through Helix proxy
	// CRITICAL: Zed reads api_url from settings.json, NOT from ANTHROPIC_BASE_URL env var!
	// The env vars set in wolf_executor.go are NOT used by Zed's language model providers.
	// We must explicitly set api_url in language_models for each provider.
	//
	// IMPORTANT: Anthropic and OpenAI have different URL conventions in Zed:
	// - Anthropic: base URL only (Zed appends /v1/messages)
	// - OpenAI: base URL + /v1 (Zed appends /chat/completions)
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
		koditMCPURL := fmt.Sprintf("%s/api/v1/mcp/kodit", helixAPIURL)
		config.ContextServers["kodit"] = ContextServerConfig{
			URL: koditMCPURL,
			Headers: map[string]string{
				"Authorization": fmt.Sprintf("Bearer %s", helixToken),
			},
		}
	}

	// 3. Add desktop MCP server (screenshot, clipboard, input, window management tools)
	// This runs locally in the sandbox container on port 9877 (alongside screenshot-server)
	// Provides take_screenshot, save_screenshot, type_text, mouse_click, get_clipboard, set_clipboard,
	// list_windows, focus_window, maximize_window, tile_window, move_to_workspace, switch_to_workspace, get_workspaces
	config.ContextServers["helix-desktop"] = ContextServerConfig{
		URL: "http://localhost:9877/mcp",
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
		Args:    []string{"chrome-devtools-mcp@latest"},
		Env: map[string]string{
			// Use headless mode in sandbox containers (no visible browser window)
			"CHROME_DEVTOOLS_MCP_HEADLESS": "true",
			// Set viewport to match typical desktop resolution
			"CHROME_DEVTOOLS_MCP_VIEWPORT": "1920x1080",
		},
	}

	// 6. Pass-through external MCP servers
	if assistant != nil {
		for _, mcp := range assistant.MCPs {
			serverName := sanitizeName(mcp.Name)
			config.ContextServers[serverName] = mcpToContextServer(mcp)
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

// mcpToContextServer converts Helix MCP config to Zed context server config
func mcpToContextServer(mcp types.AssistantMCP) ContextServerConfig {
	// Parse MCP URL to determine connection type
	if strings.HasPrefix(mcp.URL, "http://") || strings.HasPrefix(mcp.URL, "https://") {
		// HTTP/SSE transport - direct HTTP connection
		return ContextServerConfig{
			URL:     mcp.URL,
			Headers: mcp.Headers,
		}
	}

	// Stdio transport - direct command execution
	// Parse command from URL (e.g., "stdio://npx @modelcontextprotocol/server-filesystem /tmp")
	cmd, args := parseStdioURL(mcp.URL)
	return ContextServerConfig{
		Command: cmd,
		Args:    args,
		Env:     buildMCPEnv(mcp),
	}
}

func buildMCPEnv(mcp types.AssistantMCP) map[string]string {
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

	// Claude 4.5 models (new naming: claude-opus-4-5, claude-sonnet-4-5, claude-haiku-4-5)
	if strings.HasPrefix(modelID, "claude-opus-4-5") {
		return "claude-opus-4-5-latest"
	}
	if strings.HasPrefix(modelID, "claude-sonnet-4-5") {
		return "claude-sonnet-4-5-latest"
	}
	if strings.HasPrefix(modelID, "claude-haiku-4-5") {
		return "claude-haiku-4-5-latest"
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

	// Generate runner token for this session
	helixToken := os.Getenv("RUNNER_TOKEN")
	if helixToken == "" {
		log.Warn().Msg("RUNNER_TOKEN not set, Zed MCP tools may not work")
	}

	// Check if Kodit is enabled (defaults to true)
	koditEnabled := os.Getenv("KODIT_ENABLED") != "false"

	config, err := GenerateZedMCPConfig(app, session.Owner, sessionID, helixAPIURL, helixToken, koditEnabled)
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
