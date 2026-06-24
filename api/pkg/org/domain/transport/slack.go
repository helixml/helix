package transport

import (
	"encoding/json"
	"errors"
	"fmt"
)

// KindSlack binds a Topic to a single Slack channel within one of its
// org's installed Slack workspaces. The three tenancy layers live apart
// (see design/2026-06-23-helix-org-slack-serviceconnection.md):
//
//   - The global Slack *app* (client id/secret, signing secret, app
//     token, ingress-mode toggle) is a global ServiceConnection
//     (type=slack_app, OrganizationID=""), helix-admin configured,
//     instance-wide.
//   - The per-org *workspace install* (bot token + team id) is an
//     org-scoped ServiceConnection (type=slack_workspace). An org may
//     have several (e.g. winderai + mlops-community).
//   - This per-Topic config picks WHICH workspace (ServiceConnectionID)
//     and WHICH channel (Channel) the Topic is bound to.
//
// Inbound: Slack delivers channel messages over one shared ingest path
// (REST Events API or Socket Mode — operator's choice). The ingest
// routes team_id → workspace ServiceConnection → org, drops the bot's
// own events, then publishes the message onto every Topic in that org
// whose SlackConfig matches the workspace + channel. Routing to a
// specific Worker is the job of the org's processor/filter layer, not
// the transport.
//
// Outbound: a Worker's publish to a slack Topic becomes a
// chat.postMessage under the Worker's persona (name + avatar) through
// the org's shared bot for that workspace.
const KindSlack Kind = "slack"

// SlackConfig is the parsed shape of Transport.Config when
// Kind == KindSlack. A Slack Topic is workspace-scoped: it ingests every
// channel the workspace bot is in and posts replies back to whichever
// channel/thread a message came from. Workspace credentials live on the
// org-scoped slack_workspace ServiceConnection referenced here.
type SlackConfig struct {
	// ServiceConnectionID is the id of the org-scoped slack_workspace
	// ServiceConnection whose bot this Topic ingests from and posts to.
	// Required.
	ServiceConnectionID string `json:"service_connection_id,omitempty"`
}

// Validate enforces that the workspace connection is set. The id is a
// Helix UUID matched exactly, so non-empty is the only invariant.
func (c SlackConfig) Validate() error {
	if c.ServiceConnectionID == "" {
		return errors.New("slack transport: service_connection_id is required")
	}
	return nil
}

// SlackConfig parses t.Config as a SlackConfig. Same shape and
// semantics as the other per-Kind accessors.
func (t Transport) SlackConfig() (SlackConfig, error) {
	if t.Kind != KindSlack {
		return SlackConfig{}, fmt.Errorf("transport kind is %q, not slack", t.Kind)
	}
	return parseSlackConfig(t.Config)
}

// slack is the Strategy for KindSlack.
type slack struct{}

// ParseConfig satisfies Strategy.
func (slack) ParseConfig(raw json.RawMessage) (Config, error) {
	c, err := parseSlackConfig(raw)
	return c, err
}

// parseSlackConfig is the typed parser. Returns the zero value with no
// error when Config is empty.
func parseSlackConfig(raw json.RawMessage) (SlackConfig, error) {
	var c SlackConfig
	if len(raw) == 0 {
		return c, nil
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return SlackConfig{}, fmt.Errorf("parse slack config: %w", err)
	}
	return c, nil
}
