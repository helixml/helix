package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type AuthenticateRequest func(r *http.Request) bool

type RunnerConnectionWrapper struct {
	conn   *websocket.Conn
	runner string
}

// StartRunnerWebSocketServer starts a WebSocket server
func (apiServer *HelixAPIServer) startRunnerWebSocketServer(
	_ context.Context,
	r *mux.Router,
	path string,
) {
	var mutex = &sync.Mutex{}

	connections := map[*websocket.Conn]*RunnerConnectionWrapper{}

	addConnection := func(conn *websocket.Conn, runner string) {
		mutex.Lock()
		defer mutex.Unlock()
		connections[conn] = &RunnerConnectionWrapper{
			conn:   conn,
			runner: runner,
		}
	}

	removeConnection := func(conn *websocket.Conn) {
		mutex.Lock()
		defer mutex.Unlock()
		delete(connections, conn)
	}

	r.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		user, err := apiServer.authMiddleware.getUserFromToken(r.Context(), getRequestToken(r))
		if err != nil {
			log.Error().Msgf("Error getting user: %s", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if user == nil || !isRunner(user) {
			log.Error().Msgf("Error authorizing runner websocket")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		conn, err := userWebsocketUpgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error().Msgf("Error upgrading websocket: %s", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		defer conn.Close()
		defer removeConnection(conn)

		// extract the runner ID from the query parameter
		runnerID := r.URL.Query().Get("runnerid")
		addConnection(conn, runnerID)

		log.Debug().
			Str("action", "🟠 runner ws CONNECT").
			Msgf("connected runner websocket: %s\n", runnerID)

		// Start server-initiated ping goroutine to keep connection alive through proxies/firewalls
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second)); err != nil {
						log.Debug().Err(err).Str("runner_id", runnerID).Msg("Runner WebSocket ping failed, connection closing")
						return
					}
				}
			}
		}()

		// we block on reading messages from the client
		// if we get any errors then we break and this will close
		// the connection and remove it from our map
		for {
			messageType, messageBytes, err := conn.ReadMessage()
			log.Trace().Msgf("User websocket event: %s", string(messageBytes))
			if err != nil || messageType == websocket.CloseMessage {
				log.Debug().
					Str("action", "🟠 runner ws DISCONNECT").
					Msgf("disconnected runner websocket: %s\n", runnerID)
				break
			}
			var event types.WebsocketEvent
			err = json.Unmarshal(messageBytes, &event)
			if err != nil {
				log.Error().Msgf("Error unmarshalling websocket event: %s", err.Error())
				continue
			}

			switch event.Type {
			case types.WebsocketLLMInferenceResponse: // LLM inference (v2) response
				if event.InferenceResponse == nil {
					log.Error().
						Any("event", event).
						Msg("inference response from the runner is nil")
					continue
				}

				err = apiServer.inferenceServer.ProcessRunnerResponse(r.Context(), event.InferenceResponse)
				if err != nil {
					log.Error().Msgf("Error processing runner response: %s", err.Error())
				}
			case
				types.WebsocketEventSessionUpdate,      // Delta session update
				types.WebsocketEventWorkerTaskResponse: // Complete response
				err = apiServer.pubsub.Publish(r.Context(), pubsub.GetSessionQueue(event.Owner, event.SessionID), messageBytes)
				if err != nil {
					log.Error().Msgf("Error publishing session update: %s", err.Error())
				}
			}
		}

		removeConnection(conn)
	})
}
