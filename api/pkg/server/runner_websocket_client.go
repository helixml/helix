package server

import (
	"context"
	"time"

	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/recws-org/recws"
	"github.com/rs/zerolog/log"
)

// this is designed to write messages from the runner to the api server
func ConnectRunnerWebSocketClient(
	url string,
	websocketEventChan chan *types.WebsocketEvent,
	ctx context.Context,
) {

	ws := recws.RecConn{
		KeepAliveTimeout: 60 * time.Second,
	}
	ws.Dial(url, nil)

	for {
		select {
		case <-ctx.Done():
			go ws.Close()
			return
		case <-time.After(10 * time.Second):
			ev := &types.WebsocketEvent{
				Type:      types.WebsocketEventSessionPing,
				SessionID: "",
				Owner:     "",
			}
			err := ws.WriteJSON(ev)
			if err != nil {
				log.Error().Msgf("Error writing ping to websocket: %s %+v", err.Error(), ev)
				continue
			}
		case ev := <-websocketEventChan:
			log.Debug().
				Str("action", "Websocket WRITE").
				Msgf("payload %+v", ev)
			err := ws.WriteJSON(ev)
			if err != nil {
				log.Error().Msgf("Error writing data to websocket: %s %+v", err.Error(), ev)
				continue
			}
		}
	}
}
