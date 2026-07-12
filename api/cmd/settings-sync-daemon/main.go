package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
	"github.com/pelletier/go-toml/v2"
)

// SettingsPath and KeymapPath are vars (not consts) so unit tests can point
// them at a tempdir without touching the real Zed config.
var (
	SettingsPath    = "/home/retro/.config/zed/settings.json"
	KeymapPath      = "/home/retro/.config/zed/keymap.json"
	CodexConfigPath = "/home/retro/.codex/config.toml"
)

const (
	PollInterval = 30 * time.Second
	DebounceTime = 500 * time.Millisecond
)

type SettingsDaemon struct {
	httpClient   *http.Client
	apiURL       string
	apiToken     string
	sessionID    string
	watcher      *fsnotify.Watcher
	lastModified time.Time

	// User's Helix API token (for authenticating with LLM proxies)
	userAPIKey string

	// Code agent configuration (from Helix API)
	codeAgentConfig *CodeAgentConfig

	// Whether user has a Claude subscription available for credential sync
	claudeSubscriptionAvailable bool
	codexSubscriptionAvailable  bool

	// Track the last expiresAt we know about, so we can detect Claude Code token refreshes
	lastKnownExpiresAt int64

	// Setup token from `claude setup-token` (alternative to file-based OAuth credentials)
	claudeSetupToken string

	// Timestamp of our last write to the credentials file (to ignore our own fsnotify events)
	lastCredWrite      time.Time
	lastCodexCredWrite time.Time
	lastCodexRefresh   time.Time

	// Current state
	helixSettings         map[string]interface{}
	helixSettingsBaseline map[string]interface{} // Pre-injection snapshot for deepEqual comparison
	userOverrides         map[string]interface{}
}

// CodeAgentConfig mirrors the API response structure for code agent configuration
type CodeAgentConfig struct {
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	AgentName       string `json:"agent_name"`
	BaseURL         string `json:"base_url"`
	APIType         string `json:"api_type"`
	Runtime         string `json:"runtime"`           // "zed_agent" or "qwen_code" or "goose_code"
	MaxTokens       int    `json:"max_tokens"`        // Model's context window size (0 if unknown)
	MaxOutputTokens int    `json:"max_output_tokens"` // Model's max completion tokens (0 if unknown)

	// Goose-specific fields (only populated when Runtime == "goose_code").
	GooseRecipes       []GooseRecipe     `json:"goose_recipes,omitempty"`
	GooseRecipeRootDir string            `json:"goose_recipe_root_dir,omitempty"`
	GooseBakedRecipe   *GooseBakedRecipe `json:"goose_baked_recipe,omitempty"`
}

// GooseRecipe maps a slash-command name to a recipe YAML on disk (container path).
type GooseRecipe struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// GooseBakedRecipe is a fully-substituted recipe YAML that the daemon writes to
// a temp file and registers as a single slash_command.
type GooseBakedRecipe struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// AvailableModel represents a model entry for IDE model configuration.
// IDEs like Zed, VS Code, etc. require custom models to be explicitly listed.
// The JSON tags use common conventions that work across multiple IDEs.
type AvailableModel struct {
	Name            string `json:"name"`
	DisplayName     string `json:"display_name"`
	MaxTokens       int    `json:"max_tokens"`
	MaxOutputTokens int    `json:"max_output_tokens,omitempty"`
}

// helixConfigResponse is the response structure from the Helix API's zed-config endpoint
type helixConfigResponse struct {
	ContextServers              map[string]interface{} `json:"context_servers"`
	LanguageModels              map[string]interface{} `json:"language_models"`
	Assistant                   map[string]interface{} `json:"assistant"`
	ExternalSync                map[string]interface{} `json:"external_sync"`
	Agent                       map[string]interface{} `json:"agent"`
	Theme                       string                 `json:"theme"`
	ColorScheme                 string                 `json:"color_scheme"`
	Version                     int64                  `json:"version"`
	CodeAgentConfig             *CodeAgentConfig       `json:"code_agent_config"`
	ClaudeSubscriptionAvailable bool                   `json:"claude_subscription_available,omitempty"`
	CodexSubscriptionAvailable  bool                   `json:"codex_subscription_available,omitempty"`
}

