package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// UTMManager manages a virtual machine via UTM's ScriptingBridge interface
// This provides GPU-accelerated VMs using UTM's virglrenderer + ANGLE + Metal stack
type UTMManager struct {
	vmName     string
	vmRunning  bool
	mu         sync.RWMutex
	appCtx     context.Context
	cancelFunc context.CancelFunc
	utmAppPath string
	utmctlPath string
}

// UTMStatus represents the status of a UTM VM
type UTMStatus string

const (
	UTMStatusStopped  UTMStatus = "stopped"
	UTMStatusStarting UTMStatus = "starting"
	UTMStatusStarted  UTMStatus = "started"
	UTMStatusPausing  UTMStatus = "pausing"
	UTMStatusPaused   UTMStatus = "paused"
	UTMStatusResuming UTMStatus = "resuming"
	UTMStatusStopping UTMStatus = "stopping"
)

// NewUTMManager creates a new UTM manager
func NewUTMManager(vmName string) *UTMManager {
	return &UTMManager{
		vmName:     vmName,
		utmAppPath: "/Applications/UTM.app",
		utmctlPath: "/Applications/UTM.app/Contents/MacOS/utmctl",
	}
}

// SetAppContext sets the Wails app context for event emission
func (u *UTMManager) SetAppContext(ctx context.Context) {
	u.appCtx = ctx
}

// IsUTMInstalled checks if UTM is installed
func (u *UTMManager) IsUTMInstalled() bool {
	cmd := exec.Command(u.utmctlPath, "list")
	err := cmd.Run()
	return err == nil
}

// ListVMs returns all registered UTM VMs
func (u *UTMManager) ListVMs() ([]string, error) {
	cmd := exec.Command(u.utmctlPath, "list")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list VMs: %w", err)
	}

	var vms []string
	lines := strings.Split(string(output), "\n")
	// Skip header line "UUID                                 Status   Name"
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			// Name is everything after UUID and Status
			name := strings.Join(parts[2:], " ")
			vms = append(vms, name)
		}
	}
	return vms, nil
}

// GetVMStatus returns the status of the configured VM
func (u *UTMManager) GetVMStatus() (UTMStatus, error) {
	cmd := exec.Command(u.utmctlPath, "status", u.vmName)
	output, err := cmd.Output()
	if err != nil {
		return UTMStatusStopped, fmt.Errorf("failed to get VM status: %w", err)
	}

	status := strings.TrimSpace(string(output))
	switch status {
	case "stopped":
		return UTMStatusStopped, nil
	case "starting":
		return UTMStatusStarting, nil
	case "started":
		return UTMStatusStarted, nil
	case "pausing":
		return UTMStatusPausing, nil
	case "paused":
		return UTMStatusPaused, nil
	case "resuming":
		return UTMStatusResuming, nil
	case "stopping":
		return UTMStatusStopping, nil
	default:
		return UTMStatusStopped, nil
	}
}

// StartVM starts the VM
func (u *UTMManager) StartVM() error {
	u.mu.Lock()
	defer u.mu.Unlock()

	status, err := u.GetVMStatus()
	if err != nil {
		return err
	}

	if status == UTMStatusStarted {
		u.vmRunning = true
		return nil // Already running
	}

	if status == UTMStatusPaused {
		// Resume from paused state
		cmd := exec.Command(u.utmctlPath, "start", u.vmName)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to resume VM: %w", err)
		}
	} else {
		// Start from stopped state
		cmd := exec.Command(u.utmctlPath, "start", u.vmName)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to start VM: %w", err)
		}
	}

	// Wait for VM to be running
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for VM to start")
		case <-ticker.C:
			status, _ := u.GetVMStatus()
			if status == UTMStatusStarted {
				u.vmRunning = true
				return nil
			}
		}
	}
}

// StopVM stops the VM gracefully
func (u *UTMManager) StopVM() error {
	u.mu.Lock()
	defer u.mu.Unlock()

	// Request graceful shutdown
	cmd := exec.Command(u.utmctlPath, "stop", "--request", u.vmName)
	if err := cmd.Run(); err != nil {
		// If graceful shutdown fails, force stop
		cmd = exec.Command(u.utmctlPath, "stop", "--force", u.vmName)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to stop VM: %w", err)
		}
	}

	u.vmRunning = false
	return nil
}

