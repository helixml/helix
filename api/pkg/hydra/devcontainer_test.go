package hydra

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/stretchr/testify/require"
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

func TestSandboxResourceLimits(t *testing.T) {
	nanoCPUs, memory, memorySwap := sandboxResourceLimits(4, 8192)
	if nanoCPUs != 4_000_000_000 {
		t.Fatalf("NanoCPUs = %d, want 4000000000", nanoCPUs)
	}
	wantMemory := int64(8192 * 1024 * 1024)
	if memory != wantMemory {
		t.Fatalf("Memory = %d, want %d", memory, wantMemory)
	}
	if memorySwap != wantMemory*2 {
		t.Fatalf("MemorySwap = %d, want %d", memorySwap, wantMemory*2)
	}
}

// Docker refuses a NanoCPUs above the host CPU count outright, so an unclamped
// default larger than a small host fails container creation rather than degrade.
func TestSandboxResourceLimitsClampsVCPUsToHost(t *testing.T) {
	hostCPUs := runtime.NumCPU()

	nanoCPUs, _, _ := sandboxResourceLimits(hostCPUs*2, 1024)
	if want := int64(hostCPUs) * 1_000_000_000; nanoCPUs != want {
		t.Fatalf("NanoCPUs = %d, want %d (clamped to host)", nanoCPUs, want)
	}

	// Memory is deliberately NOT clamped: Docker accepts a limit above host RAM,
	// and an unreachable ceiling beats one that OOM-kills the desktop.
	_, memory, _ := sandboxResourceLimits(1, 1024*1024)
	if want := int64(1024*1024) * 1024 * 1024; memory != want {
		t.Fatalf("Memory = %d, want %d (unclamped)", memory, want)
	}
}

func TestBuildEnvHeadlessOmitsDisplayAndGPU(t *testing.T) {
	t.Setenv("HELIX_FRAME_EXPORT_PORT", "19000")
	gpuIndex := 0
	env := (&DevContainerManager{}).buildEnv(&CreateDevContainerRequest{
		ContainerType: DevContainerTypeHeadless,
		DisplayWidth:  1920,
		DisplayHeight: 1080,
		DisplayFPS:    60,
		GPUVendor:     "nvidia",
		GPUIndex:      &gpuIndex,
	})

	for _, entry := range env {
		for _, prefix := range []string{
			"GAMESCOPE_",
			"NVIDIA_",
			"GPU_VENDOR=",
			"GST_DEBUG=",
			"CUDA_VISIBLE_DEVICES=",
			"HELIX_GPU_INDEX=",
			"HELIX_SCANOUT_MODE=",
			"HELIX_VIDEO_MODE=",
			"HELIX_FRAME_EXPORT_PORT=",
		} {
			if strings.HasPrefix(entry, prefix) {
				t.Fatalf("headless environment contains display/GPU setting %q", entry)
			}
		}
	}
}

func TestBuildHostConfigHeadlessUsesDockerSecurityDefaults(t *testing.T) {
	dm := &DevContainerManager{manager: &Manager{dataDir: t.TempDir()}}

	hostConfig, err := dm.buildHostConfig(&CreateDevContainerRequest{
		ContainerType: DevContainerTypeHeadless,
		Privileged:    false,
	})
	require.NoError(t, err)
	require.False(t, hostConfig.Privileged)
	require.Empty(t, hostConfig.CapAdd)
	require.Equal(t, []string{"SYS_ADMIN", "SYS_NICE", "SYS_PTRACE", "NET_RAW", "MKNOD", "NET_ADMIN"}, []string(hostConfig.CapDrop))
	require.Empty(t, hostConfig.SecurityOpt)
	require.Empty(t, hostConfig.IpcMode)
	require.Zero(t, hostConfig.ShmSize)
	require.Empty(t, hostConfig.Resources.Devices)
}

