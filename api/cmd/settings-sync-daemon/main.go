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
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	SettingsPath = "/home/retro/.config/zed/settings.json"
	KeymapPath   = "/home/retro/.config/zed/keymap.json"
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

	// Track the last expiresAt we know about, so we can detect Claude Code token refreshes
	lastKnownExpiresAt int64

	// Timestamp of our last write to the credentials file (to ignore our own fsnotify events)
	lastCredWrite time.Time

	// Current state
	helixSettings map[string]interface{}
	userOverrides map[string]interface{}
}

// CodeAgentConfig mirrors the API response structure for code agent configuration
type CodeAgentConfig struct {
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	AgentName       string `json:"agent_name"`
	BaseURL         string `json:"base_url"`
	APIType         string `json:"api_type"`
	Runtime         string `json:"runtime"`           // "zed_agent" or "qwen_code"
	MaxTokens       int    `json:"max_tokens"`        // Model's context window size (0 if unknown)
	MaxOutputTokens int    `json:"max_output_tokens"` // Model's max completion tokens (0 if unknown)
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
	Version                     int64                  `json:"version"`
	CodeAgentConfig             *CodeAgentConfig       `json:"code_agent_config"`
	ClaudeSubscriptionAvailable bool                   `json:"claude_subscription_available,omitempty"`
}

// generateAgentServerConfig creates the agent_servers configuration for custom agents (like qwen).
// Returns nil for runtimes that use Zed's built-in agent.
//
// There are two code agent runtimes:
// 1. zed_agent - Zed's built-in agent panel. No agent_servers needed. Zed reads
//    env vars (ANTHROPIC_API_KEY, OPENAI_API_KEY, etc.) from the container environment.
// 2. qwen_code - Qwen code agent as a custom agent_server. Requires agent_servers
//    with qwen command and env vars (OPENAI_BASE_URL, OPENAI_API_KEY, OPENAI_MODEL).
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
				"name":    "qwen", // Required: Zed expects a name field for agent_servers
				"type":    "custom", // Required: Zed deserializes agent_servers using tagged enum
				"command": "qwen",
				"args": []string{
					"--experimental-acp",
					"--no-telemetry",
					"--include-directories", "/home/retro/work",
				},
				"env": env,
			},
		}

	case "claude_code":
		// Claude Code: Uses Zed's built-in Claude Code ACP (@zed-industries/claude-code-acp).
		// We only set env vars — Zed handles installing and launching the ACP wrapper.
		// Two modes based on whether baseURL is set:
		// 1. API key mode (baseURL set): Claude Code uses Helix API proxy
		// 2. Subscription mode (no baseURL): Claude Code uses OAuth credentials
		env := map[string]interface{}{
			"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
			"DISABLE_TELEMETRY":                        "1",
		}

		if d.codeAgentConfig.BaseURL != "" {
			// API key mode: route through Helix API proxy
			baseURL := d.rewriteLocalhostURL(d.codeAgentConfig.BaseURL)
			env["ANTHROPIC_BASE_URL"] = baseURL
			if d.userAPIKey != "" {
				env["ANTHROPIC_API_KEY"] = d.userAPIKey
			}
			log.Printf("Using claude_code runtime (API key mode): base_url=%s", baseURL)
		} else {
			// Subscription mode: Claude Code reads OAuth credentials
			// (including refresh token) from ~/.claude/.credentials.json.
			// Gate on the file existing — Claude Code's credential reader is
			// memoized, so if it reads before the file is written, it caches
			// null permanently. By returning nil here, we omit agent_servers
			// from Zed settings so Claude Code won't start. On the next poll
			// cycle, syncClaudeCredentials() will have written the file and
			// this check will pass.
			if _, err := os.Stat(ClaudeCredentialsPath); err != nil {
				log.Printf("Claude credentials file not yet available, deferring claude_code agent_servers: %v", err)
				return nil
			}
			// Write marker so start-zed-core.sh knows to wait for credentials
			// before launching Zed (belt-and-suspenders with the os.Stat gate above).
			_ = os.WriteFile(ClaudeSubscriptionMarkerPath, []byte("1"), 0644)
			// IMPORTANT: Hydra sets ANTHROPIC_BASE_URL on ALL containers, which
			// leaks into Claude Code's process via env inheritance. We must
			// explicitly override it to the real Anthropic API so Claude Code
			// talks directly to Anthropic with OAuth credentials.
			env["ANTHROPIC_BASE_URL"] = "https://api.anthropic.com"
			log.Printf("Using claude_code runtime (subscription mode)")
		}

		// Only set env — no command/args. Zed uses its built-in
		// @zed-industries/claude-code-acp npm package which speaks ACP.
		// The raw `claude` CLI does NOT support --experimental-acp.
		return map[string]interface{}{
			"claude": map[string]interface{}{
				"default_mode": "bypassPermissions",
				"env":          env,
			},
		}

	default: // "zed_agent" or empty (default)
		// Zed Agent: Uses Zed's built-in agent panel - no agent_servers needed
		// The container env vars (ANTHROPIC_API_KEY, OPENAI_API_KEY, etc.) are set by wolf_executor
		log.Printf("Using zed_agent runtime (no agent_servers needed), api_type=%s", d.codeAgentConfig.APIType)
		return nil
	}
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
		log.Printf("Warning: failed to parse apiURL %s: %v", d.apiURL, err)
		return originalURL
	}

	// Parse the original URL
	origParsed, err := url.Parse(originalURL)
	if err != nil {
		log.Printf("Warning: failed to parse original URL %s: %v", originalURL, err)
		return originalURL
	}

	// Replace the host with our working API host
	origParsed.Host = apiParsed.Host

	rewritten := origParsed.String()
	log.Printf("Rewrote localhost URL for container networking: %s -> %s", originalURL, rewritten)
	return rewritten
}

