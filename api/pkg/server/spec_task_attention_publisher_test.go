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
	// Notification fields coerced onto first-class Message fields.
	got := pub.calls[0].msg
	if got.Subject != "PR ready" {
		t.Errorf("Subject = %q, want %q", got.Subject, "PR ready")
	}
	if got.ThreadID != "task_1" {
		t.Errorf("ThreadID = %q, want the spec task id task_1", got.ThreadID)
	}
	if got.MessageID != "ae_1" {
		t.Errorf("MessageID = %q, want the attention event id ae_1", got.MessageID)
	}
	// Routing keys with no natural Message field stay in Extra.
	if !strings.Contains(string(got.Extra), "pr_ready") || !strings.Contains(string(got.Extra), "prj_1") {
		t.Errorf("Extra missing event_type/project_id: %s", got.Extra)
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

// TestEnsureSpecTaskTopic_Idempotent pins that the shared helper creates
// the topic once and returns the same id on a second call — so a wiring
// path can pre-create a project's input topic without racing the publisher.
func TestEnsureSpecTaskTopic_Idempotent(t *testing.T) {
	t.Parallel()
	topics := &fakeTopics{}
	n := 0
	newID := func() string { n++; return "top_ensure" }
	now := func() time.Time { return time.Unix(1700000000, 0).UTC() }

	first, err := EnsureSpecTaskTopic(context.Background(), topics, newID, now, "org-1", "prj_1")
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	second, err := EnsureSpecTaskTopic(context.Background(), topics, newID, now, "org-1", "prj_1")
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if first != second {
		t.Errorf("ids differ: %q vs %q", first, second)
	}
	if len(topics.created) != 1 {
		t.Errorf("created %d topics, want 1 (idempotent)", len(topics.created))
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
