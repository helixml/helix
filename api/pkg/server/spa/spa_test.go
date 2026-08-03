package spa

import (
	"bufio"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type hijackableResponseWriter struct {
	header http.Header
	conn   net.Conn
	rw     *bufio.ReadWriter
}

func (w *hijackableResponseWriter) Header() http.Header {
	return w.header
}

func (w *hijackableResponseWriter) Write(body []byte) (int, error) {
	return len(body), nil
}

func (w *hijackableResponseWriter) WriteHeader(_ int) {}

func (w *hijackableResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.conn, w.rw, nil
}

func TestStatusRecorderPreservesHijacker(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() {
		require.NoError(t, serverConn.Close())
		require.NoError(t, clientConn.Close())
	})

	buffer := bufio.NewReadWriter(bufio.NewReader(serverConn), bufio.NewWriter(serverConn))
	underlying := &hijackableResponseWriter{
		header: make(http.Header),
		conn:   serverConn,
		rw:     buffer,
	}
	recorder := &statusRecorder{ResponseWriter: underlying, status: http.StatusOK}

	conn, rw, err := recorder.Hijack()
	require.NoError(t, err)
	require.Same(t, serverConn, conn)
	require.Same(t, buffer, rw)
}
