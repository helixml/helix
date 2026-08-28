// Package publishing owns the publish use case — the append→notify→route
// trio that must stay atomic and ordered.
//
// Every event enters the org through exactly one terminal source: a
// Trigger (an inbound transport, or a system-managed internal channel) or
// one durable output branch of a Processor. Publish appends the event to
// the stream that source owns, wakes long-poll observers on that stream,
// then hands the event to the Router, which fans it out to the Workers
// attached to the source and the Processors reading it.
//
// Notifier and Router are optional collaborators behind narrow interfaces
// so the service does not depend on the concrete wakebus.Bus /
// dispatch.Dispatcher and the import edge stays one-way. CLAUDE.md §5.0.
package publishing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
)

// ErrNotAnInternalChannel is returned when a Worker tries to send to a
// Trigger that is not an internal channel. Inbound Triggers (GitHub,
// GitLab, Slack, email, webhook, cron) are read-only to Workers: acting
// on the provider is the Worker's job via its own credentials and the
// provider's native API. Adapters map it to 409 Conflict.
var ErrNotAnInternalChannel = errors.New("this trigger is inbound-only; a Worker cannot send to it — use the provider's own API with get_secret")

// Notifier wakes long-poll observers blocked on a stream. *wakebus.Bus
// satisfies it.
type Notifier interface {
	Notify(orgID string, streamID streaming.StreamID)
}

// Router fans a freshly-appended event out to the Workers attached to
// its source and the Processors reading it. dispatch.Dispatcher
// satisfies it.
type Router interface {
	Route(ctx context.Context, e eventsource.Event) error
}

// Publishing owns the publish use case.
type Publishing struct {
	triggers store.Triggers
	events   store.Events
	hub      Notifier
	router   Router
	now      func() time.Time
	newID    func() string
}

// Deps are the constructor-injected collaborators for New. Hub and
// Router are optional — leave them nil and the corresponding step is
// skipped (tests / runtimes without a hub or router).
type Deps struct {
	Triggers store.Triggers
	Events   store.Events
	Hub      Notifier
	Router   Router
	Now      func() time.Time
	NewID    func() string
}

// New constructs the Publishing service.
func New(deps Deps) *Publishing {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Publishing{
		triggers: deps.Triggers,
		events:   deps.Events,
		hub:      deps.Hub,
		router:   deps.Router,
		now:      now,
		newID:    deps.NewID,
	}
}

// Publish appends msg to the stream identified by streamID, attributed to
// `from`, and routes the resulting event as coming from src. msg.From is
// set to `from` so attribution stays consistent regardless of what the
// caller passed.
//
// streamID is passed separately from src because a Processor branch's
// stream is recorded on the branch, not derivable from its id — that is
// what preserves a converted branch's pre-cutover history.
func (p *Publishing) Publish(ctx context.Context, orgID string, src eventsource.SourceRef, streamID streaming.StreamID, from string, msg streaming.Message) (streaming.Event, error) {
	// Attribution is the caller's, not the message's: a Worker cannot
	// publish as somebody else by setting From itself.
	msg.From = from
	return p.append(ctx, orgID, src, streamID, streaming.EventID("e-"+p.newID()), from, msg)
}

// append is the shared write path. `source` is the event's originating
// Worker — empty for anything the org did not author (an inbound
// delivery, a cron tick, a Processor branch). msg is already final:
// callers that own attribution set From before calling.
func (p *Publishing) append(ctx context.Context, orgID string, src eventsource.SourceRef, streamID streaming.StreamID, eventID streaming.EventID, source string, msg streaming.Message) (streaming.Event, error) {
	if err := src.Validate(); err != nil {
		return streaming.Event{}, fmt.Errorf("publish: %w", err)
	}
	event, err := streaming.NewMessageEvent(eventID, streamID, source, msg, p.now(), orgID)
	if err != nil {
		return streaming.Event{}, err
	}
	if err := p.events.Append(ctx, event); err != nil {
		return streaming.Event{}, err
	}
	if p.hub != nil {
		p.hub.Notify(orgID, streamID)
	}
	if p.router == nil {
		return event, nil
	}
	routed, err := eventsource.NewEvent(event.ID, orgID, src, msg, source, event.CreatedAt)
	if err != nil {
		return event, fmt.Errorf("publish: build routed event: %w", err)
	}
	if err := p.router.Route(ctx, routed); err != nil {
		return event, fmt.Errorf("publish: route %s: %w", src.Key(), err)
	}
	return event, nil
}

// PublishToTrigger resolves the Trigger, then publishes onto the stream
// it owns (a Trigger's stream is its own id). Returns store.ErrNotFound
// when the Trigger is absent.
func (p *Publishing) PublishToTrigger(ctx context.Context, orgID, triggerID, from string, msg streaming.Message) (streaming.Event, error) {
	if _, err := p.trigger(ctx, orgID, triggerID); err != nil {
		return streaming.Event{}, err
	}
	return p.Publish(ctx, orgID, eventsource.Trigger(triggerID), triggerID, from, msg)
}

// PublishDelivery appends one inbound provider delivery under a
// caller-supplied deterministic event id, so a provider redelivery
// collides on the events table's primary key instead of being processed
// twice. Returns store.ErrConflict (wrapped) on a duplicate — ingress
// handlers turn that into a 204.
//
// Unlike Publish it does NOT rewrite msg.From: the sender of an inbound
// delivery is the external author the transport resolved (a GitHub
// login, an email address), and there is no Helix caller whose identity
// could be spoofed. The event's Source stays empty — nobody in the org
// authored it.
func (p *Publishing) PublishDelivery(ctx context.Context, orgID, triggerID string, eventID streaming.EventID, msg streaming.Message) (streaming.Event, error) {
	if _, err := p.trigger(ctx, orgID, triggerID); err != nil {
		return streaming.Event{}, err
	}
	return p.append(ctx, orgID, eventsource.Trigger(triggerID), triggerID, eventID, "", msg)
}

// SendToChannel is the Worker-authored path: it refuses anything that is
// not an internal channel before appending, so a Worker can send into a
// DM, a team chat or another local conversation but can never write back
// out through an inbound transport.
func (p *Publishing) SendToChannel(ctx context.Context, orgID, triggerID, from string, msg streaming.Message) (streaming.Event, error) {
	t, err := p.trigger(ctx, orgID, triggerID)
	if err != nil {
		return streaming.Event{}, err
	}
	if t.Kind != transport.KindLocal {
		return streaming.Event{}, fmt.Errorf("trigger %q (%s): %w", triggerID, t.Kind, ErrNotAnInternalChannel)
	}
	return p.Publish(ctx, orgID, eventsource.Trigger(triggerID), triggerID, from, msg)
}

func (p *Publishing) trigger(ctx context.Context, orgID, triggerID string) (trigger.Trigger, error) {
	rows, err := p.triggers.Find(ctx, store.WithOrg(orgID), store.WithID(triggerID), store.WithLimit(1))
	if err != nil {
		return trigger.Trigger{}, fmt.Errorf("trigger %q: %w", triggerID, err)
	}
	if len(rows) == 0 {
		return trigger.Trigger{}, fmt.Errorf("trigger %q: %w", triggerID, store.ErrNotFound)
	}
	return rows[0], nil
}
