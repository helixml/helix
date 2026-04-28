package hydra

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImageTag(t *testing.T) {
	tests := []struct {
		image string
		want  string
	}{
		{"helix-ubuntu:abc123", "abc123"},
		{"registry:5000/helix-ubuntu:d1363fb", "d1363fb"},
		{"helix-ubuntu", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			got := imageTag(tt.image)
			if got != tt.want {
				t.Errorf("imageTag(%q) = %q, want %q", tt.image, got, tt.want)
			}
		})
	}
}

func TestResolveRegistryImage(t *testing.T) {
	// Create a temp dir to act as /opt/images for tests
	tmpDir := t.TempDir()

	// Override the runtime-ref path by testing the function behavior
	// Since resolveRegistryImage reads from /opt/images/, we need to test via
	// a helper that accepts a base path. But the function is hardcoded, so we
	// test the logic indirectly by creating actual files in a temp dir and
	// using symlinks, OR we refactor. For now, test the non-file-dependent cases.

	t.Run("non-helix image passes through", func(t *testing.T) {
		got := resolveRegistryImage("nginx:latest")
		if got != "nginx:latest" {
			t.Errorf("expected nginx:latest, got %s", got)
		}
	})

	t.Run("tagless helix image passes through", func(t *testing.T) {
		got := resolveRegistryImage("helix-ubuntu")
		if got != "helix-ubuntu" {
			t.Errorf("expected helix-ubuntu, got %s", got)
		}
	})

	t.Run("helix image with no ref file passes through", func(t *testing.T) {
		// No ref file exists at /opt/images/ so this should return the original
		got := resolveRegistryImage("helix-ubuntu:abc123")
		// This will pass through because /opt/images/helix-ubuntu.runtime-ref doesn't exist
		if got != "helix-ubuntu:abc123" {
			t.Errorf("expected helix-ubuntu:abc123, got %s", got)
		}
	})

	// For file-dependent tests, use resolveRegistryImageWithBase
	t.Run("matching tags returns registry ref", func(t *testing.T) {
		refFile := filepath.Join(tmpDir, "helix-ubuntu.runtime-ref")
		os.WriteFile(refFile, []byte("registry:5000/helix-ubuntu:abc123\n"), 0644)

		got := resolveRegistryImageWithBase("helix-ubuntu:abc123", tmpDir)
		if got != "registry:5000/helix-ubuntu:abc123" {
			t.Errorf("expected registry:5000/helix-ubuntu:abc123, got %s", got)
		}
	})

	t.Run("mismatched tags returns original image", func(t *testing.T) {
		refFile := filepath.Join(tmpDir, "helix-sway.runtime-ref")
		os.WriteFile(refFile, []byte("registry:5000/helix-sway:oldtag\n"), 0644)

		got := resolveRegistryImageWithBase("helix-sway:newtag", tmpDir)
		if got != "helix-sway:newtag" {
			t.Errorf("expected helix-sway:newtag, got %s", got)
		}
	})

	t.Run("empty ref tag returns original image", func(t *testing.T) {
		refFile := filepath.Join(tmpDir, "helix-test.runtime-ref")
		os.WriteFile(refFile, []byte("registry:5000/helix-test\n"), 0644)

		got := resolveRegistryImageWithBase("helix-test:abc123", tmpDir)
		if got != "helix-test:abc123" {
			t.Errorf("expected helix-test:abc123, got %s", got)
		}
	})

	t.Run("empty ref file returns original image", func(t *testing.T) {
		refFile := filepath.Join(tmpDir, "helix-empty.runtime-ref")
		os.WriteFile(refFile, []byte(""), 0644)

		got := resolveRegistryImageWithBase("helix-empty:abc123", tmpDir)
		if got != "helix-empty:abc123" {
			t.Errorf("expected helix-empty:abc123, got %s", got)
		}
	})

	t.Run("ref file does not exist returns original image", func(t *testing.T) {
		got := resolveRegistryImageWithBase("helix-nofile:abc123", tmpDir)
		if got != "helix-nofile:abc123" {
			t.Errorf("expected helix-nofile:abc123, got %s", got)
		}
	})
}

func TestPickDRIDevices(t *testing.T) {
	// Stage a tmp dir as our fake /dev/dri so the helper's filepath.Glob
	// has something deterministic to match. We pass an absolute glob
	// pattern based on this dir.
	dir := t.TempDir()
	for _, name := range []string{"renderD128", "renderD129", "renderD130", "card0", "card1", "card2"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	idx := func(i int) *int { return &i }

	for _, tt := range []struct {
		name     string
		glob     string
		gpuIndex *int
		offset   int
		want     []string
	}{
		{
			name:     "render nil index returns all 3",
			glob:     filepath.Join(dir, "renderD*"),
			gpuIndex: nil,
			offset:   128,
			want:     []string{filepath.Join(dir, "renderD128"), filepath.Join(dir, "renderD129"), filepath.Join(dir, "renderD130")},
		},
		{
			name:     "render index 0 picks renderD128",
			glob:     filepath.Join(dir, "renderD*"),
			gpuIndex: idx(0),
			offset:   128,
			want:     []string{filepath.Join(dir, "renderD128")},
		},
		{
			name:     "render index 2 picks renderD130",
			glob:     filepath.Join(dir, "renderD*"),
			gpuIndex: idx(2),
			offset:   128,
			want:     []string{filepath.Join(dir, "renderD130")},
		},
		{
			name:     "card index 0 picks card0",
			glob:     filepath.Join(dir, "card*"),
			gpuIndex: idx(0),
			offset:   0,
			want:     []string{filepath.Join(dir, "card0")},
		},
		{
			name:     "card index 2 picks card2",
			glob:     filepath.Join(dir, "card*"),
			gpuIndex: idx(2),
			offset:   0,
			want:     []string{filepath.Join(dir, "card2")},
		},
		{
			name:     "render index 99 (no match) returns empty",
			glob:     filepath.Join(dir, "renderD*"),
			gpuIndex: idx(99),
			offset:   128,
			want:     nil,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := pickDRIDevices(tt.glob, tt.gpuIndex, tt.offset)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d devices, want %d: got=%v want=%v", len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d]=%s want=%s", i, got[i], tt.want[i])
				}
			}
		})
	}
}
