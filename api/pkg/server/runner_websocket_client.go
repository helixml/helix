package server

import (
	"context"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// this is designed to write messages from the runner to the api server
func ConnectRunnerWebSocketClient(
	url string,
	messageChan chan []byte,
	ctx context.Context,
) *websocket.Conn {
	closed := false

	var conn *websocket.Conn

	go func() {
		for {
			select {
			case <-ctx.Done():
				closed = true
				if conn != nil {
					conn.Close()
				}

				return
			}
		}
	}()

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
					conn = ConnectRunnerWebSocketClient(url, messageChan, ctx)
					continue
				}
				if messageType == websocket.TextMessage {
					log.Debug().
						Str("action", "ws READ").
						Str("payload", string(p)).
						Msgf("")
					messageChan <- p
				}
			}
		}()
	}

	return conn
}
