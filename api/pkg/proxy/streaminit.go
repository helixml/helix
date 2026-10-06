package proxy

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"sync"

	"github.com/rs/zerolog/log"
)

// maxInitPrefix bounds how much of the client stream we will buffer looking for
// the init frame. A stream client sends init within its first few hundred bytes;
// anything that has sent 64KB of binary without a text frame is not one, and we
// stop rather than grow without limit.
const maxInitPrefix = 64 * 1024

// opcodeText is the only WebSocket opcode this parser acts on (RFC 6455 §5.2);
// binary, ping, pong and close frames are counted and skipped.
const opcodeText = 0x1

// StreamInitReplay captures the desktop stream's `init` frame from the client
// byte stream and re-sends it after each backend reconnect.
//
// desktop-bridge blocks on a mandatory init text frame with a 30s read deadline
// before it will start a streamer (api/pkg/desktop/ws_stream.go). The browser
// sends it once, on the original socket. ResilientProxy re-dials the backend
// transparently, so without replay every reconnect produces a backend socket
// that waits 30s, dies, and reconnects — while the client socket stays open and
// the user sees a live-but-frozen picture with no error anywhere.
//
// This parses only as much WebSocket framing as it takes to find one text frame
// in the client→server direction. ResilientProxy is otherwise a raw byte proxy
// and deliberately stays that way.
type StreamInitReplay struct {
	mu sync.Mutex
	// pending holds client bytes not yet resolved into complete frames.
	pending []byte
	// frame is the encoded replay, ready to write. Nil until init is seen.
	frame []byte
	// done stops all further parsing: one init per connection, and a later text
	// frame is not session state.
	done bool
}

func NewStreamInitReplay() *StreamInitReplay { return &StreamInitReplay{} }

// Observe feeds client bytes to the frame parser. It becomes a no-op once the
// init frame has been captured (or the prefix bound is exceeded).
func (r *StreamInitReplay) Observe(b []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done || len(b) == 0 {
		return
	}

	r.pending = append(r.pending, b...)
	for {
		payload, opcode, consumed, ok := parseClientFrame(r.pending)
		if !ok {
			break // incomplete frame; wait for more bytes
		}
		r.pending = r.pending[consumed:]
		if opcode == opcodeText {
			r.capture(payload)
			return
		}
		// Binary keepalives, ping/pong and close are skipped, exactly as
		// desktop-bridge skips them while waiting for init.
	}

	if len(r.pending) > maxInitPrefix {
		log.Warn().
			Int("buffered", len(r.pending)).
			Msg("No stream init frame within prefix bound; giving up on init replay")
		r.pending = nil
		r.done = true
	}
}

// capture stores the replay frame for an observed init payload. Caller holds mu.
func (r *StreamInitReplay) capture(payload []byte) {
	r.pending = nil
	r.done = true

	replayed, err := clearUserRetry(payload)
	if err != nil {
		// Not JSON we understand. Replaying it verbatim would risk re-asserting
		// user_retry, so decline to replay rather than guess.
		log.Warn().Err(err).Msg("Stream init frame is not usable JSON; init replay disabled")
		return
	}

	frame, err := encodeClientTextFrame(replayed)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to encode stream init replay frame")
		return
	}
	r.frame = frame
}

// Frames returns the encoded init frame, or nil if none was captured — a
// reconnect that happens before the client sent init replays nothing rather
// than sending garbage into the backend's init read.
func (r *StreamInitReplay) Frames() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frame == nil {
		return nil
	}
	out := make([]byte, len(r.frame))
	copy(out, r.frame)
	return out
}

// clearUserRetry removes user_retry from an init payload.
//
// user_retry marks an explicit user-initiated retry (the Restart button) and is
// the only thing that clears a latched shared-video circuit breaker
// (api/pkg/desktop/ws_stream.go). A proxy reconnect is by definition automatic:
// if replay preserved the flag, one press of Restart would re-assert it on every
// subsequent backend drop, the breaker would reset each time, and it could never
// latch — which is the retry storm the breaker exists to stop. The Restart button
// is unaffected; it opens a NEW client socket whose init is not a replay.
//
// Decoded into a map rather than desktop.StreamConfig on purpose: round-tripping
// through the struct would drop fields a newer browser sends that this build does
// not know about, and re-emit omitempty fields the client deliberately left out.
// Everything except user_retry passes through unchanged.
func clearUserRetry(payload []byte) ([]byte, error) {
	var config map[string]any
	if err := json.Unmarshal(payload, &config); err != nil {
		return nil, err
	}
	delete(config, "user_retry")
	return json.Marshal(config)
}

// parseClientFrame decodes one WebSocket frame from a client (masked) stream.
//
// Returns the unmasked payload, the opcode, how many bytes the frame occupied,
// and whether a complete frame was available. A continuation opcode reports the
// frame's own opcode; init is sent unfragmented by every stream client, so a
// fragmented text frame simply is not recognised as init.
func parseClientFrame(b []byte) (payload []byte, opcode byte, consumed int, ok bool) {
	if len(b) < 2 {
		return nil, 0, 0, false
	}
	opcode = b[0] & 0x0f
	masked := b[1]&0x80 != 0
	length := uint64(b[1] & 0x7f)
	offset := 2

	switch length {
	case 126:
		if len(b) < offset+2 {
			return nil, 0, 0, false
		}
		length = uint64(binary.BigEndian.Uint16(b[offset : offset+2]))
		offset += 2
	case 127:
		if len(b) < offset+8 {
			return nil, 0, 0, false
		}
		length = binary.BigEndian.Uint64(b[offset : offset+8])
		offset += 8
	}

	var maskKey []byte
	if masked {
		if len(b) < offset+4 {
			return nil, 0, 0, false
		}
		maskKey = b[offset : offset+4]
		offset += 4
	}

	// A 64-bit length from a hostile or corrupt stream must not be used to size
	// an allocation; we only ever slice what has actually arrived.
	if length > maxInitPrefix || uint64(len(b)) < uint64(offset)+length {
		return nil, 0, 0, false
	}

	payload = make([]byte, length)
	copy(payload, b[offset:offset+int(length)])
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}
	return payload, opcode, offset + int(length), true
}

// encodeClientTextFrame builds a masked, unfragmented text frame. Client→server
// frames must be masked (RFC 6455 §5.3), and the payload changed when user_retry
// was stripped, so this re-encodes rather than replaying the original bytes.
func encodeClientTextFrame(payload []byte) ([]byte, error) {
	var maskKey [4]byte
	if _, err := rand.Read(maskKey[:]); err != nil {
		return nil, err
	}

	header := []byte{0x80 | opcodeText} // FIN + text
	length := len(payload)
	switch {
	case length < 126:
		header = append(header, 0x80|byte(length))
	case length <= 0xffff:
		header = append(header, 0x80|126, byte(length>>8), byte(length))
	default:
		header = append(header, 0x80|127)
		var extended [8]byte
		binary.BigEndian.PutUint64(extended[:], uint64(length))
		header = append(header, extended[:]...)
	}
	header = append(header, maskKey[:]...)

	frame := make([]byte, 0, len(header)+length)
	frame = append(frame, header...)
	for i, c := range payload {
		frame = append(frame, c^maskKey[i%4])
	}
	return frame, nil
}
