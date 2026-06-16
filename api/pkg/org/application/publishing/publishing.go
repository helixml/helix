// Package publishing is the application service that owns the publish
// use case — the append→notify→dispatch trio that must stay atomic and
// ordered. It was implemented twice (the MCP publish tool and the REST
// publishToStream handler), each doing the same three steps inline; the
// service is now the single home so the ordering and the github-inbound
// rejection cannot drift.
//
// Hub and Dispatcher are optional collaborators behind narrow
// interfaces (a Notifier and a Dispatcher) so the service does not
// depend on the concrete wakebus.Bus / tools.EventDispatcher and the
// import edge stays one-way. CLAUDE.md §5.0.
package publishing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// ErrPublishToGitHub is returned when a caller tries to publish to a
// github-transport Stream. GitHub streams are inbound-only — acting on
// the repo is the Worker's job via `gh`. Adapters map it: the MCP tool
// returns it verbatim, the REST handler maps it to 409 Conflict.
var ErrPublishToGitHub = errors.New("publish is not supported on github transport streams; use `gh` from your environment to act on the repo")

// Notifier wakes long-poll observers blocked on a stream. *wakebus.Bus
// satisfies it.
type Notifier interface {
	Notify(orgID string, streamID streaming.StreamID)
}

// Dispatcher fans a freshly-published Event out to subscribed AI
// Workers. tools.EventDispatcher / api.Dispatcher satisfy it.
type Dispatcher interface {
	Dispatch(ctx context.Context, event streaming.Event)
}

// Publishing owns the publish use case.
type Publishing struct {
	streams    store.Streams
	events     store.Events
	hub        Notifier
	dispatcher Dispatcher
	now        func() time.Time
	newID      func() string
}

// Deps are the constructor-injected collaborators for New. Hub and
// Dispatcher are optional — leave them nil and the corresponding step
// is skipped (tests / runtimes without a hub or dispatcher).
type Deps struct {
	Streams    store.Streams
	Events     store.Events
	Hub        Notifier
	Dispatcher Dispatcher
	Now        func() time.Time
	NewID      func() string
}

// New constructs the Publishing service.
func New(deps Deps) *Publishing {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Publishing{
		streams:    deps.Streams,
		events:     deps.Events,
		hub:        deps.Hub,
		dispatcher: deps.Dispatcher,
		now:        now,
		newID:      deps.NewID,
	}
}

// Publish appends a Message Event to the Stream attributed to `from`,
// then — in this order — notifies long-poll observers and dispatches to
// subscribed AI Workers. msg.From is set to `from` so attribution stays
// consistent regardless of what the caller passed. Returns the created
// Event. Rejects github-transport streams with ErrPublishToGitHub
// before any write, and store.ErrNotFound when the stream is absent.
func (p *Publishing) Publish(ctx context.Context, orgID string, streamID streaming.StreamID, from string, msg streaming.Message) (streaming.Event, error) {
	stream, err := p.streams.Get(ctx, orgID, streamID)
	if err != nil {
		return streaming.Event{}, fmt.Errorf("stream %q: %w", streamID, err)
	}
	if stream.Transport.Kind == transport.KindGitHub {
		return streaming.Event{}, fmt.Errorf("stream %q: %w", streamID, ErrPublishToGitHub)
	}
	msg.From = from
	event, err := streaming.NewMessageEvent(
		streaming.EventID("e-"+p.newID()),
		streamID,
		from,
		msg,
		p.now(),
		orgID,
	)
	if err != nil {
		return streaming.Event{}, err
	}
	if err := p.events.Append(ctx, event); err != nil {
		return streaming.Event{}, err
	}
	if p.hub != nil {
		p.hub.Notify(orgID, streamID)
	}
	if p.dispatcher != nil {
		p.dispatcher.Dispatch(ctx, event)
	}
	return event, nil
}
