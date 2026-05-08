// Package gpuarch maps vendor-specific GPU identifiers to canonical
// architecture strings used by the runner profile compatibility check.
//
// One file is shared between the runner (writer — labels its GPUs) and the
// API server (reader — validates whether a profile fits a runner). Adding a
// new architecture means one entry in this file.
package gpuarch

import "strings"

// NVIDIA compute capability is the canonical identifier for an NVIDIA GPU's
// architecture. Sources:
//   - https://developer.nvidia.com/cuda-gpus
//   - https://docs.nvidia.com/cuda/cuda-c-programming-guide/index.html
const (
	ArchNVIDIABlackwell = "blackwell" // CC 12.x — B100, B200, GB200, RTX 50xx
	ArchNVIDIAHopper    = "hopper"    // CC 9.x  — H100, H200, GH200
	ArchNVIDIAAda       = "ada"       // CC 8.9  — RTX 40xx, L4, L40, L40S
	ArchNVIDIAAmpere    = "ampere"    // CC 8.0/8.6 — A100, A30, A40, A6000, RTX 30xx
	ArchNVIDIATuring    = "turing"    // CC 7.5  — T4, RTX 20xx
	ArchNVIDIAVolta     = "volta"     // CC 7.0  — V100
	ArchNVIDIAPascal    = "pascal"    // CC 6.x  — P100, GTX 10xx
)

// AMD architectures keyed off the LLVM/ROCm gfx target string. Sources:
//   - https://llvm.org/docs/AMDGPUUsage.html
//   - https://rocm.docs.amd.com/projects/install-on-linux/en/latest/reference/system-requirements.html
const (
	ArchAMDCDNA3 = "cdna3" // gfx942 — MI300, MI300X, MI325X
	ArchAMDCDNA2 = "cdna2" // gfx90a — MI250, MI250X
	ArchAMDCDNA1 = "cdna1" // gfx908 — MI100
	ArchAMDRDNA3 = "rdna3" // gfx1100/gfx1101/gfx1102 — RX 7900 XTX, W7900
	ArchAMDRDNA2 = "rdna2" // gfx1030 — RX 6900 XT
	ArchAMDVega  = "vega"  // gfx906/gfx900 — Vega 20, Vega 10, MI50/MI60
)

// FromNVIDIAComputeCapability returns the canonical architecture string for
// an NVIDIA GPU given its compute capability (e.g. "9.0", "8.6", "12.0").
// Empty string return means "unknown / future architecture".
//
// We map by major version primarily, with a few minor-version overrides for
// architectures that share a major (Ada is 8.9 within the Ampere major).
func FromNVIDIAComputeCapability(cc string) string {
	cc = strings.TrimSpace(cc)
	if cc == "" {
		return ""
	}
	// Special cases first.
	if cc == "8.9" {
		return ArchNVIDIAAda
	}
	major, _, _ := strings.Cut(cc, ".")
	switch major {
	case "12":
		return ArchNVIDIABlackwell
	case "10", "11":
		// CC 10/11 are reserved/unreleased as of writing; treat as unknown
		// rather than guessing.
		return ""
	case "9":
		return ArchNVIDIAHopper
	case "8":
		return ArchNVIDIAAmpere
	case "7":
		// 7.5 = Turing, 7.0/7.2 = Volta.
		if strings.HasPrefix(cc, "7.5") {
			return ArchNVIDIATuring
		}
		return ArchNVIDIAVolta
	case "6":
		return ArchNVIDIAPascal
	}
	return ""
}

// FromAMDGFX returns the canonical architecture string for an AMD GPU given
// its gfx target (e.g. "gfx942", "gfx90a", "gfx1100"). Empty return means
// "unknown / future architecture".
func FromAMDGFX(gfx string) string {
	gfx = strings.ToLower(strings.TrimSpace(gfx))
	if !strings.HasPrefix(gfx, "gfx") {
		return ""
	}
	switch gfx {
	case "gfx942":
		return ArchAMDCDNA3
	case "gfx90a":
		return ArchAMDCDNA2
	case "gfx908":
		return ArchAMDCDNA1
	case "gfx1100", "gfx1101", "gfx1102":
		return ArchAMDRDNA3
	case "gfx1030", "gfx1031", "gfx1032":
		return ArchAMDRDNA2
	case "gfx900", "gfx906":
		return ArchAMDVega
	}
	return ""
}

// IsNVIDIA reports whether the given canonical arch is an NVIDIA architecture.
// Useful for quick sanity-checks at the call site.
func IsNVIDIA(arch string) bool {
	switch arch {
	case ArchNVIDIABlackwell, ArchNVIDIAHopper, ArchNVIDIAAda,
		ArchNVIDIAAmpere, ArchNVIDIATuring, ArchNVIDIAVolta, ArchNVIDIAPascal:
		return true
	}
	return false
}

// IsAMD reports whether the given canonical arch is an AMD architecture.
func IsAMD(arch string) bool {
	switch arch {
	case ArchAMDCDNA3, ArchAMDCDNA2, ArchAMDCDNA1,
		ArchAMDRDNA3, ArchAMDRDNA2, ArchAMDVega:
		return true
	}
	return false
}
