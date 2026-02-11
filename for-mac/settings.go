package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// AppSettings holds the persisted application settings
type AppSettings struct {
	// VM configuration
	VMCPUs     int `json:"vm_cpus"`
	VMMemoryMB int `json:"vm_memory_mb"`

	// Storage
	DataDiskSizeGB int `json:"data_disk_size_gb"` // ZFS data disk size (can only grow)

	// Network ports
	SSHPort int `json:"ssh_port"`
	APIPort int `json:"api_port"`

	// Network access
	ExposeOnNetwork bool `json:"expose_on_network"` // Bind to 0.0.0.0 instead of localhost

	// Display
	AutoStartVM bool `json:"auto_start_vm"`

	// Paths
	VMDiskPath string `json:"vm_disk_path"`

	// License
	LicenseKey     string     `json:"license_key,omitempty"`
	TrialStartedAt *time.Time `json:"trial_started_at,omitempty"`
}

// DefaultSettings returns the default settings with system-aware CPU and memory defaults.
// CPUs default to half the system's logical cores (min 2).
// Memory defaults to half the system's physical RAM (min 4GB).
func DefaultSettings() AppSettings {
	cpus := runtime.NumCPU() / 2
	if cpus < 2 {
		cpus = 2
	}

	memoryMB := 8192 // fallback
	if totalMem := getSystemMemoryMB(); totalMem > 0 {
		memoryMB = totalMem / 2
		if memoryMB < 4096 {
			memoryMB = 4096
		}
	}

	return AppSettings{
		VMCPUs:         cpus,
		VMMemoryMB:     memoryMB,
		DataDiskSizeGB: 256,
		SSHPort:        2222,
		APIPort:        8080,
		AutoStartVM: false,
		VMDiskPath:     filepath.Join(getHelixDataDir(), "vm", "helix-desktop", "disk.qcow2"),
	}
}

// getSystemMemoryMB returns the total physical memory in MB using sysctl on macOS.
func getSystemMemoryMB() int {
	var mem uint64
	size := uint64(8)
	name := [2]int32{6 /* CTL_HW */, 24 /* HW_MEMSIZE */}
	_, _, err := syscall.Syscall6(
		syscall.SYS___SYSCTL,
		uintptr(unsafe.Pointer(&name[0])),
		2,
		uintptr(unsafe.Pointer(&mem)),
		uintptr(unsafe.Pointer(&size)),
		0, 0,
	)
	if err != 0 {
		return 0
	}
	return int(mem / (1024 * 1024))
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
