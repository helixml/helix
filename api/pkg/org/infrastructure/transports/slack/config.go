// Package slack implements helix-org's Slack transport. It binds
// helix-org Streams to Slack channels and gives Workers a way to post
// to and receive from those channels as personas of a single shared
// per-org bot.
//
// Three tenancy layers, three storage homes
// (design/2026-06-16-helix-org-slack-stream.md §9.2):
//
//   - Global app (instance-wide, admin): client id/secret, signing
//     secret, app-level token, ingress-mode toggle — an
//     OAuthProvider(type=slack) row, read through the GlobalApp port.
//   - Per-org install: bot token (xoxb-…) + team id — the operational
//     config registry key `transport.slack` (this file's Config).
//   - Channel binding: one Slack channel id — per-stream
//     transport.SlackConfig.
//
// Inbound flows through one shared path (ingest.Receive) fed by either
// the REST Events API source or the Socket Mode source. Outbound is
// ingress-agnostic: the dispatcher emits a chat.postMessage per
// appended Event on a KindSlack stream.
package slack

import (
	"context"
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
)

// nowUTC returns the current wall-clock time in UTC. Matches the helper
// convention the other transports use; trivial to override in a test if
// a deterministic timestamp is ever needed.
func nowUTC() time.Time { return time.Now().UTC() }

// configKey is the operational-config registry key under which each
// org's Slack installation (bot token + team id) is stored.
const configKey = "transport.slack"

// Config is the parsed shape of the per-org operational-config row
// `transport.slack`. Read on every operation so an install (or a
// re-install via OAuth) takes effect without a restart.
type Config struct {
	// BotToken is the org's bot user OAuth token (xoxb-…), minted when
	// the org installed the global app into its workspace. Used as the
	// auth token for every outbound chat.postMessage and for membership
	// checks. Secret — redacted/encrypted at the registry layer.
	BotToken string `json:"bot_token"`

	// TeamID is the Slack workspace/team id (T0123…). It is the routing
	// key the inbound ingest uses to map a delivery back to this org
	// (FR-17). Exactly one org owns a given team id (one workspace per
	// org — §6 Out of Scope).
	TeamID string `json:"team_id"`
}

// Validate enforces both fields are present. A row missing either is
// an incomplete install and the transport treats the org as not
// connected.
func (c Config) Validate() error {
	if c.BotToken == "" {
		return errors.New("bot_token is empty")
	}
	if c.TeamID == "" {
		return errors.New("team_id is empty")
	}
	return nil
}

// readConfig loads and validates the per-org install config.
func readConfig(ctx context.Context, reg *configregistry.Registry, orgID string) (Config, error) {
	var c Config
	if err := reg.GetObject(ctx, orgID, configKey, &c); err != nil {
		return Config{}, err
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}
