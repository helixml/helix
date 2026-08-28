package proxy

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// clientTextFrame builds a masked client text frame, as a browser would send.
func clientTextFrame(t *testing.T, payload string) []byte {
	t.Helper()
	frame, err := encodeClientTextFrame([]byte(payload))
	require.NoError(t, err)
	return frame
}

// clientBinaryFrame builds a masked client binary frame — the 13-byte keepalives
// the stream client sends before and during init.
func clientBinaryFrame(t *testing.T, payload []byte) []byte {
	t.Helper()
	require.Less(t, len(payload), 126, "helper only builds short frames")
	mask := []byte{0xa1, 0xb2, 0xc3, 0xd4}
	frame := []byte{0x82, 0x80 | byte(len(payload))}
	frame = append(frame, mask...)
	for i, c := range payload {
		frame = append(frame, c^mask[i%4])
	}
	return frame
}

// decodeReplay parses a frame produced by StreamInitReplay back into its JSON.
func decodeReplay(t *testing.T, frame []byte) map[string]any {
	t.Helper()
	payload, opcode, consumed, ok := parseClientFrame(frame)
	require.True(t, ok, "replay frame must be complete")
	require.Equal(t, byte(opcodeText), opcode, "replay must be a text frame")
	require.Equal(t, len(frame), consumed, "replay must be exactly one frame")
	require.True(t, frame[1]&0x80 != 0, "client frames must be masked (RFC 6455 §5.3)")

	var out map[string]any
	require.NoError(t, json.Unmarshal(payload, &out))
	return out
}

func TestStreamInitReplayCapturesInit(t *testing.T) {
	replay := NewStreamInitReplay()
	require.Nil(t, replay.Frames(), "nothing to replay before init is seen")

	replay.Observe(clientTextFrame(t, `{"type":"init","width":1920,"height":1080,"fps":60}`))

	config := decodeReplay(t, replay.Frames())
	require.Equal(t, "init", config["type"])
	require.EqualValues(t, 1920, config["width"])
	require.EqualValues(t, 1080, config["height"])
	require.EqualValues(t, 60, config["fps"])
}

// The headline requirement: a reconnect must re-send init, or desktop-bridge
// sits in its 30s read and the browser shows a frozen picture with no error.
func TestReconnectReplaysInit(t *testing.T) {
	clientLocal, clientRemote := net.Pipe()
	defer clientLocal.Close()
	defer clientRemote.Close()

	serverInitial, _ := net.Pipe()

	// Each dial hands back one end of a fresh pipe and records the other, so the
	// test can read exactly what the proxy wrote to the reconnected backend.
	var mu sync.Mutex
	var reconnected net.Conn
	dialed := make(chan struct{}, 1)
	dialFunc := func(_ context.Context) (net.Conn, error) {
		ours, theirs := net.Pipe()
		mu.Lock()
		reconnected = theirs
		mu.Unlock()
		dialed <- struct{}{}
		return ours, nil
	}

	replay := NewStreamInitReplay()
	p := NewResilientProxy(ResilientProxyConfig{
		SessionID:  "test",
		ClientConn: clientLocal,
		ServerConn: serverInitial,
		DialFunc:   dialFunc,
		Replay:     replay,
	})

	// The client sent init on the original socket, exactly once.
	replay.Observe(clientTextFrame(t,
		`{"type":"init","width":1920,"height":1080,"session_id":"ses_1"}`))

	// Kill the backend and let the proxy re-dial.
	serverInitial.Close()
	go func() { _ = p.reconnect(context.Background()) }()

	select {
	case <-dialed:
	case <-time.After(5 * time.Second):
		t.Fatal("proxy never re-dialled the backend")
	}

	mu.Lock()
	backend := reconnected
	mu.Unlock()
	require.NotNil(t, backend)
	defer backend.Close()

	require.NoError(t, backend.SetReadDeadline(time.Now().Add(5*time.Second)))
	buf := make([]byte, 512)
	n, err := backend.Read(buf)
	require.NoError(t, err, "reconnected backend must receive the replayed init")

	config := decodeReplay(t, buf[:n])
	require.Equal(t, "init", config["type"],
		"the first thing a reconnected backend sees must be init")
	require.Equal(t, "ses_1", config["session_id"],
		"replay must subscribe to the same shared source, not a new one")
	require.EqualValues(t, 1920, config["width"])
}

