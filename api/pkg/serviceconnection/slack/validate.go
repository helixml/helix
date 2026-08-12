package slack

import (
	"context"

	"github.com/slack-go/slack"
)

// ValidateAppToken verifies a Socket Mode app-level token (xapp-) by
// calling apps.connections.open. A valid token returns a wss URL — which
// we discard, opening no persistent connection; an invalid token returns
// a Slack auth error. apiURL overrides the Slack base for tests (must end
// in "/"); empty uses slack.com.
func ValidateAppToken(ctx context.Context, appToken, apiURL string) error {
	opts := []slack.Option{slack.OptionAppLevelToken(appToken)}
	if apiURL != "" {
		opts = append(opts, slack.OptionAPIURL(apiURL))
	}
	_, _, err := slack.New("", opts...).StartSocketModeContext(ctx)
	return err
}

// ValidateBotToken verifies a bot token (xoxb-) by calling auth.test.
// apiURL overrides the Slack base for tests; empty uses slack.com.
func ValidateBotToken(ctx context.Context, botToken, apiURL string) error {
	_, err := AuthTest(ctx, New(botToken, apiURL))
	return err
}
