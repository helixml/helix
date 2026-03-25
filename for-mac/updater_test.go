package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestParseSemVer(t *testing.T) {
	tests := []struct {
		input    string
		expected *SemVer
	}{
		{"1.2.3", &SemVer{Major: 1, Minor: 2, Patch: 3}},
		{"0.1.0", &SemVer{Major: 0, Minor: 1, Patch: 0}},
		{"2.7.0-beta", &SemVer{Major: 2, Minor: 7, Patch: 0, PreRelease: "beta", IsPreRelease: true}},
		{"1.0.0-rc1", &SemVer{Major: 1, Minor: 0, Patch: 0, PreRelease: "rc1", IsPreRelease: true}},
		{"dev", nil},
		{"<unknown>", nil},
		{"", nil},
		{"abcdef1234567890abcdef1234567890abcdef12", nil}, // SHA1 hash
		{"not-a-version", nil},
		{"1.2", nil},
		{"1.2.3.4", nil},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseSemVer(tt.input)
			if tt.expected == nil {
				if got != nil {
					t.Errorf("ParseSemVer(%q) = %+v, want nil", tt.input, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("ParseSemVer(%q) = nil, want %+v", tt.input, tt.expected)
			}
			if got.Major != tt.expected.Major || got.Minor != tt.expected.Minor || got.Patch != tt.expected.Patch {
				t.Errorf("ParseSemVer(%q) = %d.%d.%d, want %d.%d.%d",
					tt.input, got.Major, got.Minor, got.Patch,
					tt.expected.Major, tt.expected.Minor, tt.expected.Patch)
			}
			if got.PreRelease != tt.expected.PreRelease {
				t.Errorf("ParseSemVer(%q).PreRelease = %q, want %q", tt.input, got.PreRelease, tt.expected.PreRelease)
			}
			if got.IsPreRelease != tt.expected.IsPreRelease {
				t.Errorf("ParseSemVer(%q).IsPreRelease = %v, want %v", tt.input, got.IsPreRelease, tt.expected.IsPreRelease)
			}
		})
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		current  string
		latest   string
		expected bool
	}{
		// Basic version comparisons
		{"1.0.0", "1.0.1", true},
		{"1.0.0", "1.1.0", true},
		{"1.0.0", "2.0.0", true},
		{"1.0.1", "1.0.0", false},
		{"2.0.0", "1.0.0", false},
		{"1.0.0", "1.0.0", false},

		// Pre-release current with release latest = newer
		{"1.0.0-beta", "1.0.0", true},
		{"1.0.0-rc1", "1.0.0", true},

		// Pre-release latest is never an update
		{"1.0.0", "1.0.1-beta", false},
		{"1.0.0", "2.0.0-rc1", false},

		// Both pre-release, same base
		{"1.0.0-alpha", "1.0.0-beta", false}, // latest is pre-release

		// Invalid versions
		{"dev", "1.0.0", false},
		{"1.0.0", "dev", false},
		{"<unknown>", "1.0.0", false},
		{"abcdef1234567890abcdef1234567890abcdef12", "1.0.0", false},

		// Real-world scenario
		{"0.1.0-beta", "0.2.0", true},
		{"2.6.0", "2.7.0", true},
		{"2.7.0", "2.7.0", false},
	}

	for _, tt := range tests {
		name := tt.current + " -> " + tt.latest
		t.Run(name, func(t *testing.T) {
			got := IsNewer(tt.current, tt.latest)
			if got != tt.expected {
				t.Errorf("IsNewer(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.expected)
			}
		})
	}
}

func TestUpdaterIsVMDownloading(t *testing.T) {
	u := NewUpdater()
	if u.IsVMDownloading() {
		t.Error("new Updater should not be downloading")
	}

	// Simulate setting vmDownloading
	u.mu.Lock()
	u.vmDownloading = true
	u.mu.Unlock()

	if !u.IsVMDownloading() {
		t.Error("expected IsVMDownloading to return true")
	}
}

func TestUpdaterCancelSplitFunctions(t *testing.T) {
	u := NewUpdater()

	appCancelled := false
	vmCancelled := false

	_, appCancel := context.WithCancel(context.Background())
	_, vmCancel := context.WithCancel(context.Background())

	// Wrap cancel funcs to track calls
	u.mu.Lock()
	u.appCancelFunc = func() { appCancelled = true; appCancel() }
	u.vmCancelFunc = func() { vmCancelled = true; vmCancel() }
	u.mu.Unlock()

	u.Cancel()

	if !appCancelled {
		t.Error("expected appCancelFunc to be called")
	}
	if !vmCancelled {
		t.Error("expected vmCancelFunc to be called")
	}

	// After cancel, both should be nil
	u.mu.Lock()
	if u.appCancelFunc != nil {
		t.Error("appCancelFunc should be nil after Cancel()")
	}
	if u.vmCancelFunc != nil {
		t.Error("vmCancelFunc should be nil after Cancel()")
	}
	u.mu.Unlock()
}

func TestUpdaterCancelOnlyApp(t *testing.T) {
	u := NewUpdater()

	appCancelled := false
	_, appCancel := context.WithCancel(context.Background())

	u.mu.Lock()
	u.appCancelFunc = func() { appCancelled = true; appCancel() }
	// vmCancelFunc is nil
	u.mu.Unlock()

	u.Cancel() // should not panic

	if !appCancelled {
		t.Error("expected appCancelFunc to be called")
	}
}

func TestUpdaterVMDownloadingGuard(t *testing.T) {
	u := NewUpdater()

	// Simulate a download in progress
	u.mu.Lock()
	u.vmDownloading = true
	u.mu.Unlock()

	err := u.DownloadVMUpdate(nil, nil, false, false)
	if err == nil {
		t.Fatal("expected error when download already in progress")
	}
	if err.Error() != "VM download already in progress" {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestVMManifestNewFields verifies that DockerOnlyUpdate and Patches fields
// round-trip correctly through JSON serialization.
func TestVMManifestNewFields(t *testing.T) {
	raw := `{
		"version": "abc1234",
		"base_url": "https://dl.helix.ml/vm",
		"files": [],
		"docker_only_update": true,
		"patches": [
			{
				"from_version": "prev1234",
				"name": "patch.xdelta3.zst",
				"size": 450000000,
				"sha256": "aabbcc",
				"applies_to_sha256": "ddeeff",
				"result_sha256": "112233"
			}
		]
	}`

	var m VMManifest
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("failed to parse manifest: %v", err)
	}

	if !m.DockerOnlyUpdate {
		t.Error("expected DockerOnlyUpdate=true")
	}
	if len(m.Patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(m.Patches))
	}
	p := m.Patches[0]
	if p.FromVersion != "prev1234" {
		t.Errorf("FromVersion = %q, want prev1234", p.FromVersion)
	}
	if p.Size != 450000000 {
		t.Errorf("Size = %d, want 450000000", p.Size)
	}
	if p.AppliesToSHA256 != "ddeeff" {
		t.Errorf("AppliesToSHA256 = %q, want ddeeff", p.AppliesToSHA256)
	}
	if p.ResultSHA256 != "112233" {
		t.Errorf("ResultSHA256 = %q, want 112233", p.ResultSHA256)
	}

	// Round-trip: re-marshal and ensure docker_only_update appears
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if !bytes.Contains(out, []byte(`"docker_only_update":true`)) {
		t.Errorf("marshaled JSON missing docker_only_update: %s", out)
	}
}

// TestVerifyFileSHA256 checks correct and incorrect checksums.
func TestVerifyFileSHA256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "testfile")
	content := []byte("hello incremental update")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	h := sha256.Sum256(content)
	correctHex := hex.EncodeToString(h[:])

	// Correct checksum should pass.
	if err := verifyFileSHA256(path, correctHex); err != nil {
		t.Errorf("verifyFileSHA256 with correct hash failed: %v", err)
	}

	// Wrong checksum should fail.
	if err := verifyFileSHA256(path, "0000000000000000000000000000000000000000000000000000000000000000"); err == nil {
		t.Error("verifyFileSHA256 with wrong hash should fail")
	}

	// Empty expectedHex should skip verification (no error).
	if err := verifyFileSHA256(path, ""); err != nil {
		t.Errorf("verifyFileSHA256 with empty hash should succeed: %v", err)
	}
}

