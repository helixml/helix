package server

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReadRevdialResponsePreservesUpgradeConnection(t *testing.T) {
	client, upstream := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = upstream.Close()
	})

	request, err := http.NewRequest(http.MethodGet, "http://hydra/preview", nil)
	require.NoError(t, err)

	upstreamRead := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(upstream)
		_, _ = http.ReadRequest(reader)
		_, _ = io.WriteString(upstream, "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\nserver-frame")
		buf := make([]byte, len("client-frame"))
		_, _ = io.ReadFull(reader, buf)
		upstreamRead <- string(buf)
	}()

	require.NoError(t, request.Write(client))
	response, err := readRevdialResponse(client, request)
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, response.StatusCode)

	stream, ok := response.Body.(io.ReadWriteCloser)
	require.True(t, ok, "101 response body must be bidirectional for ReverseProxy")

	serverFrame := make([]byte, len("server-frame"))
	_, err = io.ReadFull(stream, serverFrame)
	require.NoError(t, err)
	require.Equal(t, "server-frame", string(serverFrame))

	_, err = stream.Write([]byte("client-frame"))
	require.NoError(t, err)
	select {
	case got := <-upstreamRead:
		require.Equal(t, "client-frame", got)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for upgraded client frame")
	}
	require.NoError(t, stream.Close())
}
