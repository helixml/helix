package hydra

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/docker/docker/client"
	"github.com/gorilla/mux"
)

func TestHealthStaysLiveWhenDockerIsNotReady(t *testing.T) {
	var dockerReady atomic.Bool
	dockerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_ping" {
			http.NotFound(w, r)
			return
		}
		if !dockerReady.Load() {
			http.Error(w, "dockerd unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("API-Version", "1.47")
		_, _ = w.Write([]byte("OK"))
	}))
	defer dockerServer.Close()

	dockerClient, err := client.NewClientWithOpts(
		client.WithHost(dockerServer.URL),
		client.WithHTTPClient(dockerServer.Client()),
	)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{devContainerManager: &DevContainerManager{docker: dockerClient}}
	router := mux.NewRouter()
	server.registerRoutes(router)

	for _, tc := range []struct {
		path string
		want int
	}{
		{path: "/health", want: http.StatusOK},
		{path: "/ready", want: http.StatusServiceUnavailable},
	} {
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if resp.Code != tc.want {
			t.Fatalf("%s status = %d, want %d", tc.path, resp.Code, tc.want)
		}
	}

	dockerReady.Store(true)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("/ready status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
}

func TestRevDialReadyUsesReadyEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name       string
		statusLine string
		wantErr    bool
	}{
		{name: "ready", statusLine: "200 OK"},
		{name: "old Hydra without ready endpoint", statusLine: "404 Not Found", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			defer clientConn.Close()
			go func() {
				defer serverConn.Close()
				req, err := http.ReadRequest(bufio.NewReader(serverConn))
				if err != nil {
					return
				}
				if req.URL.Path != "/ready" {
					return
				}
				_, _ = serverConn.Write([]byte("HTTP/1.1 " + tc.statusLine + "\r\nContent-Length: 0\r\n\r\n"))
			}()

			var dials atomic.Int32
			c := NewRevDialClient(probeConnmanFunc(func(context.Context, string) (net.Conn, error) {
				dials.Add(1)
				return clientConn, nil
			}), "hydra-host")
			err := c.Ready(context.Background())
			if (err != nil) != tc.wantErr {
				t.Fatalf("Ready() error = %v, wantErr %v", err, tc.wantErr)
			}
			if got := dials.Load(); got != 1 {
				t.Fatalf("RevDial attempts = %d, want 1", got)
			}
		})
	}
}

func TestRevDialReadyHonorsCancellation(t *testing.T) {
	c := NewRevDialClient(probeConnmanFunc(func(ctx context.Context, _ string) (net.Conn, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}), "hydra-host")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := c.Ready(ctx); err == nil {
		t.Fatal("expected readiness timeout")
	}
}
