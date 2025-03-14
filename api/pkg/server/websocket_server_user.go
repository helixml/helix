package server

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/rs/zerolog/log"
)

var userWebsocketUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(*http.Request) bool {
		return true
	},
}

type GetUserIDFromRequest func(r *http.Request) (string, error)

// startUserWebSocketServer starts a WebSocket server
func (apiServer *HelixAPIServer) startUserWebSocketServer(
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

		if user == nil || !hasUser(user) {
			log.Error().Msgf("Error getting user")
			http.Error(w, "unauthorized", http.StatusInternalServerError)
			return
		}

		sessionID := r.URL.Query().Get("session_id")
		if sessionID == "" {
			log.Error().Msgf("No session_id supplied")
			http.Error(w, "No session_id supplied", http.StatusInternalServerError)
			return
		}

		conn, err := userWebsocketUpgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error().Msgf("Error upgrading websocket: %s", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		defer conn.Close()

		sub, err := apiServer.pubsub.Subscribe(r.Context(), pubsub.GetSessionQueue(user.ID, sessionID), func(payload []byte) error {
			if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				log.Error().Msgf("Error writing to websocket: %s", err.Error())
			}
			return nil
		})
		if err != nil {
			log.Error().Msgf("Error subscribing to internal updates: %s", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		defer func() {
			if err := sub.Unsubscribe(); err != nil {
				log.Error().Msgf("failed to unsubscribe: %v", err)
			}
		}()

		log.Trace().
			Str("user_id", user.ID).
			Str("session_id", sessionID).
			Msg("user websocket connected")

		// we block on reading messages from the client
		// if we get any errors then we break and this will close
		// the connection and remove it from our map
		for {
			messageType, _, err := conn.ReadMessage()
			if err != nil {
				log.Trace().Msgf("Client disconnected: %s", err.Error())
				break
			}
			if messageType == websocket.CloseMessage {
				log.Trace().Msgf("Received close frame from client.")
				break
			}
		}
	})
}
