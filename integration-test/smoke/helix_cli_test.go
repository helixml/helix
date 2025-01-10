//go:build integration
// +build integration

package smoke

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/integration-test/smoke/helper"
	"github.com/stretchr/testify/require"
)

func InstallHelixCLI(t *testing.T, tmpDir string) string {
	helper.LogStep(t, "Installing Helix CLI")
	// Change to temp dir
	err := os.Chdir(tmpDir)
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

	// Create a custom CLI install path
	cliInstallPath := filepath.Join(tmpDir, "helix")

	// Run install script, using the shim for sudo
	installCmd := exec.Command("bash", "install.sh", "-y", "--controlplane", "--cli", "--cli-install-path", cliInstallPath)
	installCmd.Env = append(os.Environ(), "PATH="+tmpDir+":"+os.Getenv("PATH"))
	output, err = installCmd.CombinedOutput()
	require.NoError(t, err, "Install script failed: %s", string(output))

	// Verify CLI was installed
	_, err = os.Stat(cliInstallPath)
	require.NoError(t, err, "CLI should be installed at %s", cliInstallPath)

	return cliInstallPath
}

func TestHelixCLIInstall(t *testing.T) {
	t.Parallel()

	// Create temp dir for test
	tmpDir, err := os.MkdirTemp("", "helix-install-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	cliInstallPath := InstallHelixCLI(t, tmpDir)

	// Get latest version from get.helix.ml
	latestVersionCmd := exec.Command("curl", "-sL", "https://get.helix.ml/latest.txt")
	output, err := latestVersionCmd.CombinedOutput()
	require.NoError(t, err, "Failed to get latest version: %s", string(output))
	latestVersion := strings.TrimSpace(string(output))

	// Run the CLI, should return a semantic version
	cliCmd := exec.Command(cliInstallPath, "version")
	output, err = cliCmd.CombinedOutput()
	require.NoError(t, err, "CLI version command failed: %s", string(output))
	require.Contains(t, string(output), latestVersion, "CLI version command should return version %s", latestVersion)
}

func TestHelixCLITest(t *testing.T) {
	t.Parallel()
	ctx := helper.SetTestTimeout(t, 30*time.Second)

	browser := createBrowser(ctx)
	defer browser.MustClose()

	page := browser.MustPage(helper.GetServerURL())
	defer page.MustClose()
	page.MustWaitLoad()

	err := helper.PerformLogin(t, page)
	require.NoError(t, err, "login should succeed")

	// Get the API key
	apiKey := helper.GetFirstAPIKey(t, page)

	// Create temp dir for test
	tmpDir, err := os.MkdirTemp("", "helix-install-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	cliInstallPath := InstallHelixCLI(t, tmpDir)

	filePath := helper.DownloadFile(t, "https://raw.githubusercontent.com/helixml/testing-genai/refs/heads/main/comedian.yaml", filepath.Join(tmpDir, "comedian.yaml"))

	helper.LogStep(t, "Replacing model with helix llama3.1:8b-instruct-q8_0")
	yamlFile, err := os.ReadFile(filePath)
	require.NoError(t, err)
	re := regexp.MustCompile(`model:.*`)
	yamlFile = re.ReplaceAll(yamlFile, []byte("model: llama3.1:8b-instruct-q8_0"))
	os.WriteFile(filePath, yamlFile, 0644)

	helper.LogStep(t, "Running helix test")
	helixTestCmd := exec.Command(cliInstallPath, "test", "-f", filePath)
	helixTestCmd.Env = append(os.Environ(), "HELIX_API_KEY="+apiKey, "HELIX_URL="+helper.GetServerURL())
	helixTestCmd.Dir = tmpDir
	output, err := helixTestCmd.CombinedOutput()
	require.NoError(t, err, "Helix test failed: %s", string(output))
}
