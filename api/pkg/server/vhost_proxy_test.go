package server

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInjectSandboxBrowserBridge(t *testing.T) {
	original := "<!doctype html><html><head><title>App</title></head><body>hello</body></html>"
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":            []string{"text/html; charset=utf-8"},
			"Content-Length":          []string{strconv.Itoa(len(original))},
			"Content-Security-Policy": []string{"default-src 'self'; script-src 'self'"},
			"ETag":                    []string{`"old"`},
		},
		Body:          io.NopCloser(strings.NewReader(original)),
		ContentLength: int64(len(original)),
	}

	require.NoError(t, injectSandboxBrowserBridge(response))
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "<head>"+sandboxBrowserBridgeScript+"<title>")
	require.Equal(t, int64(len(body)), response.ContentLength)
	require.Equal(t, strconv.Itoa(len(body)), response.Header.Get("Content-Length"))
	require.Empty(t, response.Header.Get("ETag"))
	require.Contains(t, response.Header.Get("Content-Security-Policy"), "script-src 'self' 'sha256-")
}

func TestInjectSandboxBrowserBridgeSkipsCompressedHTML(t *testing.T) {
	original := "<html><head></head><body>compressed</body></html>"
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":     []string{"text/html"},
			"Content-Encoding": []string{"gzip"},
		},
		Body: io.NopCloser(strings.NewReader(original)),
	}

	require.NoError(t, injectSandboxBrowserBridge(response))
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, original, string(body))
}

func TestInjectSandboxBrowserBridgePreservesDoctypeWithoutHead(t *testing.T) {
	original := "<!doctype html><html><body>hello</body></html>"
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":     []string{"text/html"},
			"Content-Encoding": []string{"identity"},
		},
		Body: io.NopCloser(strings.NewReader(original)),
	}

	require.NoError(t, injectSandboxBrowserBridge(response))
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(body), "<!doctype html><html>"+sandboxBrowserBridgeScript))
}

func TestInjectSandboxBrowserBridgeLeavesOversizedHTMLIntact(t *testing.T) {
	original := bytes.Repeat([]byte("a"), maxSandboxBrowserHTMLSize+1)
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/html"},
		},
		Body: io.NopCloser(bytes.NewReader(original)),
	}

	require.NoError(t, injectSandboxBrowserBridge(response))
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, original, body)
}

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
