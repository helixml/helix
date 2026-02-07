//go:build !linux

package drm

import (
	"fmt"
	"os"
)

// Stubs for non-Linux platforms. helix-drm-manager only runs on Linux.

func openDRM(path string) (*os.File, error) {
	return nil, fmt.Errorf("DRM ioctls only supported on Linux")
}

func setMaster(f *os.File) error {
	return fmt.Errorf("DRM ioctls only supported on Linux")
}

func dropMaster(f *os.File) error {
	return fmt.Errorf("DRM ioctls only supported on Linux")
}

func getResources(f *os.File) (crtcIDs, connectorIDs []uint32, err error) {
	return nil, nil, fmt.Errorf("DRM ioctls only supported on Linux")
}

func getConnectorStatus(f *os.File, connectorID uint32) (uint32, error) {
	return 0, fmt.Errorf("DRM ioctls only supported on Linux")
}

func createLease(f *os.File, objectIDs []uint32) (leaseFD int, lesseeID uint32, err error) {
	return -1, 0, fmt.Errorf("DRM ioctls only supported on Linux")
}

func revokeLease(f *os.File, lesseeID uint32) error {
	return fmt.Errorf("DRM ioctls only supported on Linux")
}

func reprobeConnector(scanoutIdx uint32) error {
	return fmt.Errorf("DRM ioctls only supported on Linux")
}

func activateCrtc(f *os.File, connectorID, crtcID, width, height uint32) error {
	return fmt.Errorf("DRM ioctls only supported on Linux")
}
