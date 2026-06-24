// Package slack is the generic, reusable Slack protocol layer that sits
// behind Helix's Slack ServiceConnections. It owns the mechanics —
// building a slack-go client, verifying + parsing Events API requests,
// running a Socket Mode connection, exchanging an OAuth install code,
// and posting messages under a persona — with ZERO knowledge of orgs,
// the org-graph store, Topics, or multi-tenancy. Org-specific wiring
// (resolving a team_id to an org, matching Topics, publishing) lives in
// api/pkg/org/infrastructure/transports/slack and depends on this.
//
// Keeping the protocol layer here (next to the ServiceConnection types
// it serves) makes it the obvious home for any future Slack
// ServiceConnection consumer — e.g. unifying the per-agent
// api/pkg/trigger/slack onto it.
package slack

import (
	"context"

	"github.com/slack-go/slack"
)

// New builds a slack-go client for a bot token. apiURL overrides the
// Slack API base (must end in "/"); empty uses the real slack.com
// endpoint. Tests point apiURL at an httptest.Server. This is the one
// place the underlying SDK is constructed for API calls, so the base
// override is threaded uniformly through every caller.
func New(token, apiURL string) *slack.Client {
	if apiURL != "" {
		return slack.New(token, slack.OptionAPIURL(apiURL))
	}
	return slack.New(token)
}

// Identity is the subset of auth.test we use to derive a workspace's
// team id (e.g. on first Socket Mode connect, where there is no OAuth
// install to read it from).
type Identity struct {
	TeamID string
	Team   string
	UserID string
	BotID  string
}

// AuthTest calls auth.test on the bot token's workspace.
func AuthTest(ctx context.Context, client *slack.Client) (Identity, error) {
	resp, err := client.AuthTestContext(ctx)
	if err != nil {
		return Identity{}, err
	}
	return Identity{TeamID: resp.TeamID, Team: resp.Team, UserID: resp.UserID, BotID: resp.BotID}, nil
}
