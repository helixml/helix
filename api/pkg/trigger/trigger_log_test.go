package trigger

import (
	"bytes"
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
)

// The webhook route is unauthenticated: anyone who knows a trigger ID reaches
// ProcessWebhook. Rejecting an unsupported trigger must not log its tokens.
func TestProcessWebhook_UnknownTriggerDoesNotLogCredentials(t *testing.T) {
	var buf bytes.Buffer
	previous := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = previous })

	err := (&Manager{}).ProcessWebhook(context.Background(), &types.TriggerConfiguration{
		ID:    "trg_1",
		AppID: "app_1",
		Trigger: types.Trigger{Slack: &types.SlackTrigger{
			AppToken: "xapp-secret-app-token",
			BotToken: "xoxb-secret-bot-token",
		}},
	}, nil)

	require.Error(t, err)
	require.Contains(t, buf.String(), "trg_1")
	require.NotContains(t, buf.String(), "xapp-secret-app-token")
	require.NotContains(t, buf.String(), "xoxb-secret-bot-token")
}
