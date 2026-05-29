package hydra

import (
	"bytes"
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
	// maxSubscribers caps the number of live tailers per LogBuffer. The
	// admin /logs WS endpoint is the only caller; in practice this means
	// "at most N concurrent admin tabs across all browsers." Each
	// subscriber holds a per-channel buffer plus a fanout cost in Write, so
	// uncapped subscribers would let one buggy admin client (or a
	// reconnect loop) starve the writer side.
	maxSubscribers = 32
)

// ErrTooManySubscribers is returned by Subscribe / SnapshotAndSubscribe when
// the per-buffer subscriber cap is exceeded.
var ErrTooManySubscribers = errSubscriberCap{}

type errSubscriberCap struct{}

func (errSubscriberCap) Error() string { return "log buffer: too many subscribers" }

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

// Snapshot returns the most recent n lines in chronological order.
//
// n=0 returns an empty slice (matches the WS handler "no history" contract).
// n<0 is normalised to 0. n>capacity is clamped to capacity. Pass a large n
// (e.g. defaultLogBufferCapacity) to mean "everything in the ring."
func (b *LogBuffer) Snapshot(n int) []LogLine {
	b.mu.Lock()
	defer b.mu.Unlock()

	if n < 0 {
		n = 0
	}
	var size int
	if b.filled {
		size = b.capacity
	} else {
		size = b.head
	}
	if n > size {
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
//
// Returns ErrTooManySubscribers if maxSubscribers concurrent subscriptions
// already exist on this buffer.
func (b *LogBuffer) Subscribe() (<-chan LogLine, func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.subscribeLocked()
}

func (b *LogBuffer) subscribeLocked() (<-chan LogLine, func(), error) {
	if len(b.subs) >= maxSubscribers {
		return nil, func() {}, ErrTooManySubscribers
	}
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
	return ch, cancel, nil
}

// SnapshotAndSubscribe atomically returns the most recent n lines and a
// subscription that delivers every line written after the snapshot point.
// Use this when you want gap-free history-then-tail; Snapshot() followed by
// Subscribe() has a small race window where lines written in between can be
// missed.
//
// Returns ErrTooManySubscribers if maxSubscribers concurrent subscriptions
// already exist. In that case the snapshot is still returned so a caller can
// choose to render history without a live tail; the cancel function is a
// no-op.
//
// n=0 means "no snapshot, live tail only" (matches the WS handler contract).
// n<0 is normalised to 0. n>capacity is clamped to capacity.
func (b *LogBuffer) SnapshotAndSubscribe(n int) ([]LogLine, <-chan LogLine, func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if n < 0 {
		n = 0
	}
	var size int
	if b.filled {
		size = b.capacity
	} else {
		size = b.head
	}
	if n > size {
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

	ch, cancel, err := b.subscribeLocked()
	return out, ch, cancel, err
}

// Writer returns an io.Writer that splits incoming bytes on newlines and
// feeds each line into the buffer. Use this to tee a process's stdout/stderr
// or a zerolog ConsoleWriter into the ring so the same admin /logs endpoint
// surfaces hydra's own diagnostics, not just inner-container output.
//
// Partial lines are accumulated and emitted only on newline, so multi-write
// log statements (which zerolog can produce when a level is split across two
// Write calls) don't get fragmented. The accumulator caps at 64KiB; a single
// line larger than that is force-flushed as one entry to avoid unbounded
// memory growth from a pathological producer.
func (b *LogBuffer) Writer() *LogBufferWriter {
	return &LogBufferWriter{buf: b}
}

// LogBufferWriter implements io.Writer; see LogBuffer.Writer.
type LogBufferWriter struct {
	buf *LogBuffer
	mu  sync.Mutex
	acc bytes.Buffer
}

const lineWriterFlushCap = 64 * 1024

// Write splits p on '\n' and forwards each completed line to the buffer.
// Always returns len(p), nil — this writer is fan-out only and never blocks
// or errors out the real stdout chain it sits alongside.
func (w *LogBufferWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, b := range p {
		if b == '\n' {
			line := w.acc.String()
			w.acc.Reset()
			// Strip a trailing \r for CRLF producers (Windows / some
			// docker log shapes); zerolog ConsoleWriter on Unix
			// produces LF-only so this is cheap.
			if n := len(line); n > 0 && line[n-1] == '\r' {
				line = line[:n-1]
			}
			if line != "" {
				w.buf.Write(line)
			}
			continue
		}
		w.acc.WriteByte(b)
		if w.acc.Len() >= lineWriterFlushCap {
			w.buf.Write(w.acc.String())
			w.acc.Reset()
		}
	}
	return len(p), nil
}
