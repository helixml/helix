package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/lukemarsden/helix/api/pkg/pubsub"
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
	var mutex = &sync.Mutex{}

	connections := map[*websocket.Conn]*UserConnectionWrapper{}

	// TODO: make this more efficient so we don't need to loop over all connections every time
	// we should have a list of connections per sessionID
	addConnection := func(conn *websocket.Conn, user string, sessionID string) {
		mutex.Lock()
		defer mutex.Unlock()
		connections[conn] = &UserConnectionWrapper{
			conn:    conn,
			user:    user,
			session: sessionID,
		}
	}

	removeConnection := func(conn *websocket.Conn) {
		mutex.Lock()
		defer mutex.Unlock()
		delete(connections, conn)
	}

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

				err = apiServer.pubsub.Publish(ctx, event.SessionID, message,
					pubsub.WithPublishNamespace(event.Owner),
				)
				if err != nil {
					log.Error().Msgf("Error publishing session update: %s", err.Error())
				}

				func() {
					// hold the mutex while we iterate over connections because
					// you can't modify a mutex while iterating over it (fatal
					// error: concurrent map iteration and map write)
					mutex.Lock()
					defer mutex.Unlock()
					for _, connWrapper := range connections {
						// each user websocket connection is only interested in a single session
						if connWrapper.user != event.Owner || connWrapper.session != event.SessionID {
							continue
						}
						// wrap in a func so that we can defer the unlock so we can
						// unlock the mutex on panics as well as errors
						func() {
							connWrapper.mu.Lock()
							defer connWrapper.mu.Unlock()
							if err := connWrapper.conn.WriteMessage(websocket.TextMessage, message); err != nil {
								log.Error().Msgf("Error writing to websocket: %s", err.Error())
								return
							}
						}()
					}
				}()
			case <-ctx.Done():
				return
			}
		}
	}()

	r.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		userID, err := apiServer.keyCloakMiddleware.userIDFromRequestBothModes(r)
		if err != nil {
			log.Error().Msgf("Error getting user id: %s", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		sessionID := r.URL.Query().Get("session_id")
		if sessionID == "" {
			log.Error().Msgf("No session_id supplied")
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
		addConnection(conn, userID, sessionID)

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

		removeConnection(conn)
	})
}
