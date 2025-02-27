//go:build integration
// +build integration

package smoke

import (
	"testing"

	"github.com/helixml/helix/integration-test/smoke/helper"
	"github.com/stretchr/testify/require"
)

func TestStartNewSession(t *testing.T) {
	ctx := helper.CreateContext(t)

	browser := createBrowser(ctx)
	defer browser.MustClose()

	page := createPage(browser)

	err := helper.PerformLogin(t, page)
	require.NoError(t, err, "login should succeed")

	helper.SendMessage(t, page, "hello helix")
	helper.WaitForHelixResponse(ctx, t, page)
}
