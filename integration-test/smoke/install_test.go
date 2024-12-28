//go:build integration
// +build integration

package smoke

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInstallScript(t *testing.T) {
	t.Parallel()

	// Create temp dir for test
	tmpDir, err := os.MkdirTemp("", "helix-install-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Change to temp dir
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	// Download install script
	downloadCmd := exec.Command("curl", "-sL", "-O", "https://get.helix.ml/install.sh")
	output, err := downloadCmd.CombinedOutput()
	require.NoError(t, err, "Failed to download install script: %s", string(output))

	// Verify script was downloaded
	_, err = os.Stat("install.sh")
	require.NoError(t, err, "install.sh should exist")

	// Create a shim for sudo that runs commands without sudo
	sudoShim := filepath.Join(tmpDir, "sudo")
	sudoShimFile, err := os.Create(sudoShim)
	require.NoError(t, err)
	sudoShimFile.WriteString("#!/bin/sh\n$@\n")
	sudoShimFile.Close()
	os.Chmod(sudoShim, 0755)

	// Create a shim for docker that does nothing but return success
	dockerShim := filepath.Join(tmpDir, "docker")
	dockerShimFile, err := os.Create(dockerShim)
	require.NoError(t, err)
	dockerShimFile.WriteString("#!/bin/sh\nexit 0\n")
	dockerShimFile.Close()
	os.Chmod(dockerShim, 0755)

	// Run install script, using the shim for sudo
	installCmd := exec.Command("bash", "install.sh", "-y", "--controlplane")
	installCmd.Env = append(os.Environ(), "PATH="+tmpDir+":"+os.Getenv("PATH"))
	output, err = installCmd.CombinedOutput()
	require.NoError(t, err, "Install script failed: %s", string(output))
}
