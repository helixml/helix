package server

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// registerRunnerNATSProxy exposes the embedded NATS bus to runners at
// GET /api/v1/runner/{runner_id}/ws when the deployment enables it
// (RUNNER_NATS_PROXY_ENABLED). The route sits outside the auth middleware:
// NATS itself checks the token, and that token is the shared runner token,
// which grants every subject. Nothing in Helix connects here any more, so the
// route is not registered by default.
func (apiServer *HelixAPIServer) registerRunnerNATSProxy(router *mux.Router) {
	if apiServer.Cfg == nil || !apiServer.Cfg.WebServer.RunnerNATSProxyEnabled {
		return
	}
	router.HandleFunc("/runner/{runner_id}/ws", apiServer.proxyRunnerNATS)
}

func (apiServer *HelixAPIServer) proxyRunnerNATS(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	runnerID := vars["runner_id"]
	log.Info().
		Str("runner_id", runnerID).
		Str("request_path", r.URL.Path).
		Msg("proxying runner websocket request to NATS")

	defer log.Info().Str("runner_id", runnerID).Msg("websocket proxy to NATS disconnected")

	// Upgrade the incoming HTTP connection to a WebSocket connection.
	upgrader := websocket.Upgrader{
		// TODO(Phil): check origin
		CheckOrigin: func(r *http.Request) bool {
			log.Debug().Interface("headers", r.Header).Interface("vars", r.RemoteAddr).Msg("nats check origin")
			return true
		},
	}

	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to upgrade client connection")
		return
	}
	// Ensure the client connection is closed on function exit.
	defer clientConn.Close()

	// Connect to the backend WebSocket server.
	backendConn, resp, err := websocket.DefaultDialer.Dial("ws://localhost:8433", nil) // TODO(Phil): make this configurable
	if err != nil {
		log.Error().Err(err).Msg("Failed to connect to backend WebSocket server")
		return
	}
	// Ensure the backend connection is closed on function exit.
	defer backendConn.Close()
	defer resp.Body.Close()

	// Mutex for thread-safe writes to client WebSocket (ping and backend→client can race)
	var clientMu sync.Mutex

	// Start three goroutines: client→backend, backend→client, and ping
	errCh := make(chan error, 3)

	// Start server-initiated ping goroutine to keep client connection alive through proxies/firewalls
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			<-ticker.C
			clientMu.Lock()
			err := clientConn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second))
			clientMu.Unlock()
			if err != nil {
				log.Debug().Err(err).Str("runner_id", runnerID).Msg("NATS WebSocket proxy ping failed, connection closing")
				errCh <- nil
				return
			}
		}
	}()

	// Copy messages from the client to the backend.
	go func() {
		for {
			messageType, message, err := clientConn.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}
			if err := backendConn.WriteMessage(messageType, message); err != nil {
				errCh <- err
				return
			}
		}
	}()

	// Copy messages from the backend to the client.
	go func() {
		for {
			messageType, message, err := backendConn.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}
			// Mutex protects against concurrent ping writes
			clientMu.Lock()
			writeErr := clientConn.WriteMessage(messageType, message)
			clientMu.Unlock()
			if writeErr != nil {
				errCh <- writeErr
				return
			}
		}
	}()

	// Wait until one side returns an error (or closes the connection).
	if err := <-errCh; err != nil {
		log.Printf("WebSocket proxy error: %v", err)
	}
}
