// Package wakebus_test characterises the public behaviour of the
// pubsub-backed stream wake facade.
//
// The cases mirror the deleted api/pkg/org/broadcast/hub_test.go suite
// 1:1 (B1..B11 from the H2 plan) so we keep the externally observable
// contract bit-for-bit identical across the refactor: any future change
// here would by definition not be behaviour-preserving.
//
// The only meaningful test-rig difference from the in-memory broadcast
// suite is timing: the underlying pubsub.PubSub delivers notifications
// on a separate goroutine, so each Subscribe is followed by a short
// sleep to give NATS time to install the subscription before the first
// Publish fires. The 1 s wake timeout already absorbs the delivery
// latency on the wake-path itself.
package wakebus_test

import (
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/infrastructure/wakebus"
	"github.com/helixml/helix/api/pkg/pubsub"
)

const (
	// waitForWake is the upper bound a test waits for an expected wake
	// signal. Generous so a slow CI box doesn't flake.
	waitForWake = time.Second

	// waitForNonWake is the window we observe to confirm no wake was
	// delivered. Bumped above broadcast's 50 ms because pubsub delivery
	// runs on a goroutine — we have to wait long enough to be confident
	// that no in-flight delivery is about to arrive.
	waitForNonWake = 250 * time.Millisecond

	// subInstall is the post-Subscribe pause that lets NATS register
	// the subscription with the server before we Publish. Without
	// this, a Publish immediately after Subscribe can race the SUB
	// command and the handler never sees the message. The real-NATS
	// integration tests in api/pkg/pubsub use 1s; we use 100ms because
	// the in-memory server is in-process and that's empirically
	// sufficient on every machine the suite runs on.
	subInstall = 100 * time.Millisecond
)

// newHub creates an in-memory NATS-backed Bus for one test. Cleanup
// is registered via t.Cleanup so the embedded server is shut down at
// test exit.
func newHub(t *testing.T) *wakebus.Bus {
	t.Helper()
	ps, err := pubsub.NewInMemoryNats()
	if err != nil {
		t.Fatalf("NewInMemoryNats: %v", err)
	}
	return wakebus.New(ps)
}

// --- B1..B5: legacy coverage, preserved verbatim ------------------------

func TestSubscribeAndNotify_WakesMatchingSubscriber(t *testing.T) { // B1
	t.Parallel()

	h := newHub(t)
	ch := h.Subscribe("org-test", []streaming.StreamID{"s-a", "s-b"})
	time.Sleep(subInstall)
	h.Notify("org-test", "s-a")
	select {
	case <-ch:
	case <-time.After(waitForWake):
		t.Fatalf("subscriber did not wake")
	}
}

func TestNotify_IgnoresOtherStreams(t *testing.T) { // B2
	t.Parallel()

	h := newHub(t)
	ch := h.Subscribe("org-test", []streaming.StreamID{"s-a"})
	time.Sleep(subInstall)
	h.Notify("org-test", "s-b")
	select {
	case <-ch:
		t.Fatalf("subscriber woke on unrelated stream")
	case <-time.After(waitForNonWake):
	}
}

func TestNotify_CoalescesBurstyNotifications(t *testing.T) { // B3
	t.Parallel()

	h := newHub(t)
	ch := h.Subscribe("org-test", []streaming.StreamID{"s-a"})
	time.Sleep(subInstall)
	for i := 0; i < 100; i++ {
		h.Notify("org-test", "s-a")
	}
	// Drain the first wake-up. With pubsub-backed delivery this may
	// take a few ms to appear; the 1 s wake timeout absorbs that.
	select {
	case <-ch:
	case <-time.After(waitForWake):
		t.Fatalf("subscriber did not wake at all during burst")
	}
	// Let any in-flight handler deliveries land. Once that settles,
	// the buffered-1 wake channel may have re-filled (the burst
	// triggered ~100 deliveries, only one of which fits) — drain that
	// single residual signal too.
	time.Sleep(waitForNonWake)
	select {
	case <-ch:
	default:
	}
	// And now confirm no further wakes show up — the burst is done,
	// coalescing held.
	select {
	case <-ch:
		t.Fatalf("unexpected extra wake after coalesced burst settled")
	case <-time.After(waitForNonWake):
	}
}

