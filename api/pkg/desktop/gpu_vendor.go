package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// GPUVendor identifies the graphics hardware actually present on the host.
//
// This is deliberately independent of which GStreamer encoder elements happen to
// be registered. Deriving hardware identity from plugin availability is what
// caused the 2026-07-28 incident: when a leaked CUDA context exhausted the GPU,
// nvh264enc could no longer register, so `checkGstElement("nvh264enc")` went
// false, the pipeline builder concluded the machine was AMD/Intel, and it took
// the `pipewiresrc always-copy=true` branch on NVIDIA hardware — a path that
// delivered zero frames. The logs said "GNOME + AMD/Intel detected" on an RTX
// 2000 Ada, which sent triage in the wrong direction for hours.
type GPUVendor string

const (
	GPUVendorNVIDIA  GPUVendor = "nvidia"
	GPUVendorAMD     GPUVendor = "amd"
	GPUVendorIntel   GPUVendor = "intel"
	GPUVendorUnknown GPUVendor = "unknown"
)

// PCI vendor IDs as reported by /sys/class/drm/<card>/device/vendor.
const (
	pciVendorNVIDIA = "0x10de"
	pciVendorAMD    = "0x1002"
	pciVendorIntel  = "0x8086"
)

// detectGPUVendor reports the GPU vendor from the hardware itself.
//
// It is a package-level var so tests can substitute a deterministic stub, the
// same seam pattern checkGstElement uses.
var detectGPUVendor = sync.OnceValue(func() GPUVendor {
	// An NVIDIA device node is conclusive and cheap. It exists whenever the
	// driver is loaded and the device is passed into the container, regardless of
	// whether GStreamer can currently create an encoder on it.
	if _, err := os.Stat("/dev/nvidia0"); err == nil {
		return GPUVendorNVIDIA
	}

	// Otherwise read the PCI vendor id of the DRM device. Prefer the render node
	// the desktop was actually configured with, so multi-GPU hosts report the GPU
	// in use rather than whichever card enumerates first.
	if v := vendorFromRenderNode(getRenderDevice()); v != GPUVendorUnknown {
		return v
	}
	for _, path := range drmVendorFiles() {
		if v := vendorFromFile(path); v != GPUVendorUnknown {
			return v
		}
	}
	return GPUVendorUnknown
})

// compositorName gives the compositor a name for logs, so a line that reports
// the GPU vendor also says which capture path it implies.
func compositorName(isSway bool) string {
	if isSway {
		return "sway"
	}
	return "gnome"
}

// drmVendorFiles lists the vendor files of every DRM card on the system.
func drmVendorFiles() []string {
	matches, err := filepath.Glob("/sys/class/drm/card[0-9]*/device/vendor")
	if err != nil {
		return nil
	}
	return matches
}

// vendorFromRenderNode maps /dev/dri/renderD128 to its sysfs vendor file.
func vendorFromRenderNode(node string) GPUVendor {
	if node == "" {
		return GPUVendorUnknown
	}
	return vendorFromFile(filepath.Join("/sys/class/drm", filepath.Base(node), "device", "vendor"))
}

func vendorFromFile(path string) GPUVendor {
	b, err := os.ReadFile(path)
	if err != nil {
		return GPUVendorUnknown
	}
	switch strings.ToLower(strings.TrimSpace(string(b))) {
	case pciVendorNVIDIA:
		return GPUVendorNVIDIA
	case pciVendorAMD:
		return GPUVendorAMD
	case pciVendorIntel:
		return GPUVendorIntel
	default:
		return GPUVendorUnknown
	}
}
