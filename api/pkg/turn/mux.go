package turn

import (
	"bufio"
	"io"
	"net"
	"sync"

	"github.com/rs/zerolog/log"
)

// MuxListener multiplexes STUN/TURN and HTTP traffic on the same TCP port.
// It peeks the first byte of each connection to determine the protocol:
// - Bytes 0-3: STUN/TURN protocol (RFC 7983)
// - Other bytes (especially ASCII 'G', 'P', 'H', etc.): HTTP
type MuxListener struct {
	base     net.Listener
	turnCh   chan net.Conn
	httpCh   chan net.Conn
	closed   bool
	closeMu  sync.Mutex
	closeErr error
	wg       sync.WaitGroup
}

// NewMuxListener creates a multiplexing listener that routes connections
// to either TURN or HTTP based on the first byte of the connection.
func NewMuxListener(base net.Listener) *MuxListener {
	m := &MuxListener{
		base:   base,
		turnCh: make(chan net.Conn, 16),
		httpCh: make(chan net.Conn, 16),
	}
	m.wg.Add(1)
	go m.acceptLoop()
	return m
}

func (m *MuxListener) acceptLoop() {
	defer m.wg.Done()
	for {
		conn, err := m.base.Accept()
		if err != nil {
			m.closeMu.Lock()
			if !m.closed {
				log.Error().Err(err).Msg("[MuxListener] Accept error")
			}
			m.closeMu.Unlock()
			return
		}

		// Spawn goroutine to peek and route each connection
		go m.routeConnection(conn)
	}
}

func (m *MuxListener) routeConnection(conn net.Conn) {
	// Wrap connection in buffered reader to peek without consuming
	br := bufio.NewReader(conn)

	// Peek first byte to determine protocol
	firstByte, err := br.Peek(1)
	if err != nil {
		log.Debug().Err(err).Str("remote", conn.RemoteAddr().String()).Msg("[MuxListener] Failed to peek first byte")
		conn.Close()
		return
	}

	// Create a connection that replays the peeked data
	wrappedConn := &prefixConn{
		Conn:   conn,
		reader: br,
	}

	// Route based on first byte per RFC 7983:
	// - 0-3: STUN/TURN
	// - 16-19: ZRTP (not used)
	// - 20-63: DTLS
	// - 64-79: TURN channel data
	// - 128-191: RTP/RTCP
	// Everything else is likely HTTP (ASCII letters start at 65+)
	b := firstByte[0]
	if b <= 3 || (b >= 64 && b <= 79) {
		// STUN/TURN message or TURN ChannelData
		log.Debug().Uint8("first_byte", b).Str("remote", conn.RemoteAddr().String()).Msg("[MuxListener] Routing to TURN")
		m.closeMu.Lock()
		closed := m.closed
		m.closeMu.Unlock()
		if closed {
			wrappedConn.Close()
			return
		}
		m.turnCh <- wrappedConn
	} else {
		// HTTP or other protocol
		log.Debug().Uint8("first_byte", b).Str("remote", conn.RemoteAddr().String()).Msg("[MuxListener] Routing to HTTP")
		m.closeMu.Lock()
		closed := m.closed
		m.closeMu.Unlock()
		if closed {
			wrappedConn.Close()
			return
		}
		m.httpCh <- wrappedConn
	}
}

// TURNListener returns a net.Listener that yields TURN connections only.
func (m *MuxListener) TURNListener() net.Listener {
	return &chanListener{
		ch:   m.turnCh,
		addr: m.base.Addr(),
		mux:  m,
	}
}

// HTTPListener returns a net.Listener that yields HTTP connections only.
func (m *MuxListener) HTTPListener() net.Listener {
	return &chanListener{
		ch:   m.httpCh,
		addr: m.base.Addr(),
		mux:  m,
	}
}

// Close closes the underlying listener and both channels.
func (m *MuxListener) Close() error {
	m.closeMu.Lock()
	if m.closed {
		m.closeMu.Unlock()
		return m.closeErr
	}
	m.closed = true
	m.closeMu.Unlock()

	m.closeErr = m.base.Close()
	m.wg.Wait()

	// Drain and close channels
	close(m.turnCh)
	close(m.httpCh)

	// Close any remaining connections in channels
	for conn := range m.turnCh {
		conn.Close()
	}
	for conn := range m.httpCh {
		conn.Close()
	}

	return m.closeErr
}

// prefixConn wraps a net.Conn with a buffered reader that replays peeked bytes.
type prefixConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *prefixConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

// chanListener implements net.Listener using a channel of connections.
type chanListener struct {
	ch   chan net.Conn
	addr net.Addr
	mux  *MuxListener
}

func (l *chanListener) Accept() (net.Conn, error) {
	conn, ok := <-l.ch
	if !ok {
		return nil, io.EOF
	}
	return conn, nil
}

func (l *chanListener) Close() error {
	// Don't close the channel here; the MuxListener owns it
	return nil
}

func (l *chanListener) Addr() net.Addr {
	return l.addr
}