// rewriteLocalhostURLsInExternalSync rewrites any localhost URLs in the external_sync config
func (d *SettingsDaemon) rewriteLocalhostURLsInExternalSync(externalSync map[string]interface{}) {
	if wsSync, ok := externalSync["websocket_sync"].(map[string]interface{}); ok {
		if extURL, ok := wsSync["external_url"].(string); ok {
			wsSync["external_url"] = d.rewriteLocalhostURL(extURL)
		}
	}
}

// injectLanguageModelAPIKey adds the API token to language_models config.
// Zed reads api_key from settings.json to authenticate LLM API calls.
// The token comes from HELIX_API_TOKEN env var (set by Hydra when starting the desktop).
func (d *SettingsDaemon) injectLanguageModelAPIKey() {
	if d.apiToken == "" {
		log.Printf("Warning: HELIX_API_TOKEN not set, language models may not authenticate")
		return
	}

	languageModels, ok := d.helixSettings["language_models"].(map[string]interface{})
	if !ok {
		return
	}

	// Inject api_key into each provider's config
	for provider, config := range languageModels {
		if providerConfig, ok := config.(map[string]interface{}); ok {
			providerConfig["api_key"] = d.apiToken
			log.Printf("Injected api_key into language_models.%s", provider)
		}
	}
}

