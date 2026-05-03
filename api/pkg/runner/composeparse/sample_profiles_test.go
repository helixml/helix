package composeparse

import (
	"os"
	"path/filepath"
	"testing"
)

// Make sure all sample profiles in design/sample-profiles/ parse cleanly
// and produce the expected counts. This is documentation-as-test: a future
// agent who breaks the parser will see exactly which sample profile broke.
func TestParse_SampleProfiles(t *testing.T) {
	cases := []struct {
		file        string
		wantModels  int
		wantGPUCount int
	}{
		// 8xH100: 5 services across GPUs 0,1,2,3,4,5,6 = 7 distinct GPUs.
		// (services qwen3-vl-embedding and qwen3-text-embedding both use GPU 0.)
		{"8xH100-vllm.yaml", 5, 7},
		// 8x RTX PRO 6000 Blackwell multi-model profile: same 5-service
		// shape as the 8xH100 layout (qwen3 embeddings sharing GPU 0,
		// qwen3.5-35b on GPU 1, minimax-m2.7 on GPUs 2-5 TP=4, gemma-4-26b
		// on GPU 6) — leaves GPU 7 free so Hydra can pin desktops to it
		// (Decision 15). Crucial property: GPUCount = 7 not 8, so the
		// compatibility check on an 8-GPU host passes with desktop headroom.
		{"8xRTX6000Pro-vllm.yaml", 5, 7},
		// 4x A100 80GB multi-model layout. 4 services on GPUs 0,1,2; GPU 3
		// reserved for desktops (software-encoded — A100 has no NVENC and
		// no display engine, so desktop video falls back to libx264 in
		// GStreamer; CPU-bound but workable). GPUCount = 3 so the
		// compatibility check on a 4-GPU host passes with explicit
		// desktop headroom.
		{"4xA100-vllm.yaml", 4, 3},
		// 4x L40S 48GB multi-model layout. Same 4-services-on-3-GPUs shape
		// as the A100 profile; sized for L40S's 48GB VRAM so different
		// model choices. Unlike A100, L40S has full NVENC + display
		// engine, so the reserved GPU 3 gets hardware-accelerated desktop
		// streaming. Designed to be deployed identically across a fleet
		// of nodes (sandboxes serving the same model names → inference
		// router round-robins).
		{"4xL40S-vllm.yaml", 4, 3},
		// 8x MI300X 192GB (CDNA-3 compute-only) flagship layout. One
		// service: DeepSeek-V4-Pro 862B FP8 with TP=8 across all GPUs.
		// GPUCount = 8 (no desktop reserve possible — CDNA can't render).
		{"8xMI300X-deepseek-v4-pro.yaml", 1, 8},
		// 4-GPU Blackwell single TP=4 model on GPUs 0,1,2,3.
		{"any-nvidia-blackwell-4gpu.yaml", 1, 4},
		// Single GPU dev profile.
		{"any-nvidia-dev-single-gpu.yaml", 1, 1},
		// AMD MI300X, single GPU (one renderD entry).
		{"amd-mi300x-vllm.yaml", 1, 1},
		// Tiny dev spike.
		{"dev-spike-tiny.yaml", 1, 1},
	}

	root := repoRootForTests(t)
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, "design", "sample-profiles", tc.file)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			r, err := Parse(data)
			if err != nil {
				t.Fatalf("parse %s: %v", tc.file, err)
			}
			if len(r.Models) != tc.wantModels {
				t.Errorf("%s: got %d models (%v), want %d", tc.file, len(r.Models), r.Models, tc.wantModels)
			}
			if r.GPUCount != tc.wantGPUCount {
				t.Errorf("%s: got %d GPUs, want %d", tc.file, r.GPUCount, tc.wantGPUCount)
			}
		})
	}
}

// repoRootForTests walks up from this file's location to find the repo root
// (the dir containing go.mod). Standard pattern for tests that need to read
// fixtures outside their own package.
func repoRootForTests(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found above %s", wd)
		}
		dir = parent
	}
}
