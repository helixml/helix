package slack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	slackcore "github.com/helixml/helix/api/pkg/serviceconnection/slack"
)

// Publisher is the publish use case the ingest depends on — the
// application Publishing service satisfies it. Publishing an inbound
// event with from="" attributes it to the external sender (no Helix
// Worker source), and the dispatcher's outbound emitter skips
// system-emitted (source="") events, so inbound messages never loop
// straight back out to Slack.
type Publisher interface {
	Publish(ctx context.Context, orgID string, topicID streaming.TopicID, from string, msg streaming.Message) (streaming.Event, error)
}

// Ingest is the one shared inbound path. A single instance serves every
// org and both ingress sources (REST Events API, Socket Mode); OnEvent
// resolves the owning org from the delivery's team id on each call.
// Routing to a specific Worker is NOT done here — OnEvent publishes onto
// the matching Topic and the existing dispatcher + processor/filter
// layer decides which Workers activate.
type Ingest struct {
	workspaces Workspaces
	store      *store.Store
	publisher  Publisher
	logger     *slog.Logger
}

// NewIngest builds the ingest.
func NewIngest(ws Workspaces, st *store.Store, pub Publisher, logger *slog.Logger) *Ingest {
	if logger == nil {
		logger = slog.Default()
	}
	return &Ingest{workspaces: ws, store: st, publisher: pub, logger: logger}
}

// slackExtra is the Slack-specific metadata carried in Message.Extra so
// processor/filter predicates can route on channel/team and outbound
// can stay self-consistent. Message bodies are never logged.
type slackExtra struct {
	Channel string `json:"slack_channel,omitempty"`
	TeamID  string `json:"slack_team_id,omitempty"`
}

// OnEvent is the slackcore.EventHandler the ingress sources call. It:
//
//  1. drops bot-authored events (self-echo guard),
//  2. resolves the owning org from teamID (the workspace install),
//  3. finds every KindSlack Topic in that org bound to this
//     workspace+channel,
//  4. publishes a canonical Message onto each (append → notify →
//     dispatch → processors → Workers).
//
// An unknown team or a channel with no bound Topic is a no-op (nil
// error) — there's nothing to deliver to, and the caller should answer
// Slack 2xx so it stops retrying.
func (i *Ingest) OnEvent(ctx context.Context, teamID string, ev slackcore.Event) error {
	if teamID == "" {
		return errors.New("slack ingest: empty team id")
	}
	if ev.BotID != "" {
		i.logger.Debug("slack.ingest: dropping bot event", "team", teamID, "channel", ev.Channel, "bot_id", ev.BotID)
		return nil
	}

	ws, err := i.workspaces.ByTeamID(ctx, teamID)
	if err != nil {
		if errors.Is(err, ErrNoWorkspace) {
			i.logger.Info("slack.ingest: no workspace install for team — dropping", "team", teamID)
			return nil
		}
		return fmt.Errorf("slack ingest: resolve workspace: %w", err)
	}

	topics, err := i.matchingTopics(ctx, ws)
	if err != nil {
		return err
	}
	if len(topics) == 0 {
		i.logger.Info("slack.ingest: no topic for workspace", "org", ws.OrgID, "workspace", ws.ID)
		return nil
	}

	extra, _ := json.Marshal(slackExtra{Channel: ev.Channel, TeamID: teamID})
	msg := streaming.Message{
		From:      ev.User,
		Body:      ev.Text,
		ThreadID:  ev.ThreadTS,
		MessageID: ev.TS,
		Extra:     extra,
	}

	for _, t := range topics {
		if _, err := i.publisher.Publish(ctx, ws.OrgID, t.ID, "", msg); err != nil {
			i.logger.Error("slack.ingest: publish", "org", ws.OrgID, "topic", t.ID, "err", err)
			continue
		}
		i.logger.Info("slack.ingest", "org", ws.OrgID, "topic", t.ID, "channel", ev.Channel, "from", ev.User)
	}
	return nil
}

// matchingTopics returns the KindSlack Topic(s) in the workspace's org
// bound to this workspace connection. A Slack Topic is workspace-scoped —
// it receives every channel the bot is in — so the only match key is the
// ServiceConnectionID. The org-scoped list enforces tenant isolation.
func (i *Ingest) matchingTopics(ctx context.Context, ws Workspace) ([]streaming.Topic, error) {
	all, err := i.store.Topics.List(ctx, ws.OrgID)
	if err != nil {
		return nil, fmt.Errorf("slack ingest: list topics: %w", err)
	}
	var matched []streaming.Topic
	for _, t := range all {
		if t.Transport.Kind != transport.KindSlack {
			continue
		}
		cfg, err := t.Transport.SlackConfig()
		if err != nil {
			i.logger.Warn("slack.ingest: topic config parse", "topic", t.ID, "err", err)
			continue
		}
		if cfg.ServiceConnectionID == ws.ID {
			matched = append(matched, t)
		}
	}
	return matched, nil
}

// compile-time assertion that OnEvent matches the generic handler type.
var _ slackcore.EventHandler = (*Ingest)(nil).OnEvent
