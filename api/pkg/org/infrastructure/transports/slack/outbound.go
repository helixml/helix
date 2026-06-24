package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	slackcore "github.com/helixml/helix/api/pkg/serviceconnection/slack"
)

// Outbound delivers a worker's reply on a workspace-scoped KindSlack Topic
// back to the channel + thread the triggering message came from. A Slack
// Topic spans every channel the bot is in, so there's no fixed channel:
// the target is resolved from the inbound event the reply is answering.
type Outbound struct {
	workspaces Workspaces
	events     store.Events
	persona    PersonaResolver
	apiURL     string
	logger     *slog.Logger
}

// NewOutbound builds the emitter. A nil persona resolver falls back to
// DefaultPersona. events is used to resolve the reply's target channel
// from the inbound message it answers.
func NewOutbound(ws Workspaces, events store.Events, persona PersonaResolver, logger *slog.Logger) *Outbound {
	if persona == nil {
		persona = DefaultPersona
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Outbound{workspaces: ws, events: events, persona: persona, logger: logger}
}

// SetAPIURL overrides the Slack API base (trailing slash). Tests point
// it at an httptest.Server; production leaves it empty.
func (o *Outbound) SetAPIURL(u string) { o.apiURL = u }

func extraChannel(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var e slackExtra
	if json.Unmarshal(raw, &e) != nil {
		return ""
	}
	return e.Channel
}

// resolveTarget finds the channel + thread a reply should post to. The
// reply itself usually carries neither (workers just publish a body), so
// we look at the inbound Slack messages on this topic: prefer the one the
// reply threads under (ThreadID/InReplyTo matches its MessageID), else the
// most recent inbound message. Returns ("", "") when nothing resolves.
func (o *Outbound) resolveTarget(ctx context.Context, topic streaming.Topic, reply streaming.Message) (channel, thread string) {
	// An explicit channel on the reply wins (a processor or worker set it).
	if ch := extraChannel(reply.Extra); ch != "" {
		return ch, reply.ThreadID
	}
	if o.events == nil {
		return "", reply.ThreadID
	}
	anchor := reply.InReplyTo
	if anchor == "" {
		anchor = reply.ThreadID
	}
	events, err := o.events.ListForTopic(ctx, topic.OrganizationID, topic.ID, 50)
	if err != nil {
		o.logger.Warn("slack.emit: list events", "topic", topic.ID, "err", err)
		return "", reply.ThreadID
	}
	var latestChannel, latestThread string
	for _, e := range events {
		if e.Source != "" { // skip worker-published events; we want inbound
			continue
		}
		m, err := e.Message()
		if err != nil {
			continue
		}
		ch := extraChannel(m.Extra)
		if ch == "" {
			continue
		}
		// Exact thread match → reply in that thread/channel.
		if anchor != "" && m.MessageID == anchor {
			thr := m.ThreadID
			if thr == "" {
				thr = m.MessageID
			}
			return ch, thr
		}
		if latestChannel == "" { // ListForTopic is newest-first
			latestChannel = ch
			latestThread = m.ThreadID
			if latestThread == "" {
				latestThread = m.MessageID
			}
		}
	}
	if reply.ThreadID != "" {
		return latestChannel, reply.ThreadID
	}
	return latestChannel, latestThread
}

// Emit posts the worker's reply to the channel + thread it's answering,
// under the worker's persona. A missing workspace install or unresolvable
// channel is a typed error: the dispatcher logs and drops it (the append
// already succeeded).
func (o *Outbound) Emit(ctx context.Context, topic streaming.Topic, event streaming.Event) error {
	msg, err := event.Message()
	if err != nil {
		return fmt.Errorf("slack emit: parse message: %w", err)
	}
	sc, err := topic.Transport.SlackConfig()
	if err != nil {
		return fmt.Errorf("slack emit: topic config: %w", err)
	}
	ws, err := o.workspaces.ByID(ctx, sc.ServiceConnectionID)
	if err != nil {
		return fmt.Errorf("slack emit: workspace %q: %w", sc.ServiceConnectionID, err)
	}

	channel, thread := o.resolveTarget(ctx, topic, msg)
	if channel == "" {
		o.logger.Warn("slack.emit: could not resolve a channel for reply — dropping", "org", event.OrganizationID, "topic", topic.ID)
		return nil
	}

	persona, err := o.persona(ctx, event.OrganizationID, event.Source)
	if err != nil {
		o.logger.Warn("slack.emit: persona resolve", "org", event.OrganizationID, "worker", event.Source, "err", err)
		persona = slackcore.Persona{}
	}

	client := slackcore.New(ws.BotToken, o.apiURL)
	ts, err := slackcore.PostAs(ctx, client, channel, thread, persona, msg.Body)
	if err != nil {
		return fmt.Errorf("slack emit: postMessage to %s: %w", channel, err)
	}
	o.logger.Info("slack.emit", "org", event.OrganizationID, "topic", topic.ID, "channel", channel, "ts", ts, "persona", persona.Username)
	return nil
}

// compile-time assertion that Outbound satisfies the port.
var _ streaming.Outbound = (*Outbound)(nil)
