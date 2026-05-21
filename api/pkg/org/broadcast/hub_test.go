// Package broadcast_test characterises the public behaviour of the
// wake-only broadcaster lifted from helix-org/broadcast/ into this
// canonical home in migration H2.
//
// Coverage matches the success-criteria invariants B1..B11 named in
// the H2 plan. B1..B5 were pinned by the legacy
// helix-org/broadcast/broadcaster_test.go (deleted in the same
// commit); B6..B11 (multi-stream subscribers, SubscribeAll /
// UnsubscribeAll, Notify non-blocking on a full subscriber,
// Unsubscribe with an empty list, concurrent safety) are new
// characterisation coverage added on the way through.
//
// All cases were authored against the unmoved code (with a temporary
// upward import) and ran green before the lift; the lift only changed
// the import path. Per the characterisation-tests rule in
// helix-org/CLAUDE.md, any future refactor that needs these cases to
// change is by definition not behaviour-preserving — split the work.
package broadcast_test

import (
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/broadcast"
	"github.com/helixml/helix/api/pkg/org/stream"
)

const waitForWake = time.Second
const waitForNonWake = 50 * time.Millisecond

// --- B1..B5: legacy coverage, preserved verbatim ------------------------

func TestSubscribeAndNotify_WakesMatchingSubscriber(t *testing.T) { // B1
	t.Parallel()

	b := broadcast.New()
	ch := b.Subscribe([]stream.ID{"s-a", "s-b"})
	b.Notify("s-a")
	select {
	case <-ch:
	case <-time.After(waitForWake):
		t.Fatalf("subscriber did not wake")
	}
}

func TestNotify_IgnoresOtherStreams(t *testing.T) { // B2
	t.Parallel()

	b := broadcast.New()
	ch := b.Subscribe([]stream.ID{"s-a"})
	b.Notify("s-b")
	select {
	case <-ch:
		t.Fatalf("subscriber woke on unrelated stream")
	case <-time.After(waitForNonWake):
	}
}

func TestNotify_CoalescesBurstyNotifications(t *testing.T) { // B3
	t.Parallel()

	b := broadcast.New()
	ch := b.Subscribe([]stream.ID{"s-a"})
	for i := 0; i < 100; i++ {
		b.Notify("s-a")
	}
	// Drain — we expect exactly one wake-up (coalesced because the
	// subscriber's channel is buffered size 1).
	<-ch
	select {
	case <-ch:
		t.Fatalf("unexpected second wake-up from coalesced burst")
	case <-time.After(waitForNonWake):
	}
}

func TestUnsubscribe_StopsDelivery(t *testing.T) { // B4
	t.Parallel()

	b := broadcast.New()
	ch := b.Subscribe([]stream.ID{"s-a"})
	b.Unsubscribe([]stream.ID{"s-a"}, ch)
	b.Notify("s-a")
	select {
	case <-ch:
		t.Fatalf("woke after unsubscribe")
	case <-time.After(waitForNonWake):
	}
}

func TestNotify_WakesEveryMatchingSubscriber(t *testing.T) { // B5
	t.Parallel()

	b := broadcast.New()
	const n = 10
	var wg sync.WaitGroup
	channels := make([]chan struct{}, n)
	for i := range channels {
		channels[i] = b.Subscribe([]stream.ID{"s-a"})
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
	b.Notify("s-a")
	wg.Wait()
}

// --- B6..B11: new characterisation coverage -----------------------------

func TestSubscriber_RegisteredForMultipleStreams_WakesOnAny(t *testing.T) { // B6
	t.Parallel()

	b := broadcast.New()
	ch := b.Subscribe([]stream.ID{"s-a", "s-b", "s-c"})

	// Notify on s-b — the middle one — to prove order doesn't matter.
	b.Notify("s-b")
	select {
	case <-ch:
	case <-time.After(waitForWake):
		t.Fatalf("subscriber did not wake on s-b")
	}

	// And again on s-c after draining.
	b.Notify("s-c")
	select {
	case <-ch:
	case <-time.After(waitForWake):
		t.Fatalf("subscriber did not wake on s-c")
	}
}

func TestSubscribeAll_WakesOnAnyNotify(t *testing.T) { // B7
	t.Parallel()

	b := broadcast.New()
	ch := b.SubscribeAll()

	// No registration for a specific stream, yet any Notify wakes it.
	b.Notify("s-anything")
	select {
	case <-ch:
	case <-time.After(waitForWake):
		t.Fatalf("SubscribeAll listener did not wake on s-anything")
	}

	// A second, unrelated stream — same listener wakes again.
	b.Notify("s-different")
	select {
	case <-ch:
	case <-time.After(waitForWake):
		t.Fatalf("SubscribeAll listener did not wake on s-different")
	}
}

func TestUnsubscribeAll_StopsDelivery(t *testing.T) { // B8
	t.Parallel()

	b := broadcast.New()
	ch := b.SubscribeAll()
	b.UnsubscribeAll(ch)

	b.Notify("s-x")
	select {
	case <-ch:
		t.Fatalf("SubscribeAll listener woke after UnsubscribeAll")
	case <-time.After(waitForNonWake):
	}
}

func TestNotify_NonBlockingOnFullSubscriberChannel(t *testing.T) { // B9
	t.Parallel()

	b := broadcast.New()
	// Subscribe but never drain the channel — the second notify must
	// not block. If Notify weren't non-blocking, this test would hang
	// past the timeout under `-race` and cause a goroutine leak.
	ch := b.Subscribe([]stream.ID{"s-a"})
	_ = ch // not draining intentionally

	// First Notify fills the channel.
	b.Notify("s-a")
	// Subsequent Notifies must coalesce silently — none of them block.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			b.Notify("s-a")
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

	b := broadcast.New()
	ch := b.Subscribe([]stream.ID{"s-a"})

	// Calling Unsubscribe with an empty list MUST NOT panic, MUST NOT
	// drop the existing subscription. After the no-op, the subscriber
	// is still woken by Notify on s-a.
	b.Unsubscribe(nil, ch)
	b.Unsubscribe([]stream.ID{}, ch)

	b.Notify("s-a")
	select {
	case <-ch:
	case <-time.After(waitForWake):
		t.Fatalf("subscriber dropped after no-op Unsubscribe")
	}
}

func TestConcurrent_SubscribeNotifyUnsubscribe_RaceFree(t *testing.T) { // B11
	t.Parallel()
	// Hammer the broadcaster from N goroutines doing each operation.
	// Under `make test`'s -race flag, any data race here fails the
	// test — that's the real assertion. The wake-count check below is
	// deterministic (the durable subscriber is registered before any
	// notify fires) so the test never flakes.
	b := broadcast.New()
	const goroutines = 8
	const iterations = 200

	// One durable subscriber registered before any Notify — guaranteed
	// to observe at least one wake.
	durable := b.Subscribe([]stream.ID{"s-shared"})

	var wg sync.WaitGroup
	start := make(chan struct{})

	// Notify spamming
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < iterations; i++ {
				b.Notify("s-shared")
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
				ch := b.Subscribe([]stream.ID{"s-shared"})
				b.Unsubscribe([]stream.ID{"s-shared"}, ch)
			}
		}()
	}

	close(start)
	wg.Wait()

	// Durable subscriber MUST have observed at least one wake — it was
	// registered before any Notify ran. (Coalescing means we can't
	// assert on the count, just on non-zero.)
	select {
	case <-durable:
	case <-time.After(waitForNonWake):
		t.Fatalf("durable subscriber did not observe a wake across %d notifies", goroutines*iterations)
	}
}