// generateAgentServerConfig creates the agent_servers configuration for custom agents (like qwen).
// Returns nil for runtimes that use Zed's built-in agent.
//
// There are two code agent runtimes:
//  1. zed_agent - Zed's built-in agent panel. No agent_servers needed. Zed reads
//     env vars (ANTHROPIC_API_KEY, OPENAI_API_KEY, etc.) from the container environment.
//  2. qwen_code - Qwen code agent as a custom agent_server. Requires agent_servers
//     with qwen command and env vars (OPENAI_BASE_URL, OPENAI_API_KEY, OPENAI_MODEL).
func (d *SettingsDaemon) generateAgentServerConfig() map[string]interface{} {
	if d.codeAgentConfig == nil {
		// No code agent configured - return nil (no agent_servers will be set)
		log.Printf("Warning: No code agent configuration received from Helix API")
		return nil
	}

	switch d.codeAgentConfig.Runtime {
	case "qwen_code":
		// Qwen Code: Uses the qwen command as a custom agent_server
		// Rewrite localhost URLs for container networking (dev mode fix)
		baseURL := d.rewriteLocalhostURL(d.codeAgentConfig.BaseURL)
		env := map[string]interface{}{
			"GEMINI_TELEMETRY_ENABLED": "false",
			"OPENAI_BASE_URL":          baseURL,
			// Store sessions in persistent workspace directory (survives container restarts)
			"QWEN_DATA_DIR": "/home/retro/work/.qwen-state",
		}

		if d.userAPIKey != "" {
			env["OPENAI_API_KEY"] = d.userAPIKey
		}
		if d.codeAgentConfig.Model != "" {
			env["OPENAI_MODEL"] = d.codeAgentConfig.Model
		}

		log.Printf("Using qwen_code runtime: base_url=%s, model=%s",
			baseURL, d.codeAgentConfig.Model)

		return map[string]interface{}{
			"qwen": map[string]interface{}{
				"name":    "qwen",   // Required: Zed expects a name field for agent_servers
				"type":    "custom", // Required: Zed deserializes agent_servers using tagged enum
				"command": "qwen",
				"args": []string{
					// --yolo makes qwen start its ACP session in YOLO mode so it
					// auto-approves every tool call. This is passed on the command
					// line (not just via the "default_mode" setting below) on
					// purpose: default_mode only takes effect if the host IDE reads
					// it and sends an ACP session/set_mode after new_session. The
					// Zed builds pinned for spec-task sandboxes don't do that for
					// custom agent servers, so without --yolo qwen stays in
					// ApprovalMode.DEFAULT and every edit round-trips a
					// session/request_permission that nobody clicks in a headless
					// sandbox — the agent stalls on an "Allow all edits?" prompt.
					"--yolo",
					"--experimental-acp",
					"--no-telemetry",
					"--include-directories", "/home/retro/work",
				},
				"env": env,
				// default_mode is the IDE-mediated equivalent of --yolo: newer Zed
				// reads it and issues session/set_mode("yolo"), which also keeps the
				// Zed UI mode indicator in sync. Mirrors claude_code's
				// "bypassPermissions" entry below. --yolo above is the version-
				// independent guarantee; this is the nicety for IDEs that honour it.
				"default_mode": "yolo",
			},
		}

	case "claude_code":
		// Claude Code: Uses Zed's built-in claude-agent-acp npm package.
		// We configure it via /etc/claude-code/managed-settings.json (read by the
		// package at startup) rather than agent_servers.claude in Zed settings.
		// Writing to agent_servers.claude suppresses the model selector and
		// bypass-permissions toggle in Zed's UI; using managed settings avoids this.
		//
		// Two modes based on whether baseURL is set:
		// 1. API key mode (baseURL set): Claude Code uses Helix API proxy
		// 2. Subscription mode (no baseURL): Claude Code uses OAuth credentials
		env := map[string]string{}

		if d.codeAgentConfig.BaseURL != "" {
			// API key mode: route through Helix API proxy
			baseURL := d.rewriteLocalhostURL(d.codeAgentConfig.BaseURL)
			env["ANTHROPIC_BASE_URL"] = baseURL
			if d.userAPIKey != "" {
				env["ANTHROPIC_API_KEY"] = d.userAPIKey
			}
			log.Printf("Using claude_code runtime (API key mode): base_url=%s", baseURL)
		} else if d.claudeSetupToken != "" {
			// Setup token mode: inject CLAUDE_CODE_OAUTH_TOKEN env var.
			// No credentials file needed — Claude Code reads the token from the environment.
			env["CLAUDE_CODE_OAUTH_TOKEN"] = d.claudeSetupToken
			env["ANTHROPIC_BASE_URL"] = "https://api.anthropic.com"
			_ = os.WriteFile(ClaudeSubscriptionMarkerPath, []byte("1"), 0644)
			log.Printf("Using claude_code runtime (setup token mode)")
		} else {
			// OAuth subscription mode: Claude Code reads credentials from ~/.claude/.credentials.json.
			if _, err := os.Stat(ClaudeCredentialsPath); err != nil {
				log.Printf("Claude credentials file not yet available, deferring claude_code agent_servers: %v", err)
				return nil
			}
			_ = os.WriteFile(ClaudeSubscriptionMarkerPath, []byte("1"), 0644)
			env["ANTHROPIC_BASE_URL"] = "https://api.anthropic.com"
			log.Printf("Using claude_code runtime (subscription mode)")
		}

		// Use "claude-acp" (Zed's registry ID) with "type": "registry" so that
		// is_settings_registry("claude-acp") returns true immediately at startup,
		// before the AgentRegistryStore finishes its async network fetch. This
		// also triggers refresh_if_stale() earlier via has_registry_agents().
		// Helix sends agent_name="claude" over WebSocket; thread_service.rs
		// maps it to "claude-acp" before calling server.connect().
		// Write the model to managed-settings.json so the ACP agent picks it up at
		// session initialization. The SettingsManager reads this file and passes
		// settings.model to getAvailableModels(), which calls resolveModelPreference()
		// to set the correct currentModelId in the new_session response.
		// This drives the ConfigOptionsView model selector (config_options.current_value),
		// which is separate from session.models.current_model_id.
		d.writeClaudeManagedSettings()

		claudeACPConfig := map[string]interface{}{
			"type":         "registry",
			"default_mode": "bypassPermissions",
			"env":          env,
		}
		if d.codeAgentConfig.Model != "" {
			claudeACPConfig["default_model"] = d.codeAgentConfig.Model
		}
		return map[string]interface{}{
			"claude-acp": claudeACPConfig,
		}

	case "codex_cli":
		if err := ensureCodexNonInteractiveConfig(CodexConfigPath); err != nil {
			log.Printf("Failed to configure Codex non-interactive permissions: %v", err)
			return nil
		}
		env := map[string]interface{}{
			"CODEX_HOME": "/home/retro/.codex",
		}
		if d.codeAgentConfig.BaseURL != "" {
			env["OPENAI_BASE_URL"] = d.rewriteLocalhostURL(d.codeAgentConfig.BaseURL)
			if d.userAPIKey != "" {
				env["OPENAI_API_KEY"] = d.userAPIKey
			}
		} else {
			if _, err := os.Stat(CodexCredentialsPath); err != nil {
				log.Printf("Codex credentials file not yet available, deferring codex agent server: %v", err)
				return nil
			}
			env["OPENAI_API_KEY"] = ""
			env["OPENAI_BASE_URL"] = ""
		}
		config := map[string]interface{}{
			"type":         "registry",
			"default_mode": "full-access",
			"env":          env,
		}
		if d.codeAgentConfig.Model != "" {
			config["default_model"] = d.codeAgentConfig.Model
		}
		return map[string]interface{}{"codex-acp": config}

	case "goose_code":
		// Goose: Uses the `goose acp` command as a custom agent_server.
		// LLM provider/model are passed via GOOSE_PROVIDER + GOOSE_MODEL,
		// and provider-specific *_API_KEY / *_BASE_URL env vars override
		// goose's config file at startup.
		// Phase 2 will add per-recipe agent_servers entries on top of this
		// plain entry; for now we always emit one "goose" entry.
		baseURL := d.rewriteLocalhostURL(d.codeAgentConfig.BaseURL)
		env := map[string]interface{}{}

		// Map Helix APIType → goose provider + env var names. Goose
		// natively supports multiple providers, so unlike Qwen we don't
		// shoehorn everything through OPENAI_*.
		var gooseProvider string
		switch d.codeAgentConfig.APIType {
		case "anthropic":
			gooseProvider = "anthropic"
			if baseURL != "" {
				env["ANTHROPIC_BASE_URL"] = baseURL
			}
			if d.userAPIKey != "" {
				env["ANTHROPIC_API_KEY"] = d.userAPIKey
			}
		case "openai", "azure_openai", "":
			// Default to openai for any OpenAI-compatible API.
			gooseProvider = "openai"
			if baseURL != "" {
				env["OPENAI_BASE_URL"] = baseURL
			}
			if d.userAPIKey != "" {
				env["OPENAI_API_KEY"] = d.userAPIKey
			}
		default:
			// Unknown APIType — treat as OpenAI-compatible and log so
			// operators can see what happened.
			log.Printf("goose_code: unknown api_type %q, defaulting to openai provider",
				d.codeAgentConfig.APIType)
			gooseProvider = "openai"
			if baseURL != "" {
				env["OPENAI_BASE_URL"] = baseURL
			}
			if d.userAPIKey != "" {
				env["OPENAI_API_KEY"] = d.userAPIKey
			}
		}

		env["GOOSE_PROVIDER"] = gooseProvider
		if d.codeAgentConfig.Model != "" {
			env["GOOSE_MODEL"] = d.codeAgentConfig.Model
		}

		// Set GOOSE_RECIPE_PATH so subrecipes and fragments referenced by
		// project recipes resolve relative paths against the recipe repo
		// root rather than goose's default cwd.
		if d.codeAgentConfig.GooseRecipeRootDir != "" {
			env["GOOSE_RECIPE_PATH"] = d.codeAgentConfig.GooseRecipeRootDir
		}

		// Use a per-session XDG_CONFIG_HOME so the goose config we write
		// doesn't trample any user-level config that may also live under
		// ~/.config. goose uses XDG via etcetera::choose_app_strategy.
		xdgConfig := "/home/retro/.config/helix-goose"
		env["XDG_CONFIG_HOME"] = xdgConfig

		if err := d.writeGooseConfig(xdgConfig); err != nil {
			log.Printf("goose_code: failed to write config.yaml: %v", err)
		}

		log.Printf("Using goose_code runtime: provider=%s, model=%s, base_url=%s, recipes=%d, baked=%v",
			gooseProvider, d.codeAgentConfig.Model, baseURL,
			len(d.codeAgentConfig.GooseRecipes), d.codeAgentConfig.GooseBakedRecipe != nil)

		return map[string]interface{}{
			"goose": map[string]interface{}{
				"name":    "goose",
				"type":    "custom",
				"command": "goose",
				"args":    []string{"acp"},
				"env":     env,
			},
		}

	default: // "zed_agent" or empty (default)
		// Zed Agent: Uses Zed's built-in agent panel - no agent_servers needed
		// The container env vars (ANTHROPIC_API_KEY, OPENAI_API_KEY, etc.) are set by wolf_executor
		log.Printf("Using zed_agent runtime (no agent_servers needed), api_type=%s", d.codeAgentConfig.APIType)
		return nil
	}
}

func ensureCodexNonInteractiveConfig(path string) error {
	config := map[string]interface{}{}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := toml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("parse existing Codex config: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read existing Codex config: %w", err)
	}

	config["approval_policy"] = "never"
	config["sandbox_mode"] = "danger-full-access"
	data, err = toml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal Codex config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create Codex config directory: %w", err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write Codex config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("install Codex config: %w", err)
	}
	return nil
}

// writeGooseConfig writes ${xdgConfigHome}/goose/config.yaml so the goose acp
// process picks up our slash_commands. We use a dedicated XDG_CONFIG_HOME
// (set on the agent_servers env) to avoid clobbering any user-level goose
// config — goose resolves config via etcetera::choose_app_strategy which
// honours XDG_CONFIG_HOME on Linux.
//
// Two sources contribute slash_commands:
//   - GooseRecipes: project-declared recipes (Phase 2a).
//   - GooseBakedRecipe: a single recipe with parameters pre-substituted by the
//     API for a spec-task (Phase 2b). Written to a stable per-session path so
//     the recipe survives daemon restarts within the session.
func (d *SettingsDaemon) writeGooseConfig(xdgConfigHome string) error {
	configDir := filepath.Join(xdgConfigHome, "goose")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", configDir, err)
	}

	type slashCommand struct {
		Command    string `yaml:"command"`
		RecipePath string `yaml:"recipe_path"`
	}

	var slashCommands []slashCommand
	for _, r := range d.codeAgentConfig.GooseRecipes {
		if r.Name == "" || r.Path == "" {
			continue
		}
		slashCommands = append(slashCommands, slashCommand{
			Command:    r.Name,
			RecipePath: r.Path,
		})
	}

	if baked := d.codeAgentConfig.GooseBakedRecipe; baked != nil && baked.Name != "" && baked.Content != "" {
		bakedDir := filepath.Join(xdgConfigHome, "goose", "baked-recipes")
		if err := os.MkdirAll(bakedDir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", bakedDir, err)
		}
		bakedPath := filepath.Join(bakedDir, baked.Name+".yaml")
		if err := os.WriteFile(bakedPath, []byte(baked.Content), 0o644); err != nil {
			return fmt.Errorf("write baked recipe %s: %w", bakedPath, err)
		}
		slashCommands = append(slashCommands, slashCommand{
			Command:    baked.Name,
			RecipePath: bakedPath,
		})
	}

	// goose config.yaml is plain YAML; we hand-render to avoid pulling a
	// yaml dep into this binary. Keys are stable and don't need quoting.
	var buf bytes.Buffer
	buf.WriteString("# Generated by helix settings-sync-daemon. Do not edit by hand.\n")
	if len(slashCommands) == 0 {
		buf.WriteString("slash_commands: []\n")
	} else {
		buf.WriteString("slash_commands:\n")
		for _, sc := range slashCommands {
			fmt.Fprintf(&buf, "  - command: %q\n", sc.Command)
			fmt.Fprintf(&buf, "    recipe_path: %q\n", sc.RecipePath)
		}
	}

	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", configPath, err)
	}
	log.Printf("goose_code: wrote %s with %d slash_commands", configPath, len(slashCommands))
	return nil
}

// rewriteLocalhostURL replaces localhost in a URL with our known-working API host.
// This fixes the issue where the API server returns its SERVER_URL (localhost:8080 in dev),
// which is unreachable from inside containers. We use HELIX_API_URL's host instead,
// which we know works because the daemon connected with it.
// Only rewrites if URL contains "localhost" - production URLs pass through unchanged.
func (d *SettingsDaemon) rewriteLocalhostURL(originalURL string) string {
	if !strings.Contains(originalURL, "localhost") {
		return originalURL // Production URL, leave unchanged
	}

	// Parse our known-working API URL to get the host
	apiParsed, err := url.Parse(d.apiURL)
	if err != nil {
		log.Printf("Warning: failed to parse API URL")
		return originalURL
	}

	// Parse the original URL
	origParsed, err := url.Parse(originalURL)
	if err != nil {
		log.Printf("Warning: failed to parse model endpoint URL")
		return originalURL
	}

	// Replace the host with our working API host
	origParsed.Host = apiParsed.Host

	log.Printf("Rewrote localhost URL for container networking")
	return origParsed.String()
}

