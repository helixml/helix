package slack

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/infrastructure/wakebus"
)

// routerConfigKey is the per-org operational-config key naming the
// Router implementation for inbound Slack disambiguation (§9.4).
const routerConfigKey = "slack.router"

// Event is the transport-neutral inbound Slack event the ingest
// consumes. Both ingress sources (REST Events API, Socket Mode)
// normalise their provider-specific payloads into this shape before
// calling Receive, so the processing path is identical across modes
// (FR-19 / NFR-1).
type Event struct {
	// Channel is the Slack channel id the message landed in. Routed
	// against each org-scoped KindSlack stream's SlackConfig.Channel.
	Channel string
	// User is the Slack user id of the poster. Carried verbatim as
	// Message.From.
	User string
	// Text is the message body.
	Text string
	// TS is the Slack message timestamp ("1700000000.000100"). Unique
	// per message; carried as Message.MessageID.
	TS string
	// ThreadTS is the parent message ts when the message is in a thread;
	// empty for top-level messages. Carried as Message.ThreadID so
	// outbound replies can preserve threading (FR-13).
	ThreadTS string
	// BotID is non-empty when the message was posted by a bot (including
	// our own shared bot). Used as the self-echo guard (FR-20): any
	// bot-authored event is dropped so Workers' own posts — and other
	// bots — never re-enter as inbound events.
	BotID string
	// AppID is the Slack app id that authored the message, when present.
	AppID string
}

// Dispatcher is the subset of the application dispatcher the ingest
// needs. Defining it here keeps the import edge one-directional. The
// ingest resolves the channel's subscribers, narrows them through the
// per-org Router, and activates exactly the chosen targets — so it uses
// the router-aware DispatchTo rather than the broadcast Dispatch.
type Dispatcher interface {
	DispatchTo(ctx context.Context, event streaming.Event, targets []orgchart.WorkerID)
}

// Receiver is the inbound seam the ingress sources (REST Events API,
// Socket Mode) depend on. Both normalise their provider payloads into a
// transport-neutral Event and call Receive — the single shared
// processing path (FR-19). *Ingest is the production implementation.
type Receiver interface {
	Receive(ctx context.Context, teamID string, ev Event) error
}

// compile-time assertion that Ingest is a Receiver.
var _ Receiver = (*Ingest)(nil)

// Ingest is the one shared inbound path (§9.1). A single instance
// serves every org and both ingress sources; Receive resolves the
// owning org from the event's team id on each call.
type Ingest struct {
	registry    *configregistry.Registry
	store       *store.Store
	broadcaster *wakebus.Bus
	dispatcher  Dispatcher
	logger      *slog.Logger
}

// NewIngest builds the ingest. broadcaster and dispatcher may be nil in
// tests that don't exercise those paths.
func NewIngest(reg *configregistry.Registry, st *store.Store, bc *wakebus.Bus, d Dispatcher, logger *slog.Logger) *Ingest {
	if logger == nil {
		logger = slog.Default()
	}
	return &Ingest{
		registry:    reg,
		store:       st,
		broadcaster: bc,
		dispatcher:  d,
		logger:      logger,
	}
}

// errUnknownTeam signals that no installed org owns the delivery's team
// id. Treated as a drop, not an error, by Receive's callers.
var errUnknownTeam = errors.New("no org installed for team id")

// Receive is the single entry point for inbound Slack events. It:
//
//  1. resolves the owning org from teamID (per-org install; FR-17),
//  2. drops the bot's own events (self-echo guard; FR-20),
//  3. finds every KindSlack stream in that org bound to the channel,
//  4. builds a canonical Message envelope,
//  5. appends an Event, wakes long-poll observers, and dispatches to
//     subscribed Workers (existing fan-out; FR-18).
//
// An unknown team id or a channel with no bound stream is a no-op (nil
// error) — there is nothing to deliver to, and the caller should answer
// the provider 2xx so it stops retrying.
func (i *Ingest) Receive(ctx context.Context, teamID string, ev Event) error {
	if teamID == "" {
		return errors.New("slack ingest: empty team id")
	}
	// Self-echo guard: any bot-authored message (our shared bot, or any
	// other bot in the channel) is dropped so Workers' own posts never
	// loop back in (FR-20).
	if ev.BotID != "" {
		i.logger.Debug("slack.ingest: dropping bot event", "team", teamID, "channel", ev.Channel, "bot_id", ev.BotID)
		return nil
	}

	orgID, err := i.resolveOrg(ctx, teamID)
	if err != nil {
		if errors.Is(err, errUnknownTeam) {
			i.logger.Info("slack.ingest: no org for team — dropping", "team", teamID)
			return nil
		}
		return err
	}

	streams, err := i.matchingStreams(ctx, orgID, ev.Channel)
	if err != nil {
		return err
	}
	if len(streams) == 0 {
		i.logger.Info("slack.ingest: no bound stream for channel", "org", orgID, "channel", ev.Channel)
		return nil
	}

	msg := streaming.Message{
		From:      ev.User,
		Body:      ev.Text,
		ThreadID:  ev.ThreadTS,
		MessageID: ev.TS,
	}

	for _, s := range streams {
		event, err := streaming.NewMessageEvent(
			streaming.EventID("e-"+uuid.NewString()),
			s.ID,
			"", // system-emitted: external sender, no helix Worker source
			msg,
			nowUTC(),
			orgID,
		)
		if err != nil {
			i.logger.Error("slack.ingest: build event", "stream", s.ID, "err", err)
			continue
		}
		if err := i.store.Events.Append(ctx, event); err != nil {
			i.logger.Error("slack.ingest: append", "stream", s.ID, "err", err)
			continue
		}
		if i.broadcaster != nil {
			i.broadcaster.Notify(orgID, s.ID)
		}
		if i.dispatcher != nil {
			targets, err := i.route(ctx, orgID, s, msg)
			if err != nil {
				i.logger.Error("slack.ingest: route", "stream", s.ID, "err", err)
				continue
			}
			i.dispatcher.DispatchTo(ctx, event, targets)
		}
		i.logger.Info("slack.ingest", "org", orgID, "stream", s.ID, "channel", ev.Channel, "from", ev.User)
	}
	return nil
}

