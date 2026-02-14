package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct holds the application state
type App struct {
	ctx              context.Context
	vm               *VMManager
	settings         *SettingsManager
	zfsCollector     *ZFSCollector
	scanoutCollector *ScanoutCollector
	downloader       *VMDownloader
	licenseValidator *LicenseValidator
	tray             *TrayManager
	authProxy        *AuthProxy
}

// NewApp creates a new App application struct
func NewApp() *App {
	settings := NewSettingsManager()

	app := &App{
		vm:               NewVMManager(),
		settings:         settings,
		zfsCollector:     NewZFSCollector(settings.Get().SSHPort),
		scanoutCollector: NewScanoutCollector(settings.Get().SSHPort),
	}

	// VM image downloader (CDN download on first launch)
	app.downloader = NewVMDownloader()

	// License validator (offline ECDSA verification with embedded Launchpad public key)
	validator, err := NewLicenseValidator()
	if err != nil {
		log.Printf("Warning: failed to initialize license validator: %v", err)
	}
	app.licenseValidator = validator

	return app
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.vm.SetAppContext(ctx)
	s := a.settings.Get()
	a.vm.desktopSecret = s.DesktopSecret
	a.vm.consolePassword = s.ConsolePassword
	a.vm.licenseKey = s.LicenseKey
	a.vm.newUsersAreAdmin = s.NewUsersAreAdmin
	a.vm.allowRegistration = s.AllowRegistration

	// Wire VM state changes to system tray
	a.vm.onStateChange = func(state string) {
		if a.tray != nil {
			a.tray.UpdateState(state)
		}
	}

	// Wire API ready callback to start the auth proxy
	a.vm.onAPIReady = func() {
		a.ensureAuthProxy()
	}

	// Start ZFS stats collector
	a.zfsCollector.Start()

	// Start scanout stats collector
	a.scanoutCollector.Start()

	// Initialize system tray
	a.tray = NewTrayManager(a)
	a.tray.Start()

	log.Println("Helix Desktop started")

	// Auto-start VM if enabled in settings
	if a.settings.Get().AutoStartVM {
		log.Println("Auto-starting VM...")
		go func() {
			if err := a.StartVM(); err != nil {
				log.Printf("Auto-start VM failed: %v", err)
			}
		}()
	}
}

// shutdown is called when the app is closing
func (a *App) shutdown(ctx context.Context) {
	log.Println("Helix Desktop shutting down...")

	// Stop system tray
	if a.tray != nil {
		a.tray.Stop()
	}

	// Stop auth proxy
	if a.authProxy != nil {
		a.authProxy.Stop()
	}

	// Stop ZFS collector
	if a.zfsCollector != nil {
		a.zfsCollector.Stop()
	}

	// Stop scanout collector
	if a.scanoutCollector != nil {
		a.scanoutCollector.Stop()
	}

	// Stop VM if running
	if a.vm.GetStatus().State == VMStateRunning {
		a.vm.Stop()
	}
}

// GetVMStatus returns the current VM status
func (a *App) GetVMStatus() VMStatus {
	return a.vm.GetStatus()
}

// GetVMConfig returns the current VM configuration
func (a *App) GetVMConfig() VMConfig {
	return a.vm.GetConfig()
}

// SetVMConfig updates the VM configuration
func (a *App) SetVMConfig(config VMConfig) error {
	return a.vm.SetConfig(config)
}

// StartVM starts the virtual machine after checking license and download status
func (a *App) StartVM() error {
	// Check license/trial status
	if a.licenseValidator != nil {
		if err := a.licenseValidator.CanStartVM(a.settings.Get()); err != nil {
			return fmt.Errorf("license check failed: %w", err)
		}
	}

	// Refresh VM settings from current state — the user may have changed
	// settings (e.g., added a license key) while the VM was stopped.
	s := a.settings.Get()
	a.vm.desktopSecret = s.DesktopSecret
	a.vm.consolePassword = s.ConsolePassword
	a.vm.licenseKey = s.LicenseKey
	if s.LicenseKey != "" {
		log.Printf("StartVM: license key loaded from settings (length=%d)", len(s.LicenseKey))
	}

	if err := a.vm.Start(); err != nil {
		if err == ErrVMImagesNotDownloaded {
			return fmt.Errorf("needs_download: VM images must be downloaded before starting")
		}
		return err
	}

	return nil
}

