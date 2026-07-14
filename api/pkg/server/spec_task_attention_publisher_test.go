package server

import (
	"context"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/helixevents"
	orgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/types"
)

// fakeTopics is a minimal, stateful orgstore.Topics for the publisher
// tests. Get resolves against created rows so the reconciler's
// get-or-create is exercised realistically.
type fakeTopics struct {
	list    []streaming.Topic
	created []streaming.Topic
}

func (f *fakeTopics) Create(_ context.Context, s streaming.Topic) error {
	f.created = append(f.created, s)
	f.list = append(f.list, s)
	return nil
}
func (f *fakeTopics) Get(_ context.Context, orgID string, id streaming.TopicID) (streaming.Topic, error) {
	for _, t := range f.list {
		if t.OrganizationID == orgID && t.ID == id {
			return t, nil
		}
	}
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
	return &attentionTopicPublisher{
		reconciler: helixevents.New(helixevents.Deps{Topics: topics}),
		publisher:  pub,
	}
}

// TestAttentionPublisher_CreatesTopicAndPublishes pins that, with no
// existing topic, the publisher ensures the single org-wide Helix events
// topic and publishes the event onto it with the generic envelope.
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
	if topics.created[0].Transport.Kind != transport.KindHelixEvents {
		t.Errorf("topic kind = %q, want helix_events", topics.created[0].Transport.Kind)
	}
	if topics.created[0].ID != helixevents.TopicID {
		t.Errorf("topic id = %q, want %q", topics.created[0].ID, helixevents.TopicID)
	}
	if len(pub.calls) != 1 {
		t.Fatalf("published %d times, want 1", len(pub.calls))
	}
	if pub.calls[0].topicID != helixevents.TopicID {
		t.Errorf("published to %q, want the Helix events topic %q", pub.calls[0].topicID, helixevents.TopicID)
	}
	// Notification fields coerced onto first-class Message fields.
	got := pub.calls[0].msg
	if got.Subject != "PR ready" {
		t.Errorf("Subject = %q, want %q", got.Subject, "PR ready")
	}
	if !strings.Contains(got.Body, "review it") {
		t.Errorf("published body missing description: %q", got.Body)
	}
	if got.ThreadID != "task_1" {
		t.Errorf("ThreadID = %q, want the spec task id task_1", got.ThreadID)
	}
	if got.MessageID != "ae_1" {
		t.Errorf("MessageID = %q, want the attention event id ae_1", got.MessageID)
	}
	// The generic envelope: domain + event_type + project_id in Extra.
	extra := string(got.Extra)
	for _, want := range []string{`"domain":"spectask"`, "pr_ready", "prj_1", "task_1"} {
		if !strings.Contains(extra, want) {
			t.Errorf("Extra missing %q: %s", want, extra)
		}
	}
}

// TestAttentionPublisher_ReusesSingleTopic pins that a second event
// reuses the single org-wide topic instead of creating another.
func TestAttentionPublisher_ReusesSingleTopic(t *testing.T) {
	t.Parallel()
	topics := &fakeTopics{}
	pub := &fakeEventPublisher{}
	p := newTestPublisher(topics, pub)

	ev := &types.AttentionEvent{ID: "ae_1", OrganizationID: "org-1", ProjectID: "prj_1", EventType: types.AttentionEventPRReady, Title: "x"}
	if err := p.PublishAttentionEvent(context.Background(), ev); err != nil {
		t.Fatalf("first: %v", err)
	}
	// A different project on the same org must NOT create a second topic.
	ev2 := &types.AttentionEvent{ID: "ae_2", OrganizationID: "org-1", ProjectID: "prj_2", EventType: types.AttentionEventPRReady, Title: "y"}
	if err := p.PublishAttentionEvent(context.Background(), ev2); err != nil {
		t.Fatalf("second: %v", err)
	}
	if len(topics.created) != 1 {
		t.Errorf("created %d topics across two projects, want 1", len(topics.created))
	}
	if len(pub.calls) != 2 {
		t.Errorf("published %d times, want 2", len(pub.calls))
	}
}

// TestAttentionPublisher_SkipsWithoutOrgScope pins that an event without
// an org is a no-op — nothing to route.
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
