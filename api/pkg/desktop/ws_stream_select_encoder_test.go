package desktop

import (
	"io"
	"log/slog"
	"testing"
)

// withStubbedGstElements installs a stub for checkGstElement for the
// duration of the test. The stub reports true only for elements in
// `present` and false otherwise, letting us pin the selector's
// auto-detect behaviour without needing a real GStreamer install.
func withStubbedGstElements(t *testing.T, present ...string) {
	t.Helper()
	set := map[string]bool{}
	for _, e := range present {
		set[e] = true
	}
	original := checkGstElement
	checkGstElement = func(element string) bool { return set[element] }
	t.Cleanup(func() { checkGstElement = original })
}

// newStreamerForSelectEncoder produces a minimally-populated
// VideoStreamer suitable for calling selectEncoder(). Only the logger
// is read along that path; everything else is zero-value.
func newStreamerForSelectEncoder() *VideoStreamer {
	return &VideoStreamer{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestSelectEncoder_HelixEncoderOverrides(t *testing.T) {
	cases := []struct {
		name      string
		override  string
		available []string
		want      string
	}{
		{"vsock override", "vsock", nil, "vsock"},
		{"openh264 override", "openh264", nil, "openh264"},
		{"x264 override", "x264", nil, "x264"},
		{"nvenc override", "nvenc", nil, "nvenc"},
		{"vaapi override with element available", "vaapi", []string{"vah264enc"}, "vaapi"},
		{"vaapi-legacy override with element available", "vaapi-legacy", []string{"vaapih264enc"}, "vaapi-legacy"},
		{"uppercase value is normalised", "VAAPI", []string{"vah264enc"}, "vaapi"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HELIX_ENCODER", tc.override)
			t.Setenv("LIBVA_DRIVER_NAME", "")
			withStubbedGstElements(t, tc.available...)
			got := newStreamerForSelectEncoder().selectEncoder()
			if got != tc.want {
				t.Fatalf("HELIX_ENCODER=%q: got %q, want %q", tc.override, got, tc.want)
			}
		})
	}
}

func TestSelectEncoder_VaapiOverrideFallsBackWhenElementMissing(t *testing.T) {
	// If the operator asks for VA-API but the element isn't installed,
	// the override should fail open to auto-detect rather than returning
	// a string the pipeline builder can't satisfy.
	t.Setenv("HELIX_ENCODER", "vaapi")
	t.Setenv("LIBVA_DRIVER_NAME", "")
	withStubbedGstElements(t, "openh264enc")

	got := newStreamerForSelectEncoder().selectEncoder()
	if got != "openh264" {
		t.Fatalf("expected fallback to openh264 when vah264enc missing, got %q", got)
	}
}

func TestSelectEncoder_VaapiLegacyOverrideFallsBackWhenElementMissing(t *testing.T) {
	t.Setenv("HELIX_ENCODER", "vaapi-legacy")
	t.Setenv("LIBVA_DRIVER_NAME", "")
	withStubbedGstElements(t, "openh264enc")

	got := newStreamerForSelectEncoder().selectEncoder()
	if got != "openh264" {
		t.Fatalf("expected fallback to openh264 when vaapih264enc missing, got %q", got)
	}
}

func TestSelectEncoder_UnknownOverrideFallsThrough(t *testing.T) {
	t.Setenv("HELIX_ENCODER", "banana")
	t.Setenv("LIBVA_DRIVER_NAME", "")
	withStubbedGstElements(t, "nvh264enc")

	got := newStreamerForSelectEncoder().selectEncoder()
	if got != "nvenc" {
		t.Fatalf("expected auto-detect to pick nvenc, got %q", got)
	}
}

func TestSelectEncoder_AutoDetectSkipsVaapiOnAMD(t *testing.T) {
	// LIBVA_DRIVER_NAME=radeonsi marks AMD; VA-API must be skipped in
	// auto-detect. OpenH264 is the expected fallback.
	t.Setenv("HELIX_ENCODER", "")
	t.Setenv("LIBVA_DRIVER_NAME", "radeonsi")
	withStubbedGstElements(t, "vah264enc", "vaapih264enc", "openh264enc")

	got := newStreamerForSelectEncoder().selectEncoder()
	if got != "openh264" {
		t.Fatalf("expected AMD auto-detect to fall through to openh264, got %q", got)
	}
}

func TestSelectEncoder_AutoDetectPicksVaapiOnIntel(t *testing.T) {
	// On non-AMD the VA-API path is allowed.
	t.Setenv("HELIX_ENCODER", "")
	t.Setenv("LIBVA_DRIVER_NAME", "iHD")
	withStubbedGstElements(t, "vah264enc", "openh264enc")

	got := newStreamerForSelectEncoder().selectEncoder()
	if got != "vaapi" {
		t.Fatalf("expected Intel auto-detect to pick vaapi, got %q", got)
	}
}
