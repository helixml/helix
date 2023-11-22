package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/lukemarsden/helix/api/pkg/controller"
	"github.com/lukemarsden/helix/api/pkg/types"
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
	conn *websocket.Conn
	mu   sync.Mutex
	user string
}

// StartUserWebSocketServer starts a WebSocket server
func StartUserWebSocketServer(
	ctx context.Context,
	r *mux.Router,
	Controller *controller.Controller,
	path string,
	websocketEventChan chan *types.WebsocketEvent,
	getUserIDFromRequest GetUserIDFromRequest,
) {
	var mutex = &sync.Mutex{}

	connections := map[*websocket.Conn]*UserConnectionWrapper{}

	addConnection := func(conn *websocket.Conn, user string) {
		mutex.Lock()
		defer mutex.Unlock()
		connections[conn] = &UserConnectionWrapper{
			conn: conn,
			user: user,
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
			case event := <-websocketEventChan:
				log.Trace().Msgf("User websocket event: %+v", event)
				message, err := json.Marshal(event)
				if err != nil {
					log.Error().Msgf("Error marshalling session update: %s", err.Error())
					continue
				}
				func() {
					// hold the mutex while we iterate over connections because
					// you can't modify a mutex while iterating over it (fatal
					// error: concurrent map iteration and map write)
					mutex.Lock()
					defer mutex.Unlock()
					for _, connWrapper := range connections {
						if connWrapper.user != event.Owner {
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
		userID, err := getUserIDFromRequest(r)
		if err != nil {
			log.Error().Msgf("Error getting user id: %s", err.Error())
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
		addConnection(conn, userID)

		log.Debug().
			Str("action", "⚪ user ws CONNECT").
			Msgf("connected user websocket: %s\n", userID)

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