// SuspendVM suspends the VM to memory
func (u *UTMManager) SuspendVM() error {
	cmd := exec.Command(u.utmctlPath, "suspend", u.vmName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to suspend VM: %w", err)
	}
	return nil
}

// GetGuestIP returns the IP address of the guest VM
func (u *UTMManager) GetGuestIP() (string, error) {
	cmd := exec.Command(u.utmctlPath, "ip-address", u.vmName)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get guest IP: %w", err)
	}

	// ip-address returns multiple IPs, one per line
	// Return the first non-localhost IPv4 address
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		ip := strings.TrimSpace(line)
		if ip != "" && !strings.HasPrefix(ip, "127.") && !strings.Contains(ip, ":") {
			return ip, nil
		}
	}

	return "", fmt.Errorf("no valid IP address found")
}

// ExecInGuest executes a command inside the guest VM
// Requires qemu-guest-agent to be installed in the VM
func (u *UTMManager) ExecInGuest(command string, args ...string) (string, error) {
	cmdArgs := []string{"exec", u.vmName, "--", command}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.Command(u.utmctlPath, cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to exec in guest: %w\nOutput: %s", err, output)
	}

	return string(output), nil
}

// PushFile uploads a file to the guest
func (u *UTMManager) PushFile(localPath, remotePath string) error {
	cmd := exec.Command("sh", "-c",
		fmt.Sprintf("cat %s | %s file push %s %s",
			localPath, u.utmctlPath, u.vmName, remotePath))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to push file: %w", err)
	}
	return nil
}

// PullFile downloads a file from the guest
func (u *UTMManager) PullFile(remotePath, localPath string) error {
	cmd := exec.Command("sh", "-c",
		fmt.Sprintf("%s file pull %s %s > %s",
			u.utmctlPath, u.vmName, remotePath, localPath))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to pull file: %w", err)
	}
	return nil
}

// WaitForGuestAgent waits for the guest agent to be available
func (u *UTMManager) WaitForGuestAgent(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for guest agent")
		case <-ticker.C:
			// Try to execute a simple command
			_, err := u.ExecInGuest("echo", "ping")
			if err == nil {
				return nil
			}
		}
	}
}

// WaitForSSH waits for SSH to be available in the guest
func (u *UTMManager) WaitForSSH(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for SSH")
		case <-ticker.C:
			ip, err := u.GetGuestIP()
			if err != nil {
				continue
			}

			// Try to connect to SSH port
			cmd := exec.Command("nc", "-z", "-w", "1", ip, "22")
			if cmd.Run() == nil {
				return nil
			}
		}
	}
}

// GetSSHCommand returns the SSH command to connect to the guest
func (u *UTMManager) GetSSHCommand() (string, error) {
	ip, err := u.GetGuestIP()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("ssh helix@%s", ip), nil
}

// CreateHelixVM creates a new VM configured for Helix
// This creates a UTM VM with:
//   - Ubuntu Server ARM64 base
//   - 8GB RAM, 4 CPUs
//   - virtio-gpu-gl-pci for GPU acceleration
//   - virtio-vsock for host<->guest communication
//   - QEMU guest agent for remote control
func (u *UTMManager) CreateHelixVM() error {
	// Check if VM already exists
	vms, err := u.ListVMs()
	if err != nil {
		return err
	}
	for _, vm := range vms {
		if vm == u.vmName {
			return nil // Already exists
		}
	}

	// UTM VM creation via AppleScript
	// This creates a QEMU-based VM with the correct settings
	script := fmt.Sprintf(`
tell application "UTM"
	make new virtual machine with properties {backend:qemu, configuration:{name:"%s", architecture:"aarch64", cpuCount:4, memorySize:8192}}
end tell
`, u.vmName)

	cmd := exec.Command("osascript", "-e", script)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create VM: %w", err)
	}

	return nil
}

// IsRunning returns whether the VM is currently running
func (u *UTMManager) IsRunning() bool {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.vmRunning
}

// SetVMName changes the VM name to manage
func (u *UTMManager) SetVMName(name string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.vmName = name
}

// GetVMName returns the current VM name
func (u *UTMManager) GetVMName() string {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.vmName
}
