package desktop

import (
	"io"
	"log/slog"
	"strings"
	"testing"
)

// withStubbedGPUVendor pins detectGPUVendor for the duration of the test.
func withStubbedGPUVendor(t *testing.T, vendor GPUVendor) {
	t.Helper()
	original := detectGPUVendor
	detectGPUVendor = func() GPUVendor { return vendor }
	t.Cleanup(func() { detectGPUVendor = original })
}

func newStreamerForPipeline() *VideoStreamer {
	return &VideoStreamer{
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		nodeID:    48,
		videoMode: VideoModePlugin,
		config:    StreamConfig{Width: 1920, Height: 1080, FPS: 60, Bitrate: 8294},
	}
}

// The 2026-07-28 incident in one test: NVIDIA hardware whose nvh264enc has
// stopped registering (because the GPU is exhausted) must NOT be reclassified as
// AMD/Intel and sent down the always-copy=true path, which delivers no frames.
func TestBuildPipeline_NvidiaWithoutNvencFailsLoudly(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")
	t.Setenv("SWAYSOCK", "")
	t.Setenv("HELIX_ENCODER", "")
	withStubbedGPUVendor(t, GPUVendorNVIDIA)
	withStubbedGstElements(t, "openh264enc") // no nvh264enc, no vsockenc

	v := newStreamerForPipeline()
	got := v.buildPipelineString("openh264")

	if got != "" {
		t.Fatalf("expected no pipeline on NVIDIA-without-NVENC, got %q", got)
	}
	if v.pipelineErr == nil {
		t.Fatal("expected pipelineErr to be set so the client sees a real error")
	}
	if strings.Contains(got, "always-copy=true") {
		t.Fatal("NVIDIA hardware must never take the AMD/Intel always-copy path")
	}
}

func TestBuildPipeline_NvidiaWithNvencUsesZeroCopy(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")
	t.Setenv("SWAYSOCK", "")
	withStubbedGPUVendor(t, GPUVendorNVIDIA)
	withStubbedGstElements(t, "nvh264enc")

	v := newStreamerForPipeline()
	got := v.buildPipelineString("nvenc")

	if v.pipelineErr != nil {
		t.Fatalf("unexpected pipelineErr: %v", v.pipelineErr)
	}
	if !strings.Contains(got, "pipewirezerocopysrc") || !strings.Contains(got, "buffer-type=dmabuf") {
		t.Fatalf("expected GNOME+NVIDIA zero-copy DMA-BUF source, got %q", got)
	}
	if strings.Contains(got, "always-copy=true") {
		t.Fatalf("NVIDIA must not use the always-copy path, got %q", got)
	}
}

func TestBuildPipeline_AmdUsesNativePipewiresrc(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")
	t.Setenv("SWAYSOCK", "")
	withStubbedGPUVendor(t, GPUVendorAMD)
	withStubbedGstElements(t, "vah264enc")

	v := newStreamerForPipeline()
	got := v.buildPipelineString("vaapi")

	if v.pipelineErr != nil {
		t.Fatalf("unexpected pipelineErr: %v", v.pipelineErr)
	}
	if !strings.Contains(got, "pipewiresrc") || !strings.Contains(got, "always-copy=true") {
		t.Fatalf("expected AMD native pipewiresrc with always-copy, got %q", got)
	}
}

// An AMD host that happens to have nvh264enc registered (a mixed-GPU box, or the
// plugin present without the hardware) must still take the AMD path. The old
// code keyed off the element and would have chosen NVIDIA here.
func TestBuildPipeline_AmdWithNvencElementStillUsesAmdPath(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")
	t.Setenv("SWAYSOCK", "")
	withStubbedGPUVendor(t, GPUVendorAMD)
	withStubbedGstElements(t, "nvh264enc", "vah264enc")

	v := newStreamerForPipeline()
	got := v.buildPipelineString("vaapi")

	if !strings.Contains(got, "always-copy=true") {
		t.Fatalf("expected AMD path to be chosen from hardware, got %q", got)
	}
}

func TestBuildPipeline_SwayUsesWaylandCapture(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "sway")
	withStubbedGPUVendor(t, GPUVendorNVIDIA)
	withStubbedGstElements(t, "nvh264enc")

	v := newStreamerForPipeline()
	got := v.buildPipelineString("nvenc")

	if v.pipelineErr != nil {
		t.Fatalf("unexpected pipelineErr: %v", v.pipelineErr)
	}
	if !strings.Contains(got, "capture-source=wayland") || !strings.Contains(got, "buffer-type=shm") {
		t.Fatalf("expected Sway wayland/shm capture, got %q", got)
	}
}

func TestBuildPipeline_MacOSVirtioGpuUnaffectedByVendor(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")
	t.Setenv("SWAYSOCK", "")
	withStubbedGPUVendor(t, GPUVendorUnknown)
	withStubbedGstElements(t, "vsockenc")

	v := newStreamerForPipeline()
	got := v.buildPipelineString("vsock")

	if v.pipelineErr != nil {
		t.Fatalf("unexpected pipelineErr: %v", v.pipelineErr)
	}
	if !strings.Contains(got, "pipewiresrc") || strings.Contains(got, "always-copy=true") {
		t.Fatalf("expected virtio-gpu native pipewiresrc without always-copy, got %q", got)
	}
}

// An explicit HELIX_ENCODER override is an operator decision and must still be
// honoured on NVIDIA hardware — the loud failure only guards auto-detect.
func TestBuildPipeline_NvidiaHonoursExplicitEncoderOverride(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")
	t.Setenv("SWAYSOCK", "")
	t.Setenv("HELIX_ENCODER", "x264")
	withStubbedGPUVendor(t, GPUVendorNVIDIA)
	withStubbedGstElements(t, "x264enc")

	v := newStreamerForPipeline()
	got := v.buildPipelineString("x264")

	if v.pipelineErr != nil {
		t.Fatalf("explicit override must not hard-fail, got %v", v.pipelineErr)
	}
	if got == "" {
		t.Fatal("expected a pipeline for an explicitly overridden encoder")
	}
}

func TestCompositorName(t *testing.T) {
	if compositorName(true) != "sway" || compositorName(false) != "gnome" {
		t.Fatal("compositorName should distinguish sway from gnome")
	}
}
