//go:build windows

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const wslDistroName = "Helix"

// runInVM creates a command to execute a script inside the WSL2 distro.
// On Windows, this uses wsl.exe instead of SSH.
func (vm *VMManager) runInVM(script string) *exec.Cmd {
	return exec.Command("wsl.exe", "-d", wslDistroName, "--", "bash", "-c", script)
}

// Start starts the Helix WSL2 environment
func (vm *VMManager) Start() error {
	vm.statusMu.Lock()
	if vm.status.State != VMStateStopped && vm.status.State != VMStateError {
		vm.statusMu.Unlock()
		return fmt.Errorf("VM is not stopped (current state: %s)", vm.status.State)
	}
	vm.status.State = VMStateStarting
	vm.status.ErrorMsg = ""
	vm.statusMu.Unlock()
	vm.emitStatus()

	// Ensure WSL2 is available and the distro exists
	if err := vm.ensureWSL2Ready(); err != nil {
		vm.setError(err)
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	vm.ctx = ctx
	vm.cancelFunc = cancel

	vm.startTime = time.Now()
	vm.statusMu.Lock()
	vm.status.State = VMStateRunning
	vm.statusMu.Unlock()
	vm.emitStatus()

	// Wait for services to be ready
	go vm.waitForReady(ctx)

	return nil
}

// Stop terminates the WSL2 distro
func (vm *VMManager) Stop() error {
	vm.statusMu.Lock()
	if vm.status.State != VMStateRunning && vm.status.State != VMStateStarting {
		vm.statusMu.Unlock()
		return fmt.Errorf("VM is not running (current state: %s)", vm.status.State)
	}
	vm.status.State = VMStateStopping
	vm.statusMu.Unlock()
	vm.emitStatus()

	// Gracefully stop Docker containers first
	stopCmd := vm.runInVM("cd ~/helix 2>/dev/null && docker compose down 2>/dev/null; true")
	stopCmd.CombinedOutput()

	// Terminate the WSL distro
	cmd := exec.Command("wsl.exe", "--terminate", wslDistroName)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("WSL terminate warning: %v (output: %s)", err, string(out))
	}

	if vm.cancelFunc != nil {
		vm.cancelFunc()
	}

	vm.statusMu.Lock()
	vm.status.State = VMStateStopped
	vm.status.APIReady = false
	vm.statusMu.Unlock()
	vm.emitStatus()

	return nil
}

// ensureWSL2Ready checks that WSL2 is installed and the Helix distro exists.
// If the distro doesn't exist yet, it imports from the downloaded rootfs tarball.
func (vm *VMManager) ensureWSL2Ready() error {
	// Check if WSL is available
	cmd := exec.Command("wsl.exe", "--status")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("WSL2 is not available. Please install WSL2: https://learn.microsoft.com/en-us/windows/wsl/install\nError: %v\nOutput: %s", err, string(out))
	}

	// Check if our distro exists
	listCmd := exec.Command("wsl.exe", "--list", "--quiet")
	listOut, err := listCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list WSL distributions: %w", err)
	}

	distros := strings.TrimSpace(string(listOut))
	for _, line := range splitLines(distros) {
		// WSL output may include BOM or null bytes (UTF-16)
		cleaned := strings.TrimSpace(strings.ReplaceAll(line, "\x00", ""))
		if cleaned == wslDistroName {
			log.Printf("WSL distro '%s' already exists", wslDistroName)
			return nil
		}
	}

	// Distro doesn't exist — import from rootfs tarball
	return vm.importDistro()
}

// importDistro imports the Helix WSL2 distribution from a rootfs tarball.
// The tarball is downloaded from the CDN alongside the disk images.
func (vm *VMManager) importDistro() error {
	rootfsPath := filepath.Join(getHelixDataDir(), "vm", "helix-desktop", "rootfs.tar.gz")
	if _, err := os.Stat(rootfsPath); err != nil {
		return ErrVMImagesNotDownloaded
	}

	installDir := filepath.Join(getHelixDataDir(), "wsl", wslDistroName)
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return fmt.Errorf("failed to create WSL install directory: %w", err)
	}

	log.Printf("Importing WSL distro '%s' from %s...", wslDistroName, rootfsPath)
	cmd := exec.Command("wsl.exe", "--import", wslDistroName, installDir, rootfsPath, "--version", "2")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to import WSL distro: %w (output: %s)", err, string(out))
	}

	log.Printf("WSL distro '%s' imported successfully", wslDistroName)

	// Set default user to ubuntu
	setUserCmd := vm.runInVM("echo '[user]\ndefault=ubuntu' | sudo tee /etc/wsl.conf > /dev/null")
	setUserCmd.CombinedOutput()

	return nil
}

