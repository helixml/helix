package hydra

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestParallelCopyDir_BasicFiles(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	// Create files with known content
	os.WriteFile(filepath.Join(src, "file1.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(src, "file2.txt"), []byte("world"), 0600)

	if err := parallelCopyDir(src, dst, 4); err != nil {
		t.Fatalf("parallelCopyDir failed: %v", err)
	}

	// Verify content
	for _, tc := range []struct {
		name    string
		content string
	}{
		{"file1.txt", "hello"},
		{"file2.txt", "world"},
	} {
		data, err := os.ReadFile(filepath.Join(dst, tc.name))
		if err != nil {
			t.Errorf("failed to read %s: %v", tc.name, err)
			continue
		}
		if string(data) != tc.content {
			t.Errorf("%s: got %q, want %q", tc.name, string(data), tc.content)
		}
	}
}

func TestParallelCopyDir_Permissions(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	// Create files with various permissions
	os.WriteFile(filepath.Join(src, "readonly.txt"), []byte("ro"), 0444)
	os.WriteFile(filepath.Join(src, "executable.sh"), []byte("#!/bin/sh"), 0755)
	os.WriteFile(filepath.Join(src, "private.key"), []byte("secret"), 0600)

	if err := parallelCopyDir(src, dst, 4); err != nil {
		t.Fatalf("parallelCopyDir failed: %v", err)
	}

	// Verify permissions are preserved
	for _, tc := range []struct {
		name string
		perm os.FileMode
	}{
		{"readonly.txt", 0444},
		{"executable.sh", 0755},
		{"private.key", 0600},
	} {
		info, err := os.Stat(filepath.Join(dst, tc.name))
		if err != nil {
			t.Errorf("failed to stat %s: %v", tc.name, err)
			continue
		}
		got := info.Mode().Perm()
		if got != tc.perm {
			t.Errorf("%s: permissions got %o, want %o", tc.name, got, tc.perm)
		}
	}
}

func TestParallelCopyDir_NestedDirectories(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	// Create nested structure mimicking Docker overlay2
	os.MkdirAll(filepath.Join(src, "overlay2", "layer1", "diff"), 0755)
	os.MkdirAll(filepath.Join(src, "overlay2", "layer2", "diff"), 0755)
	os.MkdirAll(filepath.Join(src, "image", "overlay2"), 0755)
	os.WriteFile(filepath.Join(src, "overlay2", "layer1", "diff", "data.bin"), []byte("layer1data"), 0644)
	os.WriteFile(filepath.Join(src, "overlay2", "layer2", "diff", "data.bin"), []byte("layer2data"), 0644)
	os.WriteFile(filepath.Join(src, "image", "overlay2", "repositories.json"), []byte("{}"), 0644)

	if err := parallelCopyDir(src, dst, 4); err != nil {
		t.Fatalf("parallelCopyDir failed: %v", err)
	}

	// Verify nested files
	for _, tc := range []struct {
		path    string
		content string
	}{
		{"overlay2/layer1/diff/data.bin", "layer1data"},
		{"overlay2/layer2/diff/data.bin", "layer2data"},
		{"image/overlay2/repositories.json", "{}"},
	} {
		data, err := os.ReadFile(filepath.Join(dst, tc.path))
		if err != nil {
			t.Errorf("failed to read %s: %v", tc.path, err)
			continue
		}
		if string(data) != tc.content {
			t.Errorf("%s: got %q, want %q", tc.path, string(data), tc.content)
		}
	}
}

func TestParallelCopyDir_Symlinks(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	// Create a file and a symlink to it
	os.WriteFile(filepath.Join(src, "target.txt"), []byte("target"), 0644)
	os.Symlink("target.txt", filepath.Join(src, "link.txt"))

	// Create a directory symlink
	os.MkdirAll(filepath.Join(src, "realdir"), 0755)
	os.WriteFile(filepath.Join(src, "realdir", "inside.txt"), []byte("inside"), 0644)
	os.Symlink("realdir", filepath.Join(src, "linkdir"))

	if err := parallelCopyDir(src, dst, 4); err != nil {
		t.Fatalf("parallelCopyDir failed: %v", err)
	}

	// Verify symlink is preserved (not dereferenced)
	linkTarget, err := os.Readlink(filepath.Join(dst, "link.txt"))
	if err != nil {
		t.Fatalf("failed to readlink: %v", err)
	}
	if linkTarget != "target.txt" {
		t.Errorf("symlink target: got %q, want %q", linkTarget, "target.txt")
	}

	// Verify file content through symlink
	data, err := os.ReadFile(filepath.Join(dst, "link.txt"))
	if err != nil {
		t.Fatalf("failed to read through symlink: %v", err)
	}
	if string(data) != "target" {
		t.Errorf("symlink content: got %q, want %q", string(data), "target")
	}

	// Verify directory symlink
	dirTarget, err := os.Readlink(filepath.Join(dst, "linkdir"))
	if err != nil {
		t.Fatalf("failed to readlink dir: %v", err)
	}
	if dirTarget != "realdir" {
		t.Errorf("dir symlink target: got %q, want %q", dirTarget, "realdir")
	}
}

func TestParallelCopyDir_Overlay2Splitting(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	// Create overlay2 with many layers — verifies the splitting logic
	// creates the parent overlay2 dir and copies each layer independently
	for i := 0; i < 20; i++ {
		layerDir := filepath.Join(src, "overlay2", filepath.Base(t.TempDir()))
		os.MkdirAll(filepath.Join(layerDir, "diff"), 0755)
		os.WriteFile(filepath.Join(layerDir, "diff", "file"), []byte("data"), 0644)
		os.WriteFile(filepath.Join(layerDir, "link"), []byte("link"), 0644)
	}

	// Also add a non-overlay2 dir
	os.MkdirAll(filepath.Join(src, "image"), 0755)
	os.WriteFile(filepath.Join(src, "image", "metadata"), []byte("meta"), 0644)

	if err := parallelCopyDir(src, dst, 4); err != nil {
		t.Fatalf("parallelCopyDir failed: %v", err)
	}

	// Count overlay2 layers in destination
	entries, err := os.ReadDir(filepath.Join(dst, "overlay2"))
	if err != nil {
		t.Fatalf("failed to read dst overlay2: %v", err)
	}

	srcEntries, _ := os.ReadDir(filepath.Join(src, "overlay2"))
	if len(entries) != len(srcEntries) {
		t.Errorf("overlay2 layer count: got %d, want %d", len(entries), len(srcEntries))
	}

	// Verify non-overlay2 dir
	data, err := os.ReadFile(filepath.Join(dst, "image", "metadata"))
	if err != nil {
		t.Fatalf("failed to read image/metadata: %v", err)
	}
	if string(data) != "meta" {
		t.Errorf("image/metadata: got %q, want %q", string(data), "meta")
	}
}

func TestParallelCopyDir_EmptyDir(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	// Create empty subdirectories
	os.MkdirAll(filepath.Join(src, "empty1"), 0755)
	os.MkdirAll(filepath.Join(src, "empty2"), 0700)

	if err := parallelCopyDir(src, dst, 4); err != nil {
		t.Fatalf("parallelCopyDir failed: %v", err)
	}

	// Verify empty dirs exist
	for _, name := range []string{"empty1", "empty2"} {
		info, err := os.Stat(filepath.Join(dst, name))
		if err != nil {
			t.Errorf("missing dir %s: %v", name, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", name)
		}
	}
}

func TestParallelCopyDir_PreservesOwnership(t *testing.T) {
	// This test verifies that cp -a preserves ownership information.
	// When running as non-root, ownership will be the current user,
	// but the test structure verifies the stat fields match.
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	os.WriteFile(filepath.Join(src, "owned.txt"), []byte("test"), 0644)

	if err := parallelCopyDir(src, dst, 4); err != nil {
		t.Fatalf("parallelCopyDir failed: %v", err)
	}

	srcStat, _ := os.Stat(filepath.Join(src, "owned.txt"))
	dstStat, err := os.Stat(filepath.Join(dst, "owned.txt"))
	if err != nil {
		t.Fatalf("failed to stat dst file: %v", err)
	}

	srcSys := srcStat.Sys().(*syscall.Stat_t)
	dstSys := dstStat.Sys().(*syscall.Stat_t)

	if srcSys.Uid != dstSys.Uid {
		t.Errorf("UID mismatch: src=%d, dst=%d", srcSys.Uid, dstSys.Uid)
	}
	if srcSys.Gid != dstSys.Gid {
		t.Errorf("GID mismatch: src=%d, dst=%d", srcSys.Gid, dstSys.Gid)
	}
}

func TestParallelCopyDir_ErrorOnMissingSrc(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "dst")

	err := parallelCopyDir("/nonexistent/path", dst, 4)
	if err == nil {
		t.Fatal("expected error for missing source, got nil")
	}
}

func TestParallelCopyDir_SingleWorker(t *testing.T) {
	// Verify it works with workers=1 (no parallelism, edge case)
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(src, "b.txt"), []byte("b"), 0644)

	if err := parallelCopyDir(src, dst, 1); err != nil {
		t.Fatalf("parallelCopyDir with 1 worker failed: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dst, "a.txt"))
	if string(data) != "a" {
		t.Errorf("a.txt: got %q, want %q", string(data), "a")
	}
}
