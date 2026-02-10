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

// TrayStatus holds status info for the system tray tooltip
type TrayStatus struct {
	VMState      string `json:"vm_state"`
	SessionCount int    `json:"session_count"`
	APIReady     bool   `json:"api_ready"`
}

// App struct holds the application state
type App struct {
	ctx              context.Context
	vm               *VMManager
	encoder          *VideoEncoder
	videoServer      *VideoServer
	vtEncoder        *VideoToolboxEncoder // Hardware H.264 encoder
	vsockServer      *VsockServer         // vsock server for guest frame requests
	settings         *SettingsManager
	zfsCollector     *ZFSCollector
	scanoutCollector *ScanoutCollector
	downloader       *VMDownloader
	licenseValidator *LicenseValidator
}

// NewApp creates a new App application struct
func NewApp() *App {
	encoder := NewVideoEncoder(1920, 1080, 60)

	// Create VideoToolbox hardware encoder (5 Mbps bitrate)
	vtEncoder := NewVideoToolboxEncoder(1920, 1080, 60, 5000000)

	settings := NewSettingsManager()

	app := &App{
		vm:               NewVMManager(),
		encoder:          encoder,
		vtEncoder:        vtEncoder,
		settings:         settings,
		zfsCollector:     NewZFSCollector(settings.Get().SSHPort),
		scanoutCollector: NewScanoutCollector(settings.Get().SSHPort),
	}

	// WebSocket server for browser clients (port 8765)
	app.videoServer = NewVideoServer(8765, encoder)

	// vsock server for receiving frame requests from guest (socket path)
	// The socket path is typically /tmp/helix-vsock.sock, configured in UTM
	vsockPath := filepath.Join(os.TempDir(), "helix-vsock.sock")
	app.vsockServer = NewVsockServer(vsockPath, vtEncoder)

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

	// Start the WebSocket server for video streaming
	if err := a.videoServer.Start(); err != nil {
		log.Printf("Failed to start video server: %v", err)
	}

	// Start the encoder
	if err := a.encoder.Start(); err != nil {
		log.Printf("Failed to start encoder: %v", err)
	}

	// Start VideoToolbox encoder with callback that sends frames to vsock guests
	if err := a.vtEncoder.Start(func(nalData []byte, isKeyframe bool, pts int64) {
		// Send encoded frame back to all connected guests via vsock
		response := &FrameResponse{
			PTS:        pts,
			IsKeyframe: isKeyframe,
			NALData:    nalData,
		}
		if err := a.vsockServer.SendEncodedFrame(response); err != nil {
			log.Printf("Failed to send encoded frame: %v", err)
		}
	}); err != nil {
		log.Printf("Failed to start VideoToolbox encoder: %v", err)
	}

	// Start vsock server for frame requests from guest
	if err := a.vsockServer.Start(); err != nil {
		log.Printf("Failed to start vsock server: %v", err)
	}

	// Start ZFS stats collector
	a.zfsCollector.Start()

	// Start scanout stats collector
	a.scanoutCollector.Start()

	log.Println("Helix Desktop started")
}

// shutdown is called when the app is closing
func (a *App) shutdown(ctx context.Context) {
	log.Println("Helix Desktop shutting down...")

	// Stop video components
	a.videoServer.Stop()
	a.encoder.Stop()

	// Stop VideoToolbox encoder
	if a.vtEncoder != nil {
		a.vtEncoder.Stop()
	}

	// Stop vsock server
	if a.vsockServer != nil {
		a.vsockServer.Stop()
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

	if err := a.vm.Start(); err != nil {
		if err == ErrVMImagesNotDownloaded {
			return fmt.Errorf("needs_download: VM images must be downloaded before starting")
		}
		return err
	}

	// Start the video encoder with the VM's video stream port
	// The encoder will be ready to process frames when containers stream
	if err := a.encoder.StartWithVideoPort(a.vm.GetVideoPort()); err != nil {
		log.Printf("Warning: Failed to start encoder: %v", err)
	}

	return nil
}

// StopVM stops the virtual machine
func (a *App) StopVM() error {
	// Stop the encoder first
	a.encoder.Stop()

	return a.vm.Stop()
}

// GetEncoderStats returns video encoder statistics
func (a *App) GetEncoderStats() EncoderStats {
	return a.encoder.GetStats()
}

// GetClientCount returns number of connected video clients
func (a *App) GetClientCount() int {
	return a.videoServer.ClientCount()
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

// IsVMImageReady checks if all required VM images exist
func (a *App) IsVMImageReady() bool {
	vmDir := filepath.Join(getHelixDataDir(), "vm", "helix-desktop")
	rootDisk := filepath.Join(vmDir, "disk.qcow2")
	zfsDisk := filepath.Join(vmDir, "zfs-data.qcow2")

	if _, err := os.Stat(rootDisk); err != nil {
		return false
	}
	if _, err := os.Stat(zfsDisk); err != nil {
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

// GetStreamURL returns the WebSocket URL for video streaming
func (a *App) GetStreamURL(sessionID string) string {
	return fmt.Sprintf("ws://localhost:%d/stream/%s", a.videoServer.port, sessionID)
}

// CheckDependencies checks if required dependencies are installed
func (a *App) CheckDependencies() map[string]bool {
	deps := make(map[string]bool)

	// Check for QEMU
	_, err := exec.LookPath("qemu-system-aarch64")
	if err != nil {
		// Check UTM's bundled QEMU
		_, err = os.Stat("/Applications/UTM.app/Contents/XPCServices/QEMUHelper.xpc/Contents/MacOS/qemu-system-aarch64")
		deps["qemu"] = err == nil
	} else {
		deps["qemu"] = true
	}

	// Check for GStreamer
	_, err = exec.LookPath("gst-launch-1.0")
	deps["gstreamer"] = err == nil

	// Check for vtenc_h264
	if deps["gstreamer"] {
		cmd := exec.Command("gst-inspect-1.0", "vtenc_h264_hw")
		err = cmd.Run()
		deps["videotoolbox"] = err == nil
	} else {
		deps["videotoolbox"] = false
	}

	return deps
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

// GetZFSStats returns the latest ZFS pool statistics
func (a *App) GetZFSStats() ZFSStats {
	return a.zfsCollector.GetStats()
}

// GetDiskUsage returns disk usage breakdown
func (a *App) GetDiskUsage() DiskUsage {
	return a.zfsCollector.GetDiskUsage()
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

// GetScanoutStats returns DRM scanout/display usage statistics
func (a *App) GetScanoutStats() ScanoutStats {
	return a.scanoutCollector.GetStats()
}

// GetTrayStatus returns status info for the system tray
func (a *App) GetTrayStatus() TrayStatus {
	status := a.vm.GetStatus()
	return TrayStatus{
		VMState:      string(status.State),
		SessionCount: status.Sessions,
		APIReady:     status.APIReady,
	}
}

// openBrowser opens a URL in the default browser
func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported platform")
	}

	return cmd.Start()
}
