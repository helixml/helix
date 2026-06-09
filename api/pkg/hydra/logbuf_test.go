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

	got := buf.Snapshot(100)
	if len(got) != 3 {
		t.Fatalf("want 3 lines, got %d", len(got))
	}
	for i, want := range []string{"a", "b", "c"} {
		if got[i].Line != want {
			t.Errorf("Snapshot[%d] = %q, want %q", i, got[i].Line, want)
		}
	}
}

func TestLogBuffer_SnapshotZeroIsEmpty(t *testing.T) {
	buf := NewLogBuffer(10)
	buf.Write("a")
	buf.Write("b")

	got := buf.Snapshot(0)
	if len(got) != 0 {
		t.Fatalf("Snapshot(0) want 0 lines, got %d", len(got))
	}
}

func TestLogBuffer_RingWraps(t *testing.T) {
	buf := NewLogBuffer(3)
	for _, s := range []string{"a", "b", "c", "d", "e"} {
		buf.Write(s)
	}
	got := buf.Snapshot(100)
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
	ch, cancel, err := buf.Subscribe()
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
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
	ch, cancel, err := buf.Subscribe()
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	cancel()

	// Channel should be closed; receive should not block.
	_, ok := <-ch
	if ok {
		t.Fatal("expected channel to be closed after cancel")
	}
}

func TestLogBuffer_SlowSubscriberDoesNotBlockWriter(t *testing.T) {
	buf := NewLogBuffer(10)
	_, cancel, err := buf.Subscribe() // never read from this channel
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
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
			ch, cancel, err := buf.Subscribe()
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
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

	got := buf.Snapshot(2000)
	if len(got) != 1000 {
		t.Fatalf("want capacity (1000) lines after %d writes, got %d", writers*linesPerWriter, len(got))
	}
}

func TestLogBufferWriter_SplitsOnNewline(t *testing.T) {
	buf := NewLogBuffer(10)
	w := buf.Writer()

	// Single Write with multiple lines.
	if _, err := w.Write([]byte("alpha\nbravo\ncharlie\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// Partial line (no trailing newline) is held.
	if _, err := w.Write([]byte("delta")); err != nil {
		t.Fatalf("Write partial: %v", err)
	}
	if got := buf.Snapshot(100); len(got) != 3 {
		t.Errorf("after partial, want 3 lines, got %d: %v", len(got), got)
	}
	// Finishing the line flushes it.
	if _, err := w.Write([]byte(" extra\n")); err != nil {
		t.Fatalf("Write rest: %v", err)
	}
	got := buf.Snapshot(100)
	if len(got) != 4 {
		t.Fatalf("after complete, want 4 lines, got %d", len(got))
	}
	for i, want := range []string{"alpha", "bravo", "charlie", "delta extra"} {
		if got[i].Line != want {
			t.Errorf("Snapshot[%d] = %q, want %q", i, got[i].Line, want)
		}
	}
}

func TestLogBufferWriter_StripsCRLF(t *testing.T) {
	buf := NewLogBuffer(10)
	w := buf.Writer()
	_, _ = w.Write([]byte("hello\r\nworld\r\n"))
	got := buf.Snapshot(100)
	if len(got) != 2 || got[0].Line != "hello" || got[1].Line != "world" {
		t.Errorf("CRLF strip failed: %v", got)
	}
}

func TestLogBufferWriter_LongLineFlushesEarly(t *testing.T) {
	buf := NewLogBuffer(10)
	w := buf.Writer()
	huge := make([]byte, lineWriterFlushCap+10) // no newline
	for i := range huge {
		huge[i] = 'x'
	}
	_, _ = w.Write(huge)
	got := buf.Snapshot(100)
	if len(got) == 0 {
		t.Fatal("expected force-flush of oversized line")
	}
}

func TestLogBuffer_SubscriberCap(t *testing.T) {
	buf := NewLogBuffer(10)
	cancels := make([]func(), 0, maxSubscribers)
	for i := 0; i < maxSubscribers; i++ {
		_, cancel, err := buf.Subscribe()
		if err != nil {
			t.Fatalf("subscribe %d failed: %v", i, err)
		}
		cancels = append(cancels, cancel)
	}
	defer func() {
		for _, c := range cancels {
			c()
		}
	}()

	// One past the cap should fail.
	_, _, err := buf.Subscribe()
	if err == nil {
		t.Fatal("Subscribe past cap should have failed")
	}
	if err != ErrTooManySubscribers {
		t.Errorf("want ErrTooManySubscribers, got %v", err)
	}

	// After releasing one, a new subscription should succeed.
	cancels[0]()
	cancels = cancels[1:]
	ch, cancel, err := buf.Subscribe()
	if err != nil {
		t.Fatalf("subscribe after release failed: %v", err)
	}
	cancels = append(cancels, cancel)
	if ch == nil {
		t.Fatal("returned channel is nil")
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

	hist, sub, cancel, err := buf.SnapshotAndSubscribe(1000)
	if err != nil {
		t.Fatalf("SnapshotAndSubscribe failed: %v", err)
	}
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
