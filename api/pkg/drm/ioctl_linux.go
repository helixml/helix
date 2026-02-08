package drm

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// DRM ioctl numbers for arm64 Linux.
// These use the standard Linux ioctl encoding:
//   _IO(type, nr)          = (type << 8) | nr
//   _IOR(type, nr, size)   = 0x80000000 | (size << 16) | (type << 8) | nr
//   _IOW(type, nr, size)   = 0x40000000 | (size << 16) | (type << 8) | nr
//   _IOWR(type, nr, size)  = 0xC0000000 | (size << 16) | (type << 8) | nr
const (
	// DRM_IOCTL_SET_MASTER = _IO('d', 0x1e)
	ioctlSetMaster = 0x641e

	// DRM_IOCTL_DROP_MASTER = _IO('d', 0x1f)
	ioctlDropMaster = 0x641f

	// DRM_IOCTL_MODE_GETRESOURCES = _IOWR('d', 0xa0, struct drm_mode_card_res)
	// struct drm_mode_card_res is 64 bytes on arm64
	ioctlModeGetResources = 0xc04064a0

	// DRM_IOCTL_MODE_GETCONNECTOR = _IOWR('d', 0xa7, struct drm_mode_get_connector)
	// struct drm_mode_get_connector is 80 bytes on arm64
	ioctlModeGetConnector = 0xc05064a7

	// DRM_IOCTL_MODE_CREATE_LEASE = _IOWR('d', 0xc6, struct drm_mode_create_lease)
	// struct drm_mode_create_lease is 24 bytes
	ioctlModeCreateLease = 0xc01864c6

	// DRM_IOCTL_MODE_REVOKE_LEASE = _IOW('d', 0xc9, struct drm_mode_revoke_lease)
	// struct drm_mode_revoke_lease is 4 bytes
	ioctlModeRevokeLease = 0x400464c9
)

// Connector status values
const (
	connectorStatusConnected    = 1
	connectorStatusDisconnected = 2
	connectorStatusUnknown      = 3
)

// drmModeCardRes corresponds to struct drm_mode_card_res.
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

// drmModeGetConnector corresponds to struct drm_mode_get_connector.
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

// drmModeCreateLease corresponds to struct drm_mode_create_lease.
type drmModeCreateLease struct {
	ObjectIDs  uint64 // pointer to array of object IDs
	ObjectCount uint32
	Flags      uint32
	LesseeID   uint32
	FD         int32
}

// drmModeRevokeLease corresponds to struct drm_mode_revoke_lease.
type drmModeRevokeLease struct {
	LesseeID uint32
}

// DRM client capability constants
const (
	// DRM_CLIENT_CAP_UNIVERSAL_PLANES = 2
	drmClientCapUniversalPlanes = 2
	// DRM_IOCTL_SET_CLIENT_CAP = _IOW('d', 0x0d, struct drm_set_client_cap)
	// struct drm_set_client_cap { uint64 capability, uint64 value }
	ioctlSetClientCap = 0x4010640d
)

type drmSetClientCap struct {
	Capability uint64
	Value      uint64
}

// openDRM opens the DRM device, acquires master, and sets capabilities.
func openDRM(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if err := setMaster(f); err != nil {
		f.Close()
		return nil, err
	}

	// Enable universal planes so plane format info is available in leases.
	// Without this, DRM_IOCTL_MODE_GETPLANE returns no formats, and Mutter
	// can't create framebuffers ("Plane has no advertised formats").
	cap := drmSetClientCap{
		Capability: drmClientCapUniversalPlanes,
		Value:      1,
	}
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), ioctlSetClientCap,
		uintptr(unsafe.Pointer(&cap)))
	if errno != 0 {
		// Non-fatal - log and continue
		fmt.Fprintf(os.Stderr, "DRM_CLIENT_CAP_UNIVERSAL_PLANES failed: %v\n", errno)
	}

	return f, nil
}

func setMaster(f *os.File) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), ioctlSetMaster, 0)
	if errno != 0 {
		return fmt.Errorf("DRM_IOCTL_SET_MASTER: %w", errno)
	}
	return nil
}

func dropMaster(f *os.File) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), ioctlDropMaster, 0)
	if errno != 0 {
		return fmt.Errorf("DRM_IOCTL_DROP_MASTER: %w", errno)
	}
	return nil
}

