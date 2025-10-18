package main

import (
	"bytes"
	"context"
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

	// Current state
	helixSettings map[string]interface{}
	userOverrides map[string]interface{}
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

	if sessionID == "" {
		log.Fatal("HELIX_SESSION_ID environment variable is required")
	}

	log.Printf("Starting settings sync daemon for session %s", sessionID)
	log.Printf("Helix API URL: %s", helixURL)
	log.Printf("Settings path: %s", SettingsPath)

	daemon := &SettingsDaemon{
		httpClient: http.DefaultClient,
		apiURL:     helixURL,
		apiToken:   helixToken,
		sessionID:  sessionID,
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

	var config struct {
		ContextServers map[string]interface{} `json:"context_servers"`
		LanguageModels map[string]interface{} `json:"language_models"`
		Assistant      map[string]interface{} `json:"assistant"`
		ExternalSync   map[string]interface{} `json:"external_sync"`
		Agent          map[string]interface{} `json:"agent"`
		Theme          string                 `json:"theme"`
		Version        int64                  `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return fmt.Errorf("failed to parse Helix config: %w", err)
	}

	d.helixSettings = map[string]interface{}{
		"context_servers": config.ContextServers,
	}
	if config.LanguageModels != nil {
		d.helixSettings["language_models"] = config.LanguageModels
	}
	if config.Assistant != nil {
		d.helixSettings["assistant"] = config.Assistant
	}
	if config.ExternalSync != nil {
		d.helixSettings["external_sync"] = config.ExternalSync
	}
	if config.Agent != nil {
		d.helixSettings["agent"] = config.Agent
	}
	if config.Theme != "" {
		d.helixSettings["theme"] = config.Theme
	}

	// Don't load existing settings as user overrides on startup
	// This prevents old/stale settings from being treated as user preferences
	// User overrides are only tracked from changes made AFTER Helix config is written
	d.userOverrides = make(map[string]interface{})

	// Write Helix settings directly (no merge on initial sync)
	return d.writeSettings(d.helixSettings)
}

// mergeSettings applies three-way merge: Helix base + User overrides
func mergeSettings(helix, user map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{})

	// Copy Helix settings as base
	for k, v := range helix {
		merged[k] = v
	}

	// Apply user overrides (deep merge for context_servers)
	if userServers, ok := user["context_servers"].(map[string]interface{}); ok {
		if helixServers, ok := merged["context_servers"].(map[string]interface{}); ok {
			// Deep merge context_servers
			for name, config := range userServers {
				helixServers[name] = config // User override/addition wins
			}
		} else {
			merged["context_servers"] = userServers
		}
	}

	// Apply other user settings (non-context_servers)
	for k, v := range user {
		if k != "context_servers" {
			merged[k] = v
		}
	}

	return merged
}

// extractUserOverrides finds settings that differ from Helix base
func extractUserOverrides(current, helix map[string]interface{}) map[string]interface{} {
	overrides := make(map[string]interface{})

	// Extract user-added context_servers (not in helix namespace)
	if currentServers, ok := current["context_servers"].(map[string]interface{}); ok {
		helixServers, _ := helix["context_servers"].(map[string]interface{})
		userServers := make(map[string]interface{})

		for name, config := range currentServers {
			// If not in Helix config, or user modified it
			if helixConfig, inHelix := helixServers[name]; !inHelix {
				userServers[name] = config // User addition
			} else if !deepEqual(config, helixConfig) {
				userServers[name] = config // User modification
			}
		}

		if len(userServers) > 0 {
			overrides["context_servers"] = userServers
		}
	}

	// Extract other user settings (theme, vim_mode, etc.)
	for k, v := range current {
		if k == "context_servers" {
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

	var config struct {
		ContextServers map[string]interface{} `json:"context_servers"`
		LanguageModels map[string]interface{} `json:"language_models"`
		Assistant      map[string]interface{} `json:"assistant"`
		ExternalSync   map[string]interface{} `json:"external_sync"`
		Agent          map[string]interface{} `json:"agent"`
		Theme          string                 `json:"theme"`
		Version        int64                  `json:"version"`
	}
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
	if config.ExternalSync != nil {
		newHelixSettings["external_sync"] = config.ExternalSync
	}
	if config.Agent != nil {
		newHelixSettings["agent"] = config.Agent
	}
	if config.Theme != "" {
		newHelixSettings["theme"] = config.Theme
	}

	// Check if Helix settings changed
	if !deepEqual(newHelixSettings, d.helixSettings) {
		log.Println("Detected Helix config change, updating settings.json")
		d.helixSettings = newHelixSettings

		// Merge with user overrides and write
		merged := mergeSettings(d.helixSettings, d.userOverrides)
		if err := d.writeSettings(merged); err != nil {
			return err
		}
	}

	return nil
}

// writeSettings atomically writes settings.json
func (d *SettingsDaemon) writeSettings(settings map[string]interface{}) error {
	// Ensure directory exists
	dir := filepath.Dir(SettingsPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
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
	merged := mergeSettings(d.helixSettings, d.userOverrides)
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
