package hydra

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"testing"
	"time"
)

// fileEntry captures the complete filesystem state of a single entry
// (file, directory, or symlink) for comparison purposes.
type fileEntry struct {
	RelPath    string      // relative path from root
	Mode       os.FileMode // full mode including type bits
	Size       int64       // file size (0 for dirs/symlinks)
	Content    string      // file content (empty for dirs/symlinks)
	LinkTarget string      // symlink target (empty for files/dirs)
	UID        uint32
	GID        uint32
}

// walkDir recursively walks a directory and returns a sorted list of fileEntries.
// Captures: path, mode, size, content, symlink target, uid, gid.
func walkDir(t *testing.T, root string) []fileEntry {
	t.Helper()
	var entries []fileEntry

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(root, path)
		if rel == "." {
			return nil // skip root itself
		}

		entry := fileEntry{
			RelPath: rel,
			Mode:    info.Mode(),
			Size:    info.Size(),
		}

		// Get ownership
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			entry.UID = stat.Uid
			entry.GID = stat.Gid
		}

		// Capture symlink target (Walk follows symlinks, so use Lstat)
		linfo, lerr := os.Lstat(path)
		if lerr == nil && linfo.Mode()&os.ModeSymlink != 0 {
			entry.Mode = linfo.Mode() // use lstat mode (has symlink bit)
			entry.Size = 0
			target, _ := os.Readlink(path)
			entry.LinkTarget = target
		} else if info.Mode().IsRegular() {
			// Read file content for comparison
			data, _ := os.ReadFile(path)
			entry.Content = string(data)
		}

		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		t.Fatalf("walkDir(%s) failed: %v", root, err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].RelPath < entries[j].RelPath
	})

	return entries
}

// Note: filepath.Walk follows symlinks, so we also need to capture symlinks
// via Lstat. We use a custom walker for the comparison.
func walkDirWithSymlinks(t *testing.T, root string) []fileEntry {
	t.Helper()
	var entries []fileEntry

	var walk func(dir, rel string) error
	walk = func(dir, rel string) error {
		dirEntries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, de := range dirEntries {
			name := de.Name()
			fullPath := filepath.Join(dir, name)
			relPath := filepath.Join(rel, name)
			if rel == "" {
				relPath = name
			}

			// Use Lstat to not follow symlinks
			linfo, err := os.Lstat(fullPath)
			if err != nil {
				return err
			}

			entry := fileEntry{
				RelPath: relPath,
				Mode:    linfo.Mode(),
				Size:    linfo.Size(),
			}

			if stat, ok := linfo.Sys().(*syscall.Stat_t); ok {
				entry.UID = stat.Uid
				entry.GID = stat.Gid
			}

			if linfo.Mode()&os.ModeSymlink != 0 {
				target, _ := os.Readlink(fullPath)
				entry.LinkTarget = target
				entry.Size = 0 // symlink size varies by platform
			} else if linfo.Mode().IsRegular() {
				data, _ := os.ReadFile(fullPath)
				entry.Content = string(data)
			} else if linfo.IsDir() {
				entry.Size = 0 // directory size varies
				if err := walk(fullPath, relPath); err != nil {
					return err
				}
			}

			entries = append(entries, entry)
		}
		return nil
	}

	if err := walk(root, ""); err != nil {
		t.Fatalf("walkDirWithSymlinks(%s) failed: %v", root, err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].RelPath < entries[j].RelPath
	})

	return entries
}