// getResources retrieves CRTCs and connectors from DRM.
func getResources(f *os.File) (crtcIDs, connectorIDs []uint32, err error) {
	// First call: get counts
	var res drmModeCardRes
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), ioctlModeGetResources,
		uintptr(unsafe.Pointer(&res)))
	if errno != 0 {
		return nil, nil, fmt.Errorf("MODE_GETRESOURCES (count): %w", errno)
	}

	if res.CountCrtcs == 0 || res.CountConnectors == 0 {
		return nil, nil, fmt.Errorf("no CRTCs or connectors found (crtcs=%d connectors=%d)",
			res.CountCrtcs, res.CountConnectors)
	}

	// Allocate arrays
	crtcIDs = make([]uint32, res.CountCrtcs)
	connectorIDs = make([]uint32, res.CountConnectors)
	fbIDs := make([]uint32, res.CountFbs)
	encoderIDs := make([]uint32, res.CountEncoders)

	// Second call: fill arrays
	res2 := drmModeCardRes{
		CrtcIDPtr:      uint64(uintptr(unsafe.Pointer(&crtcIDs[0]))),
		ConnectorIDPtr: uint64(uintptr(unsafe.Pointer(&connectorIDs[0]))),
		CountCrtcs:     res.CountCrtcs,
		CountConnectors: res.CountConnectors,
		CountFbs:       res.CountFbs,
		CountEncoders:  res.CountEncoders,
	}
	if res.CountFbs > 0 {
		res2.FbIDPtr = uint64(uintptr(unsafe.Pointer(&fbIDs[0])))
	}
	if res.CountEncoders > 0 {
		res2.EncoderIDPtr = uint64(uintptr(unsafe.Pointer(&encoderIDs[0])))
	}

	_, _, errno = unix.Syscall(unix.SYS_IOCTL, f.Fd(), ioctlModeGetResources,
		uintptr(unsafe.Pointer(&res2)))
	if errno != 0 {
		return nil, nil, fmt.Errorf("MODE_GETRESOURCES (fill): %w", errno)
	}

	return crtcIDs, connectorIDs, nil
}

// getConnectorStatus checks if a connector is connected.
func getConnectorStatus(f *os.File, connectorID uint32) (uint32, error) {
	conn := drmModeGetConnector{
		ConnectorID: connectorID,
	}
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), ioctlModeGetConnector,
		uintptr(unsafe.Pointer(&conn)))
	if errno != 0 {
		return 0, fmt.Errorf("MODE_GETCONNECTOR(%d): %w", connectorID, errno)
	}
	return conn.Connection, nil
}

// createLease creates a DRM lease for the given object IDs (connector + CRTC).
// Returns the lease FD and lessee ID.
func createLease(f *os.File, objectIDs []uint32) (leaseFD int, lesseeID uint32, err error) {
	if len(objectIDs) == 0 {
		return -1, 0, fmt.Errorf("no object IDs provided")
	}

	req := drmModeCreateLease{
		ObjectIDs:   uint64(uintptr(unsafe.Pointer(&objectIDs[0]))),
		ObjectCount: uint32(len(objectIDs)),
	}

	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), ioctlModeCreateLease,
		uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		return -1, 0, fmt.Errorf("MODE_CREATE_LEASE: %w", errno)
	}

	return int(req.FD), req.LesseeID, nil
}

// revokeLease revokes a DRM lease by lessee ID.
func revokeLease(f *os.File, lesseeID uint32) error {
	req := drmModeRevokeLease{LesseeID: lesseeID}
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), ioctlModeRevokeLease,
		uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		return fmt.Errorf("MODE_REVOKE_LEASE(%d): %w", lesseeID, errno)
	}
	return nil
}

// drmModeModeInfo corresponds to struct drm_mode_modeinfo (68 bytes).
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

// drmModeCrtc corresponds to struct drm_mode_crtc (72 bytes).
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

// drmModeCreateDumb corresponds to struct drm_mode_create_dumb.
type drmModeCreateDumb struct {
	Height uint32
	Width  uint32
	Bpp    uint32
	Flags  uint32
	Handle uint32
	Pitch  uint32
	Size   uint64
}

// drmModeFbCmd corresponds to struct drm_mode_fb_cmd.
type drmModeFbCmd struct {
	FbID   uint32
	Width  uint32
	Height uint32
	Pitch  uint32
	Bpp    uint32
	Depth  uint32
	Handle uint32
}

const (
	// DRM_IOCTL_MODE_SETCRTC = _IOWR('d', 0xa2, struct drm_mode_crtc)
	// struct drm_mode_crtc is 104 bytes
	ioctlModeSetCrtc = 0xc06864a2

	// DRM_IOCTL_MODE_CREATE_DUMB = _IOWR('d', 0xb2, struct drm_mode_create_dumb)
	ioctlModeCreateDumb = 0xc02064b2

	// DRM_IOCTL_MODE_ADDFB = _IOWR('d', 0xae, struct drm_mode_fb_cmd)
	ioctlModeAddFb = 0xc01c64ae

	// DRM_IOCTL_MODE_RMFB = _IOWR('d', 0xaf, uint32)
	ioctlModeRmFb = 0xc00464af
)

