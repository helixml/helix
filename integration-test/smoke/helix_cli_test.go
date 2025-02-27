//go:build integration
// +build integration

package smoke

import (
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/helixml/helix/integration-test/smoke/helper"
	"github.com/stretchr/testify/require"
)

func TestHelixCLITest(t *testing.T) {
	ctx := helper.CreateContext(t)

	browser := createBrowser(ctx)
	defer browser.MustClose()

	page := createPage(browser)

	err := helper.PerformLogin(t, page)
	require.NoError(t, err, "login should succeed")

	// Get the API key
	apiKey := helper.GetFirstAPIKey(t, page)

	// Create temp dir for test
	tmpDir, err := os.MkdirTemp("", "helix-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	repoDir := helper.DownloadRepository(t, tmpDir)

	cli := helper.NewCLI(t, tmpDir)

	fileName := "basic_test.yaml"

	file := path.Join(repoDir, "examples", fileName)
	helper.LogStep(t, fmt.Sprintf("Running helix apply for %s", file))

	helper.LogStep(t, "Running helix test")
	output := cli.Test(t, file, apiKey)
	require.Contains(t, output, "PASS", "Helix test should pass")
}