func TestBuildHostConfigRootlessContainerEngine(t *testing.T) {
	dm := &DevContainerManager{manager: &Manager{dataDir: t.TempDir()}}

	hostConfig, err := dm.buildHostConfig(&CreateDevContainerRequest{
		ContainerType:           DevContainerTypeHeadless,
		RootlessContainerEngine: true,
	})
	require.NoError(t, err)
	require.False(t, hostConfig.Privileged)
	require.Equal(t, []string{"SYS_ADMIN"}, []string(hostConfig.CapAdd))
	require.Equal(t, []string{"SYS_NICE", "SYS_PTRACE", "NET_RAW", "MKNOD", "NET_ADMIN"}, []string(hostConfig.CapDrop))
	require.Equal(t, []string{"seccomp=unconfined"}, hostConfig.SecurityOpt)
	require.NotNil(t, hostConfig.MaskedPaths)
	require.Empty(t, hostConfig.MaskedPaths)
	require.NotNil(t, hostConfig.ReadonlyPaths)
	require.Empty(t, hostConfig.ReadonlyPaths)
	require.Empty(t, hostConfig.IpcMode)
	require.Zero(t, hostConfig.ShmSize)
	require.Len(t, hostConfig.Resources.Devices, 2)
	require.Equal(t, "/dev/fuse", hostConfig.Resources.Devices[0].PathOnHost)
	require.Equal(t, "/dev/fuse", hostConfig.Resources.Devices[0].PathInContainer)
	require.Equal(t, "rwm", hostConfig.Resources.Devices[0].CgroupPermissions)
	require.Equal(t, "/dev/net/tun", hostConfig.Resources.Devices[1].PathOnHost)
	require.Equal(t, "/dev/net/tun", hostConfig.Resources.Devices[1].PathInContainer)
	require.Equal(t, "rwm", hostConfig.Resources.Devices[1].CgroupPermissions)
}

func TestBuildHostConfigRejectsInvalidRootlessContainerEngineModes(t *testing.T) {
	dm := &DevContainerManager{manager: &Manager{dataDir: t.TempDir()}}

	_, err := dm.buildHostConfig(&CreateDevContainerRequest{
		ContainerType:           DevContainerTypeUbuntu,
		RootlessContainerEngine: true,
	})
	require.EqualError(t, err, "rootless container engine requires a headless container")

	_, err = dm.buildHostConfig(&CreateDevContainerRequest{
		ContainerType:           DevContainerTypeHeadless,
		Privileged:              true,
		RootlessContainerEngine: true,
	})
	require.EqualError(t, err, "rootless container engine cannot run in privileged mode")
}

func TestBuildEnvRootlessContainerEngine(t *testing.T) {
	t.Setenv("BUILDKIT_HOST", "tcp://buildkit:1234")
	t.Setenv("HELIX_REGISTRY", "registry:5000")

	env := (&DevContainerManager{}).buildEnv(&CreateDevContainerRequest{
		ContainerType:           DevContainerTypeHeadless,
		RootlessContainerEngine: true,
		Env: []string{
			"DOCKER_HOST=tcp://attacker:2375",
			"CONTAINER_HOST=tcp://attacker:2375",
			"DOCKER_BUILDKIT=1",
			"BUILDX_BUILDER=attacker",
			"BUILDKIT_HOST=tcp://attacker:1234",
			"HELIX_REGISTRY=attacker:5000",
		},
	})

	require.Equal(t, 1, countEnvVar(env, "HELIX_ROOTLESS_CONTAINER_ENGINE"))
	require.Contains(t, env, "HELIX_ROOTLESS_CONTAINER_ENGINE=1")
	require.Equal(t, 1, countEnvVar(env, "DOCKER_HOST"))
	require.Contains(t, env, "DOCKER_HOST=unix:///run/user/1000/podman/podman.sock")
	require.Equal(t, 1, countEnvVar(env, "CONTAINER_HOST"))
	require.Contains(t, env, "CONTAINER_HOST=unix:///run/user/1000/podman/podman.sock")
	require.Equal(t, 1, countEnvVar(env, "DOCKER_BUILDKIT"))
	require.Contains(t, env, "DOCKER_BUILDKIT=0")
	require.Equal(t, 1, countEnvVar(env, "BUILDX_BUILDER"))
	require.Contains(t, env, "BUILDX_BUILDER=helix-rootless")
	require.Zero(t, countEnvVar(env, "BUILDKIT_HOST"))
	require.Zero(t, countEnvVar(env, "HELIX_REGISTRY"))
}

