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

type AuthenticateRequest func(r *http.Request) bool

type RunnerConnectionWrapper struct {
	conn   *websocket.Conn
	runner string
}

// StartRunnerWebSocketServer starts a WebSocket server
func StartRunnerWebSocketServer(
	ctx context.Context,
	r *mux.Router,
	Controller *controller.Controller,
	path string,
	websocketEventChan chan *types.WebsocketEvent,
	authHandler AuthenticateRequest,
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
		authed := authHandler(r)
		if !authed {
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

			// we don't need to tell anyone about ping events
			if event.Type == types.WebsocketEventSessionPing {
				continue
			}

			websocketEventChan <- &event
		}

		removeConnection(conn)
	})
}