// injectAvailableModels adds the configured model to the provider's available_models list.
// Zed only recognizes models that are either built-in (gpt-4, claude-3, etc.) or listed
// in available_models. Without this, custom models like "helix/qwen3:8b" are rejected.
//
// IMPORTANT: For providers with native Zed support (e.g. "anthropic"), we skip injection
// entirely. Zed already has built-in definitions for all Claude models with correct context
// lengths, cache config, beta headers, thinking mode, etc. Injecting a Custom model from
// available_models would override the built-in with worse metadata.
func (d *SettingsDaemon) injectAvailableModels() {
	if d.codeAgentConfig == nil || d.codeAgentConfig.Model == "" {
		return
	}

	// claude_code uses the claude-agent-acp adapter, which resolves its model
	// from managed-settings.json — not Zed's language_models. Never inject a
	// Custom model entry for it. (In api_key mode APIType=="anthropic" is caught
	// below, but in subscription mode APIType is empty, so guard on runtime.)
	if d.codeAgentConfig.Runtime == "claude_code" {
		return
	}

	// Skip injection for providers where Zed has built-in model definitions.
	// Zed's built-ins have correct context lengths (e.g. 200K for claude-opus-4-6),
	// cache configuration, beta headers, thinking mode support, etc.
	// Injecting into available_models creates a degraded Custom model that's missing all that.
	if d.codeAgentConfig.APIType == "anthropic" {
		log.Printf("Skipping available_models injection for %s — Zed has built-in definitions for %s provider models",
			d.codeAgentConfig.Model, d.codeAgentConfig.APIType)
		return
	}

	languageModels, ok := d.helixSettings["language_models"].(map[string]interface{})
	if !ok {
		return
	}

	// Determine which provider to add the model to based on APIType
	providerName := d.codeAgentConfig.APIType
	if providerName == "" {
		providerName = "openai" // Default to OpenAI-compatible
	}

	providerConfig, ok := languageModels[providerName].(map[string]interface{})
	if !ok {
		providerConfig = make(map[string]interface{})
		languageModels[providerName] = providerConfig
	}

	// Create available_models entry with our custom model
	// Use token limits from model_info.json if available, otherwise use sensible defaults
	maxTokens := d.codeAgentConfig.MaxTokens
	if maxTokens == 0 {
		maxTokens = 200000 // Default context window for custom models if not found in model_info (200K matches most current frontier models)
	}

	modelEntry := AvailableModel{
		Name:            d.codeAgentConfig.Model,
		DisplayName:     d.codeAgentConfig.Model,
		MaxTokens:       maxTokens,
		MaxOutputTokens: d.codeAgentConfig.MaxOutputTokens, // 0 = omitted (uses model default)
	}

	// Get existing available_models or create new slice
	var availableModels []interface{}
	if existing, ok := providerConfig["available_models"].([]interface{}); ok {
		availableModels = existing
	}

	// Check if model already exists
	modelExists := false
	for _, m := range availableModels {
		if model, ok := m.(map[string]interface{}); ok {
			if model["name"] == d.codeAgentConfig.Model {
				modelExists = true
				break
			}
		}
	}

	if !modelExists {
		availableModels = append(availableModels, modelEntry)
		providerConfig["available_models"] = availableModels
		log.Printf("Added %s to available_models for %s provider", d.codeAgentConfig.Model, providerName)
	}
}

// injectKoditAuth adds the user's API key to the Kodit context_server's Authorization header.
// The Kodit MCP server URL is provided by Helix API, but the auth header must use the user's
// API key (not the runner token) so that the request is authenticated as the user.
func (d *SettingsDaemon) injectKoditAuth() {
	if d.userAPIKey == "" {
		log.Printf("Warning: USER_API_TOKEN not set, Kodit MCP may not authenticate correctly")
		return
	}

	contextServers, ok := d.helixSettings["context_servers"].(map[string]interface{})
	if !ok {
		return
	}

	koditServer, ok := contextServers["kodit"].(map[string]interface{})
	if !ok {
		return
	}

	// Add or update the Authorization header with user's API key
	headers, ok := koditServer["headers"].(map[string]interface{})
	if !ok {
		headers = make(map[string]interface{})
		koditServer["headers"] = headers
	}
	headers["Authorization"] = "Bearer " + d.userAPIKey

	// Also rewrite localhost URLs for container networking
	// Zed expects "url" field for HTTP context_servers
	if serverURL, ok := koditServer["url"].(string); ok {
		koditServer["url"] = d.rewriteLocalhostURL(serverURL)
	}

	log.Printf("Injected user API key into Kodit context_server Authorization header")
}

const (
	ClaudeCredentialsPath        = "/home/retro/.claude/.credentials.json"
	ClaudeSubscriptionMarkerPath = "/tmp/helix-claude-subscription-mode"
	ClaudeManagedSettingsPath    = "/etc/claude-code/managed-settings.json"
	CodexCredentialsPath         = "/home/retro/.codex/auth.json"
)

// writeClaudeManagedSettings writes /etc/claude-code/managed-settings.json so the
// claude-agent-acp SettingsManager picks up the model preference at session init.
// resolveModelPreference() handles substring matching so tier-level shorthand
// (e.g. "opus") resolves to the latest version, and versioned IDs (e.g.
// "claude-opus-4-6") resolve to their canonical form ("claude-opus-4-6-latest").
func (d *SettingsDaemon) writeClaudeManagedSettings() {
	settings := map[string]interface{}{}
	if d.codeAgentConfig != nil && d.codeAgentConfig.Model != "" {
		settings["model"] = d.codeAgentConfig.Model
	}

	data, err := json.Marshal(settings)
	if err != nil {
		log.Printf("Failed to marshal claude managed settings: %v", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(ClaudeManagedSettingsPath), 0755); err != nil {
		log.Printf("Failed to create claude managed settings dir: %v", err)
		return
	}
	if err := os.WriteFile(ClaudeManagedSettingsPath, data, 0644); err != nil {
		log.Printf("Failed to write claude managed settings: %v", err)
		return
	}
	log.Printf("Wrote claude managed settings: model=%s", d.codeAgentConfig.Model)
}

// syncClaudeCredentials fetches Claude credentials from the Helix API.
// For OAuth credentials: writes ~/.claude/.credentials.json (Claude Code reads this).
// For setup tokens: stores in memory and injects via CLAUDE_CODE_OAUTH_TOKEN env var.
func (d *SettingsDaemon) syncClaudeCredentials() {
	if !d.claudeSubscriptionAvailable {
		return
	}

	ctx := context.Background()
	apiURL := fmt.Sprintf("%s/api/v1/sessions/%s/claude-credentials", d.apiURL, d.sessionID)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		log.Printf("Failed to create Claude credentials request: %v", err)
		return
	}

	if d.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiToken)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		log.Printf("Failed to fetch Claude credentials: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Failed to fetch Claude credentials: status %d", resp.StatusCode)
		return
	}

	// Parse the new unified response format
	var credResp struct {
		CredentialType   string `json:"credential_type"`
		SetupToken       string `json:"setup_token,omitempty"`
		OAuthCredentials *struct {
			AccessToken      string   `json:"accessToken"`
			RefreshToken     string   `json:"refreshToken"`
			ExpiresAt        int64    `json:"expiresAt"`
			Scopes           []string `json:"scopes"`
			SubscriptionType string   `json:"subscriptionType"`
			RateLimitTier    string   `json:"rateLimitTier"`
		} `json:"oauth_credentials,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&credResp); err != nil {
		log.Printf("Failed to parse Claude credentials: %v", err)
		return
	}

	// Handle setup token mode: store in memory, inject via env var in generateAgentServerConfig.
	// No file write needed — Claude Code reads CLAUDE_CODE_OAUTH_TOKEN from the environment.
	if credResp.CredentialType == "setup_token" && credResp.SetupToken != "" {
		d.claudeSetupToken = credResp.SetupToken
		// Write markers so start-zed-core.sh knows credentials are available
		_ = os.WriteFile(ClaudeSubscriptionMarkerPath, []byte("1"), 0644)
		_ = os.WriteFile("/tmp/helix-claude-setup-token-mode", []byte("1"), 0644)
		// Ensure ~/.claude.json exists with onboarding complete (required for setup tokens)
		claudeJSON := "/home/retro/.claude.json"
		if _, err := os.Stat(claudeJSON); os.IsNotExist(err) {
			_ = os.WriteFile(claudeJSON, []byte(`{"hasCompletedOnboarding":true}`), 0644)
		}
		log.Printf("Using Claude setup token (CLAUDE_CODE_OAUTH_TOKEN mode)")
		return
	}

	// OAuth credentials mode: write to file (existing behavior)
	creds := credResp.OAuthCredentials
	if creds == nil {
		log.Printf("No OAuth credentials in response")
		return
	}

	// Before writing, check if the on-disk file has a newer token (Claude Code refreshed it).
	if fileExpiresAt := readCredentialsExpiresAt(ClaudeCredentialsPath); fileExpiresAt > creds.ExpiresAt {
		log.Printf("On-disk credentials are newer (file expiresAt=%d > api expiresAt=%d), pushing to API", fileExpiresAt, creds.ExpiresAt)
		d.pushCredentialsToAPI()
		return
	}

	d.lastKnownExpiresAt = creds.ExpiresAt

	credFile := map[string]interface{}{
		"claudeAiOauth": map[string]interface{}{
			"accessToken":      creds.AccessToken,
			"refreshToken":     creds.RefreshToken,
			"expiresAt":        creds.ExpiresAt,
			"scopes":           creds.Scopes,
			"subscriptionType": creds.SubscriptionType,
			"rateLimitTier":    creds.RateLimitTier,
		},
	}

	credJSON, err := json.MarshalIndent(credFile, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal Claude credentials: %v", err)
		return
	}

	credDir := filepath.Dir(ClaudeCredentialsPath)
	if err := os.MkdirAll(credDir, 0700); err != nil {
		log.Printf("Failed to create Claude credentials directory: %v", err)
		return
	}

	tmpFile := ClaudeCredentialsPath + ".tmp"
	if err := os.WriteFile(tmpFile, credJSON, 0600); err != nil {
		log.Printf("Failed to write Claude credentials temp file: %v", err)
		return
	}
	if err := os.Rename(tmpFile, ClaudeCredentialsPath); err != nil {
		log.Printf("Failed to rename Claude credentials file: %v", err)
		return
	}

	d.lastCredWrite = time.Now()
	log.Printf("Synced Claude credentials to %s (expiresAt=%d)", ClaudeCredentialsPath, creds.ExpiresAt)
}

// readCredentialsExpiresAt reads the expiresAt field from an on-disk credentials file.
// Returns 0 if the file doesn't exist or can't be parsed.
func readCredentialsExpiresAt(path string) int64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var credFile struct {
		ClaudeAiOauth struct {
			ExpiresAt int64 `json:"expiresAt"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(data, &credFile); err != nil {
		return 0
	}
	return credFile.ClaudeAiOauth.ExpiresAt
}

