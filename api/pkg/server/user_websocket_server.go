package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/rs/zerolog/log"
)

var userWebsocketUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type GetUserIDFromRequest func(r *http.Request) (string, error)

type UserConnectionWrapper struct {
	conn    *websocket.Conn
	mu      sync.Mutex
	user    string
	session string
}

// startUserWebSocketServer starts a WebSocket server
func (apiServer *HelixAPIServer) startUserWebSocketServer(
	ctx context.Context,
	r *mux.Router,
	path string,
) {
	// spawn a reader from the incoming message channel
	// each message we get we fan out to all the currently connected websocket clients

	// TODO: we should add some subscription channels here because right now we are
	// splatting a lot of bytes down the write because everyone is hearing everything
	go func() {
		for {
			select {
			case event := <-apiServer.Controller.UserWebsocketEventChanWriter:
				log.Trace().Msgf("User websocket event: %+v", event)
				message, err := json.Marshal(event)
				if err != nil {
					log.Error().Msgf("Error marshalling session update: %s", err.Error())
					continue
				}

				err = apiServer.pubsub.Publish(ctx, pubsub.GetSessionQueue(event.Owner, event.SessionID), message)
				if err != nil {
					log.Error().Msgf("Error publishing session update: %s", err.Error())
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	r.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		userID, err := apiServer.authMiddleware.userIDFromRequestBothModes(r)
		if err != nil {
			log.Error().Msgf("Error getting user id: %s", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
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

		sub, err := apiServer.pubsub.Subscribe(r.Context(), pubsub.GetSessionQueue(userID, sessionID), func(payload []byte) error {
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

		defer sub.Unsubscribe()

		log.Trace().
			Str("action", "⚪ user ws CONNECT").
			Msgf("connected user websocket: %s for session: %s\n", userID, sessionID)

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
