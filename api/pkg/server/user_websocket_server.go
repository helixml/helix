package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/davecgh/go-spew/spew"
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
				fmt.Printf("event --------------------------------------\n")
				spew.Dump(event)
				message, err := json.Marshal(event)
				if err != nil {
					log.Error().Msgf("Error marshalling session update: %s", err.Error())
					continue
				}
				// TODO: make this more efficient
				for _, connWrapper := range connections {
					// TODO: put this back after we start sending correct user/owner
					// if connWrapper.user != sessionUpdate.Owner {
					// 	continue
					// }
					connWrapper.mu.Lock()
					if err := connWrapper.conn.WriteMessage(websocket.TextMessage, message); err != nil {
						log.Error().Msgf("Error writing to websocket: %s", err.Error())
						connWrapper.mu.Unlock()
						return
					}
					connWrapper.mu.Unlock()
				}
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
			Str("action", "⚪ ws CONNECT").
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