// pushCredentialsToAPI reads the on-disk credentials file and PUTs the
// refreshed credentials back to the Helix API so future sessions get them.
func (d *SettingsDaemon) pushCredentialsToAPI() {
	data, err := os.ReadFile(ClaudeCredentialsPath)
	if err != nil {
		log.Printf("Failed to read credentials file for push: %v", err)
		return
	}

	// Parse the file to extract the inner credentials
	var credFile struct {
		ClaudeAiOauth json.RawMessage `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(data, &credFile); err != nil {
		log.Printf("Failed to parse credentials file for push: %v", err)
		return
	}
	if credFile.ClaudeAiOauth == nil {
		log.Printf("No claudeAiOauth field in credentials file, skipping push")
		return
	}

	// Check if expiresAt is actually newer than what we last knew
	var creds struct {
		ExpiresAt int64 `json:"expiresAt"`
	}
	if err := json.Unmarshal(credFile.ClaudeAiOauth, &creds); err != nil {
		log.Printf("Failed to parse expiresAt from credentials: %v", err)
		return
	}
	if creds.ExpiresAt <= d.lastKnownExpiresAt {
		// Not a refresh — same or older token
		return
	}

	log.Printf("Detected token refresh: expiresAt %d -> %d, pushing to API", d.lastKnownExpiresAt, creds.ExpiresAt)

	ctx := context.Background()
	apiURL := fmt.Sprintf("%s/api/v1/sessions/%s/claude-credentials", d.apiURL, d.sessionID)
	req, err := http.NewRequestWithContext(ctx, "PUT", apiURL, bytes.NewReader(credFile.ClaudeAiOauth))
	if err != nil {
		log.Printf("Failed to create PUT credentials request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if d.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiToken)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		log.Printf("Failed to PUT refreshed credentials to API: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Failed to PUT refreshed credentials: status %d", resp.StatusCode)
		return
	}

	d.lastKnownExpiresAt = creds.ExpiresAt
	log.Printf("Pushed refreshed Claude credentials to API (expiresAt=%d)", creds.ExpiresAt)
}

type codexAuthCredentials struct {
	AuthMode     string  `json:"auth_mode"`
	OpenAIAPIKey *string `json:"OPENAI_API_KEY"`
	Tokens       struct {
		IDToken      string `json:"id_token"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		AccountID    string `json:"account_id"`
	} `json:"tokens"`
	LastRefresh time.Time `json:"last_refresh"`
}

func (d *SettingsDaemon) syncCodexCredentials() {
	if !d.codexSubscriptionAvailable {
		return
	}
	apiURL := fmt.Sprintf("%s/api/v1/sessions/%s/codex-credentials", d.apiURL, d.sessionID)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, apiURL, nil)
	if err != nil {
		log.Printf("Failed to create Codex credentials request: %v", err)
		return
	}
	if d.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiToken)
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		log.Printf("Failed to fetch Codex credentials: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("Failed to fetch Codex credentials: status %d", resp.StatusCode)
		return
	}
	var serverCredentials codexAuthCredentials
	if err := json.NewDecoder(resp.Body).Decode(&serverCredentials); err != nil {
		log.Printf("Failed to parse Codex credentials: %v", err)
		return
	}
	if fileCredentials, err := readCodexCredentials(); err == nil && fileCredentials.LastRefresh.After(serverCredentials.LastRefresh) {
		d.pushCodexCredentialsToAPI()
		return
	}
	data, err := json.MarshalIndent(serverCredentials, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal Codex credentials: %v", err)
		return
	}
	credentialDir := filepath.Dir(CodexCredentialsPath)
	if err := os.MkdirAll(credentialDir, 0700); err != nil {
		log.Printf("Failed to create Codex credentials directory: %v", err)
		return
	}
	tmpPath := CodexCredentialsPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		log.Printf("Failed to write Codex credentials: %v", err)
		return
	}
	if err := os.Rename(tmpPath, CodexCredentialsPath); err != nil {
		log.Printf("Failed to install Codex credentials: %v", err)
		return
	}
	d.lastCodexRefresh = serverCredentials.LastRefresh
	d.lastCodexCredWrite = time.Now()
	log.Printf("Synced Codex credentials to %s", CodexCredentialsPath)
}

func readCodexCredentials() (*codexAuthCredentials, error) {
	data, err := os.ReadFile(CodexCredentialsPath)
	if err != nil {
		return nil, err
	}
	var credentials codexAuthCredentials
	if err := json.Unmarshal(data, &credentials); err != nil {
		return nil, err
	}
	if credentials.AuthMode != "chatgpt" || credentials.Tokens.RefreshToken == "" || credentials.LastRefresh.IsZero() {
		return nil, fmt.Errorf("invalid Codex credentials")
	}
	return &credentials, nil
}

func (d *SettingsDaemon) pushCodexCredentialsToAPI() {
	credentials, err := readCodexCredentials()
	if err != nil || !credentials.LastRefresh.After(d.lastCodexRefresh) {
		return
	}
	payload, err := json.Marshal(credentials)
	if err != nil {
		return
	}
	apiURL := fmt.Sprintf("%s/api/v1/sessions/%s/codex-credentials", d.apiURL, d.sessionID)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, apiURL, bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if d.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiToken)
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		log.Printf("Failed to push Codex credentials: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("Failed to push Codex credentials: status %d", resp.StatusCode)
		return
	}
	d.lastCodexRefresh = credentials.LastRefresh
	log.Printf("Pushed refreshed Codex credentials to API")
}

// writeZedKeymap writes Zed keymap.json with terminal copy/paste bindings.
// XKB remaps Super (Command) → Ctrl, so we configure Zed's terminal to:
// - Ctrl+C: copy when text is selected, SIGINT when not (via context precedence)
// - Ctrl+V: paste (macOS users expect Command+V to paste)
func writeZedKeymap() {
	keymap := []map[string]interface{}{
		{
			"context": "Terminal && selection",
			"bindings": map[string]string{
				"ctrl-c": "terminal::Copy",
			},
		},
		{
			"context": "Terminal",
			"bindings": map[string]string{
				"ctrl-v": "terminal::Paste",
			},
		},
	}

	data, err := json.MarshalIndent(keymap, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal Zed keymap: %v", err)
		return
	}

	dir := filepath.Dir(KeymapPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("Failed to create Zed config directory: %v", err)
		return
	}

	if err := os.WriteFile(KeymapPath, data, 0644); err != nil {
		log.Printf("Failed to write Zed keymap: %v", err)
		return
	}

	log.Printf("Wrote Zed keymap to %s", KeymapPath)
}

