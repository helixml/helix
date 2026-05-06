package profile

import (
	"testing"

	"github.com/helixml/helix/api/pkg/runner/gpuarch"
	"github.com/helixml/helix/api/pkg/types"
)

func gpu(idx int, vendor types.GPUVendor, arch, model string, vram uint64) RunnerGPUInfo {
	return RunnerGPUInfo{Index: idx, Vendor: vendor, Architecture: arch, ModelName: model, TotalVRAM: vram}
}

func TestCompatibility_AllConstraintsPass(t *testing.T) {
	req := types.ProfileGPURequirement{
		Count:         2,
		Vendor:        types.GPUVendorNVIDIA,
		Architectures: []string{gpuarch.ArchNVIDIAHopper, gpuarch.ArchNVIDIABlackwell},
		ModelMatch:    "^NVIDIA H100",
		MinVRAMBytes:  80 << 30,
	}
	gpus := []RunnerGPUInfo{
		gpu(0, types.GPUVendorNVIDIA, gpuarch.ArchNVIDIAHopper, "NVIDIA H100 80GB HBM3", 80<<30),
		gpu(1, types.GPUVendorNVIDIA, gpuarch.ArchNVIDIAHopper, "NVIDIA H100 80GB HBM3", 80<<30),
	}
	if err := Compatibility(req, gpus); err != nil {
		t.Errorf("expected compatible, got: %v", err)
	}
}

func TestCompatibility_FailsCount(t *testing.T) {
	req := types.ProfileGPURequirement{Count: 4}
	gpus := []RunnerGPUInfo{
		gpu(0, types.GPUVendorNVIDIA, gpuarch.ArchNVIDIAHopper, "H100", 80<<30),
		gpu(1, types.GPUVendorNVIDIA, gpuarch.ArchNVIDIAHopper, "H100", 80<<30),
	}
	err := Compatibility(req, gpus)
	if !IsIncompatibility(err) {
		t.Fatalf("expected IncompatibilityReason, got %v", err)
	}
	if r, _ := err.(*IncompatibilityReason); r.Constraint != "count" {
		t.Errorf("constraint: got %q, want count", r.Constraint)
	}
}

func TestCompatibility_FailsVendor(t *testing.T) {
	req := types.ProfileGPURequirement{Count: 1, Vendor: types.GPUVendorNVIDIA}
	gpus := []RunnerGPUInfo{
		gpu(0, types.GPUVendorAMD, gpuarch.ArchAMDCDNA3, "AMD MI300X", 192<<30),
	}
	err := Compatibility(req, gpus)
	r, ok := err.(*IncompatibilityReason)
	if !ok {
		t.Fatalf("expected IncompatibilityReason, got %v", err)
	}
	if r.Constraint != "vendor" {
		t.Errorf("constraint: got %q, want vendor", r.Constraint)
	}
}

func TestCompatibility_FailsArchitecture(t *testing.T) {
	req := types.ProfileGPURequirement{
		Count:         1,
		Architectures: []string{gpuarch.ArchNVIDIABlackwell},
	}
	gpus := []RunnerGPUInfo{
		gpu(0, types.GPUVendorNVIDIA, gpuarch.ArchNVIDIAHopper, "H100", 80<<30),
	}
	err := Compatibility(req, gpus)
	r, ok := err.(*IncompatibilityReason)
	if !ok {
		t.Fatalf("expected IncompatibilityReason, got %v", err)
	}
	if r.Constraint != "architecture" {
		t.Errorf("constraint: got %q, want architecture", r.Constraint)
	}
}

func TestCompatibility_FailsModelMatch(t *testing.T) {
	req := types.ProfileGPURequirement{Count: 1, ModelMatch: "^NVIDIA H100"}
	gpus := []RunnerGPUInfo{
		gpu(0, types.GPUVendorNVIDIA, gpuarch.ArchNVIDIAAmpere, "NVIDIA A100 80GB", 80<<30),
	}
	err := Compatibility(req, gpus)
	r, ok := err.(*IncompatibilityReason)
	if !ok {
		t.Fatalf("expected IncompatibilityReason, got %v", err)
	}
	if r.Constraint != "model_match" {
		t.Errorf("constraint: got %q, want model_match", r.Constraint)
	}
}

func TestCompatibility_FailsMinVRAM(t *testing.T) {
	req := types.ProfileGPURequirement{Count: 1, MinVRAMBytes: 80 << 30}
	gpus := []RunnerGPUInfo{
		gpu(0, types.GPUVendorNVIDIA, gpuarch.ArchNVIDIAAmpere, "RTX A6000", 48<<30),
	}
	err := Compatibility(req, gpus)
	r, ok := err.(*IncompatibilityReason)
	if !ok {
		t.Fatalf("expected IncompatibilityReason, got %v", err)
	}
	if r.Constraint != "min_vram" {
		t.Errorf("constraint: got %q, want min_vram", r.Constraint)
	}
}

func TestCompatibility_PermissiveAnyVendor(t *testing.T) {
	// Profile with only Count=1 and nothing else — should match any GPU.
	req := types.ProfileGPURequirement{Count: 1}
	gpus := []RunnerGPUInfo{
		gpu(0, types.GPUVendorAMD, gpuarch.ArchAMDRDNA3, "RX 7900 XTX", 24<<30),
	}
	if err := Compatibility(req, gpus); err != nil {
		t.Errorf("permissive profile should match any GPU: %v", err)
	}
}

