package slack

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/slack-go/slack"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// Outbound delivers an appended Event on a KindSlack stream to its bound
// channel via chat.postMessage, applying the posting Worker's persona
// (FR-10/11). It satisfies streaming.Outbound, so the dispatcher emits
// it for every worker-published Event the same way it does for webhook
// and email — ingress-agnostic by construction (FR-12).
type Outbound struct {
	registry *configregistry.Registry
	store    *store.Store
	persona  PersonaResolver
	apiURL   string
	logger   *slog.Logger
}

// NewOutbound builds the emitter. A nil persona resolver falls back to
// DefaultPersonaResolver (bare worker id as username, bot avatar).
func NewOutbound(reg *configregistry.Registry, st *store.Store, persona PersonaResolver, logger *slog.Logger) *Outbound {
	if persona == nil {
		persona = DefaultPersonaResolver
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Outbound{registry: reg, store: st, persona: persona, logger: logger}
}

// SetAPIURL overrides the Slack API base (trailing slash). Tests point
// it at an httptest.Server; production leaves it empty (real slack.com).
func (o *Outbound) SetAPIURL(u string) { o.apiURL = u }

// Emit posts the Event's Message body to the stream's bound channel
// under the Worker's persona, preserving thread context where present
// (FR-13). A missing per-org bot token is a typed error (not a panic):
// the dispatcher logs and drops it, the underlying append already
// succeeded.
func (o *Outbound) Emit(ctx context.Context, stream streaming.Stream, event streaming.Event) error {
	msg, err := event.Message()
	if err != nil {
		return fmt.Errorf("slack emit: parse message: %w", err)
	}
	sc, err := stream.Transport.SlackConfig()
	if err != nil {
		return fmt.Errorf("slack emit: stream config: %w", err)
	}
	if sc.Channel == "" {
		return errors.New("slack emit: stream has no channel")
	}
	cfg, err := readConfig(ctx, o.registry, event.OrganizationID)
	if err != nil {
		return fmt.Errorf("slack emit: org %s not installed: %w", event.OrganizationID, err)
	}

	persona, err := o.persona(ctx, event.OrganizationID, event.Source)
	if err != nil {
		// Persona resolution failure must not block the post — fall back
		// to the bot's default identity.
		o.logger.Warn("slack.emit: persona resolve", "org", event.OrganizationID, "worker", event.Source, "err", err)
		persona = Persona{}
	}

	opts := []slack.MsgOption{slack.MsgOptionText(msg.Body, false)}
	if persona.Username != "" {
		opts = append(opts, slack.MsgOptionUsername(persona.Username))
	}
	if persona.IconURL != "" {
		opts = append(opts, slack.MsgOptionIconURL(persona.IconURL))
	}
	if msg.ThreadID != "" {
		opts = append(opts, slack.MsgOptionTS(msg.ThreadID))
	}

	client := newSlackClient(cfg.BotToken, o.apiURL)
	_, ts, err := client.PostMessageContext(ctx, sc.Channel, opts...)
	if err != nil {
		return fmt.Errorf("slack emit: postMessage to %s: %w", sc.Channel, err)
	}
	o.logger.Info("slack.emit", "org", event.OrganizationID, "stream", event.StreamID, "channel", sc.Channel, "ts", ts, "persona", persona.Username)
	return nil
}

// compile-time assertion that Outbound satisfies the port.
var _ streaming.Outbound = (*Outbound)(nil)
