package proxy

import (
	"encoding/binary"
	"sync"
	"time"
)

// Desktop stream input message types (first payload byte of a client binary
// frame): keyboard, mouse click/absolute/relative, touch, controller event and
// state. Pings, control messages and the init frame are not input.
const (
	opcodeBinary = 0x2

	firstInputMsgType = 0x10
	lastInputMsgType  = 0x16
)

// InputActivity watches the client side of a desktop stream and calls onInput,
// at most once per minGap, when the viewer sends keyboard, mouse or touch
// input.
//
// The desktop idle checker only sees chat interactions, so someone working in
// a desktop directly had it stopped after an hour without a chat message.
// Only real input counts: the player pings every 500ms, and a tab left open
// should not keep a desktop alive.
//
// Client frames are parsed as a stream: only each frame's header and first
// payload byte are kept, so large frames are skipped without being buffered.
type InputActivity struct {
	mu      sync.Mutex
	onInput func()
	minGap  time.Duration
	now     func() time.Time
	last    time.Time

	hdr       []byte
	opcode    byte
	masked    bool
	mask      [4]byte
	remaining uint64
	atFirst   bool
}

// NewInputActivity returns an observer that runs onInput in its own goroutine
// so the proxy's copy loop never waits on it.
func NewInputActivity(minGap time.Duration, onInput func()) *InputActivity {
	return &InputActivity{onInput: onInput, minGap: minGap, now: time.Now}
}

// Observe consumes bytes the client sent, in order, split at any boundary.
func (a *InputActivity) Observe(b []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for len(b) > 0 {
		if a.remaining == 0 {
			b = a.readHeader(b)
			continue
		}
		if a.atFirst {
			a.atFirst = false
			msgType := b[0]
			if a.masked {
				msgType ^= a.mask[0]
			}
			if a.opcode == opcodeBinary && msgType >= firstInputMsgType && msgType <= lastInputMsgType {
				a.record()
			}
		}
		n := uint64(len(b))
		if n > a.remaining {
			n = a.remaining
		}
		a.remaining -= n
		b = b[n:]
	}
}

// readHeader accumulates one frame header and returns the unconsumed bytes.
func (a *InputActivity) readHeader(b []byte) []byte {
	for len(b) > 0 {
		a.hdr = append(a.hdr, b[0])
		b = b[1:]
		if len(a.hdr) < 2 {
			continue
		}
		need := 2
		switch a.hdr[1] & 0x7f {
		case 126:
			need += 2
		case 127:
			need += 8
		}
		masked := a.hdr[1]&0x80 != 0
		if masked {
			need += 4
		}
		if len(a.hdr) < need {
			continue
		}

		a.opcode = a.hdr[0] & 0x0f
		a.masked = masked
		length := uint64(a.hdr[1] & 0x7f)
		off := 2
		switch length {
		case 126:
			length = uint64(binary.BigEndian.Uint16(a.hdr[2:4]))
			off = 4
		case 127:
			length = binary.BigEndian.Uint64(a.hdr[2:10])
			off = 10
		}
		if masked {
			copy(a.mask[:], a.hdr[off:off+4])
		}
		a.hdr = a.hdr[:0]
		a.remaining = length
		a.atFirst = length > 0
		return b
	}
	return b
}

func (a *InputActivity) record() {
	now := a.now()
	if !a.last.IsZero() && now.Sub(a.last) < a.minGap {
		return
	}
	a.last = now
	go a.onInput()
}
