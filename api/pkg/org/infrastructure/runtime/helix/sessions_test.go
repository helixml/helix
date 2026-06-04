package helix

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"

	"github.com/helixml/helix/api/pkg/pubsub"
)

// fakePubSub is a minimal in-process pubsub for SubscribeSessionUpdates
// tests. Only the methods SubscribeSessionUpdates uses are implemented;
// the rest return zero values and would panic if exercised.
//
// The Spawner spawns the SubscribeSessionUpdates handler in its own
// goroutine and unsubscribes from a deferred path, so concurrent
// Subscribe/Unsubscribe/publish calls land on the same map from
// different goroutines (most visibly under TestSpawnerSemaphoreSerialises,
// which intentionally runs two spawners in parallel). Guard the handlers
// map with a sync.Mutex so the race detector and Go's concurrent-map-
// write check stay happy under -count / -parallel stress and in CI.
type fakePubSub struct {
	pubsub.PubSub
	mu       sync.Mutex
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
	s.ps.mu.Lock()
	defer s.ps.mu.Unlock()
	delete(s.ps.handlers, s.topic)
	return nil
}

func (f *fakePubSub) Subscribe(_ context.Context, topic string, handler func(payload []byte) error) (pubsub.Subscription, error) {
	f.mu.Lock()
	f.handlers[topic] = append(f.handlers[topic], handler)
	f.mu.Unlock()
	return &fakeSubscription{ps: f, topic: topic}, nil
}

// handlerCount returns the number of handlers currently subscribed to
// topic. Tests use this to wait for a bridge goroutine to attach
// before publishing — racing on the raw map is unsafe under
// concurrent Subscribe/Unsubscribe.
func (f *fakePubSub) handlerCount(topic string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.handlers[topic])
}

func (f *fakePubSub) publish(t *testing.T, topic string, payload []byte) {
	t.Helper()
	f.mu.Lock()
	// Copy the handler slice under the lock so callers can't observe
	// torn state if a concurrent Subscribe/Unsubscribe mutates the map
	// while we're iterating.
	handlers := append([]func(payload []byte) error(nil), f.handlers[topic]...)
	f.mu.Unlock()
	for _, h := range handlers {
		_ = h(payload)
	}
}

// TestSubscribeSessionUpdatesEmitsSnapshotThenLiveFrames pins the
// snapshot-then-stream order: the late-joiner snapshot frame must
// arrive before any subsequent live patches.
func TestSubscribeSessionUpdatesEmitsSnapshotThenLiveFrames(t *testing.T) {
	t.Parallel()

	ps := newFakePubSub()
	snap, _ := json.Marshal(types.WebsocketEvent{Type: "snapshot", SessionID: "ses_x"})
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
	live, _ := json.Marshal(types.WebsocketEvent{Type: "interaction_patch", SessionID: "ses_x", EntryPatches: []types.EntryPatch{{Index: 0, MessageID: "m1", Type: "text", Patch: "hi"}}})
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
	live, _ := json.Marshal(types.WebsocketEvent{Type: "interaction_patch", SessionID: "ses_y"})
	ps.publish(t, pubsub.GetSessionQueue("u-test", "ses_y"), live)

	got := readOne(t, ch)
	if got.Type != "interaction_patch" {
		t.Errorf("frame type = %q", got.Type)
	}
}

// TestSubscribeSessionUpdatesNilPubSubReturnsError pins the regression
// behind the panic that crashed the helix-org-spawned bridge goroutine
// whenever buildHelixOrgSpawnerConfig forgot to wire PubSub. Before
// the fix, calling SubscribeSessionUpdates with a nil ps did
// `ps.Subscribe(...)` and segfaulted the API process; the bridge runs
// in a goroutine and the panic took the whole process down on every
// AI-worker activation. SubscribeSessionUpdates must return a regular
// error instead, so the caller's reconnect loop logs and backs off
// instead of crashing the host.
func TestSubscribeSessionUpdatesNilPubSubReturnsError(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := SubscribeSessionUpdates(ctx, nil, NoopSessionPreamble{}, "u-test", "ses_nil")
	if err == nil {
		t.Fatal("SubscribeSessionUpdates(nil pubsub): expected error, got nil")
	}
	if ch != nil {
		t.Errorf("SubscribeSessionUpdates(nil pubsub): expected nil channel, got %v", ch)
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

func readOne(t *testing.T, ch <-chan types.WebsocketEvent) types.WebsocketEvent {
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
	return types.WebsocketEvent{}
}