func TestCompatibility_PermissiveAnyNVIDIADev(t *testing.T) {
	// "Any NVIDIA dev box, ≥24 GiB" — vendor + min_vram only.
	req := types.ProfileGPURequirement{
		Count:        1,
		Vendor:       types.GPUVendorNVIDIA,
		MinVRAMBytes: 24 << 30,
	}
	gpus := []RunnerGPUInfo{
		gpu(0, types.GPUVendorNVIDIA, gpuarch.ArchNVIDIAAda, "NVIDIA RTX 4090", 24<<30),
	}
	if err := Compatibility(req, gpus); err != nil {
		t.Errorf("expected compatible: %v", err)
	}
}

func TestCompatibility_FailsFastInDeclaredOrder(t *testing.T) {
	// A profile that fails ALL constraints should report `count` (the first).
	req := types.ProfileGPURequirement{
		Count:         8,
		Vendor:        types.GPUVendorNVIDIA,
		Architectures: []string{gpuarch.ArchNVIDIABlackwell},
		ModelMatch:    "Blackwell",
		MinVRAMBytes:  192 << 30,
	}
	gpus := []RunnerGPUInfo{
		gpu(0, types.GPUVendorAMD, gpuarch.ArchAMDRDNA2, "RX 6900", 16<<30),
	}
	err := Compatibility(req, gpus)
	r, _ := err.(*IncompatibilityReason)
	if r == nil || r.Constraint != "count" {
		t.Errorf("expected count failure first, got %v", r)
	}
}

func TestCompatibility_BadModelMatchRegex(t *testing.T) {
	req := types.ProfileGPURequirement{Count: 1, ModelMatch: "[unclosed"}
	gpus := []RunnerGPUInfo{
		gpu(0, types.GPUVendorNVIDIA, gpuarch.ArchNVIDIAHopper, "H100", 80<<30),
	}
	err := Compatibility(req, gpus)
	r, ok := err.(*IncompatibilityReason)
	if !ok || r.Constraint != "model_match" {
		t.Errorf("expected model_match failure for bad regex, got %v", err)
	}
}

func TestFilterCompatible(t *testing.T) {
	hopper80 := &types.RunnerProfile{
		Name: "hopper80",
		GPURequirement: types.ProfileGPURequirement{
			Count: 1, Vendor: types.GPUVendorNVIDIA,
			Architectures: []string{gpuarch.ArchNVIDIAHopper}, MinVRAMBytes: 80 << 30,
		},
	}
	anyAmpere := &types.RunnerProfile{
		Name: "anyAmpere",
		GPURequirement: types.ProfileGPURequirement{
			Count: 1, Vendor: types.GPUVendorNVIDIA,
			Architectures: []string{gpuarch.ArchNVIDIAAmpere},
		},
	}
	tinyAnything := &types.RunnerProfile{
		Name:           "tinyAnything",
		GPURequirement: types.ProfileGPURequirement{Count: 1},
	}
	amdMI300X := &types.RunnerProfile{
		Name: "amdMI300X",
		GPURequirement: types.ProfileGPURequirement{
			Count: 1, Vendor: types.GPUVendorAMD,
			Architectures: []string{gpuarch.ArchAMDCDNA3},
		},
	}

	// Runner: single H100.
	runner := []RunnerGPUInfo{
		gpu(0, types.GPUVendorNVIDIA, gpuarch.ArchNVIDIAHopper, "NVIDIA H100 80GB HBM3", 80<<30),
	}
	got := FilterCompatible([]*types.RunnerProfile{hopper80, anyAmpere, tinyAnything, amdMI300X}, runner)
	if len(got) != 2 {
		t.Fatalf("expected 2 compatible (hopper80, tinyAnything), got %d", len(got))
	}
	names := map[string]bool{got[0].Name: true, got[1].Name: true}
	if !names["hopper80"] || !names["tinyAnything"] {
		t.Errorf("compatible set: got %v, want hopper80+tinyAnything", names)
	}
}

func TestIndirectArchCheck(t *testing.T) {
	cases := []struct {
		allowed []string
		vendor  types.GPUVendor
		input   string
		want    bool
	}{
		{nil, types.GPUVendorNVIDIA, "9.0", true}, // empty allowed = always ok
		{[]string{gpuarch.ArchNVIDIAHopper}, types.GPUVendorNVIDIA, "9.0", true},
		{[]string{gpuarch.ArchNVIDIAHopper}, types.GPUVendorNVIDIA, "8.0", false},
		{[]string{gpuarch.ArchAMDCDNA3}, types.GPUVendorAMD, "gfx942", true},
		{[]string{gpuarch.ArchAMDCDNA3}, types.GPUVendorAMD, "gfx90a", false},
		{[]string{gpuarch.ArchNVIDIAHopper}, types.GPUVendorNVIDIA, "garbage", false},
	}
	for _, tc := range cases {
		got := IndirectArchCheck(tc.allowed, tc.vendor, tc.input)
		if got != tc.want {
			t.Errorf("IndirectArchCheck(%v, %s, %q) = %v, want %v", tc.allowed, tc.vendor, tc.input, got, tc.want)
		}
	}
}