// The control plane emits canonical helix-api.internal:18080 URLs directly (see
// external-agent buildEnvVars and hydra.SandboxAPIProxyURL), so hydra's buildEnv
// no longer rewrites control-plane addresses. It passes the API URL env through
// untouched and only strips the shared build-service vars.
func TestBuildEnvOmitsSharedBuildServicesAndPassesURLsThrough(t *testing.T) {
	env := (&DevContainerManager{}).buildEnv(&CreateDevContainerRequest{
		ContainerType: DevContainerTypeUbuntu,
		Network:       "bridge",
		Env: []string{
			"HELIX_API_URL=" + SandboxAPIProxyURL,
			"HELIX_API_BASE_URL=" + SandboxAPIProxyURL,
			"ANTHROPIC_BASE_URL=" + SandboxAPIProxyURL,
			"OPENAI_BASE_URL=" + SandboxAPIProxyURL + "/v1",
			"BUILDKIT_HOST=tcp://attacker:1234",
			"HELIX_REGISTRY=attacker:5000",
		},
	})

	require.Contains(t, env, "HELIX_API_URL="+SandboxAPIProxyURL)
	require.Contains(t, env, "HELIX_API_BASE_URL="+SandboxAPIProxyURL)
	require.Contains(t, env, "ANTHROPIC_BASE_URL="+SandboxAPIProxyURL)
	require.Contains(t, env, "OPENAI_BASE_URL="+SandboxAPIProxyURL+"/v1")
	require.Zero(t, countEnvVar(env, "BUILDKIT_HOST"))
	require.Zero(t, countEnvVar(env, "HELIX_REGISTRY"))
}

// Subscription-mode / BYO external endpoints are set by the control plane and
// must survive untouched — hydra no longer inspects or rewrites them.
func TestBuildEnvPreservesExternalProviderEndpoints(t *testing.T) {
	env := (&DevContainerManager{}).buildEnv(&CreateDevContainerRequest{
		ContainerType: DevContainerTypeHeadless,
		Network:       "bridge",
		Env: []string{
			"OPENAI_BASE_URL=https://llm.internal.example/v1",
			"ANTHROPIC_BASE_URL=https://api.anthropic.com",
		},
	})

	require.Contains(t, env, "OPENAI_BASE_URL=https://llm.internal.example/v1")
	require.Contains(t, env, "ANTHROPIC_BASE_URL=https://api.anthropic.com")
}

func TestBuildMountsOmitSharedBuildKitCache(t *testing.T) {
	dataDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dataDir, "buildkit-cache"), 0o755))
	dm := &DevContainerManager{manager: &Manager{dataDir: dataDir}}

	mounts, err := dm.buildMounts(&CreateDevContainerRequest{})
	require.NoError(t, err)
	for _, item := range mounts {
		require.NotEqual(t, "/buildkit-cache", item.Target)
	}
}

func TestBuildHostConfigDesktopUsesPrivateIPC(t *testing.T) {
	dm := &DevContainerManager{manager: &Manager{dataDir: t.TempDir()}}

	hostConfig, err := dm.buildHostConfig(&CreateDevContainerRequest{
		ContainerType: DevContainerTypeUbuntu,
		Privileged:    true,
	})
	require.NoError(t, err)
	require.True(t, hostConfig.Privileged)
	require.Empty(t, hostConfig.CapAdd)
	require.Empty(t, hostConfig.CapDrop)
	require.Equal(t, []string{"seccomp=unconfined", "apparmor=unconfined"}, hostConfig.SecurityOpt)
	require.Equal(t, "private", string(hostConfig.IpcMode))
	require.EqualValues(t, desktopShmSizeBytes, hostConfig.ShmSize)
	require.Equal(t, SandboxNetworkName, string(hostConfig.NetworkMode))
	require.Equal(t, []string{"10.213.0.1"}, hostConfig.DNS)
	require.Equal(t, []string{
		SandboxAPIProxyHostname + ":" + SandboxNetworkGateway,
		SandboxLegacyAPIProxyHostname + ":" + SandboxNetworkGateway,
	}, hostConfig.ExtraHosts)
}

