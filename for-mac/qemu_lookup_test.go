package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A release build must never run a QEMU other than the bundled one: not a
// build/dev-qemu relative to the launch directory, and not one on PATH.
func TestReleaseBuildIgnoresDevAndPathQEMU(t *testing.T) {
	dir := t.TempDir()
	devQemu := filepath.Join(dir, "build", "dev-qemu", "qemu-system-aarch64")
	pathQemu := filepath.Join(dir, "bin", "qemu-system-aarch64")
	for _, p := range []string{devQemu, pathQemu} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	t.Setenv("PATH", filepath.Dir(pathQemu))
	t.Setenv("HELIX_QEMU_PATH", "")
	t.Setenv("HELIX_DEV_IMAGE", "")
	vm := &VMManager{}

	setVersion(t, "2.13.0")
	if got := vm.findQEMUBinary(); got != "" {
		t.Fatalf("release build picked %q; want only the bundled QEMU (none here)", got)
	}

	setVersion(t, "dev")
	if got := vm.findQEMUBinary(); got != devQemu {
		t.Fatalf("dev build picked %q; want %q", got, devQemu)
	}
}