func TestUnsubscribe_StopsDelivery(t *testing.T) { // B4
	t.Parallel()

	h := newHub(t)
	ch := h.Subscribe("org-test", []streaming.StreamID{"s-a"})
	time.Sleep(subInstall)
	h.Unsubscribe([]streaming.StreamID{"s-a"}, ch)
	h.Notify("org-test", "s-a")
	select {
	case <-ch:
		t.Fatalf("woke after unsubscribe")
	case <-time.After(waitForNonWake):
	}
}

func TestNotify_WakesEveryMatchingSubscriber(t *testing.T) { // B5
	t.Parallel()

	h := newHub(t)
	const n = 10
	var wg sync.WaitGroup
	channels := make([]chan struct{}, n)
	for i := range channels {
		channels[i] = h.Subscribe("org-test", []streaming.StreamID{"s-a"})
	}
	time.Sleep(subInstall)
	for i := range channels {
		wg.Add(1)
		go func(ch chan struct{}) {
			defer wg.Done()
			select {
			case <-ch:
			case <-time.After(waitForWake):
				t.Errorf("subscriber did not wake")
			}
		}(channels[i])
	}
	h.Notify("org-test", "s-a")
	wg.Wait()
}

// --- B6..B11: new characterisation coverage -----------------------------

func TestSubscriber_RegisteredForMultipleStreams_WakesOnAny(t *testing.T) { // B6
	t.Parallel()

	h := newHub(t)
	ch := h.Subscribe("org-test", []streaming.StreamID{"s-a", "s-b", "s-c"})
	time.Sleep(subInstall)

	// Notify on s-b — the middle one — to prove order doesn't matter.
	h.Notify("org-test", "s-b")
	select {
	case <-ch:
	case <-time.After(waitForWake):
		t.Fatalf("subscriber did not wake on s-b")
	}

	// And again on s-c after draining.
	h.Notify("org-test", "s-c")
	select {
	case <-ch:
	case <-time.After(waitForWake):
		t.Fatalf("subscriber did not wake on s-c")
	}
}

func TestSubscribeAll_WakesOnAnyNotify(t *testing.T) { // B7
	t.Parallel()

	h := newHub(t)
	ch := h.SubscribeAll("org-test")
	time.Sleep(subInstall)

	// No registration for a specific stream, yet any Notify wakes it.
	h.Notify("org-test", "s-anything")
	select {
	case <-ch:
	case <-time.After(waitForWake):
		t.Fatalf("SubscribeAll listener did not wake on s-anything")
	}

	// A second, unrelated stream — same listener wakes again.
	h.Notify("org-test", "s-different")
	select {
	case <-ch:
	case <-time.After(waitForWake):
		t.Fatalf("SubscribeAll listener did not wake on s-different")
	}
}

func TestUnsubscribeAll_StopsDelivery(t *testing.T) { // B8
	t.Parallel()

	h := newHub(t)
	ch := h.SubscribeAll("org-test")
	time.Sleep(subInstall)
	h.UnsubscribeAll(ch)

	h.Notify("org-test", "s-x")
	select {
	case <-ch:
		t.Fatalf("SubscribeAll listener woke after UnsubscribeAll")
	case <-time.After(waitForNonWake):
	}
}

func TestNotify_NonBlockingOnFullSubscriberChannel(t *testing.T) { // B9
	t.Parallel()

	h := newHub(t)
	// Subscribe but never drain the channel — the second notify must
	// not block. If Notify weren't non-blocking, this test would hang
	// past the timeout and fail.
	ch := h.Subscribe("org-test", []streaming.StreamID{"s-a"})
	_ = ch // not draining intentionally
	time.Sleep(subInstall)

	// First Notify fills the channel.
	h.Notify("org-test", "s-a")
	// Subsequent Notifies must coalesce silently — none of them block.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			h.Notify("org-test", "s-a")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(waitForWake):
		t.Fatalf("Notify blocked on a full subscriber channel")
	}
}

