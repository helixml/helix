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

	page := browser.MustPage(helper.GetServerURL())
	defer page.MustClose()

	err := helper.PerformLogin(t, page)
	require.NoError(t, err, "login should succeed")

	err = helper.StartNewChat(t, page)
	require.NoError(t, err, "starting new chat should succeed")

	err = helper.SendMessage(t, page)
	require.NoError(t, err, "sending message should succeed")
}
