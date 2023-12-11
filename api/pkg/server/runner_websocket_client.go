package server

import (
	"context"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

var mu sync.Mutex

// ConnectWebSocket establishes a new WebSocket connection
func ConnectRunnerWebSocketClient(
	url string,
	websocketEventChan chan *types.WebsocketEvent,
	ctx context.Context,
) {
	closed := false
	finished := make(chan bool)

	var conn *websocket.Conn

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
			// ping every 10 seconds to keep the connection alive
			case <-time.After(10 * time.Second):
				if conn == nil {
					continue
				}
				func() {
					mu.Lock()
					defer mu.Unlock()
					conn.WriteMessage(websocket.PingMessage, []byte{})
				}()
			case <-finished:
				return
			case ev := <-websocketEventChan:
				if conn == nil {
					continue
				}
				func() {
					mu.Lock()
					defer mu.Unlock()
					conn.WriteJSON(ev)
				}()
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
					log.Error().Msgf("Read error: %s\nReconnecting in 2 seconds...", err)
					time.Sleep(2 * time.Second)
					finished <- true
					ConnectRunnerWebSocketClient(url, websocketEventChan, ctx)
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
}
