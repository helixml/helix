package helix

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
)

// newTestMirror builds a Mirror over the shared fakes, with a fast poll
// interval. The returned fakeHelixClient drives session resolution:
// set fc.setExploratory(id) to point the mirror at a session, and the
// poller will (re-)subscribe to it within a couple of poll intervals.
// The worker must have a persisted project (SaveProject) so the mirror
// resolves via ExploratorySession.
func newTestMirror(t *testing.T, s *store.Store, ps *fakePubSub, owner string) (*Mirror, *fakeHelixClient) {
	t.Helper()
	fc := &fakeHelixClient{sessionOwner: owner}
	var idCounter int32
	m := NewMirror(context.Background(), MirrorConfig{
		PubSub:             ps,
		Snapshotter:        NoopSessionPreamble{},
		Client:             fc,
		ExploratorySession: fc.ExploratorySession,
		Store:              s,
		Logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		NewID:              func() string { return fmt.Sprintf("e-%d", atomic.AddInt32(&idCounter, 1)) },
		Now:                func() time.Time { return time.Now().UTC() },
		PollInterval:       15 * time.Millisecond,
	})
	return m, fc
}

func waitForSegment(t *testing.T, s *store.Store, wid orgchart.WorkerID, want string) bool {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		events, err := s.Events.ListForStream(context.Background(), "org-test", activation.StreamID(wid), 200)
		if err != nil {
			t.Fatalf("list events: %v", err)
		}
		for _, e := range events {
			if msg, err := e.Message(); err == nil && strings.Contains(msg.Body, want) {
				return true
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func waitForHandlers(ps *fakePubSub, topic string, want int) bool {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if ps.handlerCount(topic) == want {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return ps.handlerCount(topic) == want
}

// TestMirrorCapturesTurnWithoutSpawner: the inline-chat regression — a
// frame on the worker's session topic, with NO spawner/activation, is
// mirrored onto s-activations-<worker>. The mirror tracks the worker
// and resolves its session via ExploratorySession.
func TestMirrorCapturesTurnWithoutSpawner(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	if err := SaveProject(context.Background(), s, "org-test", wid, "prj_x", "app_x", "repo_x"); err != nil {
		t.Fatalf("save project: %v", err)
	}
	ps := newFakePubSub()
	m, fc := newTestMirror(t, s, ps, "u-owner")
	fc.setExploratory("ses_inline")
	m.Ensure("org-test", wid)

	topic := pubsub.GetSessionQueue("u-owner", "ses_inline")
	if !waitForHandlers(ps, topic, 1) {
		t.Fatal("mirror never subscribed to the resolved session")
	}
	patch, _ := json.Marshal(types.WebsocketEvent{EntryPatches: []types.EntryPatch{
		{Index: 0, MessageID: "m1", Type: "text", Patch: "hello from inline chat"},
	}})
	ps.publish(t, topic, patch)
	complete, _ := json.Marshal(types.WebsocketEvent{Interaction: &types.Interaction{State: "complete"}})
	ps.publish(t, topic, complete)

	if !waitForSegment(t, s, wid, "assistant: hello from inline chat") {
		t.Fatal("inline-chat turn never reached the activation stream")
	}
}

// TestMirrorRepointsOnSessionChurn is the core fix: when the worker's
// session changes (a stale resume opens a fresh one), the mirror drops
// the old subscription and follows the new session, so the stream keeps
// flowing instead of going silent on the orphaned old session.
func TestMirrorRepointsOnSessionChurn(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	if err := SaveProject(context.Background(), s, "org-test", wid, "prj_x", "app_x", "repo_x"); err != nil {
		t.Fatalf("save project: %v", err)
	}
	ps := newFakePubSub()
	m, fc := newTestMirror(t, s, ps, "u-owner")

	fc.setExploratory("ses_old")
	m.Ensure("org-test", wid)
	oldTopic := pubsub.GetSessionQueue("u-owner", "ses_old")
	if !waitForHandlers(ps, oldTopic, 1) {
		t.Fatal("mirror never subscribed to ses_old")
	}

	// The worker's session churns to ses_new.
	fc.setExploratory("ses_new")
	newTopic := pubsub.GetSessionQueue("u-owner", "ses_new")
	if !waitForHandlers(ps, newTopic, 1) {
		t.Fatal("mirror did not re-point to ses_new after churn")
	}
	if !waitForHandlers(ps, oldTopic, 0) {
		t.Fatal("mirror did not drop the old (ses_old) subscription on re-point")
	}

	// A turn on the NEW session is captured.
	patch, _ := json.Marshal(types.WebsocketEvent{EntryPatches: []types.EntryPatch{
		{Index: 0, MessageID: "m1", Type: "text", Patch: "on the new session"},
	}})
	ps.publish(t, newTopic, patch)
	complete, _ := json.Marshal(types.WebsocketEvent{Interaction: &types.Interaction{State: "complete"}})
	ps.publish(t, newTopic, complete)
	if !waitForSegment(t, s, wid, "assistant: on the new session") {
		t.Fatal("turn on the churned-to session not captured")
	}
}

// TestMirrorCapturesUserPrompt: the mirror records the human/activation
// prompt as a `user:` segment, once per interaction, alongside the
// agent's reply.
func TestMirrorCapturesUserPrompt(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	if err := SaveProject(context.Background(), s, "org-test", wid, "prj_x", "app_x", "repo_x"); err != nil {
		t.Fatalf("save project: %v", err)
	}
	ps := newFakePubSub()
	m, fc := newTestMirror(t, s, ps, "u-owner")
	fc.setExploratory("ses_p")
	m.Ensure("org-test", wid)
	topic := pubsub.GetSessionQueue("u-owner", "ses_p")
	if !waitForHandlers(ps, topic, 1) {
		t.Fatal("mirror never subscribed")
	}

	iu, _ := json.Marshal(types.WebsocketEvent{
		Type:        types.WebsocketEventInteractionUpdate,
		Interaction: &types.Interaction{ID: "int_1", PromptMessage: "what is 2+2?"},
	})
	ps.publish(t, topic, iu)
	ps.publish(t, topic, iu) // duplicate — must not double-emit
	patch, _ := json.Marshal(types.WebsocketEvent{EntryPatches: []types.EntryPatch{
		{Index: 0, MessageID: "m1", Type: "text", Patch: "4"},
	}})
	ps.publish(t, topic, patch)
	complete, _ := json.Marshal(types.WebsocketEvent{
		Interaction: &types.Interaction{ID: "int_1", PromptMessage: "what is 2+2?", State: "complete"},
	})
	ps.publish(t, topic, complete)

	if !waitForSegment(t, s, wid, "user: what is 2+2?") {
		t.Fatal("user prompt not captured")
	}
	if !waitForSegment(t, s, wid, "assistant: 4") {
		t.Fatal("assistant reply not captured")
	}
	events, _ := s.Events.ListForStream(context.Background(), "org-test", activation.StreamID(wid), 200)
	userLines := 0
	for _, e := range events {
		if msg, err := e.Message(); err == nil && strings.HasPrefix(msg.Body, "user: ") {
			userLines++
		}
	}
	if userLines != 1 {
		t.Fatalf("user lines = %d, want exactly 1 (dedup per interaction)", userLines)
	}
}

// TestMirrorEnsureIsIdempotent: Ensure twice for the same worker must
// not stack duplicate trackers/subscriptions.
func TestMirrorEnsureIsIdempotent(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	if err := SaveProject(context.Background(), s, "org-test", wid, "prj_x", "app_x", "repo_x"); err != nil {
		t.Fatalf("save project: %v", err)
	}
	ps := newFakePubSub()
	m, fc := newTestMirror(t, s, ps, "u-owner")
	fc.setExploratory("ses_x")
	m.Ensure("org-test", wid)
	m.Ensure("org-test", wid)
	m.Ensure("org-test", wid)

	topic := pubsub.GetSessionQueue("u-owner", "ses_x")
	if !waitForHandlers(ps, topic, 1) {
		t.Fatalf("handlerCount = %d, want 1 (Ensure must not stack subscriptions)", ps.handlerCount(topic))
	}
	// Give any erroneously-spawned extra trackers a chance to subscribe.
	time.Sleep(60 * time.Millisecond)
	if got := ps.handlerCount(topic); got != 1 {
		t.Fatalf("handlerCount = %d, want 1", got)
	}
}

// TestMirrorStop tears down the tracker + subscription.
func TestMirrorStop(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	if err := SaveProject(context.Background(), s, "org-test", wid, "prj_x", "app_x", "repo_x"); err != nil {
		t.Fatalf("save project: %v", err)
	}
	ps := newFakePubSub()
	m, fc := newTestMirror(t, s, ps, "u-owner")
	fc.setExploratory("ses_x")
	m.Ensure("org-test", wid)
	topic := pubsub.GetSessionQueue("u-owner", "ses_x")
	if !waitForHandlers(ps, topic, 1) {
		t.Fatal("mirror never subscribed")
	}
	m.Stop(wid)
	if !waitForHandlers(ps, topic, 0) {
		t.Fatal("Stop did not drop the subscription")
	}
}