// assertIdenticalFS compares two directory trees and fails if they differ.
// Checks: file count, paths, modes, permissions, content, symlinks, ownership.
func assertIdenticalFS(t *testing.T, src, dst string) {
	t.Helper()
	srcEntries := walkDirWithSymlinks(t, src)
	dstEntries := walkDirWithSymlinks(t, dst)

	if len(srcEntries) != len(dstEntries) {
		t.Errorf("entry count: src=%d, dst=%d", len(srcEntries), len(dstEntries))
		// Print missing/extra for debugging
		srcMap := make(map[string]bool)
		dstMap := make(map[string]bool)
		for _, e := range srcEntries {
			srcMap[e.RelPath] = true
		}
		for _, e := range dstEntries {
			dstMap[e.RelPath] = true
		}
		for p := range srcMap {
			if !dstMap[p] {
				t.Errorf("  missing in dst: %s", p)
			}
		}
		for p := range dstMap {
			if !srcMap[p] {
				t.Errorf("  extra in dst: %s", p)
			}
		}
		return
	}

	for i := range srcEntries {
		s := srcEntries[i]
		d := dstEntries[i]

		if s.RelPath != d.RelPath {
			t.Errorf("path mismatch at index %d: src=%q, dst=%q", i, s.RelPath, d.RelPath)
			continue
		}

		path := s.RelPath

		// Check type bits (file vs dir vs symlink)
		if s.Mode.Type() != d.Mode.Type() {
			t.Errorf("%s: type mismatch: src=%s, dst=%s", path, s.Mode.Type(), d.Mode.Type())
			continue
		}

		// Check permissions
		if s.Mode.Perm() != d.Mode.Perm() {
			t.Errorf("%s: perm mismatch: src=%o, dst=%o", path, s.Mode.Perm(), d.Mode.Perm())
		}

		// Check content for regular files
		if s.Mode.IsRegular() && s.Content != d.Content {
			t.Errorf("%s: content mismatch: src=%q, dst=%q", path, truncate(s.Content, 50), truncate(d.Content, 50))
		}

		// Check symlink targets
		if s.Mode&os.ModeSymlink != 0 && s.LinkTarget != d.LinkTarget {
			t.Errorf("%s: symlink target mismatch: src=%q, dst=%q", path, s.LinkTarget, d.LinkTarget)
		}

		// Check ownership
		if s.UID != d.UID {
			t.Errorf("%s: UID mismatch: src=%d, dst=%d", path, s.UID, d.UID)
		}
		if s.GID != d.GID {
			t.Errorf("%s: GID mismatch: src=%d, dst=%d", path, s.GID, d.GID)
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// --- Test cases ---

func TestParallelCopyDir_BasicFilesExactMatch(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	os.WriteFile(filepath.Join(src, "file1.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(src, "file2.txt"), []byte("world"), 0600)
	os.WriteFile(filepath.Join(src, "empty"), []byte(""), 0644)

	if err := parallelCopyDir(src, dst, 4); err != nil {
		t.Fatalf("parallelCopyDir failed: %v", err)
	}

	assertIdenticalFS(t, src, dst)
}

func TestParallelCopyDir_Permissions(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	os.WriteFile(filepath.Join(src, "readonly"), []byte("ro"), 0444)
	os.WriteFile(filepath.Join(src, "executable"), []byte("#!/bin/sh"), 0755)
	os.WriteFile(filepath.Join(src, "private"), []byte("secret"), 0600)
	os.WriteFile(filepath.Join(src, "group_write"), []byte("gw"), 0664)
	os.MkdirAll(filepath.Join(src, "dir_0700"), 0700)
	os.MkdirAll(filepath.Join(src, "dir_0755"), 0755)
	os.WriteFile(filepath.Join(src, "dir_0700", "inner"), []byte("inner"), 0644)

	if err := parallelCopyDir(src, dst, 4); err != nil {
		t.Fatalf("parallelCopyDir failed: %v", err)
	}

	assertIdenticalFS(t, src, dst)
}

func TestParallelCopyDir_DockerOverlay2Structure(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	// Simulate realistic Docker /var/lib/docker structure
	// overlay2/ with multiple layers (the dominant directory)
	layers := []string{
		"abc123def456", "789xyz000111", "fedcba987654",
		"layer4444444", "layer5555555", "layer6666666",
	}
	for _, layer := range layers {
		diffDir := filepath.Join(src, "overlay2", layer, "diff")
		os.MkdirAll(diffDir, 0755)
		os.WriteFile(filepath.Join(diffDir, "file.txt"), []byte("content-"+layer), 0644)
		// Lower link file that overlay2 uses
		os.WriteFile(filepath.Join(src, "overlay2", layer, "lower"), []byte("l/"+layer[:6]), 0644)
		os.WriteFile(filepath.Join(src, "overlay2", layer, "link"), []byte(layer[:6]), 0644)
		// Some layers have committed marker
		if layer == "abc123def456" {
			os.WriteFile(filepath.Join(src, "overlay2", layer, "committed"), []byte(""), 0644)
		}
	}
	// overlay2/l/ directory with symlinks (Docker uses this for short names)
	os.MkdirAll(filepath.Join(src, "overlay2", "l"), 0700)
	for _, layer := range layers {
		os.Symlink("../"+layer+"/diff", filepath.Join(src, "overlay2", "l", layer[:6]))
	}

	// image/ metadata
	os.MkdirAll(filepath.Join(src, "image", "overlay2", "imagedb", "content", "sha256"), 0755)
	os.WriteFile(filepath.Join(src, "image", "overlay2", "imagedb", "content", "sha256", "abc123"), []byte(`{"config":{}}`), 0644)
	os.MkdirAll(filepath.Join(src, "image", "overlay2", "layerdb", "sha256"), 0755)

	// tmp/ build cache
	os.MkdirAll(filepath.Join(src, "tmp"), 0755)

	// engine-id
	os.WriteFile(filepath.Join(src, "engine-id"), []byte("test-engine-id"), 0644)

	if err := parallelCopyDir(src, dst, 4); err != nil {
		t.Fatalf("parallelCopyDir failed: %v", err)
	}

	assertIdenticalFS(t, src, dst)
}

func TestParallelCopyDir_Symlinks(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	// File symlink (relative)
	os.WriteFile(filepath.Join(src, "target.txt"), []byte("target"), 0644)
	os.Symlink("target.txt", filepath.Join(src, "link_relative"))

	// Directory symlink (relative)
	os.MkdirAll(filepath.Join(src, "realdir"), 0755)
	os.WriteFile(filepath.Join(src, "realdir", "inside.txt"), []byte("inside"), 0644)
	os.Symlink("realdir", filepath.Join(src, "link_dir"))

	// Absolute symlink
	os.Symlink("/dev/null", filepath.Join(src, "link_absolute"))

	// Broken symlink (target doesn't exist)
	os.Symlink("nonexistent", filepath.Join(src, "link_broken"))

	if err := parallelCopyDir(src, dst, 4); err != nil {
		t.Fatalf("parallelCopyDir failed: %v", err)
	}

	assertIdenticalFS(t, src, dst)

	// Extra verify: broken symlink is preserved
	target, err := os.Readlink(filepath.Join(dst, "link_broken"))
	if err != nil {
		t.Fatalf("broken symlink not preserved: %v", err)
	}
	if target != "nonexistent" {
		t.Errorf("broken symlink target: got %q, want %q", target, "nonexistent")
	}
}

func TestParallelCopyDir_Overlay2Splitting(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	// Create overlay2 with many layers — verifies the splitting logic
	for i := 0; i < 50; i++ {
		layerName := fmt.Sprintf("layer%04d", i)
		layerDir := filepath.Join(src, "overlay2", layerName)
		os.MkdirAll(filepath.Join(layerDir, "diff", "usr", "lib"), 0755)
		os.WriteFile(filepath.Join(layerDir, "diff", "usr", "lib", "libtest.so"), []byte(fmt.Sprintf("lib%d", i)), 0644)
		os.WriteFile(filepath.Join(layerDir, "link"), []byte(layerName[:8]), 0644)
	}

	// Non-overlay2 dirs processed normally
	os.MkdirAll(filepath.Join(src, "image"), 0755)
	os.WriteFile(filepath.Join(src, "image", "repositories.json"), []byte("{}"), 0644)
	os.MkdirAll(filepath.Join(src, "tmp"), 0755)

	if err := parallelCopyDir(src, dst, 4); err != nil {
		t.Fatalf("parallelCopyDir failed: %v", err)
	}

	assertIdenticalFS(t, src, dst)
}

func TestParallelCopyDir_EmptyDirs(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	os.MkdirAll(filepath.Join(src, "empty1"), 0755)
	os.MkdirAll(filepath.Join(src, "empty2"), 0700)
	os.MkdirAll(filepath.Join(src, "nested", "deep", "empty"), 0755)
	// overlay2 that's empty
	os.MkdirAll(filepath.Join(src, "overlay2"), 0755)

	if err := parallelCopyDir(src, dst, 4); err != nil {
		t.Fatalf("parallelCopyDir failed: %v", err)
	}

	assertIdenticalFS(t, src, dst)
}

func TestParallelCopyDir_PreservesOwnership(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	os.WriteFile(filepath.Join(src, "file1"), []byte("test1"), 0644)
	os.MkdirAll(filepath.Join(src, "dir1"), 0755)
	os.WriteFile(filepath.Join(src, "dir1", "file2"), []byte("test2"), 0644)

	if err := parallelCopyDir(src, dst, 4); err != nil {
		t.Fatalf("parallelCopyDir failed: %v", err)
	}

	// assertIdenticalFS already checks UID/GID
	assertIdenticalFS(t, src, dst)
}

func TestParallelCopyDir_SpecialFilenames(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	// Docker uses filenames with special characters
	os.WriteFile(filepath.Join(src, "file with spaces"), []byte("spaces"), 0644)
	os.WriteFile(filepath.Join(src, ".hidden"), []byte("hidden"), 0644)
	os.WriteFile(filepath.Join(src, "file-with-dashes"), []byte("dashes"), 0644)
	os.WriteFile(filepath.Join(src, "file_with_underscores"), []byte("underscores"), 0644)
	os.WriteFile(filepath.Join(src, ".golden-build-result"), []byte("0"), 0644)

	if err := parallelCopyDir(src, dst, 4); err != nil {
		t.Fatalf("parallelCopyDir failed: %v", err)
	}

	assertIdenticalFS(t, src, dst)
}

func TestParallelCopyDir_LargeFileCount(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	// Create 500 files across multiple directories
	// (scaled down version of real overlay2 which has 772K files)
	for i := 0; i < 10; i++ {
		dir := filepath.Join(src, "overlay2", fmt.Sprintf("layer%d", i), "diff")
		os.MkdirAll(dir, 0755)
		for j := 0; j < 50; j++ {
			os.WriteFile(filepath.Join(dir, fmt.Sprintf("file%d", j)), []byte(fmt.Sprintf("data-%d-%d", i, j)), 0644)
		}
	}

	if err := parallelCopyDir(src, dst, 8); err != nil {
		t.Fatalf("parallelCopyDir failed: %v", err)
	}

	assertIdenticalFS(t, src, dst)
}

func TestParallelCopyDir_ErrorOnMissingSrc(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "dst")

	err := parallelCopyDir("/nonexistent/path", dst, 4)
	if err == nil {
		t.Fatal("expected error for missing source, got nil")
	}
}

func TestParallelCopyDir_SingleWorker(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0644)
	os.MkdirAll(filepath.Join(src, "overlay2", "layer1"), 0755)
	os.WriteFile(filepath.Join(src, "overlay2", "layer1", "data"), []byte("data"), 0644)

	if err := parallelCopyDir(src, dst, 1); err != nil {
		t.Fatalf("parallelCopyDir with 1 worker failed: %v", err)
	}

	assertIdenticalFS(t, src, dst)
}

func TestParallelCopyDir_DeeplyNested(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	// Docker overlay2 layers can be deeply nested
	deep := filepath.Join(src, "overlay2", "layer1", "diff", "usr", "local", "lib", "python3.11", "site-packages", "numpy", "core")
	os.MkdirAll(deep, 0755)
	os.WriteFile(filepath.Join(deep, "__init__.py"), []byte("# numpy core"), 0644)
	os.WriteFile(filepath.Join(deep, "_multiarray_umath.so"), []byte("\x7fELF"), 0755)

	if err := parallelCopyDir(src, dst, 4); err != nil {
		t.Fatalf("parallelCopyDir failed: %v", err)
	}

	assertIdenticalFS(t, src, dst)
}

func TestParallelCopyDir_PreservesTimestamps(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	testFile := filepath.Join(src, "timestamped.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	// Set a specific timestamp
	past := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	os.Chtimes(testFile, past, past)

	if err := parallelCopyDir(src, dst, 4); err != nil {
		t.Fatalf("parallelCopyDir failed: %v", err)
	}

	srcInfo, _ := os.Stat(testFile)
	dstInfo, err := os.Stat(filepath.Join(dst, "timestamped.txt"))
	if err != nil {
		t.Fatalf("failed to stat dst: %v", err)
	}

	// cp -a preserves modification time
	if !srcInfo.ModTime().Equal(dstInfo.ModTime()) {
		t.Errorf("timestamp mismatch: src=%v, dst=%v", srcInfo.ModTime(), dstInfo.ModTime())
	}
}

func TestParallelCopyDir_MixedContent(t *testing.T) {
	// Comprehensive test: all types of entries mixed together,
	// verifying the exact filesystem state is reproduced
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	// Regular files with various sizes and permissions
	os.WriteFile(filepath.Join(src, "small"), []byte("s"), 0644)
	bigData := make([]byte, 100*1024) // 100KB
	for i := range bigData {
		bigData[i] = byte(i % 256)
	}
	os.WriteFile(filepath.Join(src, "big"), bigData, 0644)
	os.WriteFile(filepath.Join(src, "exec"), []byte("#!/bin/sh\necho hi"), 0755)

	// Directories with different permissions
	os.MkdirAll(filepath.Join(src, "overlay2", "abc", "diff"), 0755)
	os.MkdirAll(filepath.Join(src, "overlay2", "def", "diff"), 0755)
	os.WriteFile(filepath.Join(src, "overlay2", "abc", "diff", "f1"), []byte("abc"), 0644)
	os.WriteFile(filepath.Join(src, "overlay2", "def", "diff", "f2"), []byte("def"), 0600)
	os.MkdirAll(filepath.Join(src, "image", "overlay2"), 0755)
	os.MkdirAll(filepath.Join(src, "restricted"), 0700)
	os.WriteFile(filepath.Join(src, "restricted", "secret"), []byte("secret"), 0600)

	// Symlinks
	os.Symlink("small", filepath.Join(src, "link_file"))
	os.Symlink("overlay2/abc", filepath.Join(src, "link_dir"))
	os.Symlink("/etc/hosts", filepath.Join(src, "link_abs"))
	os.Symlink("missing", filepath.Join(src, "link_broken"))

	// Empty dirs
	os.MkdirAll(filepath.Join(src, "empty"), 0755)

	// Hidden files
	os.WriteFile(filepath.Join(src, ".dockerenv"), []byte(""), 0644)

	if err := parallelCopyDir(src, dst, 4); err != nil {
		t.Fatalf("parallelCopyDir failed: %v", err)
	}

	assertIdenticalFS(t, src, dst)
}
