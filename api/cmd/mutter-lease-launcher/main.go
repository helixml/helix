// mutter-lease-launcher gets a DRM lease and launches gnome-shell with a logind stub.
//
// This program:
// 1. Connects to helix-drm-manager and gets a DRM lease FD
// 2. Stops real systemd-logind
// 3. Starts the logind-stub with the lease FD
// 4. Starts PipeWire (needed for gnome-shell initialization)
// 5. Launches gnome-shell --display-server inside dbus-run-session
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

	// Step 2: Stop real logind and wait for D-Bus name to be released
	logger.Info("Stopping systemd-logind...")
	exec.Command("systemctl", "stop", "systemd-logind").Run()
	time.Sleep(3 * time.Second)

	// Step 3: Start logind stub with lease FD
	// Go's exec.Command sets CLOEXEC on all FDs. Use ExtraFiles to pass the lease FD.
	// ExtraFiles[0] becomes FD 3 in the child (after stdin=0, stdout=1, stderr=2).
	leaseFile := os.NewFile(uintptr(lease.LeaseFD), "drm-lease")
	childFD := 3 // ExtraFiles[0] maps to FD 3

	logger.Info("Starting logind-stub...",
		"parent_lease_fd", lease.LeaseFD,
		"child_fd", childFD)
	stubCmd := exec.Command("/usr/local/bin/logind-stub",
		fmt.Sprintf("--lease-fd=%d", childFD))
	stubCmd.Stdout = os.Stdout
	stubCmd.Stderr = os.Stderr
	stubCmd.ExtraFiles = []*os.File{leaseFile}

	if err := stubCmd.Start(); err != nil {
		logger.Error("Failed to start logind-stub", "err", err)
		os.Exit(1)
	}
	logger.Info("logind-stub started", "pid", stubCmd.Process.Pid)
	time.Sleep(2 * time.Second)

	// Step 4: Set up XDG runtime directory
	xdgRuntime := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntime == "" {
		xdgRuntime = "/run/user/1000"
	}
	os.MkdirAll(xdgRuntime, 0700)

	// Step 5: Start PipeWire (needed for gnome-shell to complete init)
	logger.Info("Starting PipeWire...")
	pipewireCmd := exec.Command("pipewire")
	pipewireCmd.Stdout = os.Stdout
	pipewireCmd.Stderr = os.Stderr
	pipewireCmd.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+xdgRuntime)
	if err := pipewireCmd.Start(); err != nil {
		logger.Warn("PipeWire start failed (non-fatal)", "err", err)
	} else {
		logger.Info("PipeWire started", "pid", pipewireCmd.Process.Pid)
	}
	time.Sleep(500 * time.Millisecond)

	// Start WirePlumber
	wpCmd := exec.Command("wireplumber")
	wpCmd.Stdout = os.Stdout
	wpCmd.Stderr = os.Stderr
	wpCmd.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+xdgRuntime)
	if err := wpCmd.Start(); err != nil {
		logger.Warn("WirePlumber start failed (non-fatal)", "err", err)
	} else {
		logger.Info("WirePlumber started", "pid", wpCmd.Process.Pid)
	}
	time.Sleep(500 * time.Millisecond)

	// Step 6: Launch gnome-shell inside dbus-run-session
	// This is critical - gnome-shell needs dbus-run-session to create proper
	// D-Bus services (ScreenCast, DisplayConfig, etc.)
	logger.Info("Launching gnome-shell via dbus-run-session...")

	// Inherit MUTTER_DEBUG from parent environment if set
	mutterDebug := os.Getenv("MUTTER_DEBUG")

	env := os.Environ()
	env = append(env,
		"XDG_SESSION_TYPE=wayland",
		"XDG_CURRENT_DESKTOP=GNOME",
		"XDG_SESSION_DESKTOP=gnome",
		"DESKTOP_SESSION=gnome",
		"XDG_RUNTIME_DIR="+xdgRuntime,
	)
	if mutterDebug != "" {
		env = append(env, "MUTTER_DEBUG="+mutterDebug)
	}

	gnomeCmd := exec.Command("dbus-run-session", "--",
		"gnome-shell", "--display-server", "--wayland", "--no-x11", "--unsafe-mode")
	gnomeCmd.Stdout = os.Stdout
	gnomeCmd.Stderr = os.Stderr
	gnomeCmd.Env = env

	if err := gnomeCmd.Start(); err != nil {
		logger.Error("Failed to start gnome-shell", "err", err)
		stubCmd.Process.Kill()
		os.Exit(1)
	}
	logger.Info("gnome-shell started", "pid", gnomeCmd.Process.Pid)

	// Wait for Wayland socket
	logger.Info("Waiting for Wayland socket...")
	for i := 0; i < 30; i++ {
		if _, err := os.Stat(xdgRuntime + "/wayland-0"); err == nil {
			logger.Info("Wayland socket ready!")
			break
		}
		time.Sleep(time.Second)
	}

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
	if pipewireCmd.Process != nil {
		pipewireCmd.Process.Kill()
	}
	if wpCmd.Process != nil {
		wpCmd.Process.Kill()
	}

	// Restart real logind
	exec.Command("systemctl", "start", "systemd-logind").Run()
	logger.Info("Done")
}
