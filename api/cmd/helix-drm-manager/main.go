// helix-drm-manager is the DRM lease manager for Helix container desktops.
//
// It runs in the Linux VM as the sole DRM master on /dev/dri/card0.
// Containers request DRM leases via a Unix socket, and the manager:
// 1. Allocates a free scanout index (1-15)
// 2. Sends HELIX_MSG_ENABLE_SCANOUT to QEMU (TCP:15937) to connect the connector
// 3. Triggers connector reprobe in the guest kernel
// 4. Creates a DRM lease (connector + CRTC) via DRM_IOCTL_MODE_CREATE_LEASE
// 5. Passes the lease FD to the container via SCM_RIGHTS
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	drmmanager "github.com/helixml/helix/api/pkg/drm"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	cfg := drmmanager.Config{
		DRMDevice:  envOrDefault("DRM_DEVICE", "/dev/dri/card0"),
		SocketPath: envOrDefault("DRM_SOCKET", "/run/helix-drm/drm.sock"),
		QEMUAddr:   envOrDefault("QEMU_ADDR", "10.0.2.2:15937"),
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	mgr, err := drmmanager.New(cfg, logger)
	if err != nil {
		logger.Error("failed to create DRM manager", "err", err)
		os.Exit(1)
	}

	logger.Info("starting helix-drm-manager",
		"drm_device", cfg.DRMDevice,
		"socket", cfg.SocketPath,
		"qemu_addr", cfg.QEMUAddr)

	if err := mgr.Run(ctx); err != nil && err != context.Canceled {
		logger.Error("DRM manager error", "err", err)
		os.Exit(1)
	}

	logger.Info("helix-drm-manager shutdown complete")
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
