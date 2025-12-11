package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	SettingsPath = "/home/retro/.config/zed/settings.json"
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

	// Current state
	helixSettings map[string]interface{}
	userOverrides map[string]interface{}
}

// CodeAgentConfig mirrors the API response structure for code agent configuration
type CodeAgentConfig struct {
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	AgentName string `json:"agent_name"`
	BaseURL   string `json:"base_url"`
	APIType   string `json:"api_type"`
	Runtime   string `json:"runtime"` // "zed_agent" or "qwen_code"
}

// helixConfigResponse is the response structure from the Helix API's zed-config endpoint
type helixConfigResponse struct {
	ContextServers  map[string]interface{} `json:"context_servers"`
	LanguageModels  map[string]interface{} `json:"language_models"`
	Assistant       map[string]interface{} `json:"assistant"`
	ExternalSync    map[string]interface{} `json:"external_sync"`
	Agent           map[string]interface{} `json:"agent"`
	Theme           string                 `json:"theme"`
	Version         int64                  `json:"version"`
	CodeAgentConfig *CodeAgentConfig       `json:"code_agent_config"`
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
		env := map[string]interface{}{
			"GEMINI_TELEMETRY_ENABLED": "false",
			"OPENAI_BASE_URL":          d.codeAgentConfig.BaseURL,
		}

		if d.userAPIKey != "" {
			env["OPENAI_API_KEY"] = d.userAPIKey
		}
		if d.codeAgentConfig.Model != "" {
			env["OPENAI_MODEL"] = d.codeAgentConfig.Model
		}

		log.Printf("Using qwen_code runtime: base_url=%s, model=%s",
			d.codeAgentConfig.BaseURL, d.codeAgentConfig.Model)

		return map[string]interface{}{
			"qwen": map[string]interface{}{
				"command": "qwen",
				"args": []string{
					"--experimental-acp",
					"--no-telemetry",
					"--include-directories", "/home/retro/work",
				},
				"env": env,
			},
		}

	default: // "zed_agent" or empty (default)
		// Zed Agent: Uses Zed's built-in agent panel - no agent_servers needed
		// The container env vars (ANTHROPIC_API_KEY, OPENAI_API_KEY, etc.) are set by wolf_executor
		log.Printf("Using zed_agent runtime (no agent_servers needed), api_type=%s", d.codeAgentConfig.APIType)
		return nil
	}
}

func main() {
	// Environment variables
	helixURL := os.Getenv("HELIX_API_URL")
	if helixURL == "" {
		helixURL = "http://api:8080"
	}
	helixToken := os.Getenv("HELIX_API_TOKEN")
	sessionID := os.Getenv("HELIX_SESSION_ID")
	port := os.Getenv("SETTINGS_SYNC_PORT")
	if port == "" {
		port = "9877"
	}

	// User's Helix API token (for authenticating with LLM proxies)
	userAPIKey := os.Getenv("USER_API_TOKEN")

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

	// Initial sync from Helix → local
	if err := daemon.syncFromHelix(); err != nil {
		log.Printf("Warning: Initial sync failed: %v", err)
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

	d.helixSettings = map[string]interface{}{
		"context_servers": config.ContextServers,
	}
	if config.LanguageModels != nil {
		d.helixSettings["language_models"] = config.LanguageModels
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
	agentServers := d.generateAgentServerConfig()
	if agentServers != nil {
		merged["agent_servers"] = agentServers
		merged["default_agent"] = "qwen"
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

// startWatcher monitors settings.json for Zed UI changes
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

	go func() {
		var debounceTimer *time.Timer

		for {
			select {
			case event := <-watcher.Events:
				if event.Op&fsnotify.Write == fsnotify.Write {
					// Debounce rapid writes
					if debounceTimer != nil {
						debounceTimer.Stop()
					}
					debounceTimer = time.AfterFunc(DebounceTime, func() {
						d.onFileChanged()
					})
				}
			case err := <-watcher.Errors:
				log.Printf("Watcher error: %v", err)
			}
		}
	}()

	return nil
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

	// Check if Helix settings or code agent config changed
	codeAgentChanged := !deepEqual(config.CodeAgentConfig, d.codeAgentConfig)
	if !deepEqual(newHelixSettings, d.helixSettings) || codeAgentChanged {
		log.Println("Detected Helix config change, updating settings.json")
		d.helixSettings = newHelixSettings
		d.codeAgentConfig = config.CodeAgentConfig

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
