//go:build linux

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// runInVM on Linux executes commands directly via bash (Docker runs natively).
func (vm *VMManager) runInVM(script string) *exec.Cmd {
	return exec.Command("bash", "-c", script)
}

// Start starts the Helix environment on Linux (native Docker).
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

	ctx, cancel := context.WithCancel(context.Background())
	vm.ctx = ctx
	vm.cancelFunc = cancel

	vm.startTime = time.Now()
	vm.statusMu.Lock()
	vm.status.State = VMStateRunning
	vm.statusMu.Unlock()
	vm.emitStatus()

	go vm.waitForReady(ctx)
	return nil
}

// Stop stops the Helix environment on Linux.
func (vm *VMManager) Stop() error {
	vm.statusMu.Lock()
	if vm.status.State != VMStateRunning && vm.status.State != VMStateStarting {
		vm.statusMu.Unlock()
		return fmt.Errorf("VM is not running (current state: %s)", vm.status.State)
	}
	vm.status.State = VMStateStopping
	vm.statusMu.Unlock()
	vm.emitStatus()

	stopCmd := vm.runInVM("cd ~/helix 2>/dev/null && docker compose down 2>/dev/null; true")
	stopCmd.CombinedOutput()

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

// waitForReady on Linux: Docker is native, no VM boot needed.
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

	setBootStage("Starting Docker...")

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if time.Since(bootStart) > bootTimeout {
				vm.setError(fmt.Errorf("boot timed out after %d minutes", int(bootTimeout.Minutes())))
				return
			}

			if !dockerReady {
				if err := vm.ensureDockerRunning(); err != nil {
					log.Printf("Docker not ready: %v", err)
				} else {
					dockerReady = true
				}
			}

			if dockerReady && !secretInjected && vm.desktopSecret != "" {
				setBootStage("Configuring environment...")
				if err := vm.injectDesktopSecret(); err != nil {
					log.Printf("Desktop secret: %v", err)
				} else {
					secretInjected = true
				}
			}

			if dockerReady && !stackStarted {
				setBootStage("Starting Helix services...")
				if err := vm.startHelixStack(); err != nil {
					log.Printf("Helix stack: %v", err)
				} else {
					stackStarted = true
					stackStartedAt = time.Now()
				}
			}

			if stackStarted && !apiReady {
				apiCheckCount++
				elapsed := time.Since(stackStartedAt)
				if elapsed > apiTimeout {
					errMsg := vm.diagnoseAPIFailure()
					vm.setError(fmt.Errorf("API failed to start: %s", errMsg))
					return
				}
				setBootStage("Waiting for API...")
				if apiCheckCount%5 == 0 {
					log.Printf("API check %d (%.0fs)", apiCheckCount, elapsed.Seconds())
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

// SendConsoleInput is not applicable on native Linux.
func (vm *VMManager) SendConsoleInput(input string) error {
	return fmt.Errorf("console input not supported on Linux")
}

// GetVsockCID returns 0 on Linux (no VM).
func (vm *VMManager) GetVsockCID() uint32 { return 0 }

// GetSSHCommand returns a hint for Linux.
func (vm *VMManager) GetSSHCommand() string {
	return "docker compose -f ~/helix/docker-compose.dev.yaml exec api bash"
}

func (vm *VMManager) getVMDir() string {
	return filepath.Join(getHelixDataDir(), "vm", "helix-desktop")
}

func (vm *VMManager) getVMImagePath() string {
	if vm.config.DiskPath != "" {
		return vm.config.DiskPath
	}
	return filepath.Join(vm.getVMDir(), "docker-compose.yaml")
}

func (vm *VMManager) getZFSDiskPath() string { return "" }

func (vm *VMManager) ensureVMExtracted() error {
	return nil // No extraction needed on native Linux
}

func (vm *VMManager) createEmptyQcow2(path, size string) error {
	return fmt.Errorf("qcow2 not used on Linux")
}

func (vm *VMManager) resizeQcow2(path, size string) error {
	return fmt.Errorf("qcow2 not used on Linux")
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