// activateCrtc does an initial modeset on a CRTC+connector to put it in active state.
// This is needed because Mutter can't do the first modeset on an inactive CRTC
// through a DRM lease (it reports "Plane has no advertised formats" and fails).
// After this, Mutter can inherit the active CRTC and render to it.
func activateCrtc(f *os.File, connectorID, crtcID, width, height uint32) error {
	// 1. Create a dumb buffer
	dumb := drmModeCreateDumb{
		Width:  width,
		Height: height,
		Bpp:    32,
	}
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), ioctlModeCreateDumb,
		uintptr(unsafe.Pointer(&dumb)))
	if errno != 0 {
		return fmt.Errorf("CREATE_DUMB: %w", errno)
	}

	// 2. Create a framebuffer from the dumb buffer
	fb := drmModeFbCmd{
		Width:  width,
		Height: height,
		Pitch:  dumb.Pitch,
		Bpp:    32,
		Depth:  24,
		Handle: dumb.Handle,
	}
	_, _, errno = unix.Syscall(unix.SYS_IOCTL, f.Fd(), ioctlModeAddFb,
		uintptr(unsafe.Pointer(&fb)))
	if errno != 0 {
		return fmt.Errorf("ADDFB: %w", errno)
	}

	// 3. Get the connector's preferred mode
	mode, err := getPreferredMode(f, connectorID, width, height)
	if err != nil {
		return fmt.Errorf("get mode: %w", err)
	}

	// 4. Set the CRTC with the framebuffer and mode
	connectors := []uint32{connectorID}
	crtc := drmModeCrtc{
		CrtcID:           crtcID,
		FbID:             fb.FbID,
		SetConnectorsPtr: uint64(uintptr(unsafe.Pointer(&connectors[0]))),
		CountConnectors:  1,
		ModeValid:        1,
		Mode:             mode,
	}
	_, _, errno = unix.Syscall(unix.SYS_IOCTL, f.Fd(), ioctlModeSetCrtc,
		uintptr(unsafe.Pointer(&crtc)))
	if errno != 0 {
		return fmt.Errorf("SETCRTC: %w", errno)
	}

	return nil
}

// getPreferredMode gets a mode matching the requested resolution from the connector.
func getPreferredMode(f *os.File, connectorID, width, height uint32) (drmModeModeInfo, error) {
	// First call to get mode count
	conn := drmModeGetConnector{ConnectorID: connectorID}
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), ioctlModeGetConnector,
		uintptr(unsafe.Pointer(&conn)))
	if errno != 0 {
		return drmModeModeInfo{}, fmt.Errorf("GETCONNECTOR count: %w", errno)
	}

	if conn.CountModes == 0 {
		return drmModeModeInfo{}, fmt.Errorf("connector %d has no modes", connectorID)
	}

	// Second call to get modes
	modes := make([]drmModeModeInfo, conn.CountModes)
	conn2 := drmModeGetConnector{
		ConnectorID: connectorID,
		ModesPtr:    uint64(uintptr(unsafe.Pointer(&modes[0]))),
		CountModes:  conn.CountModes,
	}
	_, _, errno = unix.Syscall(unix.SYS_IOCTL, f.Fd(), ioctlModeGetConnector,
		uintptr(unsafe.Pointer(&conn2)))
	if errno != 0 {
		return drmModeModeInfo{}, fmt.Errorf("GETCONNECTOR modes: %w", errno)
	}

	// Find exact match first
	for _, m := range modes {
		if uint32(m.Hdisplay) == width && uint32(m.Vdisplay) == height {
			return m, nil
		}
	}

	// No exact match found. This typically means QEMU's virtio-gpu EDID
	// doesn't include this resolution. Enable EDID on the virtio-gpu device
	// in UTM config: virtio-gpu-gl-pci,edid=on,xres=5120,yres=2880
	// Fall back to first mode (usually the highest available).
	return modes[0], nil
}

// reprobeConnector triggers a kernel reprobe of a DRM connector.
// Writes "1" to /sys/class/drm/card0-Virtual-{N}/status.
func reprobeConnector(scanoutIdx uint32) error {
	// Virtual-N is scanout_idx + 1 (Virtual-1 = scanout 0, Virtual-2 = scanout 1, etc.)
	path := fmt.Sprintf("/sys/class/drm/card0-Virtual-%d/status", scanoutIdx+1)
	return os.WriteFile(path, []byte("1"), 0644)
}