// A replayed init must NOT count as a user-initiated retry. user_retry is the
// only thing that clears a latched shared-video circuit breaker; if replay
// preserved it, one press of Restart would re-assert it on every later backend
// drop and the breaker could never latch.
func TestReplayedInitHasUserRetryCleared(t *testing.T) {
	replay := NewStreamInitReplay()
	replay.Observe(clientTextFrame(t,
		`{"type":"init","width":1280,"user_retry":true,"user_name":"alice"}`))

	config := decodeReplay(t, replay.Frames())
	require.NotContains(t, config, "user_retry",
		"a proxy reconnect is automatic and must not reset the circuit breaker")

	// Everything else survives untouched.
	require.Equal(t, "init", config["type"])
	require.EqualValues(t, 1280, config["width"])
	require.Equal(t, "alice", config["user_name"])
}

// An unknown field from a newer browser must survive replay: the payload is
// decoded as a map precisely so this build does not silently drop it.
func TestReplayPreservesUnknownFields(t *testing.T) {
	replay := NewStreamInitReplay()
	replay.Observe(clientTextFrame(t, `{"type":"init","future_knob":"keep me"}`))

	config := decodeReplay(t, replay.Frames())
	require.Equal(t, "keep me", config["future_knob"])
}

func TestStreamInitReplaySkipsBinaryKeepalives(t *testing.T) {
	replay := NewStreamInitReplay()

	// The client sends 13-byte binary keepalives before init, which
	// desktop-bridge also skips while waiting.
	replay.Observe(clientBinaryFrame(t, make([]byte, 13)))
	replay.Observe(clientBinaryFrame(t, make([]byte, 13)))
	require.Nil(t, replay.Frames(), "binary frames are not session state")

	replay.Observe(clientTextFrame(t, `{"type":"init","width":800}`))
	require.EqualValues(t, 800, decodeReplay(t, replay.Frames())["width"])
}

// One init per connection: a later text frame is not session state and must not
// overwrite what the backend needs to be re-told.
func TestStreamInitReplayKeepsFirstTextFrame(t *testing.T) {
	replay := NewStreamInitReplay()
	replay.Observe(clientTextFrame(t, `{"type":"init","width":1920}`))
	replay.Observe(clientTextFrame(t, `{"type":"init","width":640}`))

	require.EqualValues(t, 1920, decodeReplay(t, replay.Frames())["width"])
}

// Frames arrive split across TCP reads; the parser is fed 32KB chunks and must
// not assume a read boundary lines up with a frame boundary.
func TestStreamInitReplayHandlesSplitFrames(t *testing.T) {
	frame := clientTextFrame(t, `{"type":"init","width":1920,"height":1080}`)

	replay := NewStreamInitReplay()
	for i := 0; i < len(frame); i++ {
		replay.Observe(frame[i : i+1])
	}

	require.EqualValues(t, 1920, decodeReplay(t, replay.Frames())["width"])
}

func TestParseClientFrameLengthForms(t *testing.T) {
	// 7-bit length.
	short := clientTextFrame(t, `{"type":"init"}`)
	payload, opcode, consumed, ok := parseClientFrame(short)
	require.True(t, ok)
	require.Equal(t, byte(opcodeText), opcode)
	require.Equal(t, len(short), consumed)
	require.JSONEq(t, `{"type":"init"}`, string(payload))

	// 16-bit extended length.
	medium := make([]byte, 200)
	for i := range medium {
		medium[i] = 'a'
	}
	mediumFrame, err := encodeClientTextFrame(medium)
	require.NoError(t, err)
	require.Equal(t, byte(0x80|126), mediumFrame[1], "200 bytes must use the 16-bit form")
	payload, _, consumed, ok = parseClientFrame(mediumFrame)
	require.True(t, ok)
	require.Equal(t, len(mediumFrame), consumed)
	require.Equal(t, medium, payload)

	// 64-bit extended length, hand-built so the parser is tested rather than a
	// round-trip against our own encoder.
	body := []byte(`{"type":"init"}`)
	huge := []byte{0x81, 0x80 | 127}
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(body)))
	huge = append(huge, length[:]...)
	mask := []byte{1, 2, 3, 4}
	huge = append(huge, mask...)
	for i, c := range body {
		huge = append(huge, c^mask[i%4])
	}
	payload, _, consumed, ok = parseClientFrame(huge)
	require.True(t, ok)
	require.Equal(t, len(huge), consumed)
	require.JSONEq(t, `{"type":"init"}`, string(payload))
}

