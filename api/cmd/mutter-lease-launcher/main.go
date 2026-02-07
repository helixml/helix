// mutter-lease-launcher gets a DRM lease and launches gnome-shell with a logind stub.
//
// This program:
// 1. Connects to helix-drm-manager and gets a DRM lease FD
// 2. Stops real systemd-logind
// 3. Starts the logind-stub with the lease FD
// 4. Launches gnome-shell --display-server
//
// Usage: sudo mutter-lease-launcher [--drm-socket /run/helix-drm.sock]
package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	drmmanager "github.com/helixml/helix/api/pkg/drm"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	drmSocket := "/run/helix-drm.sock"

	logger.Info("=== Mutter Lease Launcher ===")

	// Step 1: Get DRM lease
	logger.Info("Requesting DRM lease...")
	client := drmmanager.NewClient(drmSocket)
	lease, err := client.RequestLease(1920, 1080)
	if err != nil {
		logger.Error("Failed to get lease", "err", err)
		os.Exit(1)
	}
	logger.Info("Lease acquired",
		"scanout_id", lease.ScanoutID,
		"connector", lease.ConnectorName,
		"lease_fd", lease.LeaseFD)

	// Step 2: Stop real logind
	logger.Info("Stopping systemd-logind...")
	exec.Command("systemctl", "stop", "systemd-logind").Run()
	time.Sleep(time.Second)

	// Step 3: Start logind stub with lease FD
	// The lease FD is inherited by the child process
	logger.Info("Starting logind-stub...", "lease_fd", lease.LeaseFD)
	stubCmd := exec.Command("/usr/local/bin/logind-stub",
		fmt.Sprintf("--lease-fd=%d", lease.LeaseFD))
	stubCmd.Stdout = os.Stdout
	stubCmd.Stderr = os.Stderr
	// Keep the lease FD open in the child
	stubCmd.ExtraFiles = nil // FDs are inherited by default when not set

	if err := stubCmd.Start(); err != nil {
		logger.Error("Failed to start logind-stub", "err", err)
		os.Exit(1)
	}
	logger.Info("logind-stub started", "pid", stubCmd.Process.Pid)
	time.Sleep(2 * time.Second)

	// Step 4: Set up environment for gnome-shell
	env := os.Environ()
	env = append(env,
		"XDG_SESSION_TYPE=tty",
		"XDG_CURRENT_DESKTOP=GNOME",
		"XDG_SESSION_DESKTOP=gnome",
		"MUTTER_DEBUG_FORCE_KMS_MODE=simple",
	)

	// Step 5: Launch gnome-shell
	logger.Info("Launching gnome-shell --display-server...")
	gnomeCmd := exec.Command("gnome-shell", "--display-server", "--wayland", "--no-x11")
	gnomeCmd.Stdout = os.Stdout
	gnomeCmd.Stderr = os.Stderr
	gnomeCmd.Env = env

	if err := gnomeCmd.Start(); err != nil {
		logger.Error("Failed to start gnome-shell", "err", err)
		stubCmd.Process.Kill()
		os.Exit(1)
	}
	logger.Info("gnome-shell started", "pid", gnomeCmd.Process.Pid)

	// Wait for signals
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	// Cleanup
	logger.Info("Shutting down...")
	gnomeCmd.Process.Signal(syscall.SIGTERM)
	gnomeCmd.Wait()
	stubCmd.Process.Signal(syscall.SIGTERM)
	stubCmd.Wait()

	// Restart real logind
	exec.Command("systemctl", "start", "systemd-logind").Run()
	logger.Info("Done")
}