// waitForReady waits for the WSL2 environment to be fully ready.
// Simpler than macOS — no SSH wait, no ZFS, no QEMU boot.
func (vm *VMManager) waitForReady(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	bootStart := time.Now()
	const bootTimeout = 10 * time.Minute
	const apiTimeout = 3 * time.Minute

	dockerReady := false
	secretInjected := false
	stackStarted := false
	stackStartedAt := time.Time{}
	apiReady := false
	apiCheckCount := 0

	setBootStage := func(stage string) {
		vm.statusMu.Lock()
		vm.status.BootStage = stage
		vm.statusMu.Unlock()
		vm.emitStatus()
	}

	setBootStage("Starting WSL2...")

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if time.Since(bootStart) > bootTimeout {
				log.Printf("Boot timed out after %v", bootTimeout)
				vm.statusMu.Lock()
				vm.status.BootStage = ""
				vm.statusMu.Unlock()
				vm.setError(fmt.Errorf("boot timed out after %d minutes", int(bootTimeout.Minutes())))
				return
			}

			// Start Docker inside WSL2
			if !dockerReady {
				setBootStage("Starting Docker...")
				if err := vm.ensureDockerRunning(); err != nil {
					log.Printf("Docker not ready yet: %v", err)
				} else {
					dockerReady = true
				}
			}

			// Inject desktop secret
			if dockerReady && !secretInjected && vm.desktopSecret != "" {
				setBootStage("Configuring environment...")
				if err := vm.injectDesktopSecret(); err != nil {
					log.Printf("Desktop secret injection: %v", err)
				} else {
					secretInjected = true
				}
			}

			// Start the Helix compose stack
			if dockerReady && !stackStarted {
				setBootStage("Starting Helix services...")
				if err := vm.startHelixStack(); err != nil {
					log.Printf("Helix stack start: %v", err)
				} else {
					stackStarted = true
					stackStartedAt = time.Now()
				}
			}

			// Check if API is responding
			if stackStarted && !apiReady {
				apiCheckCount++
				elapsed := time.Since(stackStartedAt)
				if elapsed > apiTimeout {
					log.Printf("API not ready after %v", apiTimeout)
					errMsg := vm.diagnoseAPIFailure()
					vm.statusMu.Lock()
					vm.status.BootStage = ""
					vm.statusMu.Unlock()
					vm.setError(fmt.Errorf("API failed to start: %s", errMsg))
					return
				}

				setBootStage("Waiting for API...")
				if apiCheckCount%5 == 0 {
					log.Printf("API health check attempt %d (%.0fs since stack start)", apiCheckCount, elapsed.Seconds())
				}
				if vm.checkAPIHealth() {
					vm.statusMu.Lock()
					vm.status.APIReady = true
					vm.status.BootStage = ""
					vm.statusMu.Unlock()
					vm.emitStatus()
					apiReady = true
				}
			}

			if apiReady {
				return
			}
		}
	}
}

// SendConsoleInput sends input to the WSL2 environment.
// On Windows/WSL2, console input is not directly supported in the same way as QEMU serial.
func (vm *VMManager) SendConsoleInput(input string) error {
	return fmt.Errorf("console input not supported on Windows — use WSL terminal instead")
}

// GetVsockCID returns 0 on Windows (no vsock with WSL2).
func (vm *VMManager) GetVsockCID() uint32 {
	return 0
}

// GetSSHCommand returns a wsl command hint for Windows.
func (vm *VMManager) GetSSHCommand() string {
	return fmt.Sprintf("wsl -d %s", wslDistroName)
}

// getVMDir returns the writable VM data directory on Windows.
func (vm *VMManager) getVMDir() string {
	return filepath.Join(getHelixDataDir(), "vm", "helix-desktop")
}

// getVMImagePath returns the path to the rootfs tarball on Windows.
func (vm *VMManager) getVMImagePath() string {
	if vm.config.DiskPath != "" {
		return vm.config.DiskPath
	}
	return filepath.Join(vm.getVMDir(), "rootfs.tar.gz")
}

// getZFSDiskPath returns empty on Windows (no ZFS).
func (vm *VMManager) getZFSDiskPath() string {
	return ""
}

// ensureVMExtracted checks if the WSL rootfs tarball exists.
func (vm *VMManager) ensureVMExtracted() error {
	rootfs := vm.getVMImagePath()
	if _, err := os.Stat(rootfs); err != nil {
		log.Printf("WSL rootfs not found at %s — download required", rootfs)
		return ErrVMImagesNotDownloaded
	}
	return nil
}

// createEmptyQcow2 is a no-op on Windows (no QEMU disk images).
func (vm *VMManager) createEmptyQcow2(path, size string) error {
	return fmt.Errorf("qcow2 images not used on Windows")
}

// resizeQcow2 is a no-op on Windows.
func (vm *VMManager) resizeQcow2(path, size string) error {
	return fmt.Errorf("qcow2 images not used on Windows — resize WSL disk via wsl.exe --manage")
}