func main() {
	// Environment variables
	helixURL := os.Getenv("HELIX_API_URL")
	if helixURL == "" {
		helixURL = "http://api:8080"
	}
	// The "api" hostname is baked into /etc/hosts by Hydra at container
	// creation time, so it always resolves to the outer API's IP even if
	// an inner compose stack later creates its own "api" service.
	sessionID := os.Getenv("HELIX_SESSION_ID")
	port := os.Getenv("SETTINGS_SYNC_PORT")
	if port == "" {
		port = "9877"
	}

	// User/sandbox API token - used for ALL API authentication
	// SECURITY: Runner token is never passed to containers
	userAPIKey := os.Getenv("USER_API_TOKEN")

	// For API calls, use USER_API_TOKEN (the user/sandbox-scoped token)
	// Legacy: fall back to HELIX_API_TOKEN if set (for backwards compatibility during rollout)
	helixToken := userAPIKey
	if helixToken == "" {
		helixToken = os.Getenv("HELIX_API_TOKEN")
	}

	if sessionID == "" {
		log.Fatal("HELIX_SESSION_ID environment variable is required")
	}

	log.Printf("Starting settings sync daemon for session %s", sessionID)
	log.Printf("Helix API URL: %s", helixURL)
	log.Printf("Settings path: %s", SettingsPath)

	// Create HTTP client with insecure TLS (TODO: make configurable)
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	daemon := &SettingsDaemon{
		httpClient: httpClient,
		apiURL:     helixURL,
		apiToken:   helixToken,
		sessionID:  sessionID,
		userAPIKey: userAPIKey,
	}

	// Write Zed keymap for terminal copy/paste behavior
	writeZedKeymap()

	// Initial sync from Helix → local with retry
	// Retry handles race condition where daemon starts before API token is fully available
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		if err := daemon.syncFromHelix(); err != nil {
			if i < maxRetries-1 {
				log.Printf("Initial sync attempt %d/%d failed: %v (retrying in 2s)", i+1, maxRetries, err)
				time.Sleep(2 * time.Second)
				continue
			}
			log.Printf("Warning: Initial sync failed after %d attempts: %v", maxRetries, err)
		} else {
			if i > 0 {
				log.Printf("Initial sync succeeded on attempt %d/%d", i+1, maxRetries)
			}
			break
		}
	}

	// Start file watcher for Zed changes
	if err := daemon.startWatcher(); err != nil {
		log.Fatalf("Failed to start file watcher: %v", err)
	}

	// Start polling loop for Helix changes (slow safety net, 30s)
	go daemon.pollHelixChanges()

	// Start a websocket subscriber for instant config-change notifications.
	// The API publishes a "config_changed" event to session-updates.<owner>.<session>
	// when the user toggles their color scheme; we re-sync immediately.
	go daemon.subscribeConfigEvents()

	// HTTP server for health checks and manual triggers
	http.HandleFunc("/health", daemon.healthCheck)
	http.HandleFunc("/settings", daemon.getSettings)
	http.HandleFunc("/reload", daemon.forceReload)

	log.Printf("Settings sync daemon listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// syncFromHelix fetches Helix-managed settings and merges with user overrides
func (d *SettingsDaemon) syncFromHelix() error {
	ctx := context.Background()

	// Fetch Helix-managed config
	url := fmt.Sprintf("%s/api/v1/sessions/%s/zed-config", d.apiURL, d.sessionID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if d.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiToken)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch Helix config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch config: status %d", resp.StatusCode)
	}

	var config helixConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return fmt.Errorf("failed to parse Helix config: %w", err)
	}

	// Store code agent config for generating agent_servers
	d.codeAgentConfig = config.CodeAgentConfig
	d.claudeSubscriptionAvailable = config.ClaudeSubscriptionAvailable
	d.codexSubscriptionAvailable = config.CodexSubscriptionAvailable

	// Sync Claude credentials if available
	d.syncClaudeCredentials()
	d.syncCodexCredentials()

	// Start from hardcoded Helix defaults, then layer on API response fields
	d.helixSettings = helixDefaults()
	d.helixSettings["context_servers"] = config.ContextServers
	if config.LanguageModels != nil {
		d.helixSettings["language_models"] = config.LanguageModels
	}
	if config.Assistant != nil {
		d.helixSettings["assistant"] = config.Assistant
	}
	if config.Agent != nil {
		d.helixSettings["agent"] = config.Agent
	}
	if t := d.effectiveTheme(config.Theme); t != "" {
		d.helixSettings["theme"] = t
	}
	injectAgentToolPermissions(d.helixSettings)

	// Save baseline before inject mutations (for deepEqual comparison in checkHelixUpdates)
	d.helixSettingsBaseline = copyMap(d.helixSettings)

	// Inject custom models (mutates d.helixSettings)
	// Note: API keys are NOT injected into settings.json — Zed reads ANTHROPIC_API_KEY /
	// OPENAI_API_KEY from container env vars (set by DesktopAgentAPIEnvVars).
	d.injectKoditAuth()
	d.injectAvailableModels()

	d.userOverrides = make(map[string]interface{})

	// Preserve telemetry settings from existing config
	if existingData, err := os.ReadFile(SettingsPath); err == nil {
		var existingSettings map[string]interface{}
		if err := json.Unmarshal(existingData, &existingSettings); err == nil {
			if value, exists := existingSettings["telemetry"]; exists {
				d.helixSettings["telemetry"] = value
			}
		}
	}

	// Inject code agent configuration (if using qwen custom agent)
	// For Anthropic/Azure, Zed's built-in agent is used (no agent_servers needed)
	// Note: We don't set "default_agent" because Zed doesn't have that setting.
	// Instead, thread_service.rs dynamically selects the agent based on agent_name from Helix.
	agentServers := d.generateAgentServerConfig()
	if agentServers != nil {
		d.helixSettings["agent_servers"] = agentServers
	}

	// Mirror the session owner's color scheme to the GNOME desktop. This is best-effort:
	// gsettings may fail if dconf is not available (e.g. not yet inside dbus-run-session)
	// or if the setting is unsupported on this distro — we just log and move on.
	d.applyGNOMEColorScheme(config.ColorScheme)

	return d.writeSettings(d.helixSettings)
}

// subscribeConfigEvents connects to the API's user websocket for this session and
// triggers an immediate re-sync whenever a config_changed event arrives. Reconnects
// forever on failure with a 1s backoff. Falls back to the 30s poll loop if the WS
// is unreachable. Pubsub events are not retained — the shorter backoff narrows
// the window in which a config_changed publish can be missed.
func (d *SettingsDaemon) subscribeConfigEvents() {
	for {
		if err := d.runConfigEventLoop(); err != nil {
			log.Printf("config event WS disconnected: %v (reconnecting in 1s)", err)
		}
		time.Sleep(1 * time.Second)
	}
}

func (d *SettingsDaemon) runConfigEventLoop() error {
	wsURL, err := url.Parse(d.apiURL)
	if err != nil {
		return fmt.Errorf("bad api url: %w", err)
	}
	switch wsURL.Scheme {
	case "https":
		wsURL.Scheme = "wss"
	default:
		wsURL.Scheme = "ws"
	}
	wsURL.Path = "/api/v1/ws/user"
	q := wsURL.Query()
	q.Set("session_id", d.sessionID)
	wsURL.RawQuery = q.Encode()

	dialer := *websocket.DefaultDialer
	dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	dialer.HandshakeTimeout = 10 * time.Second

	header := http.Header{}
	if d.apiToken != "" {
		header.Set("Authorization", "Bearer "+d.apiToken)
	}

	conn, _, err := dialer.Dial(wsURL.String(), header)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()
	log.Printf("config event WS connected (%s)", wsURL.String())

	// Re-sync on every successful (re)connect so we pick up any config_changed
	// publishes that happened while we were disconnected (pubsub doesn't
	// retain). Without this we'd have to wait up to 30s for the polling
	// fallback to repair state after a WS blip.
	if err := d.syncFromHelix(); err != nil {
		log.Printf("re-sync on WS connect failed: %v", err)
	}

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
		var evt struct {
			Type        string `json:"type"`
			Field       string `json:"field"`
			ColorScheme string `json:"color_scheme"`
		}
		if err := json.Unmarshal(msg, &evt); err != nil {
			continue // not all session-updates events are config_changed; ignore noise
		}
		if evt.Type != "config_changed" {
			continue
		}
		log.Printf("config_changed event: field=%s color_scheme=%s", evt.Field, evt.ColorScheme)
		if err := d.syncFromHelix(); err != nil {
			log.Printf("re-sync after config_changed failed: %v", err)
		}
		// In-place agent switch coordination:
		//   field="agent"          → fast path. settings.json has just been
		//      rewritten; Zed hot-reloads agent_servers + context_servers via
		//      its SettingsStore observers (no process restart). We then tell
		//      the API the new config is on disk so it can deliver the new
		//      thread to the still-running Zed over the live WebSocket.
		//   field="agent_restart"  → fallback. The API asks for a clean restart
		//      (e.g. live delivery failed / the new custom agent didn't register
		//      from the hot-reload). pkill Zed; run_zed_restart_loop respawns it
		//      and the reconnect path delivers the pending handoff.
		switch evt.Field {
		case "agent":
			d.notifyAgentConfigApplied()
		case "agent_restart":
			d.restartZed()
		}
	}
}

// notifyAgentConfigApplied tells the Helix API that settings.json has been
// rewritten for an in-place agent switch and Zed has had it hot-reloaded, so the
// API can deliver the new thread over the live external-agent WebSocket without
// waiting for a process restart + reconnect. Best-effort: if this fails, the
// API's restart fallback (it sees no new ZedThreadID within its timeout) takes
// over.
func (d *SettingsDaemon) notifyAgentConfigApplied() {
	ctx := context.Background()
	url := fmt.Sprintf("%s/api/v1/sessions/%s/agent-config-applied", d.apiURL, d.sessionID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		log.Printf("notifyAgentConfigApplied: failed to build request: %v", err)
		return
	}
	if d.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiToken)
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		log.Printf("notifyAgentConfigApplied: request failed: %v (API restart fallback will cover this)", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Printf("notifyAgentConfigApplied: API returned status %d", resp.StatusCode)
		return
	}
	log.Printf("notifyAgentConfigApplied: API notified of applied agent config for live thread delivery")
}

// restartZed kills the running Zed editor process. The desktop's
// run_zed_restart_loop (start-zed-core.sh) respawns it after a 2s sleep, so the
// new process reads the freshly-written settings.json — picking up the switched
// agent's agent_servers and MCP context_servers. Best-effort: if no Zed process
// is running (e.g. still booting), pkill is a no-op and the first launch already
// reads the new config.
func (d *SettingsDaemon) restartZed() {
	// pkill -x matches the exact process name "zed" (the editor binary), not
	// the bash restart-loop script, so we only restart the editor.
	out, err := exec.Command("pkill", "-x", "zed").CombinedOutput()
	if err != nil {
		// Exit code 1 just means "no process matched" — fine, nothing to do.
		log.Printf("restartZed: pkill zed returned: %v (%s) — likely no running Zed yet", err, strings.TrimSpace(string(out)))
		return
	}
	log.Printf("restartZed: signalled Zed to restart for agent switch; run_zed_restart_loop will respawn with new config")
}

