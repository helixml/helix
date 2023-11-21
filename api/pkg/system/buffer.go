package system

import (
	"sync"
)

// LimitedBuffer is a thread-safe buffer that stores only the most recent data up to a specified limit.
type LimitedBuffer struct {
	buf   []byte
	limit int
	mu    sync.Mutex
}

// NewLimitedBuffer creates a new LimitedBuffer with the given limit.
func NewLimitedBuffer(limit int) *LimitedBuffer {
	return &LimitedBuffer{
		buf:   make([]byte, 0, limit),
		limit: limit,
	}
}

// Write implements the io.Writer interface.
func (b *LimitedBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	lenP := len(p)
	newLen := len(b.buf) + lenP

	// If new length exceeds limit, discard the earliest bytes.
	if newLen > b.limit {
		discard := newLen - b.limit
		b.buf = append(b.buf[discard:], p...)
	} else {
		b.buf = append(b.buf, p...)
	}
	return lenP, nil
}

// Bytes returns a copy of the buffer's contents.
func (b *LimitedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf...)
}
