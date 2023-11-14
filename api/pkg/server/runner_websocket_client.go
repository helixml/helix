package server

import (
	"context"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// this is designed to write messages from the runner to the api server
func ConnectRunnerWebSocketClient(
	url string,
	websocketEventChan chan *types.WebsocketEvent,
	ctx context.Context,
) error {

	connectWebSocket := func() (*websocket.Conn, error) {
		log.Debug().Msgf("Web socket Connecting to %s", url)
		c, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			log.Error().Msgf("Dial error: %s", err.Error())
			return nil, err
		}
		return c, nil
	}

	var wsConn *websocket.Conn
	var err error
	closed := false

	wsConn, err = connectWebSocket()
	if err != nil {
		return err
	}

	go func() {
		for {
			// Read messages in a loop
			_, message, err := wsConn.ReadMessage()
			if err != nil {
				log.Error().Msgf("Websocket Read error: %s", err.Error())
				wsConn.Close()
				if closed {
					break
				}

				// Reconnect logic
				wsConn, err = connectWebSocket()
				if err != nil {
					log.Error().Msgf("Websocket Reconnect failed: %s", err.Error())
					time.Sleep(time.Second * time.Duration(1))
				}
				continue
			}

			// Process the received message
			log.Debug().Msgf("Websocket Received: %s", message)
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				closed = true
				if wsConn != nil {
					wsConn.Close()
				}
				break
			case ev := <-websocketEventChan:
				if wsConn == nil {
					continue
				}
				log.Debug().
					Str("action", "Websocket WRITE").
					Msgf("payload %+v", ev)
				err := wsConn.WriteJSON(ev)
				if err != nil {
					log.Error().Msgf("Error writing data to websocket: %s %+v", err.Error(), ev)
					continue
				}
			}
		}
	}()

	return nil
}
