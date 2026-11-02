package proxy

import (
	"encoding/binary"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// clientBinaryFrameLong builds a masked client binary frame with a 64-bit
// length, for payloads too big for clientBinaryFrame.
func clientBinaryFrameLong(payload []byte) []byte {
	mask := []byte{0x11, 0x22, 0x33, 0x44}
	frame := []byte{0x82, 0x80 | 127}
	frame = binary.BigEndian.AppendUint64(frame, uint64(len(payload)))
	frame = append(frame, mask...)
	for i, c := range payload {
		frame = append(frame, c^mask[i%4])
	}
	return frame
}

type inputCounter struct{ n atomic.Int32 }

func (c *inputCounter) observer(minGap time.Duration, clock *time.Time) *InputActivity {
	a := NewInputActivity(minGap, func() { c.n.Add(1) })
	a.now = func() time.Time { return *clock }
	return a
}

func (c *inputCounter) waitFor(t *testing.T, want int32) {
	t.Helper()
	require.Eventually(t, func() bool { return c.n.Load() == want }, time.Second, 5*time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	require.Equal(t, want, c.n.Load())
}

var (
	pingMsg     = []byte{0x40, 0, 0, 0, 0, 0, 0, 0, 1}
	keyboardMsg = []byte{0x10, 0, 1, 0, 0x41}
	mouseMsg    = []byte{0x12, 0, 10, 0, 20, 0, 0, 0, 0}
)

func TestInputActivityCountsInputNotPings(t *testing.T) {
	clock := time.Unix(1000, 0)
	var c inputCounter
	a := c.observer(time.Minute, &clock)

	a.Observe(clientTextFrame(t, `{"type":"init"}`))
	for i := 0; i < 20; i++ {
		a.Observe(clientBinaryFrame(t, pingMsg))
	}
	c.waitFor(t, 0)

	a.Observe(clientBinaryFrame(t, keyboardMsg))
	c.waitFor(t, 1)
}

func TestInputActivityParsesFramesSplitAnywhere(t *testing.T) {
	clock := time.Unix(1000, 0)
	var c inputCounter
	a := c.observer(time.Minute, &clock)

	stream := append(clientBinaryFrame(t, pingMsg), clientBinaryFrame(t, mouseMsg)...)
	for _, b := range stream {
		a.Observe([]byte{b})
	}
	c.waitFor(t, 1)
}

func TestInputActivitySkipsLargeFramesWithoutLosingSync(t *testing.T) {
	clock := time.Unix(1000, 0)
	var c inputCounter
	a := c.observer(time.Minute, &clock)

	// A 100KB non-input binary frame (first byte 0x20, a control message),
	// delivered in uneven chunks, then a keyboard frame.
	big := make([]byte, 100*1024)
	big[0] = 0x20
	for i := range big[1:] {
		big[i+1] = 0x10 // input-looking bytes inside the payload must not count
	}
	stream := append(clientBinaryFrameLong(big), clientBinaryFrame(t, keyboardMsg)...)
	for len(stream) > 0 {
		n := min(len(stream), 7919)
		a.Observe(stream[:n])
		stream = stream[n:]
	}
	c.waitFor(t, 1)
	require.Empty(t, a.hdr, "large payload must not be buffered")
}

func TestInputActivityRateLimited(t *testing.T) {
	clock := time.Unix(1000, 0)
	var c inputCounter
	a := c.observer(time.Minute, &clock)

	for i := 0; i < 50; i++ {
		a.Observe(clientBinaryFrame(t, mouseMsg))
	}
	c.waitFor(t, 1)

	clock = clock.Add(59 * time.Second)
	a.Observe(clientBinaryFrame(t, mouseMsg))
	c.waitFor(t, 1)

	clock = clock.Add(2 * time.Second)
	a.Observe(clientBinaryFrame(t, keyboardMsg))
	c.waitFor(t, 2)
}

type recordingObserver struct{ got chan []byte }

func (r *recordingObserver) Observe(b []byte) {
	r.got <- append([]byte(nil), b...)
}

// The proxy hands the client's bytes to Activity as they pass through.
func TestResilientProxyFeedsActivityObserver(t *testing.T) {
	server := newTestServer(t, true)
	defer server.close()

	clientSide, proxySide := net.Pipe()
	defer clientSide.Close()
	defer proxySide.Close()
	serverConn, err := net.Dial("tcp", server.addr)
	require.NoError(t, err)
	_, err = server.waitForConn(time.Second)
	require.NoError(t, err)

	obs := &recordingObserver{got: make(chan []byte, 16)}
	p := NewResilientProxy(ResilientProxyConfig{
		SessionID:  "act",
		ClientConn: proxySide,
		ServerConn: serverConn,
		Activity:   obs,
	})
	go p.Run(t.Context())
	defer p.Close()

	frame := clientBinaryFrame(t, keyboardMsg)
	_, err = clientSide.Write(frame)
	require.NoError(t, err)

	select {
	case got := <-obs.got:
		require.Equal(t, frame, got)
	case <-time.After(2 * time.Second):
		t.Fatal("activity observer never saw the client's bytes")
	}
}