// StopVM stops the virtual machine
func (a *App) StopVM() error {
	return a.vm.Stop()
}

// GetConsoleOutput returns the serial console buffer
func (a *App) GetConsoleOutput() string {
	return a.vm.GetConsoleOutput()
}

// SendConsoleInput sends input to the VM serial console
func (a *App) SendConsoleInput(input string) error {
	return a.vm.SendConsoleInput(input)
}

// OpenHelixUI opens the Helix web UI in the default browser
func (a *App) OpenHelixUI() error {
	url := fmt.Sprintf("http://localhost:%d", a.vm.GetConfig().APIPort)
	return openBrowser(url)
}

// OpenSession opens a specific session in the browser
func (a *App) OpenSession(sessionID string) error {
	url := fmt.Sprintf("http://localhost:%d/session/%s", a.vm.GetConfig().APIPort, sessionID)
	return openBrowser(url)
}

// DownloadVMImages downloads VM images from the CDN with progress events
func (a *App) DownloadVMImages() error {
	if a.downloader.IsRunning() {
		return fmt.Errorf("download already in progress")
	}

	// Create a context emitter that wraps the Wails app context
	emitter := &appContextEmitter{ctx: a.ctx}

	go func() {
		if err := a.downloader.DownloadAll(emitter); err != nil {
			log.Printf("VM image download failed: %v", err)
			wailsRuntime.EventsEmit(a.ctx, "download:progress", DownloadProgress{
				Status: "error",
				Error:  err.Error(),
			})
		}
	}()

	return nil
}

// CancelDownload cancels an in-progress VM image download
func (a *App) CancelDownload() {
	a.downloader.Cancel()
}

// GetDownloadStatus returns the current download progress
func (a *App) GetDownloadStatus() DownloadProgress {
	return a.downloader.GetProgress()
}

// appContextEmitter wraps the Wails context to satisfy the emitter interface
type appContextEmitter struct {
	ctx context.Context
}

func (e *appContextEmitter) EventsEmit(eventName string, data ...interface{}) {
	wailsRuntime.EventsEmit(e.ctx, eventName, data...)
}

// GetSSHCommand returns the SSH command to connect to the VM
func (a *App) GetSSHCommand() string {
	return a.vm.GetSSHCommand()
}

// IsVMImageReady checks if the root disk image exists (ZFS disk is created on first boot)
func (a *App) IsVMImageReady() bool {
	vmDir := filepath.Join(getHelixDataDir(), "vm", "helix-desktop")
	rootDisk := filepath.Join(vmDir, "disk.qcow2")

	if _, err := os.Stat(rootDisk); err != nil {
		return false
	}
	return true
}

// GetLicenseStatus returns the current license/trial state
func (a *App) GetLicenseStatus() LicenseStatus {
	if a.licenseValidator == nil {
		return LicenseStatus{State: "licensed"} // No validator = no restrictions
	}
	return a.licenseValidator.GetLicenseStatus(a.settings.Get())
}

// ValidateLicenseKey validates and saves a license key, then injects it into the VM.
func (a *App) ValidateLicenseKey(key string) error {
	if a.licenseValidator == nil {
		return fmt.Errorf("license validator not initialized")
	}

	_, err := a.licenseValidator.ValidateLicenseKey(key)
	if err != nil {
		return err
	}

	// Save the valid license key to settings
	settings := a.settings.Get()
	settings.LicenseKey = key
	if err := a.settings.Save(settings); err != nil {
		return err
	}

	// Update the VM manager and re-inject into the running VM
	a.vm.licenseKey = key
	log.Printf("License key saved to settings (length=%d)", len(key))
	if a.vm.GetStatus().State == VMStateRunning {
		go func() {
			if err := a.vm.injectDesktopSecret(); err != nil {
				log.Printf("Failed to inject license key into VM: %v", err)
			}
		}()
	} else {
		log.Printf("VM not running — license key will be injected on next boot")
	}
	return nil
}