// route resolves the channel's subscribers into routing Candidates,
// picks the per-org Router (slack.router config; default broadcast),
// and returns the WorkerIDs to activate. The Router only ever narrows
// the subscriber set — DispatchTo enforces that a returned id outside
// the subscriber/known-worker set is skipped, so routing can't invent a
// recipient.
func (i *Ingest) route(ctx context.Context, orgID string, stream streaming.Stream, msg streaming.Message) ([]orgchart.WorkerID, error) {
	subs, err := i.store.Subscriptions.ListForStream(ctx, orgID, stream.ID)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	candidates := make([]Candidate, 0, len(subs))
	for _, sub := range subs {
		workerID := orgchart.WorkerID(sub.WorkerID)
		identity := i.candidateText(ctx, orgID, workerID)
		candidates = append(candidates, Candidate{WorkerID: workerID, Identity: identity})
	}

	routerName, err := i.registry.GetString(ctx, orgID, routerConfigKey)
	if err != nil {
		// Unset / not-configured → broadcast default.
		routerName = ""
	}
	router := RouterFor(routerName)
	return router.Route(ctx, Inbound{
		OrgID:       orgID,
		Stream:      stream,
		Message:     msg,
		Subscribers: candidates,
	})
}

// candidateText gathers the Worker's identity + role text for fuzzy
// matching. A lookup failure degrades to empty text (the candidate
// simply scores zero), never an error — routing must not fail because
// one worker row is missing.
func (i *Ingest) candidateText(ctx context.Context, orgID string, workerID orgchart.WorkerID) string {
	var sb strings.Builder
	w, err := i.store.Workers.Get(ctx, orgID, workerID)
	if err != nil {
		return ""
	}
	sb.WriteString(w.IdentityContent())
	if role, err := i.store.Roles.Get(ctx, orgID, w.RoleID()); err == nil {
		sb.WriteByte(' ')
		sb.WriteString(role.Content)
	}
	return sb.String()
}

// resolveOrg maps a Slack team id to the org that installed the app
// into that workspace. It scans the orgs that own KindSlack streams and
// reads each one's per-org install config; the org whose configured
// team id matches wins. One workspace per org (§6) guarantees at most
// one match. Orgs with an install but no streams need no resolution —
// there is nothing to deliver to.
func (i *Ingest) resolveOrg(ctx context.Context, teamID string) (string, error) {
	streams, err := i.store.Streams.ListByTransportKind(ctx, transport.KindSlack)
	if err != nil {
		return "", fmt.Errorf("list slack streams: %w", err)
	}
	seen := map[string]bool{}
	for _, s := range streams {
		if seen[s.OrganizationID] {
			continue
		}
		seen[s.OrganizationID] = true
		cfg, err := readConfig(ctx, i.registry, s.OrganizationID)
		if err != nil {
			continue
		}
		if cfg.TeamID == teamID {
			return s.OrganizationID, nil
		}
	}
	return "", errUnknownTeam
}

// matchingStreams returns every KindSlack stream in the org bound to
// the given channel. Org-scoped list enforces isolation (FR-5): a
// delivery for org A's workspace can only ever match org A's streams.
func (i *Ingest) matchingStreams(ctx context.Context, orgID, channel string) ([]streaming.Stream, error) {
	all, err := i.store.Streams.List(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list streams: %w", err)
	}
	var matched []streaming.Stream
	for _, s := range all {
		if s.Transport.Kind != transport.KindSlack {
			continue
		}
		cfg, err := s.Transport.SlackConfig()
		if err != nil {
			i.logger.Warn("slack.ingest: stream config parse", "stream", s.ID, "err", err)
			continue
		}
		if cfg.Channel == channel {
			matched = append(matched, s)
		}
	}
	return matched, nil
}
