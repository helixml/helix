package hydra

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

type probeConnmanFunc func(context.Context, string) (net.Conn, error)

func (f probeConnmanFunc) Dial(ctx context.Context, deviceID string) (net.Conn, error) {
	return f(ctx, deviceID)
}

func TestProbeDevContainerPortAcceptsHTTPErrorResponse(t *testing.T) {
	serverErr := make(chan error, 1)
	client := NewRevDialClient(probeConnmanFunc(func(context.Context, string) (net.Conn, error) {
		clientConn, serverConn := net.Pipe()
		go func() {
			defer serverConn.Close()
			req, err := http.ReadRequest(bufio.NewReader(serverConn))
			if err != nil {
				serverErr <- err
				return
			}
			if req.Method != http.MethodHead || req.URL.Path != "/api/v1/dev-containers/sbx_1/proxy/8080/" {
				serverErr <- fmt.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
				return
			}
			_, err = io.WriteString(serverConn, "HTTP/1.1 404 Not Found\r\n"+devContainerUpstreamReachedHeader+": 1\r\nContent-Length: 0\r\n\r\n")
			serverErr <- err
		}()
		return clientConn, nil
	}), "device_1")

	if err := client.ProbeDevContainerPort(context.Background(), "sbx_1", 8080); err != nil {
		t.Fatalf("expected HTTP 404 to count as reachable, got %v", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}

func TestProbeDevContainerPortAcceptsLegacySuccessResponse(t *testing.T) {
	serverErr := make(chan error, 1)
	client := NewRevDialClient(probeConnmanFunc(func(context.Context, string) (net.Conn, error) {
		clientConn, serverConn := net.Pipe()
		go func() {
			defer serverConn.Close()
			if _, err := http.ReadRequest(bufio.NewReader(serverConn)); err != nil {
				serverErr <- err
				return
			}
			_, err := io.WriteString(serverConn, "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
			serverErr <- err
		}()
		return clientConn, nil
	}), "device_1")

	if err := client.ProbeDevContainerPort(context.Background(), "sbx_1", 8080); err != nil {
		t.Fatalf("expected legacy unmarked HTTP 200 to count as reachable, got %v", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}

func TestProbeDevContainerPortRejectsHydraErrorResponse(t *testing.T) {
	for _, status := range []string{"404 Not Found", "502 Bad Gateway"} {
		t.Run(status, func(t *testing.T) {
			serverErr := make(chan error, 1)
			client := NewRevDialClient(probeConnmanFunc(func(context.Context, string) (net.Conn, error) {
				clientConn, serverConn := net.Pipe()
				go func() {
					defer serverConn.Close()
					if _, err := http.ReadRequest(bufio.NewReader(serverConn)); err != nil {
						serverErr <- err
						return
					}
					_, err := io.WriteString(serverConn, "HTTP/1.1 "+status+"\r\nContent-Length: 0\r\n\r\n")
					serverErr <- err
				}()
				return clientConn, nil
			}), "device_1")

			if err := client.ProbeDevContainerPort(context.Background(), "sbx_1", 8080); err == nil {
				t.Fatalf("expected Hydra %s response to fail readiness", status)
			}
			if err := <-serverErr; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProbeDevContainerPortReturnsTransportFailure(t *testing.T) {
	client := NewRevDialClient(probeConnmanFunc(func(context.Context, string) (net.Conn, error) {
		return nil, errors.New("connection refused")
	}), "device_1")

	err := client.ProbeDevContainerPort(context.Background(), "sbx_1", 8080)
	if err == nil || !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("expected transport failure, got %v", err)
	}
}
