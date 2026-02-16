// drm-modeset-test does a simple mode set on a leased DRM connector.
// This verifies that page flips on leased connectors trigger QEMU's resource_flush.
//
// Usage: drm-modeset-test [--drm-socket /run/helix-drm.sock]
package main

import (
	"fmt"
	"os"
	"time"
	"unsafe"

	drmmanager "github.com/helixml/helix/api/pkg/drm"
	"golang.org/x/sys/unix"
)

// DRM ioctl constants
const (
	ioctlModeGetConnector = 0xc05064a7
	ioctlModeGetCrtc      = 0xc06864a1
	ioctlModeSetCrtc      = 0xc06864a2
	ioctlModeCreateDumb   = 0xc02064b2
	ioctlModeAddFB        = 0xc03464ae
	ioctlModeDestroyDumb  = 0xc00464b4
	ioctlModeRmFB         = 0xc00464af
)

type drmModeGetConnector struct {
	EncodersPtr   uint64
	ModesPtr      uint64
	PropsPtr      uint64
	PropValuesPtr uint64
	CountModes    uint32
	CountProps    uint32
	CountEncoders uint32
	EncoderID     uint32
	ConnectorID   uint32
	ConnectorType uint32
	ConnectorTypeID uint32
	Connection    uint32
	MmWidth       uint32
	MmHeight      uint32
	Subpixel      uint32
	Pad           uint32
}

