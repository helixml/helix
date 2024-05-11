package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// StartRunnerWebSocketServer starts a WebSocket server
func (apiServer *HelixAPIServer) startGptScriptRunnerWebSocketServer(
	_ context.Context,
	r *mux.Router,
	path string,
) {

	r.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		user, err := apiServer.authMiddleware.getUserFromToken(r.Context(), getRequestToken(r))
		if err != nil {
			log.Error().Msgf("Error getting user: %s", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if user == nil || !isRunner(*user) {
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

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		runnerID := r.URL.Query().Get("runnerid")

		log.Debug().
			Str("action", "🟠 GPTScript runner ws CONNECT").
			Msgf("connected runner websocket: %s\n", runnerID)

		sub, err := apiServer.pubsub.QueueSubscribe(ctx, pubsub.GetGPTScriptQueue(), "runner", func(payload []byte) error {
			err := conn.WriteMessage(websocket.TextMessage, payload)
			if err != nil {
				log.Error().Msgf("Error writing to GPTScript runner websocket: %s", err.Error())
			}
			return err
		})
		if err != nil {
			log.Error().Msgf("Error subscribing to GPTScript queue: %s", err.Error())
			return
		}
		defer sub.Unsubscribe()

		// Block reads in order to detect disconnects
		for {
			messageType, messageBytes, err := conn.ReadMessage()
			log.Trace().Msgf("GPTScript runner websocket event: %s", string(messageBytes))
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

			apiServer.Controller.RunnerWebsocketEventChanReader <- &event
		}
	})
}
