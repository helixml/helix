package hydra

import (
	"sync"
	"time"
)

// LogLine is one log line emitted by hydra or by an inner container whose logs
// hydra is streaming. Held in LogBuffer's ring and broadcast to live
// subscribers.
type LogLine struct {
	Time time.Time `json:"t"`
	Line string    `json:"line"`
}

// LogBuffer is a bounded ring buffer of log lines with multi-subscriber
// live-tail support.
//
// Writers call Write(line) for each line. Readers call Snapshot(n) for the
// trailing n lines and Subscribe() for live lines. Subscribers receive lines
// on a channel; if a subscriber falls behind by more than slowChanBuffer
// lines, the buffer drops oldest queued lines for that subscriber rather than
// blocking the writer (the live tail is best-effort, full history is in the
// ring).
type LogBuffer struct {
	mu       sync.Mutex
	ring     []LogLine
	capacity int
	// head is the index of the next slot to write. ring[head-1] is the most
	// recent line. ring is logically circular; we use len < capacity to
	// distinguish the warmup phase.
	head    int
	filled  bool
	subs    map[int]chan LogLine
	nextSub int
}

const (
	defaultLogBufferCapacity = 10000
	slowSubBuffer            = 256
)

// NewLogBuffer creates a buffer that retains up to capacity recent lines.
// A capacity of zero or less uses the default of defaultLogBufferCapacity.
func NewLogBuffer(capacity int) *LogBuffer {
	if capacity <= 0 {
		capacity = defaultLogBufferCapacity
	}
	return &LogBuffer{
		ring:     make([]LogLine, capacity),
		capacity: capacity,
		subs:     make(map[int]chan LogLine),
	}
}

// Write appends a line to the buffer and fans it out to all live subscribers.
// Subscribers whose channel buffer is full have the oldest queued line dropped
// to make room — the writer never blocks.
func (b *LogBuffer) Write(line string) {
	entry := LogLine{Time: time.Now().UTC(), Line: line}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.ring[b.head] = entry
	b.head = (b.head + 1) % b.capacity
	if b.head == 0 {
		b.filled = true
	}

	for _, ch := range b.subs {
		select {
		case ch <- entry:
		default:
			// Subscriber is slow. Drop the oldest queued line and try again
			// so the live tail keeps moving even under back-pressure.
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- entry:
			default:
			}
		}
	}
}

// Snapshot returns the most recent n lines in chronological order. If n is
// zero or negative, returns the entire buffer.
func (b *LogBuffer) Snapshot(n int) []LogLine {
	b.mu.Lock()
	defer b.mu.Unlock()

	var size int
	if b.filled {
		size = b.capacity
	} else {
		size = b.head
	}
	if n <= 0 || n > size {
		n = size
	}

	out := make([]LogLine, 0, n)
	// Walk backwards from head for n entries, then reverse.
	for i := 0; i < n; i++ {
		idx := (b.head - 1 - i + b.capacity) % b.capacity
		out = append(out, b.ring[idx])
	}
	// Reverse in place for chronological order.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// Subscribe returns a channel that receives every new line written after
// subscription. The caller must call the returned cancel function to release
// the subscription; failing to do so leaks a goroutine-sized buffer per
// abandoned subscriber.
func (b *LogBuffer) Subscribe() (<-chan LogLine, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.subscribeLocked()
}

func (b *LogBuffer) subscribeLocked() (<-chan LogLine, func()) {
	id := b.nextSub
	b.nextSub++
	ch := make(chan LogLine, slowSubBuffer)
	b.subs[id] = ch

	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if ch, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(ch)
		}
	}
	return ch, cancel
}

// SnapshotAndSubscribe atomically returns the most recent n lines and a
// subscription that delivers every line written after the snapshot point.
// Use this when you want gap-free history-then-tail; Snapshot() followed by
// Subscribe() has a small race window where lines written in between can be
// missed.
func (b *LogBuffer) SnapshotAndSubscribe(n int) ([]LogLine, <-chan LogLine, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	var size int
	if b.filled {
		size = b.capacity
	} else {
		size = b.head
	}
	if n <= 0 || n > size {
		n = size
	}

	out := make([]LogLine, 0, n)
	for i := 0; i < n; i++ {
		idx := (b.head - 1 - i + b.capacity) % b.capacity
		out = append(out, b.ring[idx])
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}

	ch, cancel := b.subscribeLocked()
	return out, ch, cancel
}
