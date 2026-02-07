// drm-flipper does continuous page flips on a DRM lease connector to test
// the QEMU scanout capture pipeline. It creates two dumb buffers and
// alternates between them to simulate compositor rendering.
//
// Usage: drm-flipper [--drm-socket /run/helix-drm.sock]
package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
	"unsafe"

	drmmanager "github.com/helixml/helix/api/pkg/drm"
	"golang.org/x/sys/unix"
)

const (
	ioctlModeCreateDumb = 0xc02064b2
	ioctlModeAddFb      = 0xc01c64ae
	ioctlModeSetCrtc    = 0xc06864a2
	ioctlModePageFlip   = 0xc01064b0
	ioctlModeDestroyDumb = 0xc00464b4

	drmModePageFlipEvent = 0x01
)

type drmModeCreateDumb struct {
	Height uint32
	Width  uint32
	Bpp    uint32
	Flags  uint32
	Handle uint32
	Pitch  uint32
	Size   uint64
}

type drmModeFbCmd struct {
	FbID   uint32
	Width  uint32
	Height uint32
	Pitch  uint32
	Bpp    uint32
	Depth  uint32
	Handle uint32
}

type drmModePageFlip struct {
	CrtcID   uint32
	FbID     uint32
	Flags    uint32
	Reserved uint32
	UserData uint64
}

// drmModeModeInfo corresponds to struct drm_mode_modeinfo
type drmModeModeInfo struct {
	Clock      uint32
	Hdisplay   uint16
	HsyncStart uint16
	HsyncEnd   uint16
	Htotal     uint16
	Hskew      uint16
	Vdisplay   uint16
	VsyncStart uint16
	VsyncEnd   uint16
	Vtotal     uint16
	Vscan      uint16
	Vrefresh   uint32
	Flags      uint32
	Type       uint32
	Name       [32]byte
}

type drmModeCrtc struct {
	SetConnectorsPtr uint64
	CountConnectors  uint32
	CrtcID           uint32
	FbID             uint32
	X                uint32
	Y                uint32
	GammaSize        uint32
	ModeValid        uint32
	Mode             drmModeModeInfo
}

func createDumbBuffer(fd uintptr, width, height uint32) (handle, fbID uint32, pitch uint32, err error) {
	dumb := drmModeCreateDumb{Width: width, Height: height, Bpp: 32}
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, fd, ioctlModeCreateDumb, uintptr(unsafe.Pointer(&dumb)))
	if errno != 0 {
		return 0, 0, 0, fmt.Errorf("CREATE_DUMB: %v", errno)
	}

	fb := drmModeFbCmd{Width: width, Height: height, Pitch: dumb.Pitch, Bpp: 32, Depth: 24, Handle: dumb.Handle}
	_, _, errno = unix.Syscall(unix.SYS_IOCTL, fd, ioctlModeAddFb, uintptr(unsafe.Pointer(&fb)))
	if errno != 0 {
		return 0, 0, 0, fmt.Errorf("ADDFB: %v", errno)
	}

	return dumb.Handle, fb.FbID, dumb.Pitch, nil
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	drmSocket := "/run/helix-drm.sock"

	logger.Info("=== DRM Page Flipper ===")

	// Get DRM lease
	client := drmmanager.NewClient(drmSocket)
	lease, err := client.RequestLease(1920, 1080)
	if err != nil {
		logger.Error("Failed to get lease", "err", err)
		os.Exit(1)
	}
	logger.Info("Lease acquired", "scanout_id", lease.ScanoutID, "connector", lease.ConnectorName, "fd", lease.LeaseFD)

	fd := uintptr(lease.LeaseFD)

	// Create two dumb buffers for double-buffering
	_, fb1, _, err := createDumbBuffer(fd, 1920, 1080)
	if err != nil {
		logger.Error("Failed to create buffer 1", "err", err)
		os.Exit(1)
	}
	_, fb2, _, err := createDumbBuffer(fd, 1920, 1080)
	if err != nil {
		logger.Error("Failed to create buffer 2", "err", err)
		os.Exit(1)
	}
	logger.Info("Created double buffers", "fb1", fb1, "fb2", fb2)

	// The CRTC was pre-activated by helix-drm-manager with the first fb.
	// We need to find the CRTC ID from the lease. For scanout 1, CRTC = 44.
	crtcID := uint32(37 + lease.ScanoutID*7)
	connectorID := uint32(38 + lease.ScanoutID*7)

	logger.Info("Using CRTC/connector", "crtc_id", crtcID, "connector_id", connectorID)

	// Do initial modeset with fb1 (in case pre-activation used a different fb)
	connectors := []uint32{connectorID}
	mode := drmModeModeInfo{
		Clock: 217140, Hdisplay: 1920, HsyncStart: 2400, HsyncEnd: 2457, Htotal: 2592,
		Vdisplay: 1080, VsyncStart: 1085, VsyncEnd: 1090, Vtotal: 1117,
		Vrefresh: 75, Flags: 0x6, Type: 0x48,
	}
	copy(mode.Name[:], "1920x1080")

	crtc := drmModeCrtc{
		CrtcID:           crtcID,
		FbID:             fb1,
		SetConnectorsPtr: uint64(uintptr(unsafe.Pointer(&connectors[0]))),
		CountConnectors:  1,
		ModeValid:        1,
		Mode:             mode,
	}
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, fd, ioctlModeSetCrtc, uintptr(unsafe.Pointer(&crtc)))
	if errno != 0 {
		logger.Error("Initial SETCRTC failed", "err", errno)
		os.Exit(1)
	}
	logger.Info("Initial modeset done with fb1")

	// Now do continuous page flips
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	currentFB := fb1
	nextFB := fb2
	flipCount := 0
	startTime := time.Now()

	ticker := time.NewTicker(16 * time.Millisecond) // ~60 FPS
	defer ticker.Stop()

	logger.Info("Starting continuous page flips at ~60 FPS...")

	for {
		select {
		case <-sig:
			elapsed := time.Since(startTime).Seconds()
			logger.Info("Stopped", "flips", flipCount, "fps", fmt.Sprintf("%.1f", float64(flipCount)/elapsed))
			return
		case <-ticker.C:
			// Swap to next buffer via SETCRTC (simpler than page flip events)
			crtc := drmModeCrtc{
				CrtcID:           crtcID,
				FbID:             nextFB,
				SetConnectorsPtr: uint64(uintptr(unsafe.Pointer(&connectors[0]))),
				CountConnectors:  1,
				ModeValid:        1,
				Mode:             mode,
			}
			_, _, errno := unix.Syscall(unix.SYS_IOCTL, fd, ioctlModeSetCrtc, uintptr(unsafe.Pointer(&crtc)))
			if errno != 0 {
				if flipCount == 0 {
					logger.Error("SETCRTC flip failed", "err", errno)
					return
				}
				continue
			}

			flipCount++
			// Swap buffers
			currentFB, nextFB = nextFB, currentFB

			if flipCount == 1 || flipCount%300 == 0 {
				elapsed := time.Since(startTime).Seconds()
				logger.Info("Flip stats", "flips", flipCount, "fps", fmt.Sprintf("%.1f", float64(flipCount)/elapsed))
			}
		}
	}
}
