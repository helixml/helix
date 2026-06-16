package transport

import (
	"encoding/json"
	"errors"
	"fmt"
)

// KindSlack binds a Stream to a single Slack channel within its org's
// installed Slack workspace. The three tenancy layers live apart:
//
//   - The global Slack *app* (client id/secret, signing secret, app
//     token, ingress-mode toggle) is an admin-configured OAuthProvider
//     row, instance-wide.
//   - The per-org *installation* (bot token + team id) lives in the
//     operational config registry under `transport.slack`.
//   - This per-stream config carries only the channel binding.
//
// Inbound: Slack delivers channel messages over one shared ingest path
// (REST Events API or Socket Mode — operator's choice). The ingest
// routes team_id → org, drops the bot's own events, then fans the
// message out to every Stream in that org whose SlackConfig.Channel
// matches.
//
// Outbound: a Worker's publish to a slack Stream becomes a
// chat.postMessage under the Worker's persona (name + avatar) through
// the org's shared bot.
//
// See design/2026-06-16-helix-org-slack-stream.md.
const KindSlack Kind = "slack"

// SlackConfig is the parsed shape of Transport.Config when
// Kind == KindSlack. Workspace credentials are not here — they live at
// the per-org install layer (configregistry `transport.slack`); this is
// purely the channel binding.
type SlackConfig struct {
	// Channel is the Slack channel id (e.g. "C0123ABCD") this Stream is
	// bound to. Required. Inbound messages in this channel fan out to
	// the Stream's subscribers; outbound publishes post here.
	Channel string `json:"channel,omitempty"`
}

// Validate enforces that a channel is set. The channel id format is not
// pinned — Slack mints opaque ids and we route on exact match, so a
// non-empty value is the only invariant.
func (c SlackConfig) Validate() error {
	if c.Channel == "" {
		return errors.New("slack transport: channel is required")
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

// parseSlackConfig is the typed parser.
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
