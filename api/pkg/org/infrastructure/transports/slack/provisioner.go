package slack

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

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

// AutoInstallOnCreate opts the Slack provisioner into create-time
// installation: when a KindSlack Topic is created, the topics service
// runs Install so the bot self-joins a public channel without a separate
// step. (GitHub does not implement this — it has an explicit webhook
// install flow.)
func (p *Provisioner) AutoInstallOnCreate() bool { return true }

// Install ensures the bot has joined the Topic's bound channel.
// `conversations.join` works for PUBLIC channels (idempotent — joining a
// channel the bot is already in succeeds). It is NOT supported for
// private channels: Slack offers no remote way for a bot to add itself,
// so a human must `/invite` it. That case is reported as a non-fatal
// Notice (not an error) — the Topic is valid; it just won't receive
// messages until the bot is invited. Returns a nil Config (the channel
// binding is already in SlackConfig).
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
		if needsManualInvite(err) {
			// Private channel (or one the bot can't see): self-join is
			// impossible by design. Tell the user to invite the bot.
			p.logger.Info("slack.install: self-join not possible — manual invite required", "org", orgID, "channel", sc.Channel, "topic", topic.ID, "reason", err.Error())
			return streaming.InstallResult{Notice: manualInviteNotice(sc.Channel)}, nil
		}
		return streaming.InstallResult{}, &streaming.Failure{Kind: streaming.FailUpstream, Err: fmt.Errorf("slack install: join %s: %w", sc.Channel, err)}
	}
	p.logger.Info("slack.install: bot joined channel", "org", orgID, "channel", sc.Channel, "topic", topic.ID)
	return streaming.InstallResult{}, nil
}

// needsManualInvite reports whether a conversations.join error means the
// bot cannot self-join and a human must invite it. Private channels
// return method_not_supported_for_channel_type; a private channel the
// bot can't see returns channel_not_found.
func needsManualInvite(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "method_not_supported_for_channel_type") ||
		strings.Contains(msg, "channel_not_found") ||
		strings.Contains(msg, "is_private")
}

// manualInviteNotice is the human-facing guidance shown when the bot
// couldn't self-join. The message lives in the domain/transport layer,
// not the UI — the UI only renders the string.
func manualInviteNotice(channel string) string {
	return fmt.Sprintf("Topic created. The Helix bot couldn't auto-join channel %s — it's private (or the bot can't see it). Invite the bot in Slack with /invite, and it will start receiving messages.", channel)
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
