package slack

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	slackcore "github.com/helixml/helix/api/pkg/serviceconnection/slack"
)

// Outbound delivers an appended Event on a KindSlack Topic to its bound
// channel via chat.postMessage, under the posting Worker's persona. It
// satisfies streaming.Outbound, so the dispatcher emits it for every
// worker-published Event the same way it does for webhook and email.
type Outbound struct {
	workspaces Workspaces
	persona    PersonaResolver
	apiURL     string
	logger     *slog.Logger
}

// NewOutbound builds the emitter. A nil persona resolver falls back to
// DefaultPersona (bare worker id as username, bot avatar).
func NewOutbound(ws Workspaces, persona PersonaResolver, logger *slog.Logger) *Outbound {
	if persona == nil {
		persona = DefaultPersona
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Outbound{workspaces: ws, persona: persona, logger: logger}
}

// SetAPIURL overrides the Slack API base (trailing slash). Tests point
// it at an httptest.Server; production leaves it empty.
func (o *Outbound) SetAPIURL(u string) { o.apiURL = u }

// Emit posts the Event's Message body to the Topic's bound channel under
// the Worker's persona, preserving thread context. A missing workspace
// install is a typed error (not a panic): the dispatcher logs and drops
// it; the underlying append already succeeded.
func (o *Outbound) Emit(ctx context.Context, topic streaming.Topic, event streaming.Event) error {
	msg, err := event.Message()
	if err != nil {
		return fmt.Errorf("slack emit: parse message: %w", err)
	}
	sc, err := topic.Transport.SlackConfig()
	if err != nil {
		return fmt.Errorf("slack emit: topic config: %w", err)
	}
	if sc.Channel == "" {
		return errors.New("slack emit: topic has no channel")
	}
	ws, err := o.workspaces.ByID(ctx, sc.ServiceConnectionID)
	if err != nil {
		return fmt.Errorf("slack emit: workspace %q: %w", sc.ServiceConnectionID, err)
	}

	persona, err := o.persona(ctx, event.OrganizationID, event.Source)
	if err != nil {
		o.logger.Warn("slack.emit: persona resolve", "org", event.OrganizationID, "worker", event.Source, "err", err)
		persona = slackcore.Persona{}
	}

	client := slackcore.New(ws.BotToken, o.apiURL)
	ts, err := slackcore.PostAs(ctx, client, sc.Channel, msg.ThreadID, persona, msg.Body)
	if err != nil {
		return fmt.Errorf("slack emit: postMessage to %s: %w", sc.Channel, err)
	}
	o.logger.Info("slack.emit", "org", event.OrganizationID, "topic", topic.ID, "channel", sc.Channel, "ts", ts, "persona", persona.Username)
	return nil
}

// compile-time assertion that Outbound satisfies the port.
var _ streaming.Outbound = (*Outbound)(nil)
