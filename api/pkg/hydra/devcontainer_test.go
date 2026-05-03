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

// stageFakeSysfs builds a synthetic /dev/dri + /sys/class/drm tree for
// tests. devs is a list of {render, card, pci, driver} tuples — render
// and card are basenames (e.g. "renderD129", "card1"); pci is a BDF
// like "0000:01:00.0"; driver is the kernel driver basename like
// "nvidia" or "amdgpu" or "" for unbound.
//
// Returns devRoot and sysfsRoot to pass into enumerateDRMDevicesIn.
func stageFakeSysfs(t *testing.T, devs []struct{ render, card, pci, driver string }) (string, string) {
	t.Helper()
	root := t.TempDir()
	devRoot := filepath.Join(root, "dev")
	sysfsRoot := filepath.Join(root, "sys")
	if err := os.MkdirAll(filepath.Join(devRoot, "dri"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sysfsRoot, "class", "drm"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Create the PCI device dirs + driver dirs we'll point to
	driversDir := filepath.Join(sysfsRoot, "bus", "pci", "drivers")
	for _, d := range devs {
		// PCI device dir: /sys/devices/pci0000:00/<pci>
		pciDir := filepath.Join(sysfsRoot, "devices", "pci0000:00", d.pci)
		if err := os.MkdirAll(pciDir, 0o755); err != nil {
			t.Fatal(err)
		}
		// Driver dir: /sys/bus/pci/drivers/<driver>
		if d.driver != "" {
			drvDir := filepath.Join(driversDir, d.driver)
			if err := os.MkdirAll(drvDir, 0o755); err != nil {
				t.Fatal(err)
			}
			// Symlink: /sys/devices/.../driver -> /sys/bus/pci/drivers/<driver>
			if err := os.Symlink(drvDir, filepath.Join(pciDir, "driver")); err != nil {
				t.Fatal(err)
			}
		}
		// drm class entries point at the PCI device dir
		mk := func(name string) {
			if name == "" {
				return
			}
			drmEntry := filepath.Join(sysfsRoot, "class", "drm", name)
			if err := os.MkdirAll(drmEntry, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(pciDir, filepath.Join(drmEntry, "device")); err != nil {
				t.Fatal(err)
			}
		}
		mk(d.render)
		mk(d.card)
		// Stub /dev/dri/<name> files so glob picks them up
		for _, name := range []string{d.render, d.card} {
			if name == "" {
				continue
			}
			if err := os.WriteFile(filepath.Join(devRoot, "dri", name), nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return devRoot, sysfsRoot
}

func TestEnumerateDRMDevices_AzureMixedHost(t *testing.T) {
	// Azure-style: virtio-gpu at card0/renderD128 + real NVIDIA at
	// card1/renderD129. A naive cardN-by-index would pin to virtio
	// when asked for "GPU 0 NVIDIA"; the PCI walk filter must pick the
	// NVIDIA one regardless of card-number suffix.
	devRoot, sysfsRoot := stageFakeSysfs(t, []struct{ render, card, pci, driver string }{
		{"renderD128", "card0", "0000:00:01.0", "virtio_gpu"},
		{"renderD129", "card1", "0000:01:00.0", "nvidia"},
	})

	t.Run("filter to NVIDIA returns only the real GPU", func(t *testing.T) {
		got := enumerateDRMDevicesIn(devRoot, sysfsRoot, "nvidia")
		if len(got) != 1 {
			t.Fatalf("want 1 NVIDIA device, got %d: %#v", len(got), got)
		}
		if filepath.Base(got[0].renderNode) != "renderD129" {
			t.Errorf("want renderD129, got %s", got[0].renderNode)
		}
		if filepath.Base(got[0].cardDevice) != "card1" {
			t.Errorf("want card1, got %s", got[0].cardDevice)
		}
		if got[0].pciAddr != "0000:01:00.0" {
			t.Errorf("want pci 0000:01:00.0, got %s", got[0].pciAddr)
		}
	})

	t.Run("no filter returns both, sorted by PCI BDF", func(t *testing.T) {
		got := enumerateDRMDevicesIn(devRoot, sysfsRoot, "")
		if len(got) != 2 {
			t.Fatalf("want 2 devices, got %d", len(got))
		}
		// Sorted by PCI BDF: virtio (00:01.0) comes before nvidia (01:00.0)
		if got[0].driver != "virtio_gpu" || got[1].driver != "nvidia" {
			t.Errorf("want [virtio_gpu, nvidia], got [%s, %s]", got[0].driver, got[1].driver)
		}
	})
}

func TestEnumerateDRMDevices_StableOrderingAcrossPCIBDFs(t *testing.T) {
	// Multi-NVIDIA host with PCI BDFs that don't line up with card numbering.
	// render/card numbering is in the order udev assigned them at boot,
	// but we want the stable PCI-BDF ordering.
	devRoot, sysfsRoot := stageFakeSysfs(t, []struct{ render, card, pci, driver string }{
		{"renderD130", "card2", "0000:01:00.0", "nvidia"}, // earliest PCI BDF
		{"renderD128", "card0", "0000:81:00.0", "nvidia"}, // late PCI
		{"renderD129", "card1", "0000:41:00.0", "nvidia"}, // middle PCI
	})

	got := enumerateDRMDevicesIn(devRoot, sysfsRoot, "nvidia")
	if len(got) != 3 {
		t.Fatalf("want 3 NVIDIA devices, got %d", len(got))
	}
	wantPCIOrder := []string{"0000:01:00.0", "0000:41:00.0", "0000:81:00.0"}
	for i, want := range wantPCIOrder {
		if got[i].pciAddr != want {
			t.Errorf("[%d] want PCI %s, got %s (renderNode=%s)", i, want, got[i].pciAddr, got[i].renderNode)
		}
	}
}

func TestGPUDevicePaths(t *testing.T) {
	// Simulate a 4× AMD MI300X box. PCI BDFs in order: 00:01.0, 00:02.0,
	// 00:03.0, 00:04.0 → render/card 128/0, 129/1, 130/2, 131/3.
	// gpu_index=2 should pin to PCI 00:03.0 = renderD130 + card2.
	devRoot, sysfsRoot := stageFakeSysfs(t, []struct{ render, card, pci, driver string }{
		{"renderD128", "card0", "0000:00:01.0", "amdgpu"},
		{"renderD129", "card1", "0000:00:02.0", "amdgpu"},
		{"renderD130", "card2", "0000:00:03.0", "amdgpu"},
		{"renderD131", "card3", "0000:00:04.0", "amdgpu"},
	})

	// We can't call gpuDevicePaths() directly because it uses the host
	// /dev + /sys; use enumerateDRMDevicesIn() to validate the same logic.
	devs := enumerateDRMDevicesIn(devRoot, sysfsRoot, "amdgpu")
	if len(devs) != 4 {
		t.Fatalf("want 4 AMD devices, got %d", len(devs))
	}
	// Index 2 = third device by PCI BDF
	got := devs[2]
	if filepath.Base(got.renderNode) != "renderD130" {
		t.Errorf("want renderD130, got %s", got.renderNode)
	}
	if filepath.Base(got.cardDevice) != "card2" {
		t.Errorf("want card2, got %s", got.cardDevice)
	}
}

func TestEnumerateDRMDevices_HeadlessCompute(t *testing.T) {
	// MI300X-style headless compute: a render node with no card device.
	// Should still be enumerated; cardDevice is "".
	devRoot, sysfsRoot := stageFakeSysfs(t, []struct{ render, card, pci, driver string }{
		{"renderD128", "", "0000:01:00.0", "amdgpu"},
	})

	got := enumerateDRMDevicesIn(devRoot, sysfsRoot, "amdgpu")
	if len(got) != 1 {
		t.Fatalf("want 1 device, got %d", len(got))
	}
	if got[0].cardDevice != "" {
		t.Errorf("want no card device for headless GPU, got %s", got[0].cardDevice)
	}
}
