package server

import (
	"context"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// ConnectWebSocket establishes a new WebSocket connection
func ConnectRunnerWebSocketClient(
	url string,
	websocketEventChan chan *types.WebsocketEvent,
	ctx context.Context,
) *websocket.Conn {
	closed := false

	var conn *websocket.Conn

	var readMessageErr chan bool

	// if we ever get a cancellation from the context, try to close the connection
	go func() {
		for {
			select {
			case <-ctx.Done():
				closed = true
				if conn != nil {
					conn.Close()
				}
				return
			case <-readMessageErr:
				log.Info().Msg("Exiting readloop because connection closed")
				return
			case ev := <-websocketEventChan:
				if conn == nil {
					log.Error().Msg("Dropping websocket message on the floor because conn is nil")
					continue
				}
				conn.WriteJSON(ev)
			}
		}
	}()

	// retry connecting until we get a connection
	for {
		var err error
		log.Debug().Msgf("WebSocket connection connecting: %s", url)
		conn, _, err = websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			log.Error().Msgf("WebSocket connection failed: %s\nReconnecting in 2 seconds...", err)
			if closed {
				break
			}
			time.Sleep(2 * time.Second)
			continue
		}
		break
	}

	// now that we have a connection, if we haven't been closed yet, forever
	// read from the connection and send messages down the channel, unless we
	// fail a read in which case we try to reconnect
	if !closed {
		go func() {
			for {
				messageType, p, err := conn.ReadMessage()
				if err != nil {
					if closed {
						return
					}
					// synchronize on the other goroutine STOPPING writing to
					// conn so that it's safe to start a new one which will do
					// so (you can't write to websocket connections
					// concurrently)
					readMessageErr <- true
					log.Error().Msgf("Read error: %s\nReconnecting in 2 seconds...", err)
					time.Sleep(2 * time.Second)
					// XXX not sure about writing to conn here, does it
					// propagate the new connection to previous callers of this
					// function? Do they use the connection?
					conn = ConnectRunnerWebSocketClient(url, websocketEventChan, ctx)
					// exit this goroutine now, another one will be spawned if
					// the recursive call to ConnectWebSocket succeeds. Not
					// exiting this goroutine here will cause goroutines to pile
					// up forever concurrently calling conn.ReadMessage(), which
					// is not thread-safe.
					return
				}
				if messageType == websocket.TextMessage {
					log.Debug().
						Str("action", "runner websocket READ").
						Str("payload", string(p)).
						Msgf("")
				}
			}
		}()
	}
	return conn
}
