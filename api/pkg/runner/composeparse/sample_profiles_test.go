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
