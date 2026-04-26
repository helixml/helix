package broadcast

import (
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix-org/domain"
)

func TestBroadcasterWakesMatchingSubscriber(t *testing.T) {
	t.Parallel()

	b := New()
	ch := b.Subscribe([]domain.StreamID{"s-a", "s-b"})
	b.Notify("s-a")
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("subscriber did not wake")
	}
}

func TestBroadcasterIgnoresOtherStreams(t *testing.T) {
	t.Parallel()

	b := New()
	ch := b.Subscribe([]domain.StreamID{"s-a"})
	b.Notify("s-b")
	select {
	case <-ch:
		t.Fatalf("subscriber woke on unrelated stream")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestBroadcasterCoalescesBurstyNotifications(t *testing.T) {
	t.Parallel()

	b := New()
	ch := b.Subscribe([]domain.StreamID{"s-a"})
	for i := 0; i < 100; i++ {
		b.Notify("s-a")
	}
	// Drain — we should get exactly one wake-up (coalesced).
	<-ch
	select {
	case <-ch:
		t.Fatalf("unexpected second wake-up from coalesced burst")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestBroadcasterUnsubscribeStopsDelivery(t *testing.T) {
	t.Parallel()

	b := New()
	ch := b.Subscribe([]domain.StreamID{"s-a"})
	b.Unsubscribe([]domain.StreamID{"s-a"}, ch)
	b.Notify("s-a")
	select {
	case <-ch:
		t.Fatalf("woke after unsubscribe")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestBroadcasterSubscribeAllWakesOnAnyStream(t *testing.T) {
	t.Parallel()

	b := New()
	ch := b.SubscribeAll()
	b.Notify("s-anything")
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("wildcard subscriber did not wake")
	}
}

func TestBroadcasterSubscribeAllStillWakesAfterPerStreamNotify(t *testing.T) {
	t.Parallel()

	b := New()
	per := b.Subscribe([]domain.StreamID{"s-a"})
	all := b.SubscribeAll()
	b.Notify("s-a")
	select {
	case <-per:
	case <-time.After(time.Second):
		t.Fatalf("per-stream subscriber did not wake")
	}
	select {
	case <-all:
	case <-time.After(time.Second):
		t.Fatalf("wildcard subscriber did not wake on per-stream notify")
	}
}

func TestBroadcasterUnsubscribeAllStopsDelivery(t *testing.T) {
	t.Parallel()

	b := New()
	ch := b.SubscribeAll()
	b.UnsubscribeAll(ch)
	b.Notify("s-a")
	select {
	case <-ch:
		t.Fatalf("woke after UnsubscribeAll")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestBroadcasterMultipleSubscribers(t *testing.T) {
	t.Parallel()

	b := New()
	const n = 10
	var wg sync.WaitGroup
	channels := make([]chan struct{}, n)
	for i := range channels {
		channels[i] = b.Subscribe([]domain.StreamID{"s-a"})
		wg.Add(1)
		go func(ch chan struct{}) {
			defer wg.Done()
			select {
			case <-ch:
			case <-time.After(time.Second):
				t.Errorf("subscriber did not wake")
			}
		}(channels[i])
	}
	b.Notify("s-a")
	wg.Wait()
}