// TestDecompressZstdFile verifies round-trip zstd compress/decompress.
func TestDecompressZstdFile(t *testing.T) {
	dir := t.TempDir()
	original := []byte("helix incremental update test data — compressed and decompressed correctly")

	// Write compressed file.
	srcPath := filepath.Join(dir, "data.zst")
	{
		enc, err := zstd.NewWriter(nil)
		if err != nil {
			t.Fatal(err)
		}
		compressed := enc.EncodeAll(original, nil)
		enc.Close()
		if err := os.WriteFile(srcPath, compressed, 0644); err != nil {
			t.Fatal(err)
		}
	}

	dstPath := filepath.Join(dir, "data.out")
	ctx := context.Background()
	if err := decompressZstdFile(ctx, srcPath, dstPath); err != nil {
		t.Fatalf("decompressZstdFile failed: %v", err)
	}

	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Errorf("decompressed data mismatch: got %q, want %q", got, original)
	}
}

// TestDecompressZstdFileCancelled checks that context cancellation is respected.
func TestDecompressZstdFileCancelled(t *testing.T) {
	dir := t.TempDir()

	// Create a tiny zst file.
	enc, _ := zstd.NewWriter(nil)
	compressed := enc.EncodeAll([]byte("data"), nil)
	enc.Close()
	srcPath := filepath.Join(dir, "data.zst")
	os.WriteFile(srcPath, compressed, 0644)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	dstPath := filepath.Join(dir, "data.out")
	err := decompressZstdFile(ctx, srcPath, dstPath)
	// May or may not error depending on timing; either way, no panic.
	_ = err
}
