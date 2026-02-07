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

// openDRM opens the DRM device and acquires master.
func openDRM(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if err := setMaster(f); err != nil {
		f.Close()
		return nil, err
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

// reprobeConnector triggers a kernel reprobe of a DRM connector.
// Writes "1" to /sys/class/drm/card0-Virtual-{N}/status.
func reprobeConnector(scanoutIdx uint32) error {
	// Virtual-N is scanout_idx + 1 (Virtual-1 = scanout 0, Virtual-2 = scanout 1, etc.)
	path := fmt.Sprintf("/sys/class/drm/card0-Virtual-%d/status", scanoutIdx+1)
	return os.WriteFile(path, []byte("1"), 0644)
}
