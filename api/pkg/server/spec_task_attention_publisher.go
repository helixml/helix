package server

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	orgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/types"
)

// attentionTopicPublisher implements services.AttentionEventSink: it
// forwards each spec-task attention event onto a per-project org topic
// (transport.KindSpecTask) so subscribed Workers are triggered via the
// normal dispatch path. This is the helix↔org bridge — it lives in the
// server package (not org infra) because it consumes helix's
// *types.AttentionEvent; org transports import only the org domain.
type attentionTopicPublisher struct {
	topics    orgstore.Topics
	publisher orgEventPublisher
	newID     func() string
	now       func() time.Time
}

// orgEventPublisher is the narrow publish surface — satisfied by the org
// application Publishing service.
type orgEventPublisher interface {
	Publish(ctx context.Context, orgID string, topicID streaming.TopicID, from string, msg streaming.Message) (streaming.Event, error)
}

// specTaskEventExtra is the structured payload carried on the topic event
// so an activated Worker (or a filter processor's predicate over
// .Message.extra) can route/react without an extra lookup. It holds the
// keys that have no natural streaming.Message field (event_type, project_id)
// plus denormalized display fields; the human-facing text and threading are
// coerced onto first-class Message fields (Subject/Body/ThreadID/MessageID)
// in PublishAttentionEvent.
type specTaskEventExtra struct {
	SpecTaskID   string `json:"spec_task_id"`
	EventType    string `json:"event_type"`
	ProjectID    string `json:"project_id"`
	ProjectName  string `json:"project_name,omitempty"`
	SpecTaskName string `json:"spec_task_name,omitempty"`
}

// PublishAttentionEvent resolves (or creates) the project's spec-task
// topic and publishes the event. A missing org/project scope is a no-op
// — there's nothing to route to.
func (p *attentionTopicPublisher) PublishAttentionEvent(ctx context.Context, ev *types.AttentionEvent) error {
	if ev == nil || ev.OrganizationID == "" || ev.ProjectID == "" {
		return nil
	}
	topicID, err := p.ensureTopic(ctx, ev.OrganizationID, ev.ProjectID)
	if err != nil {
		return fmt.Errorf("ensure spectask topic: %w", err)
	}
	extra, _ := json.Marshal(specTaskEventExtra{
		SpecTaskID:   ev.SpecTaskID,
		EventType:    string(ev.EventType),
		ProjectID:    ev.ProjectID,
		ProjectName:  ev.ProjectName,
		SpecTaskName: ev.SpecTaskName,
	})
	// Coerce the notification's fields onto first-class Message fields so
	// predicates and consumers use natural fields, not just Extra:
	//   Title       → Subject
	//   Description → Body
	//   SpecTaskID  → ThreadID   (all events for one task thread together)
	//   ID          → MessageID  (stable, unique id)
	// event_type / project_id stay in Extra (no natural Message field).
	msg := streaming.Message{
		Subject:         ev.Title,
		Body:            ev.Description,
		BodyContentType: "text/plain",
		ThreadID:        ev.SpecTaskID,
		MessageID:       ev.ID,
		Extra:           extra,
	}
	if _, err := p.publisher.Publish(ctx, ev.OrganizationID, topicID, "", msg); err != nil {
		return fmt.Errorf("publish spectask event: %w", err)
	}
	return nil
}

// ensureTopic returns the project's KindSpecTask topic, creating it on
// first use. Thin wrapper over the shared EnsureSpecTaskTopic so the
// publisher and any wiring path (e.g. pre-creating the input topic before
// a bot is subscribed to it) agree on the topic's identity and config.
func (p *attentionTopicPublisher) ensureTopic(ctx context.Context, orgID, projectID string) (streaming.TopicID, error) {
	return EnsureSpecTaskTopic(ctx, p.topics, p.newID, p.now, orgID, projectID)
}

// EnsureSpecTaskTopic find-or-creates the KindSpecTask topic for a project
// (mirrors how Slack auto-creates its workspace topic). It is the single
// source of truth for a project's spec-task topic identity + config, shared
// by the attention publisher (which creates it lazily on the first event)
// and any wiring path that needs the topic to exist deterministically
// before an event has fired — e.g. subscribing an org-wide PM Bot to a
// quiet project at creation time. Idempotent: a second call for the same
// (org, project) returns the existing topic.
func EnsureSpecTaskTopic(ctx context.Context, topics orgstore.Topics, newID func() string, now func() time.Time, orgID, projectID string) (streaming.TopicID, error) {
	existing, err := topics.List(ctx, orgID)
	if err != nil {
		return "", fmt.Errorf("list topics: %w", err)
	}
	for _, t := range existing {
		if t.Transport.Kind != transport.KindSpecTask {
			continue
		}
		cfg, cfgErr := t.Transport.SpecTaskConfig()
		if cfgErr == nil && cfg.ProjectID == projectID {
			return t.ID, nil
		}
	}
	cfgJSON, err := json.Marshal(transport.SpecTaskConfig{ProjectID: projectID})
	if err != nil {
		return "", fmt.Errorf("marshal topic config: %w", err)
	}
	id := streaming.TopicID(newID())
	topic, err := streaming.NewTopic(
		id,
		"Spec tasks: "+projectID,
		"Spec-task state changes for project "+projectID,
		"", // createdBy is optional (no worker context for automation)
		now(),
		transport.Transport{Kind: transport.KindSpecTask, Config: cfgJSON},
		orgID,
	)
	if err != nil {
		return "", fmt.Errorf("build topic: %w", err)
	}
	if err := topics.Create(ctx, topic); err != nil {
		return "", fmt.Errorf("create topic: %w", err)
	}
	return id, nil
}