func TestBuildHostConfigUsesDepthAwareDNSProxy(t *testing.T) {
	t.Setenv("HELIX_DOCKER_DEPTH", "2")
	dm := &DevContainerManager{manager: &Manager{dataDir: t.TempDir()}}
	hostConfig, err := dm.buildHostConfig(&CreateDevContainerRequest{Network: "bridge"})
	require.NoError(t, err)
	require.Equal(t, []string{"10.214.0.1"}, hostConfig.DNS)
}

func TestBuildHostConfigRejectsCustomNetwork(t *testing.T) {
	dm := &DevContainerManager{manager: &Manager{dataDir: t.TempDir()}}
	_, err := dm.buildHostConfig(&CreateDevContainerRequest{Network: "host"})
	require.EqualError(t, err, `unsupported sandbox network "host"`)
}

func TestShouldMigrateContainerNetwork(t *testing.T) {
	require.True(t, shouldMigrateContainerNetwork("bridge", SandboxNetworkName))
	require.False(t, shouldMigrateContainerNetwork(SandboxNetworkName, SandboxNetworkName))
	require.False(t, shouldRecreateForNetworkMigration("bridge", SandboxNetworkName, true))
	require.True(t, shouldRecreateForNetworkMigration("bridge", SandboxNetworkName, false))
}

