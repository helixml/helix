package main

import (
	"context"
	"testing"
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

	err := u.DownloadVMUpdate(nil, nil, false)
	if err == nil {
		t.Fatal("expected error when download already in progress")
	}
	if err.Error() != "VM download already in progress" {
		t.Errorf("unexpected error: %v", err)
	}
}
