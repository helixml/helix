//go:build integration
// +build integration

package smoke

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/helixml/helix/integration-test/smoke/helper"
	"github.com/stretchr/testify/require"
)

func TestHelixCLITest(t *testing.T) {
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
	tmpDir, err := os.MkdirTemp("", "helix-install-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	cli := helper.NewCLI(t, tmpDir)

	filePath := helper.DownloadFile(t, "https://raw.githubusercontent.com/helixml/testing-genai/refs/heads/main/comedian.yaml", filepath.Join(tmpDir, "comedian.yaml"))

	helper.LogStep(t, "Replacing model with helix llama3.1:8b-instruct-q8_0")
	yamlFile, err := os.ReadFile(filePath)
	require.NoError(t, err)
	re := regexp.MustCompile(`model:.*`)
	yamlFile = re.ReplaceAll(yamlFile, []byte("model: llama3.1:8b-instruct-q8_0"))
	os.WriteFile(filePath, yamlFile, 0644)

	helper.LogStep(t, "Running helix test")
	output := cli.Test(t, filePath, apiKey)
	require.Contains(t, output, "PASS", "Helix test should pass")
}
