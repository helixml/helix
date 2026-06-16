package slack

import "github.com/slack-go/slack"

// newSlackClient builds a slack-go client for a given bot token. apiURL
// overrides the Slack API base (must end in "/"); empty uses the real
// slack.com endpoint. Tests point apiURL at an httptest.Server standing
// in for slack.com. This is the one place the underlying SDK is
// constructed, so the API base override is threaded uniformly through
// every caller (outbound, provisioner, oauth).
func newSlackClient(token, apiURL string) *slack.Client {
	if apiURL != "" {
		return slack.New(token, slack.OptionAPIURL(apiURL))
	}
	return slack.New(token)
}
