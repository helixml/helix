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

	"github.com/helixml/helix/integration-test/smoke/helper"
	"github.com/stretchr/testify/require"
)

func TestHelixCLIApply(t *testing.T) {
	ctx := helper.CreateContext(t)

	browser := createBrowser(ctx)
	defer browser.MustClose()

	page := browser.MustPage(helper.GetServerURL())
	defer page.MustClose()

	err := helper.PerformLogin(t, page)
	require.NoError(t, err, "login should succeed")

	// Get the API key
	apiKey := helper.GetFirstAPIKey(t, page)

	// Upload the required hornet pdf file for the uploaded_files.yaml example
	helper.BrowseToFilesPage(t, page)
	helper.CreateFolder(t, page, "hornet")
	helper.BrowseToFolder(t, page, "hornet")
	helper.UploadFile(t, page, helper.TestHornetPDF)

	// Create temp dir for test
	tmpDir, err := os.MkdirTemp("", "helix-apply-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	cliInstallPath := InstallHelixCLI(t, tmpDir)

	repoDir := helper.DownloadRepository(t, tmpDir)

	for _, fileName := range []string{
		"api_tools.yaml",
		"basic_knowledge.yaml",
		"cron_app.yaml",
		"discord_bot.yaml",
		"gptscript_app.yaml",
		"guardian.yaml",
		// "helix_docs.yaml", // Global app, can't update
		// "hn-scraper.yaml", // Global app, can't update
		"marvin_paranoid_bot.yaml",
		"override_prompts.yaml",
		"uploaded_files.yaml",
		"using_secrets.yaml",
		// "website_custom_rag.yaml", // This doesn't work
		"website_knowledge.yaml",
		// "zapier.yaml", // This requires a secret
	} {
		file := path.Join(repoDir, "examples", fileName)
		helper.LogStep(t, fmt.Sprintf("Running helix apply for %s", file))
		helixApplyCmd := exec.Command(cliInstallPath, "apply", "-f", file)
		helixApplyCmd.Env = append(os.Environ(), "HELIX_API_KEY="+apiKey, "HELIX_URL="+helper.GetServerURL())
		helixApplyCmd.Dir = path.Join(repoDir, "examples")
		output, err := helixApplyCmd.CombinedOutput()
		require.NoError(t, err, "Helix apply failed: %s", string(output))

		// Parse the name of the app from the yaml file
		yamlFile, err := os.ReadFile(file)
		require.NoError(t, err)
		re := regexp.MustCompile(`name:.*`)
		matches := re.FindStringSubmatch(string(yamlFile))
		require.Greater(t, len(matches), 0, "No app name found in %s", file)
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
		page.MustNavigate(helper.GetServerURL() + "/app/" + appID)
		if helper.HasKnowledge(t, page) {
			helper.LogStep(t, "App has knowledge")
			helper.WaitForKnowledgeReady(t, page)
		}

		helper.LogStep(t, fmt.Sprintf("Testing the app: %s", appName))
		page.MustElement("#textEntry").MustInput("What do you think of the snow in Yorkshire at the moment?")
		page.MustElement("#sendButton").MustClick()

		helper.WaitForHelixResponse(ctx, t, page)
	}
}