// drm_mode_modeinfo (68 bytes)
type drmModeModeinfo struct {
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

type drmModeCreateDumb struct {
	Height uint32
	Width  uint32
	Bpp    uint32
	Flags  uint32
	Handle uint32
	Pitch  uint32
	Size   uint64
}

type drmModeFBCmd struct {
	FBID   uint32
	Width  uint32
	Height uint32
	Pitch  uint32
	Bpp    uint32
	Depth  uint32
	Handle uint32
}

type drmModeCrtc struct {
	SetConnectorsPtr uint64
	CountConnectors  uint32
	CrtcID           uint32
	FBID             uint32
	X                uint32
	Y                uint32
	GammaSize        uint32
	ModeValid        uint32
	Mode             drmModeModeinfo
}

func main() {
	drmSocket := "/run/helix-drm/drm.sock"

	fmt.Println("=== DRM Modeset Test ===")

	// Get lease
	client := drmmanager.NewClient(drmSocket)
	lease, err := client.RequestLease(1920, 1080)
	if err != nil {
		fmt.Printf("ERROR requesting lease: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Lease: scanout=%d, connector=%s, fd=%d\n",
		lease.ScanoutID, lease.ConnectorName, lease.LeaseFD)

	fd := uintptr(lease.LeaseFD)

	// Get connector info (to find modes)
	// First call to get counts
	conn := drmModeGetConnector{}
	// We need the connector ID - the lease gives us access to one connector
	// Use DRM_IOCTL_MODE_GETRESOURCES on the lease FD to find it
	type drmModeCardRes struct {
		FbIDPtr        uint64
		CrtcIDPtr      uint64
		ConnectorIDPtr uint64
		EncoderIDPtr   uint64
		CountFbs       uint32
		CountCrtcs     uint32
		CountConnectors uint32
		CountEncoders  uint32
		MinWidth       uint32
		MaxWidth       uint32
		MinHeight      uint32
		MaxHeight      uint32
	}

	var res drmModeCardRes
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, fd, 0xc04064a0, uintptr(unsafe.Pointer(&res)))
	if errno != 0 {
		fmt.Printf("MODE_GETRESOURCES failed: %v\n", errno)
		os.Exit(1)
	}
	fmt.Printf("Lease resources: %d connectors, %d CRTCs\n", res.CountConnectors, res.CountCrtcs)

	// Get connector and CRTC IDs
	connIDs := make([]uint32, res.CountConnectors)
	crtcIDs := make([]uint32, res.CountCrtcs)
	res.ConnectorIDPtr = uint64(uintptr(unsafe.Pointer(&connIDs[0])))
	res.CrtcIDPtr = uint64(uintptr(unsafe.Pointer(&crtcIDs[0])))

	_, _, errno = unix.Syscall(unix.SYS_IOCTL, fd, 0xc04064a0, uintptr(unsafe.Pointer(&res)))
	if errno != 0 {
		fmt.Printf("MODE_GETRESOURCES (fill) failed: %v\n", errno)
		os.Exit(1)
	}

	fmt.Printf("Connector IDs: %v\n", connIDs)
	fmt.Printf("CRTC IDs: %v\n", crtcIDs)

	if len(connIDs) == 0 {
		fmt.Println("No connectors in lease!")
		os.Exit(1)
	}

	connectorID := connIDs[0]
	crtcID := crtcIDs[0]

	// Get connector modes (first call for counts)
	conn.ConnectorID = connectorID
	_, _, errno = unix.Syscall(unix.SYS_IOCTL, fd, ioctlModeGetConnector, uintptr(unsafe.Pointer(&conn)))
	if errno != 0 {
		fmt.Printf("MODE_GETCONNECTOR failed: %v\n", errno)
		os.Exit(1)
	}

	fmt.Printf("Connector %d: connection=%d, modes=%d\n",
		connectorID, conn.Connection, conn.CountModes)

	if conn.CountModes == 0 {
		fmt.Println("No modes available! Connector may be disconnected.")
		os.Exit(1)
	}

	// Get modes
	modes := make([]drmModeModeinfo, conn.CountModes)
	conn.ModesPtr = uint64(uintptr(unsafe.Pointer(&modes[0])))
	conn.EncodersPtr = 0
	conn.PropsPtr = 0
	conn.PropValuesPtr = 0

	_, _, errno = unix.Syscall(unix.SYS_IOCTL, fd, ioctlModeGetConnector, uintptr(unsafe.Pointer(&conn)))
	if errno != 0 {
		fmt.Printf("MODE_GETCONNECTOR (modes) failed: %v\n", errno)
		os.Exit(1)
	}

	// Pick the first mode (preferred)
	mode := modes[0]
	modeName := ""
	for i, b := range mode.Name {
		if b == 0 {
			modeName = string(mode.Name[:i])
			break
		}
	}
	fmt.Printf("Using mode: %s (%dx%d @%dHz)\n",
		modeName, mode.Hdisplay, mode.Vdisplay, mode.Vrefresh)

	// Create dumb buffer
	width := uint32(mode.Hdisplay)
	height := uint32(mode.Vdisplay)

	createDumb := drmModeCreateDumb{
		Width:  width,
		Height: height,
		Bpp:    32,
	}
	_, _, errno = unix.Syscall(unix.SYS_IOCTL, fd, ioctlModeCreateDumb, uintptr(unsafe.Pointer(&createDumb)))
	if errno != 0 {
		fmt.Printf("CREATE_DUMB failed: %v\n", errno)
		os.Exit(1)
	}
	fmt.Printf("Dumb buffer: handle=%d, pitch=%d, size=%d\n",
		createDumb.Handle, createDumb.Pitch, createDumb.Size)

	// Add framebuffer
	fbCmd := drmModeFBCmd{
		Width:  width,
		Height: height,
		Pitch:  createDumb.Pitch,
		Bpp:    32,
		Depth:  24,
		Handle: createDumb.Handle,
	}
	_, _, errno = unix.Syscall(unix.SYS_IOCTL, fd, ioctlModeAddFB, uintptr(unsafe.Pointer(&fbCmd)))
	if errno != 0 {
		fmt.Printf("MODE_ADDFB failed: %v\n", errno)
		os.Exit(1)
	}
	fmt.Printf("Framebuffer: fb_id=%d\n", fbCmd.FBID)

	// Set CRTC mode
	connPtr := connectorID
	setCrtc := drmModeCrtc{
		SetConnectorsPtr: uint64(uintptr(unsafe.Pointer(&connPtr))),
		CountConnectors:  1,
		CrtcID:           crtcID,
		FBID:             fbCmd.FBID,
		ModeValid:        1,
		Mode:             mode,
	}
	_, _, errno = unix.Syscall(unix.SYS_IOCTL, fd, ioctlModeSetCrtc, uintptr(unsafe.Pointer(&setCrtc)))
	if errno != 0 {
		fmt.Printf("MODE_SETCRTC failed: %v\n", errno)
		os.Exit(1)
	}
	fmt.Printf("✅ Mode set successfully! CRTC %d displaying on connector %d\n", crtcID, connectorID)
	fmt.Println("QEMU should now be capturing frames from this scanout.")
	fmt.Println("Check QEMU helix debug log for DisplaySurface updates.")

	// Keep the mode set for 10 seconds
	fmt.Println("\nKeeping mode active for 10 seconds...")
	time.Sleep(10 * time.Second)

	// Cleanup
	unix.Syscall(unix.SYS_IOCTL, fd, ioctlModeRmFB, uintptr(unsafe.Pointer(&fbCmd.FBID)))
	destroyDumb := struct{ Handle uint32 }{Handle: createDumb.Handle}
	unix.Syscall(unix.SYS_IOCTL, fd, ioctlModeDestroyDumb, uintptr(unsafe.Pointer(&destroyDumb)))

	fmt.Println("Done.")
}
