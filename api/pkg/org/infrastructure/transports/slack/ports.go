// Package slack is the org-graph wiring for the Slack transport. It
// turns the generic Slack protocol layer (api/pkg/serviceconnection/slack)
// into a KindSlack Topic transport: inbound events publish onto the
// workspace's org Topic. Egress is the agent's job — a Worker mints the
// workspace bot token (mint_credential provider=slack) and drives the
// Slack Web API directly, guided by the transport-provided reply hint on
// the inbound Message.
//
// All multi-tenancy lives here. The generic layer knows nothing about
// orgs; this package resolves a Slack team_id to the org that installed
// the workspace (a slack_workspace ServiceConnection) and only ever
// touches that org's Topics.
package slack

import (
	"context"
	"errors"
)

// Workspace is the org-scoped Slack workspace install — the subset of a
// slack_workspace ServiceConnection the transport needs. BotToken is
// already decrypted.
type Workspace struct {
	ID       string // ServiceConnection id (Topic.SlackConfig.ServiceConnectionID)
	OrgID    string
	TeamID   string
	BotToken string
}

// ErrNoWorkspace is returned by Workspaces lookups when no install
// matches. Treated as a drop (not an error) on the inbound path.
var ErrNoWorkspace = errors.New("no slack workspace install for that id/team")

var ErrAmbiguousWorkspace = errors.New("multiple Slack workspaces are installed; slack_team_id is required")

// Workspaces resolves Slack workspace installs. Implemented at the
// composition root over the helix ServiceConnection store (+ bot-token
// decryption). The import edge stays one-directional — this package
// never imports the helix store.
type Workspaces interface {
	// ByTeamID resolves the org-scoped workspace a Slack delivery
	// (team_id) belongs to — the inbound routing key.
	ByTeamID(ctx context.Context, teamID string) (Workspace, error)
}
