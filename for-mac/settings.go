package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
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

	// Auth — when exposed on network, these control access
	RequireAuthOnNetwork bool `json:"require_auth_on_network"` // Require login when accessed from network (default true)
	NewUsersAreAdmin     bool `json:"new_users_are_admin"`     // New users get admin role automatically
	AllowRegistration    bool `json:"allow_registration"`      // Allow new users to register via the web UI

	// Display
	AutoStartVM bool `json:"auto_start_vm"`

	// Paths
	VMDiskPath string `json:"vm_disk_path"`

	// License
	LicenseKey     string     `json:"license_key,omitempty"`
	TrialStartedAt *time.Time `json:"trial_started_at,omitempty"`

	// Desktop auto-login shared secret (generated on first launch)
	DesktopSecret string `json:"desktop_secret,omitempty"`

	// VM console login password (generated on first launch, injected into VM on boot)
	ConsolePassword string `json:"console_password,omitempty"`

	// Installed VM image version (set after successful download/update)
	InstalledVMVersion string `json:"installed_vm_version,omitempty"`

	// Secure tokens and passwords (generated on first launch, injected into VM .env)
	RunnerToken      string `json:"runner_token,omitempty"`
	PostgresPassword string `json:"postgres_password,omitempty"`
	EncryptionKey    string `json:"encryption_key,omitempty"`
	JWTSecret        string `json:"jwt_secret,omitempty"`
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
		if memoryMB < 6144 {
			memoryMB = 6144
		}
	}

	return AppSettings{
		VMCPUs:               cpus,
		VMMemoryMB:           memoryMB,
		DataDiskSizeGB:       256,
		SSHPort:              41222,
		APIPort:              41080,
		AutoStartVM:          true,
		RequireAuthOnNetwork: true,
		NewUsersAreAdmin:     false,
		AllowRegistration:    true,
		VMDiskPath:           filepath.Join(getHelixDataDir(), "vm", "helix-desktop", "disk.qcow2"),
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

	// Ensure desktop secret exists (generate on first launch)
	needsSave := false
	if sm.settings.DesktopSecret == "" {
		sm.settings.DesktopSecret = generateSecret()
		needsSave = true
	}

	// Ensure console password exists (generate on first launch)
	if sm.settings.ConsolePassword == "" {
		sm.settings.ConsolePassword = generatePassword()
		needsSave = true
	}

	// Ensure secure tokens/passwords exist (generate on first launch)
	if sm.settings.RunnerToken == "" {
		sm.settings.RunnerToken = generateSecret()
		needsSave = true
	}
	if sm.settings.PostgresPassword == "" {
		sm.settings.PostgresPassword = generatePassword()
		needsSave = true
	}
	if sm.settings.EncryptionKey == "" {
		sm.settings.EncryptionKey = generateSecret()
		needsSave = true
	}
	if sm.settings.JWTSecret == "" {
		sm.settings.JWTSecret = generateSecret()
		needsSave = true
	}

	if needsSave {
		_ = sm.Save(sm.settings)
	}

	return sm
}

// generateSecret creates a cryptographically random hex string for desktop auto-login.
func generateSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("crypto/rand failed: %v", err)
	}
	return hex.EncodeToString(b)
}

// generatePassword creates a random alphanumeric password for the VM console.
// Uses unambiguous characters (no 0/O, 1/l/I) for readability.
func generatePassword() string {
	const chars = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789"
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("crypto/rand failed: %v", err)
	}
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b)
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

	// Start from defaults so missing JSON fields preserve their default values.
	// This is important for boolean fields that default to true (e.g. RequireAuthOnNetwork).
	s := DefaultSettings()
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("failed to parse settings: %w", err)
	}

	sm.settings = s
	return nil
}
