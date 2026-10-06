package server

import (
	"context"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/helixevents"
	orgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
	orgmemory "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/helixml/helix/api/pkg/types"
)

// newAttentionEnv wires the publisher over a real in-memory org store, so
// the reconciler's get-or-create and the publish path are exercised the
// way production runs them.
func newAttentionEnv() (*attentionTopicPublisher, *fakeEventPublisher, orgstore.Triggers) {
	st := orgmemory.New()
	pub := &fakeEventPublisher{}
	return &attentionTopicPublisher{
		reconciler: helixevents.New(helixevents.Deps{Triggers: st.Triggers}),
		publisher:  pub,
	}, pub, st.Triggers
}

// fakeEventPublisher records publishes.
type fakeEventPublisher struct {
	calls []struct {
		orgID     string
		triggerID string
		msg       streaming.Message
	}
}

func (f *fakeEventPublisher) PublishToTrigger(_ context.Context, orgID, triggerID, _ string, msg streaming.Message) (streaming.Event, error) {
	f.calls = append(f.calls, struct {
		orgID     string
		triggerID string
		msg       streaming.Message
	}{orgID, triggerID, msg})
	return streaming.Event{}, nil
}

// helixEventTriggers returns the org's Triggers, so the tests can assert
// exactly one events Trigger exists.
func helixEventTriggers(t *testing.T, repo orgstore.Triggers, orgID string) []trigger.Trigger {
	t.Helper()
	rows, err := repo.Find(context.Background(), orgstore.WithOrg(orgID))
	if err != nil {
		t.Fatalf("Triggers.Find: %v", err)
	}
	return rows
}

// TestAttentionPublisher_CreatesTriggerAndPublishes pins that, with no
// existing Trigger, the publisher ensures the single org-wide Helix
// events Trigger and publishes the event onto it with the generic
// envelope.
func TestAttentionPublisher_CreatesTriggerAndPublishes(t *testing.T) {
	t.Parallel()
	p, pub, repo := newAttentionEnv()

	ev := &types.AttentionEvent{
		ID: "ae_1", OrganizationID: "org-1", ProjectID: "prj_1", SpecTaskID: "task_1",
		EventType: types.AttentionEventPRReady, Title: "PR ready", Description: "review it",
	}
	if err := p.PublishAttentionEvent(context.Background(), ev); err != nil {
		t.Fatalf("PublishAttentionEvent: %v", err)
	}
	created := helixEventTriggers(t, repo, "org-1")
	if len(created) != 1 {
		t.Fatalf("created %d triggers, want 1", len(created))
	}
	if created[0].Kind != transport.KindHelixEvents {
		t.Errorf("trigger kind = %q, want helix_events", created[0].Kind)
	}
	if created[0].ID != helixevents.TriggerID {
		t.Errorf("trigger id = %q, want %q", created[0].ID, helixevents.TriggerID)
	}
	if len(pub.calls) != 1 {
		t.Fatalf("published %d times, want 1", len(pub.calls))
	}
	if pub.calls[0].triggerID != helixevents.TriggerID {
		t.Errorf("published to %q, want the Helix events trigger %q", pub.calls[0].triggerID, helixevents.TriggerID)
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

// TestAttentionPublisher_ReusesSingleTrigger pins that a second event
// reuses the single org-wide Trigger instead of creating another.
func TestAttentionPublisher_ReusesSingleTrigger(t *testing.T) {
	t.Parallel()
	p, pub, repo := newAttentionEnv()

	ev := &types.AttentionEvent{ID: "ae_1", OrganizationID: "org-1", ProjectID: "prj_1", EventType: types.AttentionEventPRReady, Title: "x"}
	if err := p.PublishAttentionEvent(context.Background(), ev); err != nil {
		t.Fatalf("first: %v", err)
	}
	// A different project on the same org must NOT create a second Trigger.
	ev2 := &types.AttentionEvent{ID: "ae_2", OrganizationID: "org-1", ProjectID: "prj_2", EventType: types.AttentionEventPRReady, Title: "y"}
	if err := p.PublishAttentionEvent(context.Background(), ev2); err != nil {
		t.Fatalf("second: %v", err)
	}
	if got := helixEventTriggers(t, repo, "org-1"); len(got) != 1 {
		t.Errorf("created %d triggers across two projects, want 1", len(got))
	}
	if len(pub.calls) != 2 {
		t.Errorf("published %d times, want 2", len(pub.calls))
	}
}

// TestAttentionPublisher_SkipsWithoutOrgScope pins that an event without
// an org is a no-op — nothing to route.
func TestAttentionPublisher_SkipsWithoutOrgScope(t *testing.T) {
	t.Parallel()
	p, pub, repo := newAttentionEnv()

	if err := p.PublishAttentionEvent(context.Background(), &types.AttentionEvent{ID: "ae_1", ProjectID: "prj_1"}); err != nil {
		t.Fatalf("PublishAttentionEvent: %v", err)
	}
	if got := helixEventTriggers(t, repo, ""); len(pub.calls) != 0 || len(got) != 0 {
		t.Errorf("expected no-op without org scope; created=%d published=%d", len(got), len(pub.calls))
	}
}
