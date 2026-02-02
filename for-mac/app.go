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
	ctx             context.Context
	vm              *VMManager
	encoder         *VideoEncoder
	videoServer     *VideoServer
	vtEncoder       *VideoToolboxEncoder // Hardware H.264 encoder
	vsockServer     *VsockServer         // vsock server for guest frame requests
}

// NewApp creates a new App application struct
func NewApp() *App {
	encoder := NewVideoEncoder(1920, 1080, 60)

	// Create VideoToolbox hardware encoder (5 Mbps bitrate)
	vtEncoder := NewVideoToolboxEncoder(1920, 1080, 60, 5000000)

	app := &App{
		vm:        NewVMManager(),
		encoder:   encoder,
		vtEncoder: vtEncoder,
	}

	// WebSocket server for browser clients (port 8765)
	app.videoServer = NewVideoServer(8765, encoder)

	// vsock server for receiving frame requests from guest (socket path)
	// The socket path is typically /tmp/helix-vsock.sock, configured in UTM
	vsockPath := filepath.Join(os.TempDir(), "helix-vsock.sock")
	app.vsockServer = NewVsockServer(vsockPath, vtEncoder)

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

// StartVM starts the virtual machine
func (a *App) StartVM() error {
	if err := a.vm.Start(); err != nil {
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

// SetupVM sets up the VM by downloading/creating the base image
func (a *App) SetupVM() error {
	// Create helix directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	helixDir := filepath.Join(homeDir, ".helix", "vm")
	if err := os.MkdirAll(helixDir, 0755); err != nil {
		return fmt.Errorf("failed to create helix directory: %w", err)
	}

	imagePath := filepath.Join(helixDir, "helix-ubuntu.qcow2")

	// Check if image already exists
	if _, err := os.Stat(imagePath); err == nil {
		wailsRuntime.EventsEmit(a.ctx, "setup:progress", map[string]interface{}{
			"step":     "complete",
			"message":  "VM image already exists",
			"progress": 100,
		})
		return nil
	}

	// Emit progress events
	wailsRuntime.EventsEmit(a.ctx, "setup:progress", map[string]interface{}{
		"step":     "downloading",
		"message":  "Downloading Ubuntu ARM64 image...",
		"progress": 10,
	})

	// For now, we'll create a placeholder - in production this would download from get.helix.ml
	// The actual VM setup requires downloading a pre-built image or creating one with cloud-init
	return fmt.Errorf("VM setup not yet implemented. Please create a VM image manually at %s", imagePath)
}

// GetSSHCommand returns the SSH command to connect to the VM
func (a *App) GetSSHCommand() string {
	return a.vm.GetSSHCommand()
}

// IsVMImageReady checks if the VM image exists
func (a *App) IsVMImageReady() bool {
	homeDir, _ := os.UserHomeDir()
	imagePath := filepath.Join(homeDir, ".helix", "vm", "helix-ubuntu.qcow2")
	_, err := os.Stat(imagePath)
	return err == nil
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
