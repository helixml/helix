package slack

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/slack-go/slack"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	slackcore "github.com/helixml/helix/api/pkg/serviceconnection/slack"
)

// Provisioner is the slack side of the streaming.Inbound port. "Install"
// for Slack means ensuring the org's shared bot is a member of the
// Topic's bound channel (conversations.join) — there is no per-Topic
// webhook to register, since inbound flows through the one shared
// ingest. "Status" reports whether the bot is in the channel, degrading
// to "unknown" rather than erroring when the workspace can't be reached.
type Provisioner struct {
	workspaces Workspaces
	apiURL     string
	logger     *slog.Logger
}

// NewProvisioner builds the provisioner.
func NewProvisioner(ws Workspaces, logger *slog.Logger) *Provisioner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Provisioner{workspaces: ws, logger: logger}
}

// SetAPIURL overrides the Slack API base (trailing slash). Tests point
// it at an httptest.Server; production leaves it empty.
func (p *Provisioner) SetAPIURL(u string) { p.apiURL = u }

// Install ensures the bot has joined the Topic's bound channel.
// Idempotent on Slack's side. Returns a nil Config (the channel binding
// is already in SlackConfig — nothing extra to persist).
func (p *Provisioner) Install(ctx context.Context, orgID string, topic streaming.Topic) (streaming.InstallResult, error) {
	sc, err := topic.Transport.SlackConfig()
	if err != nil {
		return streaming.InstallResult{}, &streaming.Failure{Kind: streaming.FailBadRequest, Err: fmt.Errorf("slack install: %w", err)}
	}
	ws, err := p.workspaces.ByID(ctx, sc.ServiceConnectionID)
	if err != nil {
		return streaming.InstallResult{}, &streaming.Failure{Kind: streaming.FailPrecondition, Err: fmt.Errorf("slack install: workspace %q: %w", sc.ServiceConnectionID, err)}
	}
	client := slackcore.New(ws.BotToken, p.apiURL)
	if _, _, _, err := client.JoinConversationContext(ctx, sc.Channel); err != nil {
		return streaming.InstallResult{}, &streaming.Failure{Kind: streaming.FailUpstream, Err: fmt.Errorf("slack install: join %s: %w", sc.Channel, err)}
	}
	p.logger.Info("slack.install", "org", orgID, "channel", sc.Channel, "topic", topic.ID)
	return streaming.InstallResult{}, nil
}

// Status reports whether the bot is a member of the bound channel:
// "installed" when it is, "missing" when it isn't, "unknown" when the
// workspace isn't resolvable or can't be reached. Never errors.
func (p *Provisioner) Status(ctx context.Context, orgID string, topic streaming.Topic) (streaming.InboundState, error) {
	sc, err := topic.Transport.SlackConfig()
	if err != nil {
		return streaming.InboundState{State: "unknown", Detail: err.Error()}, nil
	}
	ws, err := p.workspaces.ByID(ctx, sc.ServiceConnectionID)
	if err != nil {
		return streaming.InboundState{State: "unknown", Detail: "workspace not connected"}, nil
	}
	client := slackcore.New(ws.BotToken, p.apiURL)
	ch, err := client.GetConversationInfoContext(ctx, &slack.GetConversationInfoInput{ChannelID: sc.Channel})
	if err != nil {
		p.logger.Warn("slack.status: conversations.info", "org", orgID, "channel", sc.Channel, "err", err)
		return streaming.InboundState{State: "unknown", Detail: err.Error()}, nil
	}
	if ch.IsMember {
		return streaming.InboundState{State: "installed", Active: true}, nil
	}
	return streaming.InboundState{State: "missing"}, nil
}

// compile-time assertion that Provisioner satisfies the port.
var _ streaming.Inbound = (*Provisioner)(nil)
