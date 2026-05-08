package gpuarch

import "testing"

func TestFromNVIDIAComputeCapability(t *testing.T) {
	cases := []struct {
		cc   string
		want string
	}{
		{"12.0", ArchNVIDIABlackwell}, // B100
		{"12.1", ArchNVIDIABlackwell},
		{"9.0", ArchNVIDIAHopper}, // H100, H200
		{"9.1", ArchNVIDIAHopper},
		{"8.9", ArchNVIDIAAda}, // RTX 40xx, L4, L40
		{"8.6", ArchNVIDIAAmpere},
		{"8.0", ArchNVIDIAAmpere}, // A100
		{"7.5", ArchNVIDIATuring}, // T4
		{"7.0", ArchNVIDIAVolta},  // V100
		{"7.2", ArchNVIDIAVolta},
		{"6.1", ArchNVIDIAPascal}, // GTX 1080
		{"", ""},                  // empty input
		{" 9.0 ", ArchNVIDIAHopper},
		{"5.0", ""},  // Maxwell — pre-Pascal, unsupported
		{"99.0", ""}, // future
		{"abc", ""},  // junk
	}
	for _, tc := range cases {
		got := FromNVIDIAComputeCapability(tc.cc)
		if got != tc.want {
			t.Errorf("FromNVIDIAComputeCapability(%q) = %q, want %q", tc.cc, got, tc.want)
		}
	}
}

func TestFromAMDGFX(t *testing.T) {
	cases := []struct {
		gfx  string
		want string
	}{
		{"gfx942", ArchAMDCDNA3},  // MI300X
		{"gfx90a", ArchAMDCDNA2},  // MI250
		{"gfx908", ArchAMDCDNA1},  // MI100
		{"gfx1100", ArchAMDRDNA3}, // RX 7900 XTX
		{"gfx1101", ArchAMDRDNA3},
		{"gfx1102", ArchAMDRDNA3},
		{"gfx1030", ArchAMDRDNA2}, // RX 6900 XT
		{"gfx906", ArchAMDVega},   // MI50
		{"gfx900", ArchAMDVega},
		{"GFX942", ArchAMDCDNA3}, // case insensitive
		{" gfx942 ", ArchAMDCDNA3},
		{"", ""},
		{"gfx9999", ""}, // future
		{"hopper", ""},  // not a gfx
	}
	for _, tc := range cases {
		got := FromAMDGFX(tc.gfx)
		if got != tc.want {
			t.Errorf("FromAMDGFX(%q) = %q, want %q", tc.gfx, got, tc.want)
		}
	}
}

func TestIsVendor(t *testing.T) {
	if !IsNVIDIA(ArchNVIDIAHopper) {
		t.Errorf("IsNVIDIA(hopper) = false, want true")
	}
	if IsNVIDIA(ArchAMDCDNA3) {
		t.Errorf("IsNVIDIA(cdna3) = true, want false")
	}
	if !IsAMD(ArchAMDCDNA3) {
		t.Errorf("IsAMD(cdna3) = false, want true")
	}
	if IsAMD(ArchNVIDIAHopper) {
		t.Errorf("IsAMD(hopper) = true, want false")
	}
	if IsNVIDIA("") || IsAMD("") {
		t.Errorf("empty arch should not match either vendor")
	}
}