// applyGNOMEColorScheme runs gsettings to switch GNOME's color scheme. Empty
// string is treated as dark for now (matches the default Yaru Dark loaded by
// startup-app.sh).
//
// We have to set:
//   - color-scheme (the modern preference signal libadwaita / GNOME 42+ apps
//     respect)
//   - gtk-theme (the actual rendered theme — without this, GTK3 apps + the
//     shell stay on whatever was loaded at startup, so the desktop looks
//     unchanged even when color-scheme says prefer-light)
//   - desktop wallpaper — kept on the Helix logo in both modes; we set both
//     picture-uri and picture-uri-dark so GNOME reads the right slot
//     regardless of the active color scheme
func (d *SettingsDaemon) applyGNOMEColorScheme(scheme string) {
	colorScheme := "prefer-dark"
	gtkTheme := "Yaru-dark"
	wallpaper := "file:///usr/share/backgrounds/helix-logo.png"
	if scheme == "light" {
		colorScheme = "prefer-light"
		gtkTheme = "Yaru"
	}

	cmds := [][]string{
		{"gsettings", "set", "org.gnome.desktop.interface", "color-scheme", colorScheme},
		{"gsettings", "set", "org.gnome.desktop.interface", "gtk-theme", gtkTheme},
		{"gsettings", "set", "org.gnome.desktop.background", "picture-uri", wallpaper},
		{"gsettings", "set", "org.gnome.desktop.background", "picture-uri-dark", wallpaper},
	}
	for _, c := range cmds {
		out, err := exec.Command(c[0], c[1:]...).CombinedOutput()
		if err != nil {
			log.Printf("gsettings %v failed: %v (%s)", c[1:], err, strings.TrimSpace(string(out)))
		}
	}
	log.Printf("applied GNOME color-scheme=%s gtk-theme=%s wallpaper=%s", colorScheme, gtkTheme, wallpaper)
}

// HELIX_MANAGED_THEMES are the Zed editor themes the daemon itself sets in
// response to the session owner's color-scheme preference. An on-disk theme
// in this set (or empty) is considered Helix-owned and may be overwritten on
// the next sync; anything else is treated as a deliberate user choice (e.g.
// the user picked "Solarized Dark" in Zed's UI) and preserved.
var HELIX_MANAGED_THEMES = map[string]bool{
	"One Light": true,
	"Ayu Dark":  true,
}

// effectiveTheme decides which value to write to settings.json's "theme" key.
// It returns apiTheme when the on-disk value is unset or one of the
// Helix-managed themes; otherwise it returns the on-disk value, preserving
// the user's manual Zed-UI choice. apiTheme=="" disables the assignment in
// the caller (we don't want to delete an existing theme key).
//
// Emits one structured INFO log line per call so that future debugging of
// the helix→zed theme sync can `grep` for "theme sync:" and see which
// branch fired without having to re-derive the logic from source.
func (d *SettingsDaemon) effectiveTheme(apiTheme string) string {
	result, branch, onDiskRepr := d.computeEffectiveTheme(apiTheme)
	log.Printf("theme sync: branch=%s on_disk=%s wrote=%q api=%q",
		branch, onDiskRepr, result, apiTheme)
	return result
}

// computeEffectiveTheme is the pure decision function behind effectiveTheme.
// Split out so unit tests can assert the branch taken without having to
// scrape log output. Returns the value to write (or "" to skip the write),
// the branch label, and a human-readable repr of what was on disk.
//
// Branches:
//   - no_api_theme       — apiTheme is empty; caller should skip the assign.
//   - no_existing_file   — settings.json missing; write apiTheme.
//   - unparseable        — settings.json corrupt; write apiTheme.
//   - no_theme_key       — theme key absent; write apiTheme.
//   - structured_replace — theme is a {mode,light,dark} object; replace with apiTheme string.
//   - empty_string       — theme is "" on disk; write apiTheme.
//   - managed_overwrite  — theme is one of HELIX_MANAGED_THEMES; write apiTheme.
//   - preserve_custom    — theme is a custom string the user picked in Zed; preserve it.
func (d *SettingsDaemon) computeEffectiveTheme(apiTheme string) (result, branch, onDiskRepr string) {
	if apiTheme == "" {
		return "", "no_api_theme", "<not_read>"
	}
	data, err := os.ReadFile(SettingsPath)
	if err != nil {
		return apiTheme, "no_existing_file", "<missing>"
	}
	var existing map[string]interface{}
	if err := json.Unmarshal(data, &existing); err != nil {
		return apiTheme, "unparseable", "<unparseable>"
	}
	raw, present := existing["theme"]
	if !present {
		return apiTheme, "no_theme_key", "<absent>"
	}
	onDisk, ok := raw.(string)
	if !ok {
		// Structured theme — most likely {mode, light, dark} written by Zed's
		// own theme picker / ToggleMode action. Replace with the bare string
		// the API chose; this also dislodges any sticky Dynamic{mode:System}
		// state Zed may be holding.
		encoded, _ := json.Marshal(raw)
		return apiTheme, "structured_replace", string(encoded)
	}
	if onDisk == "" {
		return apiTheme, "empty_string", `""`
	}
	if HELIX_MANAGED_THEMES[onDisk] {
		return apiTheme, "managed_overwrite", fmt.Sprintf("%q", onDisk)
	}
	return onDisk, "preserve_custom", fmt.Sprintf("%q", onDisk)
}

// SECURITY_PROTECTED_FIELDS must not be synced to the Helix API
// Also includes deprecated fields that should be removed from settings
var SECURITY_PROTECTED_FIELDS = map[string]bool{
	"telemetry":     true,
	"agent_servers": true,
	"default_agent": true, // Deprecated: Zed doesn't have this setting; remove from old configs
	"external_sync": true, // Deprecated: Zed reads this from env vars, not settings.json
}

// USER_PREFERENCE_FIELDS used to hold "theme" so mergeSettings would always
// preserve the on-disk value. That was the wrong model once Helix started
// driving the theme from the user's color-scheme preference: it pinned the
// stale value and made dark→light→dark fail (the second dark write was
// silently overwritten by the on-disk "One Light"). Theme handling now lives
// in HELIX_MANAGED_THEMES + effectiveTheme(), called from syncFromHelix and
// checkHelixUpdates. Kept (empty) so the SECURITY_PROTECTED_FIELDS sibling
// pattern stays obvious; remove if no field ever needs this behaviour again.
var USER_PREFERENCE_FIELDS = map[string]bool{}

// HELIX_MANAGED_AGENT_FIELDS lists keys under "agent" that Helix owns and that
// user-side overrides (i.e. whatever Zed wrote to settings.json) must never
// clobber. When Zed boots and writes a different default_model — either because
// the user picked one in the model picker or because Zed's built-in agent
// profile fell back to its hardcoded Claude default — the daemon used to
// faithfully sync that back to disk on the next poll, even though Helix had
// the authoritative value.
//
// See deviqon/P1-5-zed-overrides-clobber-helix-default-model.md.
var HELIX_MANAGED_AGENT_FIELDS = map[string]bool{
	"default_model":          true,
	"inline_assistant_model": true,
	"commit_message_model":   true,
	"thread_summary_model":   true,
}

// HELIX_OWNED_CONTEXT_SERVERS lists context_server names whose configuration
// Helix unconditionally owns. The names are the ones hardcoded in
// api/pkg/external-agent/zed_config.go (chrome-devtools, helix-session,
// helix-desktop) — i.e. servers that Helix sets up itself (not from a user's
// project / app MCP config).
//
// Why: when these names already exist in the on-disk settings.json from a
// previous run AND Helix updated their config in zed_config.go since then
// (e.g. switched chrome-devtools from `npx chrome-devtools-mcp@latest` to
// the global `/usr/bin/chrome-devtools-mcp` binary in PR #2418), the
// daemon's deep-merge in `mergeSettings` would treat the on-disk entry as
// a "user override" and let it win — leaving stale `npx`-based configs in
// place forever, with the resulting 180s `chrome-devtools context server
// failed to start: Context server request timeout` reported in
// helix/design/2026-05-13-mcp-cache-contention-and-duplicate-claude-spawn.md.
//
// User-configured MCPs (from app or project skills, e.g. drone-ci, github,
// custom servers) are NOT in this set — those legitimately can be edited by
// the user in their on-disk settings.json and should round-trip back to the
// API as overrides. They're also keyed by user-chosen names that we can't
// enumerate here.
var HELIX_OWNED_CONTEXT_SERVERS = map[string]bool{
	"chrome-devtools": true,
	"helix-session":   true,
	"helix-desktop":   true,
}

// helixDefaults returns the static Helix-owned settings that must be present
// in every settings.json. Both syncFromHelix() and checkHelixUpdates() use
// this as the base, then layer on API response fields.
func helixDefaults() map[string]interface{} {
	return map[string]interface{}{
		// Use grayscale text rendering - subpixel antialiasing doesn't work well
		// over video streaming since the client display's subpixel layout is unknown
		"text_rendering_mode": "grayscale",
		// Disable dev container suggestions - Helix runs Zed inside its own containers
		"suggest_dev_container": false,
		// Disable auto-formatting globally - it mangles JS/TS/TSX in our codebases.
		// Go keeps format_on_save via per-language override (gofmt is expected).
		"format_on_save": "off",
		// Bump context-server initialize timeout from upstream's 60s default to 180s.
		// Several MCPs in our spec-task containers (chrome-devtools, github via
		// `npx <pkg>@latest`, helixos via http to a still-warming-up api:8080) all
		// fire their JSON-RPC `initialize` at the same moment Zed boots on a cold
		// container, racing for CPU against settings-sync-daemon and language
		// servers. The first npm download routinely overruns 60s and Zed marks the
		// servers as failed; tools never appear (most visibly
		// `mcp__chrome-devtools__*` go missing).
		// helixml/zed#47 tried to fix this by bumping DEFAULT_REQUEST_TIMEOUT in
		// crates/context_server/src/client.rs, but that constant is dead code in
		// our path: project_settings.rs defaults context_server_timeout to 60 and
		// passes Some(60) all the way to client.rs:370, where the .or(DEFAULT)
		// fallback never fires. Fixing it here in settings.json works regardless
		// of upstream changes and survives Zed rebases.
		"context_server_timeout": 180,
		"languages": map[string]interface{}{
			"Go": map[string]interface{}{
				"format_on_save": "on",
			},
		},
	}
}