func countEnvVar(env []string, key string) int {
	count := 0
	for _, entry := range env {
		if strings.HasPrefix(entry, key+"=") {
			count++
		}
	}
	return count
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

func TestMaterializeWorkspaceFiles(t *testing.T) {
	root := t.TempDir()
	mounts := []MountConfig{{Source: root, Destination: "/home/retro/work"}}
	files := map[string][]byte{
		"AGENTS.md": []byte("agent instructions"),
		"CLAUDE.md": []byte("claude instructions"),
	}
	if err := materializeWorkspaceFiles(mounts, files); err != nil {
		t.Fatalf("materializeWorkspaceFiles: %v", err)
	}
	for name, want := range files {
		got, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if string(got) != string(want) {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}

	files["AGENTS.md"] = []byte("updated")
	if err := materializeWorkspaceFiles(mounts, files); err != nil {
		t.Fatalf("refresh workspace files: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil || string(got) != "updated" {
		t.Fatalf("refreshed AGENTS.md = %q, %v", got, err)
	}
}

func TestMaterializeWorkspaceFilesRejectsUnsafeNames(t *testing.T) {
	mounts := []MountConfig{{Source: t.TempDir(), Destination: "/home/retro/work"}}
	for _, name := range []string{"", "../AGENTS.md", "nested/AGENTS.md", "/AGENTS.md"} {
		t.Run(name, func(t *testing.T) {
			if err := materializeWorkspaceFiles(mounts, map[string][]byte{name: []byte("x")}); err == nil {
				t.Fatalf("materializeWorkspaceFiles accepted %q", name)
			}
		})
	}
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

// fakeRecoveryClient is a hand-rolled imageRecoveryClient for testing
// recoverImageFromSources. It records call args and lets each method's
// behaviour be overridden per source ref.
type fakeRecoveryClient struct {
	pullBody  func(source string) (io.ReadCloser, error)
	tagErr    func(source, target string) error
	inspect   func(image string) (dockertypes.ImageInspect, []byte, error)
	pullCalls []string
	tagCalls  []struct{ source, target string }
	inspects  []string
}

func (f *fakeRecoveryClient) ImagePull(_ context.Context, source string, _ dockertypes.ImagePullOptions) (io.ReadCloser, error) {
	f.pullCalls = append(f.pullCalls, source)
	if f.pullBody == nil {
		return io.NopCloser(strings.NewReader(`{"status":"Pull complete"}`)), nil
	}
	return f.pullBody(source)
}

func (f *fakeRecoveryClient) ImageTag(_ context.Context, source, target string) error {
	f.tagCalls = append(f.tagCalls, struct{ source, target string }{source, target})
	if f.tagErr == nil {
		return nil
	}
	return f.tagErr(source, target)
}

func (f *fakeRecoveryClient) ImageInspectWithRaw(_ context.Context, image string) (dockertypes.ImageInspect, []byte, error) {
	f.inspects = append(f.inspects, image)
	if f.inspect == nil {
		return dockertypes.ImageInspect{RepoTags: []string{image}}, nil, nil
	}
	return f.inspect(image)
}

func TestDrainPullStream(t *testing.T) {
	t.Run("clean stream returns nil", func(t *testing.T) {
		body := `{"status":"Pulling from helixml/helix-ubuntu"}
{"status":"Pull complete"}
`
		if err := drainPullStream(strings.NewReader(body)); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("error field surfaces", func(t *testing.T) {
		body := `{"status":"Pulling fs layer"}
{"error":"no space left on device"}
`
		err := drainPullStream(strings.NewReader(body))
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "no space left on device") {
			t.Errorf("expected error mentioning disk-full, got %v", err)
		}
	})

	t.Run("errorDetail.message surfaces", func(t *testing.T) {
		body := `{"status":"Pulling"}
{"errorDetail":{"message":"manifest unknown: manifest unknown"},"error":""}
`
		err := drainPullStream(strings.NewReader(body))
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "manifest unknown") {
			t.Errorf("expected error mentioning manifest, got %v", err)
		}
	})

	t.Run("empty stream is fine", func(t *testing.T) {
		if err := drainPullStream(strings.NewReader("")); err != nil {
			t.Fatalf("expected nil for empty stream, got %v", err)
		}
	})
}

func TestRecoverImageFromSources_HappyPath(t *testing.T) {
	fc := &fakeRecoveryClient{}
	ok := recoverImageFromSources(
		context.Background(),
		fc,
		[]string{"registry:5000/helix-ubuntu:abc123"},
		"helix-ubuntu:abc123",
		"helix-ubuntu:abc123",
	)
	if !ok {
		t.Fatalf("expected recovery success")
	}
	if len(fc.pullCalls) != 1 || fc.pullCalls[0] != "registry:5000/helix-ubuntu:abc123" {
		t.Errorf("unexpected pull calls: %v", fc.pullCalls)
	}
	// source differs from both targets, so both tag calls fire
	if len(fc.tagCalls) != 2 {
		t.Errorf("expected 2 tag calls, got %v", fc.tagCalls)
	}
	if len(fc.inspects) != 1 || fc.inspects[0] != "helix-ubuntu:abc123" {
		t.Errorf("expected one inspect call on originalImage, got %v", fc.inspects)
	}
}

func TestRecoverImageFromSources_TagsBothRefs(t *testing.T) {
	fc := &fakeRecoveryClient{}
	ok := recoverImageFromSources(
		context.Background(),
		fc,
		[]string{"ghcr.io/helixml/helix-ubuntu:abc123"},
		"helix-ubuntu:abc123",
		"helix-ubuntu:abc123",
	)
	if !ok {
		t.Fatalf("expected recovery success")
	}
	// source differs from both resolvedImage and originalImage, so both
	// tag operations fire (even though the two targets happen to be the
	// same string here).
	if len(fc.tagCalls) != 2 {
		t.Fatalf("expected 2 tag calls, got %v", fc.tagCalls)
	}
	for _, c := range fc.tagCalls {
		if c.source != "ghcr.io/helixml/helix-ubuntu:abc123" ||
			c.target != "helix-ubuntu:abc123" {
			t.Errorf("unexpected tag call: %+v", c)
		}
	}
}

func TestRecoverImageFromSources_PullStreamErrorIsSurfaced(t *testing.T) {
	// Simulate the production fault: pull stream contains errorDetail.message
	// with "no space left on device". recoverImageFromSources should treat
	// this source as failed, not log "recovered successfully".
	fc := &fakeRecoveryClient{
		pullBody: func(source string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(
				`{"status":"Extracting"}
{"errorDetail":{"message":"failed to register layer: write /var/lib/docker/...: no space left on device"},"error":"failed to register layer"}
`)), nil
		},
	}
	ok := recoverImageFromSources(
		context.Background(),
		fc,
		[]string{"registry:5000/helix-ubuntu:2.11.23-linux-amd64"},
		"helix-ubuntu:2.11.23-linux-amd64",
		"helix-ubuntu:2.11.23-linux-amd64",
	)
	if ok {
		t.Fatalf("expected recovery to fail when pull stream reports disk-full")
	}
	if len(fc.tagCalls) != 0 {
		t.Errorf("expected no tag attempts after pull-stream error, got %v", fc.tagCalls)
	}
	if len(fc.inspects) != 0 {
		t.Errorf("expected no inspect attempts after pull-stream error, got %v", fc.inspects)
	}
}

func TestRecoverImageFromSources_TagFailureFailsSource(t *testing.T) {
	fc := &fakeRecoveryClient{
		tagErr: func(source, target string) error {
			return errors.New("Error response from daemon: invalid reference format")
		},
	}
	ok := recoverImageFromSources(
		context.Background(),
		fc,
		[]string{"ghcr.io/helixml/helix-ubuntu:abc123"},
		"helix-ubuntu:abc123",
		"helix-ubuntu:abc123",
	)
	if ok {
		t.Fatalf("expected recovery to fail when ImageTag errors")
	}
	if len(fc.inspects) != 0 {
		t.Errorf("inspect should not run after tag failure, got %v", fc.inspects)
	}
}

func TestRecoverImageFromSources_InspectNotFoundFailsSource(t *testing.T) {
	fc := &fakeRecoveryClient{
		inspect: func(image string) (dockertypes.ImageInspect, []byte, error) {
			return dockertypes.ImageInspect{}, nil, errors.New("Error: No such image: " + image)
		},
	}
	ok := recoverImageFromSources(
		context.Background(),
		fc,
		[]string{"registry:5000/helix-ubuntu:abc123"},
		"helix-ubuntu:abc123",
		"helix-ubuntu:abc123",
	)
	if ok {
		t.Fatalf("expected recovery to fail when post-pull inspect returns NotFound")
	}
}

func TestRecoverImageFromSources_FallsThroughToNextSource(t *testing.T) {
	// First source fails in pull stream, second succeeds. Recovery returns
	// true and reports the second source as the one that worked.
	fc := &fakeRecoveryClient{
		pullBody: func(source string) (io.ReadCloser, error) {
			if strings.HasPrefix(source, "ghcr.io/") {
				return io.NopCloser(strings.NewReader(
					`{"errorDetail":{"message":"denied: not authorized"},"error":"denied"}` + "\n",
				)), nil
			}
			return io.NopCloser(strings.NewReader(`{"status":"Pull complete"}`)), nil
		},
	}
	ok := recoverImageFromSources(
		context.Background(),
		fc,
		[]string{
			"ghcr.io/helixml/helix-ubuntu:abc123",
			"registry:5000/helix-ubuntu:abc123",
		},
		"helix-ubuntu:abc123",
		"helix-ubuntu:abc123",
	)
	if !ok {
		t.Fatalf("expected recovery to succeed via fallback source")
	}
	if len(fc.pullCalls) != 2 {
		t.Errorf("expected both sources tried, got %v", fc.pullCalls)
	}
}

func TestBuildRecoveryPullSources(t *testing.T) {
	tmp := t.TempDir()

	t.Run("no ref file, no registry host", func(t *testing.T) {
		got := buildRecoveryPullSources("helix-ubuntu:abc123", tmp, "")
		want := []string{"registry:5000/helix-ubuntu:abc123"}
		if len(got) != len(want) || got[0] != want[0] {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("ref file rewrites tag and is first", func(t *testing.T) {
		ref := filepath.Join(tmp, "helix-ubuntu.ref")
		if err := os.WriteFile(ref, []byte("ghcr.io/helixml/helix-ubuntu:oldtag\n"), 0644); err != nil {
			t.Fatal(err)
		}
		got := buildRecoveryPullSources("helix-ubuntu:abc123", tmp, "10.0.0.5:5000")
		want := []string{
			"ghcr.io/helixml/helix-ubuntu:abc123",
			"10.0.0.5:5000/helix-ubuntu:abc123",
			"registry:5000/helix-ubuntu:abc123",
		}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("[%d] got %q, want %q", i, got[i], want[i])
			}
		}
	})
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