// StartTrial starts the 24-hour free trial
func (a *App) StartTrial() error {
	if a.licenseValidator == nil {
		return fmt.Errorf("license validator not initialized")
	}
	return a.licenseValidator.StartTrial(a.settings)
}

// GetConsolePassword returns the VM console login password.
func (a *App) GetConsolePassword() string {
	return a.settings.Get().ConsolePassword
}

// GetHelixURL returns the URL for the Helix API
func (a *App) GetHelixURL() string {
	return fmt.Sprintf("http://localhost:%d", a.vm.GetConfig().APIPort)
}

// GetAutoLoginURL returns the URL for the authenticated proxy.
// The proxy injects the helix_session cookie server-side, working around
// WKWebView's iframe cookie isolation.
func (a *App) GetAutoLoginURL() string {
	if a.authProxy != nil {
		if u := a.authProxy.GetURL(); u != "" {
			return u
		}
	}
	secret := a.settings.Get().DesktopSecret
	if secret != "" {
		callbackURL := fmt.Sprintf("http://localhost:%d/api/v1/auth/desktop-callback?token=%s",
			a.vm.GetConfig().APIPort, secret)
		return callbackURL
	}
	return fmt.Sprintf("http://localhost:%d", a.vm.GetConfig().APIPort)
}

// ensureAuthProxy starts the auth proxy and authenticates it against the API.
// Called when the API becomes ready (health check passes).
func (a *App) ensureAuthProxy() {
	secret := a.settings.Get().DesktopSecret
	if secret == "" {
		log.Println("[AUTH] No desktop secret configured, skipping auth proxy")
		return
	}

	apiPort := a.vm.GetConfig().APIPort

	// Start the proxy if not already running
	if a.authProxy == nil {
		a.authProxy = NewAuthProxy(apiPort)
		if err := a.authProxy.Start(); err != nil {
			log.Printf("[AUTH] Failed to start auth proxy: %v", err)
			return
		}
		log.Printf("[AUTH] Auth proxy started on %s", a.authProxy.GetURL())
	}

	// Authenticate (or re-authenticate), passing the macOS user's display name
	// and licensee email (if a valid license key is configured)
	userName := getMacOSUserFullName()
	var licenseeEmail string
	if a.licenseValidator != nil {
		licenseeEmail = a.licenseValidator.GetLicenseeEmail(a.settings.Get())
	}
	if err := a.authProxy.Authenticate(secret, userName, licenseeEmail); err != nil {
		log.Printf("[AUTH] Auth proxy authentication failed: %v", err)
	} else {
		log.Printf("[AUTH] Auth proxy authenticated successfully")
	}
}

// GetSettings returns the current application settings
func (a *App) GetSettings() AppSettings {
	return a.settings.Get()
}

