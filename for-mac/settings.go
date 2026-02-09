package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// AppSettings holds the persisted application settings
type AppSettings struct {
	// VM configuration
	VMCPUs     int `json:"vm_cpus"`
	VMMemoryMB int `json:"vm_memory_mb"`

	// Network ports
	SSHPort   int `json:"ssh_port"`
	APIPort   int `json:"api_port"`
	VideoPort int `json:"video_port"`

	// Display
	AutoStartVM bool `json:"auto_start_vm"`

	// Paths
	VMDiskPath string `json:"vm_disk_path"`
}

// DefaultSettings returns the default settings
func DefaultSettings() AppSettings {
	return AppSettings{
		VMCPUs:     4,
		VMMemoryMB: 8192,
		SSHPort:    2222,
		APIPort:    8080,
		VideoPort:  8765,
		AutoStartVM: false,
		VMDiskPath: filepath.Join(getHelixDataDir(), "vm", "helix-desktop", "disk.qcow2"),
	}
}

// SettingsManager handles loading and saving settings
type SettingsManager struct {
	mu       sync.RWMutex
	settings AppSettings
	path     string
}

// NewSettingsManager creates a new settings manager
func NewSettingsManager() *SettingsManager {
	settingsPath := filepath.Join(getHelixDataDir(), "settings.json")

	sm := &SettingsManager{
		settings: DefaultSettings(),
		path:     settingsPath,
	}

	// Load existing settings if available
	if err := sm.load(); err != nil {
		// Not an error if file doesn't exist yet
		if !os.IsNotExist(err) {
			fmt.Printf("Warning: failed to load settings: %v\n", err)
		}
	}

	return sm
}

// Get returns the current settings
func (sm *SettingsManager) Get() AppSettings {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.settings
}

// Save persists the settings to disk
func (sm *SettingsManager) Save(s AppSettings) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.settings = s

	// Ensure directory exists
	dir := filepath.Dir(sm.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create settings directory: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := os.WriteFile(sm.path, data, 0644); err != nil {
		return fmt.Errorf("failed to write settings: %w", err)
	}

	return nil
}

// load reads settings from disk
func (sm *SettingsManager) load() error {
	data, err := os.ReadFile(sm.path)
	if err != nil {
		return err
	}

	var s AppSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("failed to parse settings: %w", err)
	}

	sm.settings = s
	return nil
}
