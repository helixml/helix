//go:build integration
// +build integration

package smoke

import (
	"fmt"
	"os"
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

	// Create temp dir for test
	tmpDir, err := os.MkdirTemp("", "helix-apply-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	repoDir := helper.DownloadRepository(t, tmpDir)

	cli := helper.NewCLI(t, tmpDir)

	// Create required secrets
	secrets := cli.ListSecrets(t, apiKey)
	if !strings.Contains(secrets, "MY_SECRET") {
		cli.CreateSecret(t, apiKey, "MY_SECRET", "Indiana Jones")
	}

	for _, fileName := range []string{
		"api_tools.yaml",
		// "basic_knowledge.yaml",  // This is now broken, it used to work
		"cron_app.yaml",
		"discord_bot.yaml",
		"gptscript_app.yaml",
		// "guardian.yaml", // This doesn't work. Not sure why.
		// "helix_docs.yaml", // Global app, can't update
		// "hn-scraper.yaml", // Global app, can't update
		"marvin_paranoid_bot.yaml",
		"override_prompts.yaml",
		// "website_custom_rag.yaml", // This doesn't work, just a dummy example
		"uploaded_files.yaml", // Works, but assumes hornet directory exists, and you have to wait until it's ready
		"using_secrets.yaml",
		"website_knowledge.yaml",
		// "zapier.yaml", // This requires an actual zapier API key
	} {
		file := path.Join(repoDir, "examples", fileName)
		helper.LogStep(t, fmt.Sprintf("Running helix apply for %s", file))
		cli.Apply(t, file, apiKey)

		// Parse the name of the app from the yaml file
		yamlFile, err := os.ReadFile(file)
		require.NoError(t, err)
		re := regexp.MustCompile(`name:.*`)
		matches := re.FindStringSubmatch(string(yamlFile))
		require.Greater(t, len(matches), 0, "No app name found in %s", file)
		appName := strings.TrimPrefix(matches[0], "name:")
		appName = strings.TrimSpace(appName)

		// Use helix app list to get the most recent marvin app id
		output := cli.ListApps(t, apiKey)

		re = regexp.MustCompile(`\s*(app_[a-zA-Z0-9_]+)\s+` + regexp.QuoteMeta(appName) + `\s+`)
		matches = re.FindStringSubmatch(string(output))
		require.Greater(t, len(matches), 0, "App '%s' not found in output: %s", appName, string(output))
		appID := strings.TrimSpace(matches[1])
		helper.LogStep(t, fmt.Sprintf("App id: %s", appID))

		// Check that the app is working
		page.MustNavigate(helper.GetServerURL() + "/app/" + appID + "?tab=knowledge")

		// Check to see if there is a knowledge associated with this app
		page.MustElementX(`//button[text() = 'Add Knowledge Source']`).MustWaitVisible()
		knowledgeSources := page.MustElementsX(`//div[contains(text(), 'Knowledge Source')]`)
		if len(knowledgeSources) > 0 {
			helper.LogStep(t, "Knowledge source found")
			// Wait for knowledge to be ready
			helper.LogStep(t, "Waiting for knowledge source to be ready")
			page.MustElementX(`//span[contains(text(), 'ready')]`)
		} else {
			helper.LogStep(t, "No knowledge source found")
		}

		helper.TestApp(ctx, t, page, "What do you think of the snow in Yorkshire at the moment?")
	}
}