// SaveSettings persists the application settings.
// If VM-affecting settings changed and the VM is running, it automatically
// restarts the VM to apply them.
// Server-managed fields (secrets, license, trial) are preserved from the
// existing settings so the frontend doesn't need to track them.
func (a *App) SaveSettings(s AppSettings) error {
	old := a.settings.Get()

	// Preserve server-managed fields that the frontend doesn't send
	s.LicenseKey = old.LicenseKey
	s.TrialStartedAt = old.TrialStartedAt
	s.DesktopSecret = old.DesktopSecret
	s.ConsolePassword = old.ConsolePassword

	if err := a.settings.Save(s); err != nil {
		return err
	}

	// Check if any VM-affecting settings changed (these are QEMU command-line args)
	needsReboot := old.VMCPUs != s.VMCPUs ||
		old.VMMemoryMB != s.VMMemoryMB ||
		old.SSHPort != s.SSHPort ||
		old.APIPort != s.APIPort ||
		old.VMDiskPath != s.VMDiskPath ||
		old.ExposeOnNetwork != s.ExposeOnNetwork

	// Always update VMManager fields used by injectDesktopSecret (runs during boot or hot-apply)
	a.vm.newUsersAreAdmin = s.NewUsersAreAdmin
	a.vm.allowRegistration = s.AllowRegistration

	if needsReboot && a.vm.GetStatus().State == VMStateRunning {
		log.Println("VM-affecting settings changed — restarting VM...")
		go func() {
			if err := a.vm.Stop(); err != nil {
				log.Printf("Failed to stop VM for settings restart: %v", err)
				return
			}
			// Wait for VM to fully stop
			for i := 0; i < 30; i++ {
				if a.vm.GetStatus().State == VMStateStopped {
					break
				}
				time.Sleep(time.Second)
			}
			if a.vm.GetStatus().State != VMStateStopped {
				log.Printf("VM did not stop within 30 seconds for settings restart")
				return
			}
			// Apply new config and start
			config := a.vm.GetConfig()
			config.CPUs = s.VMCPUs
			config.MemoryMB = s.VMMemoryMB
			config.SSHPort = s.SSHPort
			config.APIPort = s.APIPort
			config.DiskPath = s.VMDiskPath
			config.ExposeOnNetwork = s.ExposeOnNetwork
			a.vm.SetConfig(config)
			if err := a.StartVM(); err != nil {
				log.Printf("Failed to restart VM after settings change: %v", err)
			}
		}()
		return nil
	}

	if a.vm.GetStatus().State == VMStateStopped {
		// VM stopped — apply config so next start uses the new values
		config := a.vm.GetConfig()
		config.CPUs = s.VMCPUs
		config.MemoryMB = s.VMMemoryMB
		config.SSHPort = s.SSHPort
		config.APIPort = s.APIPort
		config.DiskPath = s.VMDiskPath
		config.ExposeOnNetwork = s.ExposeOnNetwork
		return a.vm.SetConfig(config)
	}

	// VM running, no reboot needed — hot-apply env var changes (admin, registration, etc.)
	// injectDesktopSecret is idempotent: only restarts API if .env.vm actually changed.
	go func() {
		if err := a.vm.injectDesktopSecret(); err != nil {
			log.Printf("Failed to hot-apply settings: %v", err)
		}
	}()

	return nil
}

// ResizeDataDisk resizes the ZFS data disk to the specified size in GB.
// Only allows growing (not shrinking). Requires VM to be stopped — will
// stop it automatically and restart after resize.
func (a *App) ResizeDataDisk(newSizeGB int) error {
	settings := a.settings.Get()

	if newSizeGB <= settings.DataDiskSizeGB {
		return fmt.Errorf("new size (%d GB) must be larger than current size (%d GB)", newSizeGB, settings.DataDiskSizeGB)
	}

	// Stop VM if running
	vmWasRunning := a.vm.GetStatus().State == VMStateRunning
	if vmWasRunning {
		log.Printf("Stopping VM to resize data disk...")
		if err := a.vm.Stop(); err != nil {
			return fmt.Errorf("failed to stop VM for resize: %w", err)
		}
		// Wait for VM to fully stop
		for i := 0; i < 30; i++ {
			if a.vm.GetStatus().State == VMStateStopped {
				break
			}
			time.Sleep(time.Second)
		}
		if a.vm.GetStatus().State != VMStateStopped {
			return fmt.Errorf("VM did not stop within 30 seconds")
		}
	}

	// Resize the qcow2 file
	zfsDisk := a.vm.getZFSDiskPath()
	sizeStr := fmt.Sprintf("%dG", newSizeGB)
	if err := a.vm.resizeQcow2(zfsDisk, sizeStr); err != nil {
		return fmt.Errorf("failed to resize data disk: %w", err)
	}

	// Update settings
	settings.DataDiskSizeGB = newSizeGB
	if err := a.settings.Save(settings); err != nil {
		return fmt.Errorf("disk resized but failed to save settings: %w", err)
	}

	log.Printf("Data disk resized to %d GB", newSizeGB)

	// Restart VM if it was running
	if vmWasRunning {
		log.Printf("Restarting VM after disk resize...")
		if err := a.StartVM(); err != nil {
			return fmt.Errorf("disk resized but failed to restart VM: %w", err)
		}
	}

	return nil
}

// GetZFSStats returns the latest ZFS pool statistics
func (a *App) GetZFSStats() ZFSStats {
	return a.zfsCollector.GetStats()
}



// DesktopQuota represents active and max concurrent desktop sessions
type DesktopQuota struct {
	Active int `json:"active"`
	Max    int `json:"max"`
}

