package hydra

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestLogBuffer_SnapshotReturnsChronological(t *testing.T) {
	buf := NewLogBuffer(10)
	buf.Write("a")
	buf.Write("b")
	buf.Write("c")

	got := buf.Snapshot(0)
	if len(got) != 3 {
		t.Fatalf("want 3 lines, got %d", len(got))
	}
	for i, want := range []string{"a", "b", "c"} {
		if got[i].Line != want {
			t.Errorf("Snapshot[%d] = %q, want %q", i, got[i].Line, want)
		}
	}
}

func TestLogBuffer_RingWraps(t *testing.T) {
	buf := NewLogBuffer(3)
	for _, s := range []string{"a", "b", "c", "d", "e"} {
		buf.Write(s)
	}
	got := buf.Snapshot(0)
	if len(got) != 3 {
		t.Fatalf("want 3 lines (capacity), got %d", len(got))
	}
	for i, want := range []string{"c", "d", "e"} {
		if got[i].Line != want {
			t.Errorf("Snapshot[%d] = %q, want %q", i, got[i].Line, want)
		}
	}
}

func TestLogBuffer_TailN(t *testing.T) {
	buf := NewLogBuffer(10)
	for i := 0; i < 7; i++ {
		buf.Write(fmt.Sprintf("line-%d", i))
	}
	got := buf.Snapshot(3)
	if len(got) != 3 {
		t.Fatalf("want 3 lines, got %d", len(got))
	}
	for i, want := range []string{"line-4", "line-5", "line-6"} {
		if got[i].Line != want {
			t.Errorf("Snapshot[%d] = %q, want %q", i, got[i].Line, want)
		}
	}
}

func TestLogBuffer_SubscribeReceivesLiveLines(t *testing.T) {
	buf := NewLogBuffer(10)
	ch, cancel := buf.Subscribe()
	defer cancel()

	go func() {
		for i := 0; i < 3; i++ {
			buf.Write(fmt.Sprintf("live-%d", i))
		}
	}()

	for i := 0; i < 3; i++ {
		select {
		case got := <-ch:
			want := fmt.Sprintf("live-%d", i)
			if got.Line != want {
				t.Errorf("subscriber[%d] = %q, want %q", i, got.Line, want)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("timeout waiting for line %d", i)
		}
	}
}

func TestLogBuffer_CancelStopsSubscription(t *testing.T) {
	buf := NewLogBuffer(10)
	ch, cancel := buf.Subscribe()
	cancel()

	// Channel should be closed; receive should not block.
	_, ok := <-ch
	if ok {
		t.Fatal("expected channel to be closed after cancel")
	}
}

func TestLogBuffer_SlowSubscriberDoesNotBlockWriter(t *testing.T) {
	buf := NewLogBuffer(10)
	_, cancel := buf.Subscribe() // never read from this channel
	defer cancel()

	// Write more lines than the per-subscriber buffer can hold. If the writer
	// blocks, this test will time out at the global test timeout. With the
	// drop-oldest policy, this completes immediately.
	done := make(chan struct{})
	go func() {
		for i := 0; i < slowSubBuffer*2; i++ {
			buf.Write(fmt.Sprintf("line-%d", i))
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("writer was blocked by slow subscriber")
	}
}

func TestLogBuffer_ConcurrentWritersAndSubscribers(t *testing.T) {
	buf := NewLogBuffer(1000)

	var wg sync.WaitGroup
	const writers = 4
	const linesPerWriter = 250
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < linesPerWriter; i++ {
				buf.Write(fmt.Sprintf("w%d-%d", w, i))
			}
		}(w)
	}

	// Run a couple of subscribers in parallel, draining what they receive.
	const subscribers = 2
	subWg := sync.WaitGroup{}
	subDone := make(chan struct{})
	for s := 0; s < subscribers; s++ {
		subWg.Add(1)
		go func() {
			defer subWg.Done()
			ch, cancel := buf.Subscribe()
			defer cancel()
			for {
				select {
				case <-ch:
				case <-subDone:
					return
				}
			}
		}()
	}

	wg.Wait()
	close(subDone)
	subWg.Wait()

	got := buf.Snapshot(0)
	if len(got) != 1000 {
		t.Fatalf("want capacity (1000) lines after %d writes, got %d", writers*linesPerWriter, len(got))
	}
}

func TestLogBuffer_SnapshotAndSubscribe_NoGap(t *testing.T) {
	// Ordering guarantee: history returned by SnapshotAndSubscribe should
	// chain seamlessly with the subscription, with no missed or duplicated
	// lines even if a writer fires between snapshot and subscribe under a
	// plain Snapshot+Subscribe call.

	buf := NewLogBuffer(100)
	// Prime with 5 history lines.
	for i := 0; i < 5; i++ {
		buf.Write(fmt.Sprintf("hist-%d", i))
	}

	hist, sub, cancel := buf.SnapshotAndSubscribe(0)
	defer cancel()

	// Concurrent writes after subscribe should land in the channel.
	go func() {
		for i := 0; i < 5; i++ {
			buf.Write(fmt.Sprintf("live-%d", i))
		}
	}()

	if len(hist) != 5 {
		t.Fatalf("want 5 history lines, got %d", len(hist))
	}
	for i, want := range []string{"hist-0", "hist-1", "hist-2", "hist-3", "hist-4"} {
		if hist[i].Line != want {
			t.Errorf("hist[%d] = %q, want %q", i, hist[i].Line, want)
		}
	}

	for i := 0; i < 5; i++ {
		select {
		case got := <-sub:
			want := fmt.Sprintf("live-%d", i)
			if got.Line != want {
				t.Errorf("live[%d] = %q, want %q", i, got.Line, want)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("timeout waiting for live line %d", i)
		}
	}
}
