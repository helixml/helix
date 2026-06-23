// Package slack is the org-graph wiring for the Slack transport. It
// turns the generic Slack protocol layer (api/pkg/serviceconnection/slack)
// into a KindSlack Topic transport: inbound events publish onto matching
// org Topics, outbound publishes post back as the Worker's persona, and
// the provisioner joins the shared bot to a Topic's channel.
//
// All multi-tenancy lives here. The generic layer knows nothing about
// orgs; this package resolves a Slack team_id to the org that installed
// the workspace (a slack_workspace ServiceConnection) and only ever
// touches that org's Topics.
package slack

import (
	"context"
	"errors"

	slackcore "github.com/helixml/helix/api/pkg/serviceconnection/slack"
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

// Workspaces resolves Slack workspace installs. Implemented at the
// composition root over the helix ServiceConnection store (+ bot-token
// decryption). The two lookups keep the import edge one-directional —
// this package never imports the helix store.
type Workspaces interface {
	// ByTeamID resolves the org-scoped workspace a Slack delivery
	// (team_id) belongs to — the inbound routing key.
	ByTeamID(ctx context.Context, teamID string) (Workspace, error)
	// ByID resolves a workspace by its ServiceConnection id (the value a
	// Topic's SlackConfig.ServiceConnectionID holds) — the outbound /
	// provisioning key.
	ByID(ctx context.Context, id string) (Workspace, error)
}

// PersonaResolver maps a posting Worker to its display persona for
// outbound messages. Injected as a port so the shared envelope stays
// transport-neutral.
type PersonaResolver func(ctx context.Context, orgID, workerID string) (slackcore.Persona, error)

// DefaultPersona uses the bare Worker id as the username and no avatar
// (Slack shows the bot's default). A richer resolver can be injected at
// the composition root without touching the outbound path.
func DefaultPersona(_ context.Context, _, workerID string) (slackcore.Persona, error) {
	return slackcore.Persona{Username: workerID}, nil
}
