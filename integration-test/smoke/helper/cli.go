package helper

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type CLI struct {
	cliInstallPath string
	tmpDir         string
}

func NewCLI(t *testing.T, tmpDir string) *CLI {
	cliInstallPath := InstallHelixCLI(t, tmpDir)
	return &CLI{
		cliInstallPath: cliInstallPath,
		tmpDir:         tmpDir,
	}
}

func (c *CLI) Version(t *testing.T) string {
	cliCmd := exec.Command(c.cliInstallPath, "version")
	output, err := cliCmd.CombinedOutput()
	require.NoError(t, err, "CLI version command failed: %s", string(output))
	return string(output)
}

func (c *CLI) Test(t *testing.T, filePath string, apiKey string) string {
	helixTestCmd := exec.Command(c.cliInstallPath, "test", "-f", filePath)
	helixTestCmd.Env = append(os.Environ(), "HELIX_API_KEY="+apiKey, "HELIX_URL="+GetServerURL())
	helixTestCmd.Dir = c.tmpDir
	output, err := helixTestCmd.CombinedOutput()
	require.NoError(t, err, "CLI test command failed: %s", string(output))
	return string(output)
}

func InstallHelixCLI(t *testing.T, tmpDir string) string {
	// Change to temp dir
	err := os.Chdir(tmpDir)
	require.NoError(t, err)

	// Download install script
	downloadCmd := exec.Command("curl", "-sL", "-O", "https://get.helixml.tech/install.sh")
	output, err := downloadCmd.CombinedOutput()
	require.NoError(t, err, "Failed to download install script: %s", string(output))

	// Verify script was downloaded
	_, err = os.Stat("install.sh")
	require.NoError(t, err, "install.sh should exist")

	// Create a shim for sudo that runs commands without sudo
	sudoShim := filepath.Join(tmpDir, "sudo")
	sudoShimFile, err := os.Create(sudoShim)
	require.NoError(t, err)
	_, _ = sudoShimFile.WriteString("#!/bin/sh\n$@\n")
	sudoShimFile.Close()
	_ = os.Chmod(sudoShim, 0755)

	// Create a shim for docker that does nothing but return success
	dockerShim := filepath.Join(tmpDir, "docker")
	dockerShimFile, err := os.Create(dockerShim)
	require.NoError(t, err)
	_, _ = dockerShimFile.WriteString("#!/bin/sh\nexit 0\n")
	dockerShimFile.Close()
	_ = os.Chmod(dockerShim, 0755)

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

func (c *CLI) CreateSecret(t *testing.T, apiKey, name, value string) {
	helixApplyCmd := exec.Command(c.cliInstallPath, "secret", "create", "--name", name, "--value", value)
	helixApplyCmd.Env = append(os.Environ(), "HELIX_API_KEY="+apiKey, "HELIX_URL="+GetServerURL())
	helixApplyCmd.Dir = c.tmpDir
	output, err := helixApplyCmd.CombinedOutput()
	require.NoError(t, err, "Create secret failed: %s", string(output))
}

func (c *CLI) Apply(t *testing.T, filePath, apiKey string) {
	helixApplyCmd := exec.Command(c.cliInstallPath, "apply", "-f", filePath)
	helixApplyCmd.Env = append(os.Environ(), "HELIX_API_KEY="+apiKey, "HELIX_URL="+GetServerURL())
	helixApplyCmd.Dir = c.tmpDir
	output, err := helixApplyCmd.CombinedOutput()
	require.NoError(t, err, "Helix apply failed: %s", string(output))
}

func (c *CLI) ListApps(t *testing.T, apiKey string) string {
	helixAppListCmd := exec.Command(c.cliInstallPath, "app", "list")
	helixAppListCmd.Env = append(os.Environ(), "HELIX_API_KEY="+apiKey, "HELIX_URL="+GetServerURL())
	helixAppListCmd.Dir = c.tmpDir
	output, err := helixAppListCmd.CombinedOutput()
	require.NoError(t, err, "Helix agent list failed: %s", string(output))
	return string(output)
}

func (c *CLI) ListSecrets(t *testing.T, apiKey string) string {
	helixSecretListCmd := exec.Command(c.cliInstallPath, "secret", "list")
	helixSecretListCmd.Env = append(os.Environ(), "HELIX_API_KEY="+apiKey, "HELIX_URL="+GetServerURL())
	helixSecretListCmd.Dir = c.tmpDir
	output, err := helixSecretListCmd.CombinedOutput()
	require.NoError(t, err, "Helix secret list failed: %s", string(output))
	return string(output)
}
