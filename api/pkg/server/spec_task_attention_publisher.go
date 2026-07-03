package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/helixml/helix/api/pkg/org/application/helixevents"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/types"
)

// attentionTopicPublisher implements services.AttentionEventSink: it
// forwards each spec-task attention event onto the org's single,
// generic "Helix events" topic (transport.KindHelixEvents) so
// subscribed Workers are triggered via the normal dispatch path. This
// is the helix↔org bridge — it lives in the server package (not org
// infra) because it consumes helix's *types.AttentionEvent; org
// transports import only the org domain.
//
// Spec-task events are the first `domain` on the bus; the topic is
// designed to carry every future Helix event kind, distinguished by the
// `domain` / `event_type` keys in the published Message's Extra. Routing
// to individual bots is done by filter processors over this one topic
// (keyed on domain / event_type / project_id) — there are no per-project
// topics.
type attentionTopicPublisher struct {
	reconciler *helixevents.Reconciler
	publisher  orgEventPublisher
}

// orgEventPublisher is the narrow publish surface — satisfied by the org
// application Publishing service.
type orgEventPublisher interface {
	Publish(ctx context.Context, orgID string, topicID streaming.TopicID, from string, msg streaming.Message) (streaming.Event, error)
}

// helixEventExtra is the structured payload carried on the topic event
// so an activated Worker (or a filter processor's predicate over
// .Message.extra) can route/react without an extra lookup. Domain +
// EventType identify the event family and type; the remaining keys are
// the spec-task payload (denormalized display fields avoid a lookup).
// The human-facing text and threading are coerced onto first-class
// Message fields (Subject/Body/ThreadID/MessageID) in
// PublishAttentionEvent.
type helixEventExtra struct {
	Domain       string `json:"domain"`     // event family, e.g. "spectask"
	EventType    string `json:"event_type"` // type within the family
	ProjectID    string `json:"project_id,omitempty"`
	SpecTaskID   string `json:"spec_task_id,omitempty"`
	ProjectName  string `json:"project_name,omitempty"`
	SpecTaskName string `json:"spec_task_name,omitempty"`
}

// domainSpecTask is the event family for spec-task attention events —
// the first (and today only) domain on the Helix events bus.
const domainSpecTask = "spectask"

// PublishAttentionEvent ensures the org's single Helix events topic
// exists, then publishes the event onto it. A missing org scope is a
// no-op — there's nothing to route to.
func (p *attentionTopicPublisher) PublishAttentionEvent(ctx context.Context, ev *types.AttentionEvent) error {
	if ev == nil || ev.OrganizationID == "" {
		return nil
	}
	// Defensive ensure: the helixevents reconciler creates this topic on
	// org bootstrap, but a brand-new org whose bootstrap hasn't run yet
	// still needs somewhere to publish. Idempotent.
	if err := p.reconciler.Reconcile(ctx, ev.OrganizationID); err != nil {
		return fmt.Errorf("ensure helix events topic: %w", err)
	}
	extra, _ := json.Marshal(helixEventExtra{
		Domain:       domainSpecTask,
		EventType:    string(ev.EventType),
		ProjectID:    ev.ProjectID,
		SpecTaskID:   ev.SpecTaskID,
		ProjectName:  ev.ProjectName,
		SpecTaskName: ev.SpecTaskName,
	})
	// Coerce the notification's fields onto first-class Message fields so
	// predicates and consumers use natural fields, not just Extra:
	//   Title       → Subject
	//   Description → Body
	//   SpecTaskID  → ThreadID   (all events for one task thread together)
	//   ID          → MessageID  (stable, unique id)
	// domain / event_type / project_id stay in Extra (no natural field).
	msg := streaming.Message{
		Subject:         ev.Title,
		Body:            ev.Description,
		BodyContentType: "text/plain",
		ThreadID:        ev.SpecTaskID,
		MessageID:       ev.ID,
		Extra:           extra,
	}
	if _, err := p.publisher.Publish(ctx, ev.OrganizationID, helixevents.TopicID, "", msg); err != nil {
		return fmt.Errorf("publish helix event: %w", err)
	}
	return nil
}
