package server

import (
	"context"
	"strings"
	"testing"
	"time"

	orgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/types"
)

// fakeTopics is a minimal orgstore.Topics for the publisher tests.
type fakeTopics struct {
	list    []streaming.Topic
	created []streaming.Topic
}

func (f *fakeTopics) Create(_ context.Context, s streaming.Topic) error {
	f.created = append(f.created, s)
	f.list = append(f.list, s)
	return nil
}
func (f *fakeTopics) Get(_ context.Context, _ string, _ streaming.TopicID) (streaming.Topic, error) {
	return streaming.Topic{}, orgstore.ErrNotFound
}
func (f *fakeTopics) List(_ context.Context, orgID string) ([]streaming.Topic, error) {
	var out []streaming.Topic
	for _, t := range f.list {
		if t.OrganizationID == orgID {
			out = append(out, t)
		}
	}
	return out, nil
}
func (f *fakeTopics) ListByTransportKind(_ context.Context, _ transport.Kind) ([]streaming.Topic, error) {
	return nil, nil
}
func (f *fakeTopics) Update(_ context.Context, _ streaming.Topic) error { return nil }
func (f *fakeTopics) Delete(_ context.Context, _ string, _ streaming.TopicID) error {
	return nil
}

// fakeEventPublisher records publishes.
type fakeEventPublisher struct {
	calls []struct {
		orgID   string
		topicID streaming.TopicID
		msg     streaming.Message
	}
}

func (f *fakeEventPublisher) Publish(_ context.Context, orgID string, topicID streaming.TopicID, _ string, msg streaming.Message) (streaming.Event, error) {
	f.calls = append(f.calls, struct {
		orgID   string
		topicID streaming.TopicID
		msg     streaming.Message
	}{orgID, topicID, msg})
	return streaming.Event{}, nil
}

func newTestPublisher(topics orgstore.Topics, pub orgEventPublisher) *attentionTopicPublisher {
	n := 0
	return &attentionTopicPublisher{
		topics:    topics,
		publisher: pub,
		newID:     func() string { n++; return "top_test" },
		now:       func() time.Time { return time.Unix(1700000000, 0).UTC() },
	}
}

// TestAttentionPublisher_CreatesTopicAndPublishes pins that, with no
// existing topic, the publisher creates a KindSpecTask topic for the
// project and publishes the event onto it.
func TestAttentionPublisher_CreatesTopicAndPublishes(t *testing.T) {
	t.Parallel()
	topics := &fakeTopics{}
	pub := &fakeEventPublisher{}
	p := newTestPublisher(topics, pub)

	ev := &types.AttentionEvent{
		ID: "ae_1", OrganizationID: "org-1", ProjectID: "prj_1", SpecTaskID: "task_1",
		EventType: types.AttentionEventPRReady, Title: "PR ready", Description: "review it",
	}
	if err := p.PublishAttentionEvent(context.Background(), ev); err != nil {
		t.Fatalf("PublishAttentionEvent: %v", err)
	}
	if len(topics.created) != 1 {
		t.Fatalf("created %d topics, want 1", len(topics.created))
	}
	if topics.created[0].Transport.Kind != transport.KindSpecTask {
		t.Errorf("topic kind = %q, want spectask", topics.created[0].Transport.Kind)
	}
	cfg, _ := topics.created[0].Transport.SpecTaskConfig()
	if cfg.ProjectID != "prj_1" {
		t.Errorf("topic project = %q, want prj_1", cfg.ProjectID)
	}
	if len(pub.calls) != 1 {
		t.Fatalf("published %d times, want 1", len(pub.calls))
	}
	if !strings.Contains(pub.calls[0].msg.Body, "review it") {
		t.Errorf("published body missing description: %q", pub.calls[0].msg.Body)
	}
}

// TestAttentionPublisher_ReusesExistingTopic pins that a second event for
// the same project reuses the existing topic instead of creating another.
func TestAttentionPublisher_ReusesExistingTopic(t *testing.T) {
	t.Parallel()
	topics := &fakeTopics{}
	pub := &fakeEventPublisher{}
	p := newTestPublisher(topics, pub)

	ev := &types.AttentionEvent{ID: "ae_1", OrganizationID: "org-1", ProjectID: "prj_1", EventType: types.AttentionEventPRReady, Title: "x"}
	if err := p.PublishAttentionEvent(context.Background(), ev); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := p.PublishAttentionEvent(context.Background(), ev); err != nil {
		t.Fatalf("second: %v", err)
	}
	if len(topics.created) != 1 {
		t.Errorf("created %d topics across two events, want 1", len(topics.created))
	}
	if len(pub.calls) != 2 {
		t.Errorf("published %d times, want 2", len(pub.calls))
	}
}

// TestAttentionPublisher_SkipsWithoutOrgScope pins that an event without
// an org (or project) is a no-op — nothing to route.
func TestAttentionPublisher_SkipsWithoutOrgScope(t *testing.T) {
	t.Parallel()
	topics := &fakeTopics{}
	pub := &fakeEventPublisher{}
	p := newTestPublisher(topics, pub)

	if err := p.PublishAttentionEvent(context.Background(), &types.AttentionEvent{ID: "ae_1", ProjectID: "prj_1"}); err != nil {
		t.Fatalf("PublishAttentionEvent: %v", err)
	}
	if len(pub.calls) != 0 || len(topics.created) != 0 {
		t.Errorf("expected no-op without org scope; created=%d published=%d", len(topics.created), len(pub.calls))
	}
}
