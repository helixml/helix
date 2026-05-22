package helix

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/pubsub"
)

// fakePubSub is a minimal in-process pubsub for SubscribeSessionUpdates
// tests. Only the methods SubscribeSessionUpdates uses are implemented;
// the rest return zero values and would panic if exercised.
type fakePubSub struct {
	pubsub.PubSub
	handlers map[string][]func(payload []byte) error
}

func newFakePubSub() *fakePubSub {
	return &fakePubSub{handlers: map[string][]func(payload []byte) error{}}
}

type fakeSubscription struct {
	ps    *fakePubSub
	topic string
}

func (s *fakeSubscription) Unsubscribe() error {
	delete(s.ps.handlers, s.topic)
	return nil
}

func (f *fakePubSub) Subscribe(_ context.Context, topic string, handler func(payload []byte) error) (pubsub.Subscription, error) {
	f.handlers[topic] = append(f.handlers[topic], handler)
	return &fakeSubscription{ps: f, topic: topic}, nil
}

func (f *fakePubSub) publish(t *testing.T, topic string, payload []byte) {
	t.Helper()
	for _, h := range f.handlers[topic] {
		_ = h(payload)
	}
}

// TestSubscribeSessionUpdatesEmitsSnapshotThenLiveFrames pins the
// snapshot-then-stream order: the late-joiner snapshot frame must
// arrive before any subsequent live patches.
func TestSubscribeSessionUpdatesEmitsSnapshotThenLiveFrames(t *testing.T) {
	t.Parallel()

	ps := newFakePubSub()
	snap, _ := json.Marshal(SessionUpdate{Type: "snapshot", SessionID: "ses_x"})
	snapshotter := snapshotterFunc(func(_ context.Context, _ string) ([]byte, error) {
		return snap, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := SubscribeSessionUpdates(ctx, ps, snapshotter, "u-test", "ses_x")
	if err != nil {
		t.Fatalf("SubscribeSessionUpdates: %v", err)
	}

	// Live frame after subscribe.
	live, _ := json.Marshal(SessionUpdate{Type: "interaction_patch", SessionID: "ses_x", EntryPatches: []EntryPatch{{Index: 0, MessageID: "m1", Type: "text", Patch: "hi"}}})
	ps.publish(t, pubsub.GetSessionQueue("u-test", "ses_x"), live)

	got := readOne(t, ch)
	if got.Type != "snapshot" {
		t.Errorf("first frame type = %q, want snapshot (catch-up must precede live)", got.Type)
	}
	got = readOne(t, ch)
	if got.Type != "interaction_patch" {
		t.Errorf("second frame type = %q, want interaction_patch", got.Type)
	}
}

// TestSubscribeSessionUpdatesNoSnapshotter — nil/no snapshot still
// works; live frames are emitted as they arrive.
func TestSubscribeSessionUpdatesNoSnapshotter(t *testing.T) {
	t.Parallel()
	ps := newFakePubSub()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := SubscribeSessionUpdates(ctx, ps, NoopSessionPreamble{}, "u-test", "ses_y")
	if err != nil {
		t.Fatalf("SubscribeSessionUpdates: %v", err)
	}
	live, _ := json.Marshal(SessionUpdate{Type: "interaction_patch", SessionID: "ses_y"})
	ps.publish(t, pubsub.GetSessionQueue("u-test", "ses_y"), live)

	got := readOne(t, ch)
	if got.Type != "interaction_patch" {
		t.Errorf("frame type = %q", got.Type)
	}
}

// TestSubscribeSessionUpdatesUnsubscribesOnCtxDone — closing ctx must
// drain the subscription and close the channel.
func TestSubscribeSessionUpdatesUnsubscribesOnCtxDone(t *testing.T) {
	t.Parallel()
	ps := newFakePubSub()

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := SubscribeSessionUpdates(ctx, ps, NoopSessionPreamble{}, "u-test", "ses_z")
	if err != nil {
		t.Fatalf("SubscribeSessionUpdates: %v", err)
	}
	cancel()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, ok := <-ch; !ok {
			return
		}
	}
	t.Fatal("channel did not close after ctx cancel")
}

type snapshotterFunc func(ctx context.Context, sessionID string) ([]byte, error)

func (f snapshotterFunc) Snapshot(ctx context.Context, sessionID string) ([]byte, error) {
	return f(ctx, sessionID)
}

func readOne(t *testing.T, ch <-chan SessionUpdate) SessionUpdate {
	t.Helper()
	select {
	case u, ok := <-ch:
		if !ok {
			t.Fatal("channel closed before frame")
		}
		return u
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for frame")
	}
	return SessionUpdate{}
}
