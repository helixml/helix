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
	"strconv"
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

	// Step 1: Get DRM lease at the configured resolution
	width, height := 1920, 1080
	if w, err := strconv.Atoi(os.Getenv("GAMESCOPE_WIDTH")); err == nil && w > 0 {
		width = w
	}
	if h, err := strconv.Atoi(os.Getenv("GAMESCOPE_HEIGHT")); err == nil && h > 0 {
		height = h
	}
	logger.Info("Requesting DRM lease...", "width", width, "height", height)
	client := drmmanager.NewClient(drmSocket)
	lease, err := client.RequestLease(uint32(width), uint32(height))
	if err != nil {
		logger.Error("Failed to get lease", "err", err)
		os.Exit(1)
	}
	logger.Info("Lease acquired",
		"scanout_id", lease.ScanoutID,
		"connector", lease.ConnectorName,
		"lease_fd", lease.LeaseFD)

	// Write scanout ID to file so desktop-bridge can read it
	// (desktop-bridge needs to know which QEMU scanout to subscribe to)
	xdgRuntime := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntime == "" {
		xdgRuntime = "/run/user/1000"
	}
	scanoutFile := xdgRuntime + "/helix-scanout-id"
	if err := os.WriteFile(scanoutFile, []byte(fmt.Sprintf("%d", lease.ScanoutID)), 0644); err != nil {
		logger.Warn("Failed to write scanout ID file", "err", err, "path", scanoutFile)
	} else {
		logger.Info("Wrote scanout ID file", "path", scanoutFile, "scanout_id", lease.ScanoutID)
	}

	// Step 2: Stop real logind if running, or start system D-Bus in containers
	if err := exec.Command("systemctl", "is-active", "--quiet", "systemd-logind").Run(); err == nil {
		logger.Info("Stopping systemd-logind...")
		exec.Command("systemctl", "stop", "systemd-logind").Run()
		time.Sleep(3 * time.Second)
	} else {
		logger.Info("systemd-logind not running (container mode)")
		// In containers, start a system D-Bus daemon for logind-stub
		if _, err := os.Stat("/var/run/dbus/system_bus_socket"); os.IsNotExist(err) {
			logger.Info("Starting system D-Bus daemon for container...")
			os.MkdirAll("/var/run/dbus", 0755)
			dbusCmd := exec.Command("dbus-daemon", "--system", "--fork")
			if err := dbusCmd.Run(); err != nil {
				logger.Warn("Failed to start system dbus-daemon", "err", err)
			} else {
				logger.Info("System D-Bus daemon started")
				time.Sleep(500 * time.Millisecond)
			}
		}
	}

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

	// Step 4: Set up XDG runtime directory (xdgRuntime already set above)
	os.MkdirAll(xdgRuntime, 0700)
	// Fix ownership to match current user (avoids dbus complaints about UID mismatch)
	os.Chown(xdgRuntime, os.Getuid(), os.Getgid())

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

	// If already inside a dbus-run-session (e.g., container startup), don't nest
	var gnomeCmd *exec.Cmd
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") != "" {
		logger.Info("Using existing D-Bus session bus")
		gnomeCmd = exec.Command("gnome-shell", "--display-server", "--wayland", "--no-x11", "--unsafe-mode")
	} else {
		logger.Info("Creating new D-Bus session via dbus-run-session")
		gnomeCmd = exec.Command("dbus-run-session", "--",
			"gnome-shell", "--display-server", "--wayland", "--no-x11", "--unsafe-mode")
	}
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

	// Release the DRM lease — closing the liveness connection tells the
	// manager to release the scanout. On SIGKILL, the kernel does this
	// automatically when it closes all our file descriptors.
	lease.Close()
	logger.Info("DRM lease released")

	// Restart real logind if we stopped it (not in containers)
	if err := exec.Command("systemctl", "is-system-running").Run(); err == nil {
		exec.Command("systemctl", "start", "systemd-logind").Run()
	}
	logger.Info("Done")
}
