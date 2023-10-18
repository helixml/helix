package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/bacalhau-project/lilysaas/api/pkg/types"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type GetUserIDFromRequest func(r *http.Request) (string, error)

type ConnectionWrapper struct {
	conn *websocket.Conn
	mu   sync.Mutex
	user string
}

// StartWebSocketServer starts a WebSocket server
func StartWebSocketServer(
	ctx context.Context,
	r *mux.Router,
	path string,
	jobUpdatesChan chan *types.Job,
	sessionUpdatesChan chan *types.Session,
	getUserIDFromRequest GetUserIDFromRequest,
) {
	var mutex = &sync.Mutex{}

	connections := map[*websocket.Conn]*ConnectionWrapper{}

	addConnection := func(conn *websocket.Conn, user string) {
		mutex.Lock()
		defer mutex.Unlock()
		connections[conn] = &ConnectionWrapper{
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
			case sessionUpdate := <-sessionUpdatesChan:
				event := types.WebsocketEvent{
					Type:    types.WebsocketEventSessionUpdate,
					Session: sessionUpdate,
				}
				message, err := json.Marshal(event)
				if err != nil {
					log.Error().Msgf("Error marshalling session update: %s", err.Error())
					continue
				}
				// TODO: make this more efficient
				for _, connWrapper := range connections {
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
			case jobUpdate := <-jobUpdatesChan:
				event := types.WebsocketEvent{
					Type: types.WebsocketEventJobUpdate,
					Job:  jobUpdate,
				}
				message, err := json.Marshal(event)
				if err != nil {
					log.Error().Msgf("Error marshalling job update: %s", err.Error())
					continue
				}
				// TODO: make this more efficient
				for _, connWrapper := range connections {
					if connWrapper.user != jobUpdate.Owner {
						continue
					}
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

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error().Msgf("Error upgrading websocket: %s", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer conn.Close()
		addConnection(conn, userID)

		log.Debug().
			Str("action", "⚪⚪⚪⚪⚪⚪⚪⚪⚪⚪ ws CONNECT").
			Msgf("connected user websocket: %s\n", userID)

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