func TestParseClientFrameIncomplete(t *testing.T) {
	frame := clientTextFrame(t, `{"type":"init","width":1920}`)
	for i := 0; i < len(frame); i++ {
		_, _, _, ok := parseClientFrame(frame[:i])
		require.False(t, ok, "a partial frame at %d bytes must not parse", i)
	}
	_, _, _, ok := parseClientFrame(frame)
	require.True(t, ok)
}

// Malformed JSON must disable replay rather than be re-sent verbatim: replaying
// bytes we cannot read risks re-asserting user_retry.
func TestStreamInitReplayDeclinesNonJSON(t *testing.T) {
	replay := NewStreamInitReplay()
	replay.Observe(clientTextFrame(t, `not json at all`))
	require.Nil(t, replay.Frames())
}

// A stream that never yields a parseable frame must not grow the buffer without
// limit. A 64-bit length beyond the prefix bound is the corrupt/hostile case:
// the parser refuses to size an allocation from it, so nothing ever completes.
func TestStreamInitReplayBoundsPrefix(t *testing.T) {
	replay := NewStreamInitReplay()

	header := []byte{0x82, 0x80 | 127}
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], 1<<40) // far beyond maxInitPrefix
	header = append(header, length[:]...)
	header = append(header, 1, 2, 3, 4) // mask key
	replay.Observe(header)

	for i := 0; i < 80; i++ {
		replay.Observe(make([]byte, 1024))
	}

	replay.mu.Lock()
	pending := len(replay.pending)
	done := replay.done
	replay.mu.Unlock()

	require.True(t, done, "parser must give up rather than buffer forever")
	require.Zero(t, pending)
	require.Nil(t, replay.Frames())
}

// Replay and the read-only header are the two halves of the same requirement and
// must compose: the header re-grants nothing, and the init payload has no
// privilege field to carry.
func TestReplayCarriesNoPrivilegeField(t *testing.T) {
	replay := NewStreamInitReplay()
	replay.Observe(clientTextFrame(t,
		`{"type":"init","width":1920,"user_name":"alice","user_id":"u1"}`))

	config := decodeReplay(t, replay.Frames())
	for _, forbidden := range []string{"readonly", "read_only", "X-Helix-Readonly"} {
		require.NotContains(t, config, forbidden,
			"privilege lives in the upgrade header, never in the init payload")
	}
}

// Nil Replay must leave the proxy behaving exactly as it did before.
func TestResilientProxyWithoutReplayIsUnchanged(t *testing.T) {
	p := NewResilientProxy(ResilientProxyConfig{SessionID: "test"})
	require.Nil(t, p.replay)
}

// The other half of the read-only guarantee, and the invariant SessionReplay is
// modelled on: X-Helix-Readonly rides on the upgrade, and the upgrade func runs
// again on every reconnect. Replay cannot re-grant input because privilege never
// travels in the init payload — it travels here, and it is re-sent every time.
//
// Untested before this change despite the doc comment asserting it.
func TestUpgradeFuncReemitsExtraHeadersOnEveryUpgrade(t *testing.T) {
	upgrade := CreateWebSocketUpgradeFunc("/ws/stream", "dGhlIHNhbXBsZSBub25jZQ==", "X-Helix-Readonly", "1")

	// Two independent upgrades stand in for the original connection and the
	// reconnect: the func holds no state, so both must carry the header.
	for attempt := 1; attempt <= 2; attempt++ {
		ours, theirs := net.Pipe()

		got := make(chan string, 1)
		go func() {
			defer theirs.Close()
			buf := make([]byte, 1024)
			n, err := theirs.Read(buf)
			if err != nil {
				got <- ""
				return
			}
			got <- string(buf[:n])
			// Minimal 101 so the upgrade func returns rather than blocking.
			_, _ = theirs.Write([]byte("HTTP/1.1 101 Switching Protocols\r\n" +
				"Upgrade: websocket\r\nConnection: Upgrade\r\n\r\n"))
		}()

		err := upgrade(ours)
		ours.Close()
		require.NoError(t, err, "upgrade %d", attempt)

		request := <-got
		require.Contains(t, request, "X-Helix-Readonly: 1",
			"upgrade %d dropped the read-only header; a reconnect would silently re-grant input", attempt)
		require.Contains(t, request, "GET /ws/stream HTTP/1.1")
	}
}
