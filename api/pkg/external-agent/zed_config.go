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
	APIURL          string           `json:"api_url"`                    // Custom API URL (empty = use default provider URL)
	AvailableModels []AvailableModel `json:"available_models,omitempty"` // Custom models to add
}

type AvailableModel struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	MaxTokens   int    `json:"max_tokens,omitempty"`
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
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

// GenerateZedMCPConfig creates Zed MCP configuration from Helix app config
func GenerateZedMCPConfig(
	app *types.App,
	userID string,
	sessionID string,
	helixAPIURL string,
	helixToken string,
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
	// ALWAYS use claude-sonnet-4-5-latest for Zed agents (ignore app database config)
	// The Helix UI doesn't expose model selection for Zed agents yet
	//
	// TODO: When adding model selection UI for Zed agents:
	// - Uncomment the code below to read from app.Config.Helix.Assistants[0]
	// - Add UI validation to only allow models compatible with Zed
	// - Zed supports: Anthropic, OpenAI, Google, etc - NOT all Helix models work
	//
	// var assistant types.AssistantConfig
	// if len(app.Config.Helix.Assistants) > 0 {
	// 	assistant = app.Config.Helix.Assistants[0]
	// } else {
	// 	assistant = types.AssistantConfig{
	// 		Provider: "anthropic",
	// 		Model:    "claude-sonnet-4-5-latest",
	// 	}
	// }

	// Use Haiku 4.5 for faster, cheaper responses
	assistant := types.AssistantConfig{
		Provider: "anthropic",
		Model:    "claude-haiku-4-5-latest",
	}

	// Configure agent with default model (CRITICAL: default_model goes in agent, not assistant!)
	config.Agent = &AgentConfig{
		DefaultModel: &ModelConfig{
			Provider: assistant.Provider,
			Model:    assistant.Model,
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
	config.LanguageModels = map[string]LanguageModelConfig{
		"anthropic": {
			APIURL: helixAPIURL + "/v1", // Helix Anthropic proxy
		},
		"openai": {
			APIURL: helixAPIURL + "/v1", // Helix OpenAI proxy
		},
	}

	// 1. Add Helix native tools as helix-cli MCP proxy
	if hasNativeTools(assistant) {
		config.ContextServers["helix-native"] = ContextServerConfig{
			Command: "helix-cli",
			Args: []string{
				"mcp", "run",
				"--app-id", app.ID,
				"--user-id", userID,
				"--session-id", sessionID,
			},
			Env: map[string]string{
				"HELIX_URL":   helixAPIURL,
				"HELIX_TOKEN": helixToken,
			},
		}
	}

	// 2. Pass-through external MCP servers
	for _, mcp := range assistant.MCPs {
		serverName := sanitizeName(mcp.Name)
		config.ContextServers[serverName] = mcpToContextServer(mcp)
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
		// HTTP/SSE transport - use helix-cli as proxy
		return ContextServerConfig{
			Command: "helix-cli",
			Args: []string{
				"mcp", "proxy",
				"--url", mcp.URL,
				"--name", mcp.Name,
			},
			Env: buildMCPEnv(mcp),
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
	helixAPIURL := os.Getenv("HELIX_API_URL")
	if helixAPIURL == "" {
		helixAPIURL = "http://api:8080"
	}

	// Generate runner token for this session
	helixToken := os.Getenv("RUNNER_TOKEN")
	if helixToken == "" {
		log.Warn().Msg("RUNNER_TOKEN not set, Zed MCP tools may not work")
	}

	config, err := GenerateZedMCPConfig(app, session.Owner, sessionID, helixAPIURL, helixToken)
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
