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
	ch := b.Subscribe([]domain.ChannelID{"c-a", "c-b"})
	b.Notify("c-a")
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("subscriber did not wake")
	}
}

func TestBroadcasterIgnoresOtherChannels(t *testing.T) {
	t.Parallel()

	b := New()
	ch := b.Subscribe([]domain.ChannelID{"c-a"})
	b.Notify("c-b")
	select {
	case <-ch:
		t.Fatalf("subscriber woke on unrelated channel")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestBroadcasterCoalescesBurstyNotifications(t *testing.T) {
	t.Parallel()

	b := New()
	ch := b.Subscribe([]domain.ChannelID{"c-a"})
	for i := 0; i < 100; i++ {
		b.Notify("c-a")
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
	ch := b.Subscribe([]domain.ChannelID{"c-a"})
	b.Unsubscribe([]domain.ChannelID{"c-a"}, ch)
	b.Notify("c-a")
	select {
	case <-ch:
		t.Fatalf("woke after unsubscribe")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestBroadcasterSubscribeAllWakesOnAnyChannel(t *testing.T) {
	t.Parallel()

	b := New()
	ch := b.SubscribeAll()
	b.Notify("c-anything")
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("wildcard subscriber did not wake")
	}
}

func TestBroadcasterSubscribeAllStillWakesAfterPerChannelNotify(t *testing.T) {
	t.Parallel()

	b := New()
	per := b.Subscribe([]domain.ChannelID{"c-a"})
	all := b.SubscribeAll()
	b.Notify("c-a")
	select {
	case <-per:
	case <-time.After(time.Second):
		t.Fatalf("per-channel subscriber did not wake")
	}
	select {
	case <-all:
	case <-time.After(time.Second):
		t.Fatalf("wildcard subscriber did not wake on per-channel notify")
	}
}

func TestBroadcasterUnsubscribeAllStopsDelivery(t *testing.T) {
	t.Parallel()

	b := New()
	ch := b.SubscribeAll()
	b.UnsubscribeAll(ch)
	b.Notify("c-a")
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
		channels[i] = b.Subscribe([]domain.ChannelID{"c-a"})
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
	b.Notify("c-a")
	wg.Wait()
}