// injectAgentToolPermissions sets tool_permissions.default = "allow" on the
// agent section. Extracted so both syncFromHelix and checkHelixUpdates use it.
func injectAgentToolPermissions(settings map[string]interface{}) {
	agentSection, ok := settings["agent"].(map[string]interface{})
	if !ok {
		agentSection = map[string]interface{}{}
	}
	agentSection["tool_permissions"] = map[string]interface{}{
		"default": "allow",
	}
	settings["agent"] = agentSection
}

// copyMap returns a shallow copy of a map[string]interface{}.
func copyMap(m map[string]interface{}) map[string]interface{} {
	copy := make(map[string]interface{}, len(m))
	for k, v := range m {
		copy[k] = v
	}
	return copy
}

// mergeSettings combines Helix settings with user overrides, then injects code agent config
func (d *SettingsDaemon) mergeSettings(helix, user map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{})

	for k, v := range helix {
		if SECURITY_PROTECTED_FIELDS[k] {
			continue
		}
		merged[k] = v
	}

	// Deep merge context_servers — user entries win, EXCEPT for Helix-owned
	// names where Helix's hardcoded definition unconditionally wins. Without
	// the latter clause an on-disk settings.json from a previous run pins
	// the OLD config (stale `npx chrome-devtools-mcp@latest` etc.) forever;
	// see HELIX_OWNED_CONTEXT_SERVERS for the full reasoning.
	if userServers, ok := user["context_servers"].(map[string]interface{}); ok {
		if helixServers, ok := merged["context_servers"].(map[string]interface{}); ok {
			for name, config := range userServers {
				if HELIX_OWNED_CONTEXT_SERVERS[name] {
					log.Printf("dropping user override for helix-owned context_server: %s", name)
					continue
				}
				helixServers[name] = config
			}
		} else {
			// No helix-side servers at all — adopt user's verbatim, but
			// still strip helix-owned names so a later API roll-out
			// adding them isn't pre-empted by a stale on-disk entry.
			filtered := make(map[string]interface{}, len(userServers))
			for name, config := range userServers {
				if HELIX_OWNED_CONTEXT_SERVERS[name] {
					log.Printf("dropping user override for helix-owned context_server: %s", name)
					continue
				}
				filtered[name] = config
			}
			merged["context_servers"] = filtered
		}
	}

	// Deep merge languages (same pattern as context_servers)
	if userLangs, ok := user["languages"].(map[string]interface{}); ok {
		if helixLangs, ok := merged["languages"].(map[string]interface{}); ok {
			for lang, config := range userLangs {
				helixLangs[lang] = config
			}
		} else {
			merged["languages"] = userLangs
		}
	}

	for k, v := range user {
		if k == "context_servers" || k == "languages" {
			continue
		}
		if k == "agent" {
			merged["agent"] = mergeAgentBlock(merged["agent"], v)
			continue
		}
		merged[k] = v
	}

	// Preserve security-protected and user-preference fields from on-disk config.
	// Security-protected fields (telemetry) are never synced.
	// User-preference fields (theme) are set as initial defaults but never overwritten.
	if existingData, err := os.ReadFile(SettingsPath); err == nil {
		var existing map[string]interface{}
		if err := json.Unmarshal(existingData, &existing); err == nil {
			if value, exists := existing["telemetry"]; exists {
				merged["telemetry"] = value
			}
			for field := range USER_PREFERENCE_FIELDS {
				if value, exists := existing[field]; exists {
					merged[field] = value
				}
			}
		}
	}

	// Inject code agent configuration (if using qwen custom agent)
	// For Anthropic/Azure, Zed's built-in agent is used (no agent_servers needed)
	agentServers := d.generateAgentServerConfig()
	if agentServers != nil {
		merged["agent_servers"] = agentServers
	}

	return merged
}

// mergeAgentBlock deep-merges Zed's user-side "agent" block onto the helix-managed
// one, dropping any user-side values for keys in HELIX_MANAGED_AGENT_FIELDS so
// helix's model selections always win.
func mergeAgentBlock(helixAgent, userAgent interface{}) interface{} {
	userMap, ok := userAgent.(map[string]interface{})
	if !ok {
		// User override is not an object — keep helix's agent verbatim.
		if helixAgent != nil {
			return helixAgent
		}
		return userAgent
	}

	merged := make(map[string]interface{})
	if helixMap, ok := helixAgent.(map[string]interface{}); ok {
		for k, v := range helixMap {
			merged[k] = v
		}
	}

	for k, v := range userMap {
		if HELIX_MANAGED_AGENT_FIELDS[k] {
			log.Printf("dropping user override for helix-managed agent field: %s", k)
			continue
		}
		merged[k] = v
	}

	return merged
}

// extractUserOverrides finds settings that differ from Helix base
func extractUserOverrides(current, helix map[string]interface{}) map[string]interface{} {
	overrides := make(map[string]interface{})

	// Deep diff context_servers (per-server). Helix-owned names are never
	// captured as user overrides — Helix sets them itself in zed_config.go
	// and any divergence on disk is stale state from a prior run, not a
	// user customization. Without this guard the daemon would round-trip
	// the stale value back to the API and the next sync would re-write it
	// to disk, pinning the old config forever (see
	// HELIX_OWNED_CONTEXT_SERVERS).
	if currentServers, ok := current["context_servers"].(map[string]interface{}); ok {
		helixServers, _ := helix["context_servers"].(map[string]interface{})
		userServers := make(map[string]interface{})

		for name, config := range currentServers {
			if HELIX_OWNED_CONTEXT_SERVERS[name] {
				continue
			}
			if helixConfig, inHelix := helixServers[name]; !inHelix {
				userServers[name] = config
			} else if !deepEqual(config, helixConfig) {
				userServers[name] = config
			}
		}

		if len(userServers) > 0 {
			overrides["context_servers"] = userServers
		}
	}

	// Deep diff languages (per-language, same pattern as context_servers)
	if currentLangs, ok := current["languages"].(map[string]interface{}); ok {
		helixLangs, _ := helix["languages"].(map[string]interface{})
		userLangs := make(map[string]interface{})

		for lang, config := range currentLangs {
			if helixConfig, inHelix := helixLangs[lang]; !inHelix {
				userLangs[lang] = config
			} else if !deepEqual(config, helixConfig) {
				userLangs[lang] = config
			}
		}

		if len(userLangs) > 0 {
			overrides["languages"] = userLangs
		}
	}

	for k, v := range current {
		// "theme" is a daemon-local decision (effectiveTheme reads on-disk
		// directly each sync), so it must not be uploaded to the API as a
		// user-side override — that would create a stale snapshot that
		// replays back on the next sync.
		if k == "context_servers" || k == "languages" || k == "theme" || SECURITY_PROTECTED_FIELDS[k] || USER_PREFERENCE_FIELDS[k] {
			continue
		}
		if k == "agent" {
			// Diff the agent block per-field, dropping helix-managed model fields so
			// they never get uploaded back to the API. Defense-in-depth alongside
			// the merge guard above.
			if agentDiff := diffAgentBlock(v, helix["agent"]); agentDiff != nil {
				overrides["agent"] = agentDiff
			}
			continue
		}
		if helixVal, inHelix := helix[k]; !inHelix || !deepEqual(v, helixVal) {
			overrides[k] = v
		}
	}

	return overrides
}

// diffAgentBlock returns the user-side keys under "agent" that differ from the
// helix-managed value, with helix-managed model fields excluded. Returns nil if
// there is nothing to upload.
func diffAgentBlock(current, helix interface{}) map[string]interface{} {
	currentMap, ok := current.(map[string]interface{})
	if !ok {
		return nil
	}
	helixMap, _ := helix.(map[string]interface{})

	diff := make(map[string]interface{})
	for k, v := range currentMap {
		if HELIX_MANAGED_AGENT_FIELDS[k] {
			continue
		}
		if helixVal, inHelix := helixMap[k]; !inHelix || !deepEqual(v, helixVal) {
			diff[k] = v
		}
	}
	if len(diff) == 0 {
		return nil
	}
	return diff
}

