package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

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

	// Start ZFS stats collector
	a.zfsCollector.Start()

	// Start scanout stats collector
	a.scanoutCollector.Start()

	log.Println("Helix Desktop started")
}

// shutdown is called when the app is closing
func (a *App) shutdown(ctx context.Context) {
	log.Println("Helix Desktop shutting down...")

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

// ValidateLicenseKey validates and saves a license key
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
	return a.settings.Save(settings)
}

// StartTrial starts the 24-hour free trial
func (a *App) StartTrial() error {
	if a.licenseValidator == nil {
		return fmt.Errorf("license validator not initialized")
	}
	return a.licenseValidator.StartTrial(a.settings)
}

// GetHelixURL returns the URL for the Helix API
func (a *App) GetHelixURL() string {
	return fmt.Sprintf("http://localhost:%d", a.vm.GetConfig().APIPort)
}

// GetSettings returns the current application settings
func (a *App) GetSettings() AppSettings {
	return a.settings.Get()
}

// SaveSettings persists the application settings
func (a *App) SaveSettings(s AppSettings) error {
	if err := a.settings.Save(s); err != nil {
		return err
	}

	// Apply settings to VM config
	config := a.vm.GetConfig()
	config.CPUs = s.VMCPUs
	config.MemoryMB = s.VMMemoryMB
	config.SSHPort = s.SSHPort
	config.APIPort = s.APIPort
	config.VideoPort = s.VideoPort
	config.DiskPath = s.VMDiskPath
	return a.vm.SetConfig(config)
}

// GetZFSStats returns the latest ZFS pool statistics
func (a *App) GetZFSStats() ZFSStats {
	return a.zfsCollector.GetStats()
}

// GetDiskUsage returns disk usage breakdown
func (a *App) GetDiskUsage() DiskUsage {
	return a.zfsCollector.GetDiskUsage()
}

// GetScanoutStats returns DRM scanout/display usage statistics
func (a *App) GetScanoutStats() ScanoutStats {
	return a.scanoutCollector.GetStats()
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

// openBrowser opens a URL in the default browser
func openBrowser(url string) error {
	cmd := exec.Command("open", url)
	return cmd.Start()
}
