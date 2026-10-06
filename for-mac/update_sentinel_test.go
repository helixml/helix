package main

import (
	"os"
	"path/filepath"
	"testing"
)

// stageCombinedUpdate lays out the files StartCombinedUpdate leaves behind.
func stageCombinedUpdate(t *testing.T, version string, stagedDisk bool) {
	t.Helper()
	vmDir := filepath.Join(getHelixDataDir(), "vm", "helix-desktop")
	updatesDir := filepath.Join(getHelixDataDir(), "updates")
	for _, d := range []string{vmDir, updatesDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, data string) {
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(vmDir, "disk.qcow2"), "old")
	write(filepath.Join(updatesDir, "Helix-for-Mac.dmg"), "dmg")
	write(combinedUpdateSentinelPath(), version)
	if stagedDisk {
		write(filepath.Join(vmDir, "disk.qcow2.staged"), "new")
		write(filepath.Join(vmDir, ".staged-version"), version)
	}
}

func setVersion(t *testing.T, v string) {
	t.Helper()
	old := Version
	Version = v
	t.Cleanup(func() { Version = old })
}

func TestOldAppStartupLeavesCombinedUpdateStaged(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	setVersion(t, "2.12.27")
	stageCombinedUpdate(t, "2.12.28", true)

	settings := NewSettingsManager()
	s := settings.Get()
	s.InstalledVMVersion = "2.12.27"
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}

	app := &App{settings: settings, updater: NewUpdater()}
	app.checkVMVersionOnStartup()

	if got := settings.Get().InstalledVMVersion; got != "2.12.27" {
		t.Fatalf("old app swapped the VM disk: installed=%s", got)
	}
	if !IsVMUpdateStaged() {
		t.Fatal("old app consumed the staged VM disk")
	}
	if !IsCombinedUpdateStaged(settings, "2.12.28") {
		t.Fatal("combined update no longer staged after old-app restart")
	}
}

func TestIsCombinedUpdateStaged(t *testing.T) {
	tests := []struct {
		name       string
		stagedDisk bool
		installed  string
		want       bool
	}{
		{"staged disk", true, "2.12.27", true},
		// VM applied by an older build, app never replaced: only the DMG is needed.
		{"vm already installed", false, "2.12.28", true},
		{"nothing staged", false, "2.12.27", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			stageCombinedUpdate(t, "2.12.28", tt.stagedDisk)
			settings := NewSettingsManager()
			s := settings.Get()
			s.InstalledVMVersion = tt.installed
			if err := settings.Save(s); err != nil {
				t.Fatal(err)
			}
			if got := IsCombinedUpdateStaged(settings, "2.12.28"); got != tt.want {
				t.Fatalf("IsCombinedUpdateStaged = %v, want %v", got, tt.want)
			}
			if IsCombinedUpdateStaged(settings, "2.12.29") {
				t.Fatal("staged update reported for a different version")
			}
		})
	}
}