// startWatcher monitors settings.json for Zed UI changes and
// ~/.claude/.credentials.json for Claude Code token refreshes.
func (d *SettingsDaemon) startWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	d.watcher = watcher

	// Ensure directory exists
	settingsDir := filepath.Dir(SettingsPath)
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		return fmt.Errorf("failed to create settings directory: %w", err)
	}

	// Ensure themes directory exists. Zed's theme watcher watches this path;
	// if it doesn't exist, the watcher falls back to the parent config dir
	// and ends up trying to parse settings.json as a theme file on every write.
	themesDir := filepath.Join(settingsDir, "themes")
	if err := os.MkdirAll(themesDir, 0755); err != nil {
		log.Printf("Warning: failed to create themes directory: %v", err)
	}

	// Create empty settings file if it doesn't exist
	if _, err := os.Stat(SettingsPath); os.IsNotExist(err) {
		if err := os.WriteFile(SettingsPath, []byte("{}"), 0644); err != nil {
			return fmt.Errorf("failed to create settings file: %w", err)
		}
	}

	// Watch the settings DIRECTORY (not the file itself) so atomic renames
	// in writeSettings() don't kill the watcher. On Linux, inotify watches
	// inodes — os.Rename() replaces the inode, making a file-level watcher
	// permanently dead. Same pattern as the Claude credentials watcher below.
	if err := watcher.Add(settingsDir); err != nil {
		return err
	}

	// Watch the Claude credentials directory (not the file itself) so we catch
	// atomic renames (which create a new inode). We watch the parent dir and
	// filter for events on the credentials filename.
	if d.claudeSubscriptionAvailable {
		credDir := filepath.Dir(ClaudeCredentialsPath)
		if err := os.MkdirAll(credDir, 0700); err != nil {
			log.Printf("Warning: failed to create Claude credentials directory for watcher: %v", err)
		} else if err := watcher.Add(credDir); err != nil {
			log.Printf("Warning: failed to watch Claude credentials directory: %v", err)
		} else {
			log.Printf("Watching %s for credential refreshes", credDir)
		}
	}
	if d.codexSubscriptionAvailable {
		credDir := filepath.Dir(CodexCredentialsPath)
		if err := os.MkdirAll(credDir, 0700); err != nil {
			log.Printf("Warning: failed to create Codex credentials directory for watcher: %v", err)
		} else if err := watcher.Add(credDir); err != nil {
			log.Printf("Warning: failed to watch Codex credentials directory: %v", err)
		}
	}

	go func() {
		var settingsDebounce *time.Timer
		var credsDebounce *time.Timer
		var codexCredsDebounce *time.Timer
		credFilename := filepath.Base(ClaudeCredentialsPath)
		codexCredFilename := filepath.Base(CodexCredentialsPath)

		for {
			select {
			case event := <-watcher.Events:
				if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
					if filepath.Base(event.Name) == credFilename && filepath.Dir(event.Name) == filepath.Dir(ClaudeCredentialsPath) {
						// Claude credentials file changed
						if credsDebounce != nil {
							credsDebounce.Stop()
						}
						credsDebounce = time.AfterFunc(DebounceTime, func() {
							d.onCredentialsChanged()
						})
					} else if filepath.Base(event.Name) == codexCredFilename && filepath.Dir(event.Name) == filepath.Dir(CodexCredentialsPath) {
						if codexCredsDebounce != nil {
							codexCredsDebounce.Stop()
						}
						codexCredsDebounce = time.AfterFunc(DebounceTime, d.onCodexCredentialsChanged)
					} else if filepath.Base(event.Name) == filepath.Base(SettingsPath) {
						// Zed settings file changed
						if settingsDebounce != nil {
							settingsDebounce.Stop()
						}
						settingsDebounce = time.AfterFunc(DebounceTime, func() {
							d.onFileChanged()
						})
					}
				}
			case err := <-watcher.Errors:
				log.Printf("Watcher error: %v", err)
			}
		}
	}()

	return nil
}

// onCredentialsChanged handles Claude Code writing refreshed tokens to .credentials.json.
func (d *SettingsDaemon) onCredentialsChanged() {
	// Ignore events triggered by our own writes
	if time.Since(d.lastCredWrite) < 2*time.Second {
		return
	}

	log.Printf("Detected credentials file change (not from our write)")
	d.pushCredentialsToAPI()
}

func (d *SettingsDaemon) onCodexCredentialsChanged() {
	if time.Since(d.lastCodexCredWrite) < 2*time.Second {
		return
	}
	d.pushCodexCredentialsToAPI()
}

// onFileChanged handles Zed UI modifications to settings.json
func (d *SettingsDaemon) onFileChanged() {
	// Prevent re-triggering on our own writes
	if time.Since(d.lastModified) < 1*time.Second {
		return
	}

	log.Println("Detected settings.json change from Zed UI")

	// Read current settings
	data, err := os.ReadFile(SettingsPath)
	if err != nil {
		log.Printf("Failed to read settings: %v", err)
		return
	}

	var current map[string]interface{}
	if err := json.Unmarshal(data, &current); err != nil {
		log.Printf("Failed to parse settings: %v", err)
		return
	}

	// Extract user overrides
	d.userOverrides = extractUserOverrides(current, d.helixSettings)

	// Send to Helix API for persistence
	if err := d.syncToHelix(); err != nil {
		log.Printf("Failed to sync to Helix: %v", err)
	}
}

// syncToHelix sends user overrides to Helix API
func (d *SettingsDaemon) syncToHelix() error {
	ctx := context.Background()

	payload, err := json.Marshal(d.userOverrides)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/sessions/%s/zed-config/user", d.apiURL, d.sessionID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if d.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiToken)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("failed to sync to Helix: status %d", resp.StatusCode)
	}

	return nil
}

// pollHelixChanges checks for app config updates from Helix
func (d *SettingsDaemon) pollHelixChanges() {
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()

	for range ticker.C {
		if err := d.checkHelixUpdates(); err != nil {
			log.Printf("Poll error: %v", err)
		}
	}
}

func (d *SettingsDaemon) checkHelixUpdates() error {
	ctx := context.Background()

	url := fmt.Sprintf("%s/api/v1/sessions/%s/zed-config", d.apiURL, d.sessionID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	if d.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiToken)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch config: status %d", resp.StatusCode)
	}

	var config helixConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return err
	}

	// Mirror GNOME on every poll — same call syncFromHelix makes. Idempotent
	// (gsettings set to the existing value is a no-op) and load-bearing for
	// the case where the WS subscriber missed a config_changed event during
	// a reconnect: without this, GNOME stays on the old theme indefinitely.
	d.applyGNOMEColorScheme(config.ColorScheme)

	// Build new helix settings from defaults + API response
	newHelixSettings := helixDefaults()
	newHelixSettings["context_servers"] = config.ContextServers
	if config.LanguageModels != nil {
		newHelixSettings["language_models"] = config.LanguageModels
	}
	if config.Assistant != nil {
		newHelixSettings["assistant"] = config.Assistant
	}
	if config.Agent != nil {
		newHelixSettings["agent"] = config.Agent
	}
	// Theme is governed by HELIX_MANAGED_THEMES + effectiveTheme: write the
	// API value when on-disk is unset or one of our managed themes; preserve
	// the user's manually-picked theme otherwise.
	if t := d.effectiveTheme(config.Theme); t != "" {
		newHelixSettings["theme"] = t
	}
	injectAgentToolPermissions(newHelixSettings)

	// Update Claude subscription availability and sync credentials
	d.claudeSubscriptionAvailable = config.ClaudeSubscriptionAvailable
	d.codexSubscriptionAvailable = config.CodexSubscriptionAvailable
	d.syncClaudeCredentials()
	d.syncCodexCredentials()

	// Compare against the pre-injection baseline to avoid spurious diffs
	// caused by injectAvailableModels mutations
	codeAgentChanged := !deepEqual(config.CodeAgentConfig, d.codeAgentConfig)
	if !deepEqual(newHelixSettings, d.helixSettingsBaseline) || codeAgentChanged {
		log.Println("Detected Helix config change, updating settings.json")
		d.helixSettingsBaseline = copyMap(newHelixSettings)
		d.helixSettings = newHelixSettings
		d.codeAgentConfig = config.CodeAgentConfig

		// Inject custom models (mutates d.helixSettings)
		d.injectKoditAuth()
		d.injectAvailableModels()

		// Merge with user overrides and write
		merged := d.mergeSettings(d.helixSettings, d.userOverrides)
		if err := d.writeSettings(merged); err != nil {
			return err
		}
	}

	return nil
}

// DEPRECATED_FIELDS should be removed from settings.json (Zed doesn't support them)
var DEPRECATED_FIELDS = []string{"default_agent", "external_sync"}

// writeSettings writes settings.json in place, preserving the file's inode.
//
// We deliberately do NOT write-to-temp + rename here. Zed watches the
// settings.json *inode* via inotify (watch_config_file -> RealFs::watch only
// adds a parent-directory watch for symlinks; a regular file is watched by its
// own inode). An atomic rename replaces that inode on every write, and inotify
// tears the watch down after the first replacement (IN_IGNORED) without
// re-arming it. The daemon is the sole Helix-side writer of this file, so once
// the watch dies Zed never sees another change — which is why a light/dark
// toggle changed the theme on the first click but never again until restart.
//
// Truncating and rewriting the same inode keeps Zed's watch alive across every
// write. Reads stay safe: Zed debounces file events ~100ms before loading, and
// we write the (small) JSON in a single Write + Sync, so a reader never observes
// a partially written file.
func (d *SettingsDaemon) writeSettings(settings map[string]interface{}) error {
	// Ensure directory exists
	dir := filepath.Dir(SettingsPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Remove deprecated fields that Zed doesn't support
	for _, field := range DEPRECATED_FIELDS {
		delete(settings, field)
	}

	// Marshal with indentation
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	// In-place write that preserves the inode (O_TRUNC, not rename) so Zed's
	// inotify watch on settings.json survives across repeated writes.
	f, err := os.OpenFile(SettingsPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	d.lastModified = time.Now()
	log.Println("Updated settings.json")
	return nil
}

// HTTP handlers
func (d *SettingsDaemon) healthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (d *SettingsDaemon) getSettings(w http.ResponseWriter, r *http.Request) {
	merged := d.mergeSettings(d.helixSettings, d.userOverrides)
	json.NewEncoder(w).Encode(merged)
}

func (d *SettingsDaemon) forceReload(w http.ResponseWriter, r *http.Request) {
	if err := d.syncFromHelix(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write([]byte("Reloaded"))
}

func deepEqual(a, b interface{}) bool {
	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)
	return string(aJSON) == string(bJSON)
}
