package gpudetect

import (
	"testing"

	"github.com/helixml/helix/api/pkg/runner/gpuarch"
	"github.com/helixml/helix/api/pkg/types"
)

// Real nvidia-smi output captured from RTX 2000 Ada (16 GiB).
const nvidiaSmiSample = `0, NVIDIA RTX 2000 Ada Generation, 16380, 4021, 12359, 570.211.01, 8.9
1, NVIDIA H100 80GB HBM3, 81920, 0, 81920, 570.211.01, 9.0`

func TestParseNVIDIASmiCSV(t *testing.T) {
	gpus := parseNVIDIASmiCSV(nvidiaSmiSample)
	if len(gpus) != 2 {
		t.Fatalf("got %d GPUs, want 2", len(gpus))
	}

	g0 := gpus[0]
	if g0.Index != 0 {
		t.Errorf("g0.Index: got %d, want 0", g0.Index)
	}
	if g0.ModelName != "NVIDIA RTX 2000 Ada Generation" {
		t.Errorf("g0.ModelName: got %q", g0.ModelName)
	}
	if g0.TotalMemory != 16380*1024*1024 {
		t.Errorf("g0.TotalMemory: got %d, want %d", g0.TotalMemory, 16380*1024*1024)
	}
	if g0.UsedMemory != 4021*1024*1024 {
		t.Errorf("g0.UsedMemory: got %d", g0.UsedMemory)
	}
	if g0.FreeMemory != 12359*1024*1024 {
		t.Errorf("g0.FreeMemory: got %d", g0.FreeMemory)
	}
	if g0.DriverVersion != "570.211.01" {
		t.Errorf("g0.DriverVersion: got %q", g0.DriverVersion)
	}
	if g0.Vendor != types.GPUVendorNVIDIA {
		t.Errorf("g0.Vendor: got %q", g0.Vendor)
	}
	if g0.ComputeCapability != "8.9" {
		t.Errorf("g0.ComputeCapability: got %q", g0.ComputeCapability)
	}
	if g0.Architecture != gpuarch.ArchNVIDIAAda {
		t.Errorf("g0.Architecture: got %q, want %q", g0.Architecture, gpuarch.ArchNVIDIAAda)
	}

	g1 := gpus[1]
	if g1.Architecture != gpuarch.ArchNVIDIAHopper {
		t.Errorf("g1.Architecture: got %q, want hopper", g1.Architecture)
	}
	if g1.TotalMemory != 81920*1024*1024 {
		t.Errorf("g1.TotalMemory: got %d", g1.TotalMemory)
	}
}

func TestParseNVIDIASmiCSV_Empty(t *testing.T) {
	if got := parseNVIDIASmiCSV(""); got != nil {
		t.Errorf("empty input should return nil; got %+v", got)
	}
	if got := parseNVIDIASmiCSV("\n\n  \n"); got != nil {
		t.Errorf("whitespace-only input should return nil; got %+v", got)
	}
}

func TestParseNVIDIASmiCSV_MalformedRowSkipped(t *testing.T) {
	// Row with too few fields gets skipped silently.
	in := "0, NVIDIA RTX, 16380\n1, NVIDIA H100 80GB HBM3, 81920, 0, 81920, 570.211.01, 9.0"
	got := parseNVIDIASmiCSV(in)
	if len(got) != 1 || got[0].Index != 1 {
		t.Errorf("expected 1 valid row (skipping malformed); got %+v", got)
	}
}

func TestSplitCSVTrim(t *testing.T) {
	got := splitCSVTrim(" a , b ,  c  ")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %d, want %d", len(got), len(want))
	}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("got[%d] = %q, want %q", i, got[i], v)
		}
	}
}

// TestNeuronDeviceRe ensures the Neuron device glob matches per-chip device
// nodes (/dev/neuron0, neuron1, ...) and excludes control nodes.
func TestNeuronDeviceRe(t *testing.T) {
	match := []string{"neuron0", "neuron1", "neuron15"}
	noMatch := []string{"neuron", "neuron-rtd", "neuronx", "neuron0a", "nvidia0"}
	for _, m := range match {
		if !neuronDeviceRe.MatchString(m) {
			t.Errorf("expected %q to match neuronDeviceRe", m)
		}
	}
	for _, n := range noMatch {
		if neuronDeviceRe.MatchString(n) {
			t.Errorf("expected %q NOT to match neuronDeviceRe", n)
		}
	}
}
