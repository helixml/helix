package hydra

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDesktopImageGCRegistryMatchingAndRetention(t *testing.T) {
	tmp := t.TempDir()
	imagesDir := filepath.Join(tmp, "images")
	binDir := filepath.Join(tmp, "bin")
	require.NoError(t, os.MkdirAll(imagesDir, 0o755))
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(imagesDir, "helix-ubuntu.version"), []byte("2.12.8-linux-amd64\n"), 0o644))

	dockerOutput := strings.Join([]string{
		"helix-ubuntu:2.12.8-linux-amd64",
		"ghcr.io/helixml/helix-ubuntu:2.12.7-linux-amd64",
		"registry.example:5000/team/helix-ubuntu:2.12.7-linux-amd64",
		"ghcr.io/helixml/helix-ubuntu:2.12.6-linux-amd64",
		"unrelated/helix-api:latest",
	}, "\n")
	removedFile := filepath.Join(tmp, "removed")
	mockDocker := `#!/bin/bash
if [ "$1" = images ]; then
  printf '%s\n' "$DOCKER_IMAGES"
elif [ "$1" = rmi ]; then
  printf '%s\n' "$2" >> "$REMOVED_FILE"
fi
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "docker"), []byte(mockDocker), 0o755))

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	gcScript := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../../sandbox/desktop-image-gc.sh"))
	cmd := exec.Command("bash", "-c", `source "$1"; cleanup_desktop_images test`, "bash", gcScript)
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"HELIX_DESKTOP_IMAGE_DIR="+imagesDir,
		"HELIX_DESKTOP_IMAGE_RETENTION=1",
		"DOCKER_IMAGES="+dockerOutput,
		"REMOVED_FILE="+removedFile,
	)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))

	removed, err := os.ReadFile(removedFile)
	require.NoError(t, err)
	assert.Equal(t, "ghcr.io/helixml/helix-ubuntu:2.12.6-linux-amd64\n", string(removed))
	assert.NotContains(t, string(output), "unrelated/helix-api")
}