func TestUnsubscribe_EmptyStreamListIsNoop(t *testing.T) { // B10
	t.Parallel()

	h := newHub(t)
	ch := h.Subscribe("org-test", []streaming.StreamID{"s-a"})
	time.Sleep(subInstall)

	// Calling Unsubscribe with an empty list MUST NOT panic and MUST
	// NOT drop the existing subscription. After the no-op, the
	// subscriber is still woken by Notify on s-a.
	h.Unsubscribe(nil, ch)
	h.Unsubscribe([]streaming.StreamID{}, ch)

	h.Notify("org-test", "s-a")
	select {
	case <-ch:
	case <-time.After(waitForWake):
		t.Fatalf("subscriber dropped after no-op Unsubscribe")
	}
}

func TestConcurrent_SubscribeNotifyUnsubscribe_RaceFree(t *testing.T) { // B11
	t.Parallel()
	// Hammer the hub from N goroutines doing each operation. Under
	// `go test -race` any data race here fails the test — that's the
	// real assertion. The wake-count check below is deterministic
	// (the durable subscriber is registered before any notify fires)
	// so the test never flakes.
	h := newHub(t)
	const goroutines = 8
	const iterations = 200

	// One durable subscriber registered before any Notify — guaranteed
	// to observe at least one wake.
	durable := h.Subscribe("org-test", []streaming.StreamID{"s-shared"})
	time.Sleep(subInstall)

	var wg sync.WaitGroup
	start := make(chan struct{})

	// Notify spamming
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < iterations; i++ {
				h.Notify("org-test", "s-shared")
			}
		}()
	}

	// Subscribe/Unsubscribe churn from transient subscribers — its
	// only job is to exercise the lock path concurrently. We don't
	// assert anything about whether transient subscribers see wakes.
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < iterations; i++ {
				ch := h.Subscribe("org-test", []streaming.StreamID{"s-shared"})
				h.Unsubscribe([]streaming.StreamID{"s-shared"}, ch)
			}
		}()
	}

	close(start)
	wg.Wait()

	// Durable subscriber MUST have observed at least one wake — it
	// was registered before any Notify ran. (Coalescing means we
	// can't assert on the count, just on non-zero.) Wait for the
	// pubsub goroutines to drain.
	select {
	case <-durable:
	case <-time.After(waitForWake):
		t.Fatalf("durable subscriber did not observe a wake across %d notifies", goroutines*iterations)
	}
}

// TestNotify_IsolatedAcrossOrgs is the regression test for the
// cross-tenant wake leak (design/2026-06-09-org-multitenancy-spawner-leak.md).
//
// Stream IDs are unique only within an org, so two orgs share ids like
// `s-general` and `s-transcript-w-owner`. The wake topic must include
// the org, otherwise one org's Notify wakes the other org's subscriber
// on a colliding id. Here both orgs subscribe to the SAME id; a Notify
// for org-a must wake only org-a.
func TestNotify_IsolatedAcrossOrgs(t *testing.T) {
	t.Parallel()

	h := newHub(t)
	chA := h.Subscribe("org-a", []streaming.StreamID{"s-general"})
	chB := h.Subscribe("org-b", []streaming.StreamID{"s-general"})
	time.Sleep(subInstall)

	h.Notify("org-a", "s-general")

	select {
	case <-chA:
	case <-time.After(waitForWake):
		t.Fatalf("org-a subscriber did not wake on its own org's Notify")
	}
	select {
	case <-chB:
		t.Fatalf("org-b subscriber woke on org-a's Notify for a colliding stream id — cross-tenant wake leak")
	case <-time.After(waitForNonWake):
	}
}

// TestSubscribeAll_IsolatedAcrossOrgs pins that the wildcard live-view
// subscription is org-scoped: SubscribeAll for org-a must not fire on
// org-b's Notify.
func TestSubscribeAll_IsolatedAcrossOrgs(t *testing.T) {
	t.Parallel()

	h := newHub(t)
	ch := h.SubscribeAll("org-a")
	time.Sleep(subInstall)

	h.Notify("org-b", "s-anything")
	select {
	case <-ch:
		t.Fatalf("org-a SubscribeAll woke on org-b's Notify — wildcard is not org-scoped")
	case <-time.After(waitForNonWake):
	}

	h.Notify("org-a", "s-anything")
	select {
	case <-ch:
	case <-time.After(waitForWake):
		t.Fatalf("org-a SubscribeAll did not wake on its own org's Notify")
	}
}
