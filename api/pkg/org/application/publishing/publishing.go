// Package publishing is the application service that owns the publish
// use case — the append→notify→dispatch trio that must stay atomic and
// ordered. It was implemented twice (the MCP publish tool and the REST
// publishToTopic handler), each doing the same three steps inline; the
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
// github-transport Topic. GitHub topics are inbound-only — acting on
// the repo is the Worker's job via `gh`. Adapters map it: the MCP tool
// returns it verbatim, the REST handler maps it to 409 Conflict.
var ErrPublishToGitHub = errors.New("publish is not supported on github transport topics; use `gh` from your environment to act on the repo")
var ErrPublishToGitLab = errors.New("publish is not supported on gitlab transport topics; use the GitLab API from your environment to act on the repo")
var ErrSlackChannelNotConfigured = errors.New("publish is not supported on slack transport topics without channel_id; configure channel_id for outbound delivery")

// Notifier wakes long-poll observers blocked on a topic. *wakebus.Bus
// satisfies it.
type Notifier interface {
	Notify(orgID string, topicID streaming.TopicID)
}

// Dispatcher fans a freshly-published Event out to subscribed AI
// Workers. tools.EventDispatcher / api.Dispatcher satisfy it.
type Dispatcher interface {
	Dispatch(ctx context.Context, event streaming.Event)
}

type DeliveryReceipt struct {
	Status      string `json:"status"`
	Provider    string `json:"provider"`
	Destination string `json:"destination"`
	MessageID   string `json:"messageId"`
	Error       string `json:"error,omitempty"`
}

type Deliverer interface {
	Deliver(ctx context.Context, topic streaming.Topic, msg streaming.Message) (DeliveryReceipt, error)
}

type Result struct {
	Event    streaming.Event
	Delivery *DeliveryReceipt
}

// Publishing owns the publish use case.
type Publishing struct {
	topics     store.Topics
	events     store.Events
	hub        Notifier
	dispatcher Dispatcher
	deliverers map[transport.Kind]Deliverer
	now        func() time.Time
	newID      func() string
}

// Deps are the constructor-injected collaborators for New. Hub and
// Dispatcher are optional — leave them nil and the corresponding step
// is skipped (tests / runtimes without a hub or dispatcher).
type Deps struct {
	Topics     store.Topics
	Events     store.Events
	Hub        Notifier
	Dispatcher Dispatcher
	Deliverers map[transport.Kind]Deliverer
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
		topics:     deps.Topics,
		events:     deps.Events,
		hub:        deps.Hub,
		dispatcher: deps.Dispatcher,
		deliverers: deps.Deliverers,
		now:        now,
		newID:      deps.NewID,
	}
}

func (p *Publishing) RegisterDeliverer(kind transport.Kind, deliverer Deliverer) {
	if p.deliverers == nil {
		p.deliverers = map[transport.Kind]Deliverer{}
	}
	p.deliverers[kind] = deliverer
}

// Publish appends a Message Event to the Topic attributed to `from`,
// then — in this order — notifies long-poll observers and dispatches to
// subscribed AI Workers. msg.From is set to `from` so attribution stays
// consistent regardless of what the caller passed. Returns the created
// Event. Outbound delivery runs after append, notify, and dispatch, so a
// delivery error can be returned after the internal audit event and subscriber
// activations already exist. Rejects inbound-only repository Topics and Slack
// Topics without channel_id before any write, and returns store.ErrNotFound
// when the Topic is absent.
func (p *Publishing) Publish(ctx context.Context, orgID string, topicID streaming.TopicID, from string, msg streaming.Message) (streaming.Event, error) {
	result, err := p.publish(ctx, orgID, topicID, from, msg)
	return result.Event, err
}

func (p *Publishing) PublishWithReceipt(ctx context.Context, orgID string, topicID streaming.TopicID, from string, msg streaming.Message) (Result, error) {
	return p.publish(ctx, orgID, topicID, from, msg)
}

func (p *Publishing) PublishInbound(ctx context.Context, orgID string, topicID streaming.TopicID, from string, msg streaming.Message) (streaming.Event, error) {
	result, err := p.publish(context.WithValue(ctx, inboundContextKey{}, true), orgID, topicID, from, msg)
	return result.Event, err
}

type inboundContextKey struct{}

func isInbound(ctx context.Context) bool {
	inbound, _ := ctx.Value(inboundContextKey{}).(bool)
	return inbound
}

func (p *Publishing) publish(ctx context.Context, orgID string, topicID streaming.TopicID, from string, msg streaming.Message) (Result, error) {
	topic, err := p.topics.Get(ctx, orgID, topicID)
	if err != nil {
		return Result{}, fmt.Errorf("topic %q: %w", topicID, err)
	}
	if topic.Transport.Kind == transport.KindGitHub {
		return Result{}, fmt.Errorf("topic %q: %w", topicID, ErrPublishToGitHub)
	}
	if topic.Transport.Kind == transport.KindGitLab {
		return Result{}, fmt.Errorf("topic %q: %w", topicID, ErrPublishToGitLab)
	}
	if topic.Transport.Kind == transport.KindSlack && !isInbound(ctx) {
		cfg, err := topic.Transport.SlackConfig()
		if err != nil {
			return Result{}, fmt.Errorf("topic %q: %w", topicID, err)
		}
		if cfg.ChannelID == "" {
			return Result{}, fmt.Errorf("topic %q: %w", topicID, ErrSlackChannelNotConfigured)
		}
	}
	msg.From = from
	event, err := streaming.NewMessageEvent(
		streaming.EventID("e-"+p.newID()),
		topicID,
		from,
		msg,
		p.now(),
		orgID,
	)
	if err != nil {
		return Result{}, err
	}
	if err := p.events.Append(ctx, event); err != nil {
		return Result{}, err
	}
	if p.hub != nil {
		p.hub.Notify(orgID, topicID)
	}
	if p.dispatcher != nil {
		p.dispatcher.Dispatch(ctx, event)
	}
	result := Result{Event: event}
	if !isInbound(ctx) {
		if deliverer := p.deliverers[topic.Transport.Kind]; deliverer != nil {
			receipt, err := deliverer.Deliver(ctx, topic, msg)
			if err != nil {
				receipt.Status = "failed"
				if receipt.Provider == "" {
					receipt.Provider = string(topic.Transport.Kind)
				}
				receipt.Error = "do not retry publish: " + err.Error()
				result.Delivery = &receipt
				return result, fmt.Errorf("deliver topic %q: %w", topicID, err)
			}
			result.Delivery = &receipt
		}
	}
	return result, nil
}
