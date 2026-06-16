package slack

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/slack-go/slack"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// Provisioner is the slack side of the streaming.Inbound port. "Install"
// for Slack means ensuring the org's shared bot is a member of the bound
// channel (conversations.join) — there is no per-stream webhook to
// register, since inbound flows through the one shared ingest. "Status"
// reports whether the bot is in the channel, degrading to "unknown"
// rather than erroring when the workspace can't be reached.
type Provisioner struct {
	registry *configregistry.Registry
	apiURL   string
	logger   *slog.Logger
}

// NewProvisioner builds the provisioner.
func NewProvisioner(reg *configregistry.Registry, logger *slog.Logger) *Provisioner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Provisioner{registry: reg, logger: logger}
}

// SetAPIURL overrides the Slack API base (trailing slash). Tests point
// it at an httptest.Server; production leaves it empty.
func (p *Provisioner) SetAPIURL(u string) { p.apiURL = u }

// Install ensures the bot has joined the stream's bound channel. It is
// idempotent on Slack's side — joining a channel the bot is already in
// is a success. Returns a nil Config (nothing extra to persist on the
// stream; the channel binding is already in SlackConfig).
func (p *Provisioner) Install(ctx context.Context, orgID string, stream streaming.Stream) (streaming.InstallResult, error) {
	sc, err := stream.Transport.SlackConfig()
	if err != nil {
		return streaming.InstallResult{}, &streaming.Failure{Kind: streaming.FailBadRequest, Err: fmt.Errorf("slack install: %w", err)}
	}
	if sc.Channel == "" {
		return streaming.InstallResult{}, &streaming.Failure{Kind: streaming.FailBadRequest, Err: fmt.Errorf("slack install: stream has no channel")}
	}
	cfg, err := readConfig(ctx, p.registry, orgID)
	if err != nil {
		return streaming.InstallResult{}, &streaming.Failure{Kind: streaming.FailPrecondition, Err: fmt.Errorf("slack install: org %s not connected: %w", orgID, err)}
	}
	client := newSlackClient(cfg.BotToken, p.apiURL)
	if _, _, _, err := client.JoinConversationContext(ctx, sc.Channel); err != nil {
		return streaming.InstallResult{}, &streaming.Failure{Kind: streaming.FailUpstream, Err: fmt.Errorf("slack install: join %s: %w", sc.Channel, err)}
	}
	p.logger.Info("slack.install", "org", orgID, "channel", sc.Channel, "stream", stream.ID)
	return streaming.InstallResult{}, nil
}

// Status reports whether the bot is a member of the bound channel:
// "installed" when it is, "missing" when it isn't, "unknown" when the
// org isn't connected or the workspace can't be reached. Never errors —
// "can't tell" degrades to unknown per the Inbound contract.
func (p *Provisioner) Status(ctx context.Context, orgID string, stream streaming.Stream) (streaming.InboundState, error) {
	sc, err := stream.Transport.SlackConfig()
	if err != nil {
		return streaming.InboundState{State: "unknown", Detail: err.Error()}, nil
	}
	cfg, err := readConfig(ctx, p.registry, orgID)
	if err != nil {
		return streaming.InboundState{State: "unknown", Detail: "org not connected to Slack"}, nil
	}
	client := newSlackClient(cfg.BotToken, p.apiURL)
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