// GetDesktopQuota returns the current desktop session count and limit
// by calling the Helix API's unauthenticated /config endpoint.
func (a *App) GetDesktopQuota() DesktopQuota {
	if a.vm.GetStatus().State != VMStateRunning {
		return DesktopQuota{}
	}
	port := a.vm.GetConfig().APIPort
	if port == 0 {
		port = 41080
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/config", port))
	if err != nil {
		return DesktopQuota{}
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return DesktopQuota{}
	}
	var config struct {
		ActiveConcurrentDesktops int `json:"active_concurrent_desktops"`
		MaxConcurrentDesktops    int `json:"max_concurrent_desktops"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return DesktopQuota{}
	}
	return DesktopQuota{
		Active: config.ActiveConcurrentDesktops,
		Max:    config.MaxConcurrentDesktops,
	}
}

// GetDiskUsage returns disk usage breakdown
func (a *App) GetDiskUsage() DiskUsage {
	return a.zfsCollector.GetDiskUsage()
}

// GetScanoutStats returns DRM scanout/display usage statistics
func (a *App) GetScanoutStats() ScanoutStats {
	return a.scanoutCollector.GetStats()
}

// GetLANAddress returns the probable LAN IP address of this machine,
// or empty string if none found. Prefers en0 (Wi-Fi/Ethernet on macOS).
func (a *App) GetLANAddress() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	// Prefer en0 (primary NIC on macOS), fall back to first non-loopback IPv4
	var fallback string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil {
				continue
			}
			if iface.Name == "en0" {
				return ipNet.IP.String()
			}
			if fallback == "" {
				fallback = ipNet.IP.String()
			}
		}
	}
	return fallback
}

// GetSystemInfo returns system information
func (a *App) GetSystemInfo() map[string]interface{} {
	return map[string]interface{}{
		"os":       runtime.GOOS,
		"arch":     runtime.GOARCH,
		"cpus":     runtime.NumCPU(),
		"goroot":   runtime.GOROOT(),
		"platform": fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// FactoryReset stops the VM, deletes VM disk images, and quits the app.
// When fullWipe is false, settings (license key, trial, secrets) are preserved.
// When fullWipe is true (shift-click), everything including settings is deleted.
// On next launch it will re-download and provision from scratch.
func (a *App) FactoryReset(fullWipe bool) error {
	// Stop VM if running
	if a.vm.GetStatus().State == VMStateRunning || a.vm.GetStatus().State == VMStateStarting {
		log.Println("Factory reset: stopping VM...")
		if err := a.vm.Stop(); err != nil {
			log.Printf("Factory reset: failed to stop VM (continuing): %v", err)
		}
		// Wait for VM to stop
		for i := 0; i < 30; i++ {
			if a.vm.GetStatus().State == VMStateStopped {
				break
			}
			time.Sleep(time.Second)
		}
	}

	// Delete VM directory (disk images, EFI vars, cloud-init seed, etc.)
	vmDir := filepath.Join(getHelixDataDir(), "vm")
	if err := os.RemoveAll(vmDir); err != nil {
		return fmt.Errorf("failed to delete VM data: %w", err)
	}
	log.Printf("Factory reset: deleted %s", vmDir)

	// Only delete settings on full wipe (shift-click)
	if fullWipe {
		settingsPath := filepath.Join(getHelixDataDir(), "settings.json")
		if err := os.Remove(settingsPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete settings: %w", err)
		}
		log.Println("Factory reset: deleted settings.json (full wipe)")
	} else {
		log.Println("Factory reset: preserving settings.json")
	}

	log.Println("Factory reset complete — quitting app")

	// Quit the app so it starts fresh
	go func() {
		time.Sleep(500 * time.Millisecond)
		wailsRuntime.Quit(a.ctx)
	}()

	return nil
}

// getMacOSUserFullName returns the macOS user's display name (e.g., "Luke Marsden").
// Falls back to the system username if the full name is not available.
func getMacOSUserFullName() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	if u.Name != "" {
		return u.Name
	}
	return u.Username
}

// openBrowser opens a URL in the default browser
func openBrowser(url string) error {
	cmd := exec.Command("open", url)
	return cmd.Start()
}
