package hydra

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func desktopImageMetadataScript(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../../sandbox/desktop-image-metadata.sh"))
}

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o755))
}

func TestRestoreDesktopImageMetadataFromBundledSeed(t *testing.T) {
	tmp := t.TempDir()
	imagesDir := filepath.Join(tmp, "images")
	seedDir := filepath.Join(tmp, "seed")
	binDir := filepath.Join(tmp, "bin")
	require.NoError(t, os.MkdirAll(imagesDir, 0o755))
	require.NoError(t, os.MkdirAll(seedDir, 0o755))
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(seedDir, "helix-ubuntu.version"), []byte("2ac5fd\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(seedDir, "helix-ubuntu.ref"), []byte("ghcr.io/helixml/helix-ubuntu:2ac5fd\n"), 0o644))
	writeExecutable(t, filepath.Join(binDir, "findmnt"), "#!/bin/bash\nprintf '%s\\n' '/dev/vda[/home/helix/sandbox-images]'\n")

	cmd := exec.Command("bash", "-c", `source "$1"; restore_desktop_image_metadata`, "bash", desktopImageMetadataScript(t))
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"HELIX_DESKTOP_IMAGE_DIR="+imagesDir,
		"HELIX_DESKTOP_IMAGE_SEED_DIR="+seedDir,
	)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))

	version, err := os.ReadFile(filepath.Join(imagesDir, "helix-ubuntu.version"))
	require.NoError(t, err)
	require.Equal(t, "2ac5fd\n", string(version))
	ref, err := os.ReadFile(filepath.Join(imagesDir, "helix-ubuntu.ref"))
	require.NoError(t, err)
	require.Equal(t, "ghcr.io/helixml/helix-ubuntu:2ac5fd\n", string(ref))
	require.Contains(t, string(output), "mount_source=/dev/vda[/home/helix/sandbox-images]")
	require.Contains(t, string(output), "restored=2")
}

func TestRecoverDesktopVersionFromNestedImageStore(t *testing.T) {
	tmp := t.TempDir()
	imagesDir := filepath.Join(tmp, "images")
	seedDir := filepath.Join(tmp, "seed")
	binDir := filepath.Join(tmp, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	writeExecutable(t, filepath.Join(binDir, "docker"), "#!/bin/bash\nif [ \"$1\" = images ]; then printf '%s\\n' \"$DOCKER_IMAGES\"; fi\n")

	cmd := exec.Command("bash", "-c", `source "$1"; recover_missing_desktop_version ubuntu`, "bash", desktopImageMetadataScript(t))
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"HELIX_DESKTOP_IMAGE_DIR="+imagesDir,
		"HELIX_DESKTOP_IMAGE_SEED_DIR="+seedDir,
		"DOCKER_IMAGES="+strings.Join([]string{
			"helix-ubuntu latest",
			"registry:5000/helix-ubuntu 2ac5fd",
			"ghcr.io/helixml/helix-ubuntu 2ac5fd",
		}, "\n"),
	)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))

	version, err := os.ReadFile(filepath.Join(imagesDir, "helix-ubuntu.version"))
	require.NoError(t, err)
	require.Equal(t, "2ac5fd\n", string(version))
	require.Contains(t, string(output), "adopted version 2ac5fd from nested Docker image store")
}

func TestRecoverDesktopVersionFromLocalRegistry(t *testing.T) {
	tmp := t.TempDir()
	imagesDir := filepath.Join(tmp, "images")
	seedDir := filepath.Join(tmp, "seed")
	binDir := filepath.Join(tmp, "bin")
	registryRequest := filepath.Join(tmp, "registry-request")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	writeExecutable(t, filepath.Join(binDir, "docker"), "#!/bin/bash\nexit 0\n")
	writeExecutable(t, filepath.Join(binDir, "curl"), "#!/bin/bash\nprintf '%s\\n' \"$*\" > \"$REGISTRY_REQUEST\"\nprintf '%s\\n' '{} '\n")
	writeExecutable(t, filepath.Join(binDir, "jq"), "#!/bin/bash\nprintf '%s\\n' \"$REGISTRY_TAGS\"\n")

	cmd := exec.Command("bash", "-c", `source "$1"; recover_missing_desktop_version ubuntu`, "bash", desktopImageMetadataScript(t))
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"HELIX_DESKTOP_IMAGE_DIR="+imagesDir,
		"HELIX_DESKTOP_IMAGE_SEED_DIR="+seedDir,
		"HELIX_LOCAL_DESKTOP_REGISTRY=registry.test:5000",
		"REGISTRY_REQUEST="+registryRequest,
		"REGISTRY_TAGS=2ac5fd",
	)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))

	version, err := os.ReadFile(filepath.Join(imagesDir, "helix-ubuntu.version"))
	require.NoError(t, err)
	require.Equal(t, "2ac5fd\n", string(version))
	request, err := os.ReadFile(registryRequest)
	require.NoError(t, err)
	require.Contains(t, string(request), "http://registry.test:5000/v2/helix-ubuntu/tags/list")
	require.Contains(t, string(output), "adopted version 2ac5fd from local registry fallback")
}

func TestRecoverDesktopVersionRejectsAmbiguousFallbacks(t *testing.T) {
	tmp := t.TempDir()
	imagesDir := filepath.Join(tmp, "images")
	seedDir := filepath.Join(tmp, "seed")
	binDir := filepath.Join(tmp, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	writeExecutable(t, filepath.Join(binDir, "docker"), "#!/bin/bash\nif [ \"$1\" = images ]; then printf '%s\\n' \"$DOCKER_IMAGES\"; fi\n")
	writeExecutable(t, filepath.Join(binDir, "curl"), "#!/bin/bash\nprintf '%s\\n' '{}'\n")
	writeExecutable(t, filepath.Join(binDir, "jq"), "#!/bin/bash\nprintf '%s\\n' \"$REGISTRY_TAGS\"\n")

	cmd := exec.Command("bash", "-c", `source "$1"; if recover_missing_desktop_version ubuntu; then exit 1; fi`, "bash", desktopImageMetadataScript(t))
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"HELIX_DESKTOP_IMAGE_DIR="+imagesDir,
		"HELIX_DESKTOP_IMAGE_SEED_DIR="+seedDir,
		"DOCKER_IMAGES=helix-ubuntu old-tag\nhelix-ubuntu new-tag",
		"REGISTRY_TAGS=old-tag\nnew-tag",
	)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	require.NoFileExists(t, filepath.Join(imagesDir, "helix-ubuntu.version"))
	require.Contains(t, string(output), "did not contain one unambiguous version tag")
}
