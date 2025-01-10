//go:build integration
// +build integration

package smoke

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/integration-test/smoke/helper"
	"github.com/stretchr/testify/require"
)

func TestHelixCLIApply(t *testing.T) {
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
	tmpDir, err := os.MkdirTemp("", "helix-apply-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	cliInstallPath := InstallHelixCLI(t, tmpDir)

	openAPIExample := "https://raw.githubusercontent.com/helixml/helix/refs/heads/main/examples/openapi/jobvacancies.yaml"
	helper.DownloadFile(t, openAPIExample, path.Join(tmpDir, "openapi"))

	for _, appURL := range []string{
		"https://raw.githubusercontent.com/helixml/helix/refs/heads/main/examples/marvin_paranoid_bot.yaml",
		"https://raw.githubusercontent.com/helixml/helix/refs/heads/main/examples/api_tools.yaml",
	} {
		fileName := helper.DownloadFile(t, appURL, tmpDir)

		helper.LogStep(t, "Running helix apply")
		helixApplyCmd := exec.Command(cliInstallPath, "apply", "-f", fileName)
		helixApplyCmd.Env = append(os.Environ(), "HELIX_API_KEY="+apiKey, "HELIX_URL="+helper.GetServerURL())
		helixApplyCmd.Dir = tmpDir
		output, err := helixApplyCmd.CombinedOutput()
		require.NoError(t, err, "Helix apply failed: %s", string(output))

		// Parse the name of the app from the yaml file
		yamlFile, err := os.ReadFile(fileName)
		require.NoError(t, err)
		re := regexp.MustCompile(`name:.*`)
		matches := re.FindStringSubmatch(string(yamlFile))
		require.Greater(t, len(matches), 0, "No app name found in %s", fileName)
		appName := strings.TrimPrefix(matches[0], "name:")
		appName = strings.TrimSpace(appName)

		// Use helix app list to get the most recent marvin app id
		helixAppListCmd := exec.Command(cliInstallPath, "app", "list")
		helixAppListCmd.Env = append(os.Environ(), "HELIX_API_KEY="+apiKey, "HELIX_URL="+helper.GetServerURL())
		helixAppListCmd.Dir = tmpDir
		output, err = helixAppListCmd.CombinedOutput()
		require.NoError(t, err, "Helix app list failed: %s", string(output))

		re = regexp.MustCompile(`\s*(app_[a-zA-Z0-9_]+)\s+` + regexp.QuoteMeta(appName) + `\s+`)
		matches = re.FindStringSubmatch(string(output))
		require.Greater(t, len(matches), 0, "App '%s' not found in output: %s", appName, string(output))
		appID := strings.TrimSpace(matches[1])
		helper.LogStep(t, fmt.Sprintf("App id: %s", appID))

		// Check that the app is working
		page = browser.MustPage(helper.GetServerURL() + "/app/" + appID)
		page.MustWaitLoad()

		helper.LogStep(t, "Testing the app")
		page.MustElement("#textEntry").MustInput("What do you think of the snow in Yorkshire at the moment?")
		page.MustElement("#sendButton").MustClick()

		helper.WaitForHelixResponse(t, page)
	}
}
