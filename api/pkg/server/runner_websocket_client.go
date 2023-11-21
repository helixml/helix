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
) {
	closed := false

	var conn *websocket.Conn

	connectWebsocket := func() {
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
	}

	connectWebsocket()

	go func() {
		for {
			select {
			case <-ctx.Done():
				closed = true
				if conn != nil {
					conn.Close()
				}

				return
			case ev := <-websocketEventChan:
				if conn == nil {
					continue
				}
				conn.WriteJSON(ev)
			}
		}
	}()

	if !closed {
		go func() {
			for {
				messageType, p, err := conn.ReadMessage()
				if err != nil || messageType == websocket.CloseMessage {
					if closed {
						return
					}
					log.Error().Msgf("Read error: %s\nReconnecting in 2 seconds...", err)
					time.Sleep(2 * time.Second)
					connectWebsocket()
					continue
				} else if messageType == websocket.TextMessage {
					log.Debug().
						Str("action", "runner websocket READ").
						Str("payload", string(p)).
						Msgf("")
				}
			}
		}()
	}
}