// injectAvailableModels adds the configured model to the provider's available_models list.
// Zed only recognizes models that are either built-in (gpt-4, claude-3, etc.) or listed
// in available_models. Without this, custom models like "helix/qwen3:8b" are rejected.
func (d *SettingsDaemon) injectAvailableModels() {
	if d.codeAgentConfig == nil || d.codeAgentConfig.Model == "" {
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
		maxTokens = 128000 // Default context window if not found in model_info
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
	ClaudeCredentialsPath          = "/home/retro/.claude/.credentials.json"
	ClaudeSubscriptionMarkerPath   = "/tmp/helix-claude-subscription-mode"
)

// syncClaudeCredentials fetches Claude OAuth credentials from the Helix API
// and writes them to ~/.claude/.credentials.json for Claude Code to use.
// Claude Code handles its own token refresh natively — if the on-disk file
// has a newer expiresAt than the API response, we skip the write to avoid
// clobbering a token that Claude Code just refreshed.
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

	// Parse the credentials from the API response
	var creds struct {
		AccessToken      string   `json:"accessToken"`
		RefreshToken     string   `json:"refreshToken"`
		ExpiresAt        int64    `json:"expiresAt"`
		Scopes           []string `json:"scopes"`
		SubscriptionType string   `json:"subscriptionType"`
		RateLimitTier    string   `json:"rateLimitTier"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&creds); err != nil {
		log.Printf("Failed to parse Claude credentials: %v", err)
		return
	}

	// Before writing, check if the on-disk file has a newer token (Claude Code refreshed it).
	// If so, skip the write and push the refreshed token back to the API instead.
	// We push directly here rather than relying on the fsnotify watcher because:
	// 1. The watcher may not be running (subscription became available after startup)
	// 2. The watcher's inode may be stale (workspace setup recreates ~/.claude as a symlink)
	if fileExpiresAt := readCredentialsExpiresAt(ClaudeCredentialsPath); fileExpiresAt > creds.ExpiresAt {
		log.Printf("On-disk credentials are newer (file expiresAt=%d > api expiresAt=%d), pushing to API", fileExpiresAt, creds.ExpiresAt)
		d.pushCredentialsToAPI()
		return
	}

	// Track what the API thinks the current expiresAt is
	d.lastKnownExpiresAt = creds.ExpiresAt

	// Build the credentials file in Claude's expected format
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

	// Ensure directory exists
	credDir := filepath.Dir(ClaudeCredentialsPath)
	if err := os.MkdirAll(credDir, 0700); err != nil {
		log.Printf("Failed to create Claude credentials directory: %v", err)
		return
	}

	// Atomic write
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

	// Start polling loop for Helix changes
	go daemon.pollHelixChanges()

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

	// Sync Claude credentials if available
	d.syncClaudeCredentials()

	d.helixSettings = map[string]interface{}{
		"context_servers": config.ContextServers,
	}

	// Inject API keys and custom models before writing settings
	d.injectKoditAuth()
	if config.LanguageModels != nil {
		d.helixSettings["language_models"] = config.LanguageModels
		d.injectLanguageModelAPIKey()
		d.injectAvailableModels() // Add custom model to available_models so Zed recognizes it
	}
	if config.Assistant != nil {
		d.helixSettings["assistant"] = config.Assistant
	}
	// Note: external_sync is NOT written to settings.json because:
	// 1. Zed's settings schema doesn't include it (causes "Property external_sync is not allowed" warning)
	// 2. Zed reads external_sync config from environment variables instead (ZED_EXTERNAL_SYNC_ENABLED, ZED_HELIX_URL, etc.)
	// if config.ExternalSync != nil {
	// 	d.helixSettings["external_sync"] = config.ExternalSync
	// }
	if config.Agent != nil {
		d.helixSettings["agent"] = config.Agent
	}

	// Always auto-approve tool actions — our fork of Zed respects this for all
	// agents including Claude Code. This is the Zed-level safety net that
	// auto-approves permission prompts the ACP sends to Zed.
	agentSection, ok := d.helixSettings["agent"].(map[string]interface{})
	if !ok {
		agentSection = map[string]interface{}{}
	}
	agentSection["always_allow_tool_actions"] = true
	d.helixSettings["agent"] = agentSection
	if config.Theme != "" {
		d.helixSettings["theme"] = config.Theme
	}

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

	return d.writeSettings(d.helixSettings)
}

// SECURITY_PROTECTED_FIELDS must not be synced to the Helix API
// Also includes deprecated fields that should be removed from settings
var SECURITY_PROTECTED_FIELDS = map[string]bool{
	"telemetry":     true,
	"agent_servers": true,
	"default_agent": true,  // Deprecated: Zed doesn't have this setting; remove from old configs
	"external_sync": true,  // Deprecated: Zed reads this from env vars, not settings.json
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

	// Deep merge context_servers
	if userServers, ok := user["context_servers"].(map[string]interface{}); ok {
		if helixServers, ok := merged["context_servers"].(map[string]interface{}); ok {
			for name, config := range userServers {
				helixServers[name] = config
			}
		} else {
			merged["context_servers"] = userServers
		}
	}

	for k, v := range user {
		if k != "context_servers" {
			merged[k] = v
		}
	}

	// Preserve telemetry from on-disk config
	if existingData, err := os.ReadFile(SettingsPath); err == nil {
		var existing map[string]interface{}
		if err := json.Unmarshal(existingData, &existing); err == nil {
			if value, exists := existing["telemetry"]; exists {
				merged["telemetry"] = value
			}
		}
	}

	// Inject code agent configuration (if using qwen custom agent)
	// For Anthropic/Azure, Zed's built-in agent is used (no agent_servers needed)
	// Note: We don't set "default_agent" because Zed doesn't have that setting (deprecated).
	// Thread_service.rs dynamically selects the agent based on agent_name from Helix.
	agentServers := d.generateAgentServerConfig()
	if agentServers != nil {
		merged["agent_servers"] = agentServers
	}

	return merged
}

// extractUserOverrides finds settings that differ from Helix base
func extractUserOverrides(current, helix map[string]interface{}) map[string]interface{} {
	overrides := make(map[string]interface{})

	if currentServers, ok := current["context_servers"].(map[string]interface{}); ok {
		helixServers, _ := helix["context_servers"].(map[string]interface{})
		userServers := make(map[string]interface{})

		for name, config := range currentServers {
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

	for k, v := range current {
		if k == "context_servers" || SECURITY_PROTECTED_FIELDS[k] {
			continue
		}
		if helixVal, inHelix := helix[k]; !inHelix || !deepEqual(v, helixVal) {
			overrides[k] = v
		}
	}

	return overrides
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

	// Watch the settings file
	if err := watcher.Add(SettingsPath); err != nil {
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

	go func() {
		var settingsDebounce *time.Timer
		var credsDebounce *time.Timer
		credFilename := filepath.Base(ClaudeCredentialsPath)

		for {
			select {
			case event := <-watcher.Events:
				if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
					if filepath.Base(event.Name) == credFilename {
						// Claude credentials file changed
						if credsDebounce != nil {
							credsDebounce.Stop()
						}
						credsDebounce = time.AfterFunc(DebounceTime, func() {
							d.onCredentialsChanged()
						})
					} else if event.Name == SettingsPath {
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

	newHelixSettings := map[string]interface{}{
		"context_servers": config.ContextServers,
	}
	if config.LanguageModels != nil {
		newHelixSettings["language_models"] = config.LanguageModels
	}
	if config.Assistant != nil {
		newHelixSettings["assistant"] = config.Assistant
	}
	// Note: external_sync is NOT written - Zed reads it from environment variables
	if config.Agent != nil {
		newHelixSettings["agent"] = config.Agent
	}
	if config.Theme != "" {
		newHelixSettings["theme"] = config.Theme
	}

	// Update Claude subscription availability and sync credentials
	d.claudeSubscriptionAvailable = config.ClaudeSubscriptionAvailable
	d.syncClaudeCredentials()

	// Check if Helix settings or code agent config changed
	codeAgentChanged := !deepEqual(config.CodeAgentConfig, d.codeAgentConfig)
	if !deepEqual(newHelixSettings, d.helixSettings) || codeAgentChanged {
		log.Println("Detected Helix config change, updating settings.json")
		d.helixSettings = newHelixSettings
		d.codeAgentConfig = config.CodeAgentConfig

		// Inject API keys and custom models
		d.injectKoditAuth()
		d.injectLanguageModelAPIKey()
		d.injectAvailableModels() // Add custom model to available_models so Zed recognizes it

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

// writeSettings atomically writes settings.json
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

	// Atomic write (write to temp file, then rename)
	tmpFile := SettingsPath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}

	if err := os.Rename(tmpFile, SettingsPath); err != nil {
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

