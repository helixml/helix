package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// StartRunnerWebSocketServer starts a WebSocket server to which GPTScript runners can connect
// and wait for the tasks to run
func (apiServer *HelixAPIServer) startGptScriptRunnerWebSocketServer(r *mux.Router, path string) {
	r.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		user, err := apiServer.authMiddleware.getUserFromToken(r.Context(), getRequestToken(r))
		if err != nil {
			log.Error().Msgf("Error getting user: %s", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if user == nil || !isRunner(user) {
			log.Error().Msgf("Error authorizing runner websocket")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		wsConn, err := userWebsocketUpgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error().Msgf("Error upgrading websocket: %s", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer wsConn.Close()

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		runnerID := r.URL.Query().Get("runnerid")
		conccurencyStr := r.URL.Query().Get("concurrency")

		concurrency, err := strconv.Atoi(conccurencyStr)
		if err != nil {
			log.Error().Msgf("Error parsing concurrency: %s", err.Error())
			concurrency = 1
		}

		log.Info().
			Str("action", "🟢 GPTScript runner connected").
			Int("concurrency", concurrency).
			Msgf("connected runner websocket: %s\n", runnerID)

		appSub, err := apiServer.pubsub.StreamConsume(ctx, pubsub.ScriptRunnerStream, pubsub.AppQueue, concurrency, func(msg *pubsub.Message) error {
			var messageType types.RunnerEventRequestType

			switch msg.Header.Get("kind") {
			case "app":
				messageType = types.RunnerEventRequestApp
			case "tool":
				messageType = types.RunnerEventRequestTool
			}

			err := wsConn.WriteJSON(&types.RunnerEventRequestEnvelope{
				RequestID: system.GenerateRequestID(),
				Reply:     msg.Reply, // Runner will need this inbox channel to send messages back to the requestor
				Type:      messageType,
				Payload:   msg.Data, // The actual payload (GPTScript request)
			})
			if err != nil {
				log.Error().Msgf("Error writing to GPTScript runner websocket: %s", err.Error())
				if nakErr := msg.Nak(); nakErr != nil {
					return fmt.Errorf("Error writing to GPTScript runner websocket: %v, failed to Nak the message: %v", err, nakErr)
				}
				return err
			}

			if err := msg.Ack(); err != nil {
				return fmt.Errorf("failed to ack the message: %v", err)
			}
			return nil
		})
		if err != nil {
			log.Error().Msgf("Error subscribing to GPTScript app queue: %s", err.Error())
			return
		}
		defer func() {
			if err := appSub.Unsubscribe(); err != nil {
				log.Err(err).Msg("failed to unsubscribe")
			}
		}()

		// Block reads in order to detect disconnects
		for {
			messageType, messageBytes, err := wsConn.ReadMessage()
			log.Trace().Msgf("GPTScript runner websocket event: %s", string(messageBytes))
			if err != nil || messageType == websocket.CloseMessage {
				log.Info().
					Str("action", "🟠 GPTScript runner ws DISCONNECT").
					Msgf("disconnected runner websocket: %s\n", runnerID)
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			var resp types.RunnerEventResponseEnvelope
			err = json.Unmarshal(messageBytes, &resp)
			if err != nil {
				log.Error().Msgf("Error unmarshalling websocket event: %s", err.Error())
				continue
			}

			err = apiServer.pubsub.Publish(ctx, resp.Reply, resp.Payload)
			if err != nil {
				log.Error().Msgf("Error publishing GPTScript response: %s", err.Error())
			}
		}
	})
}
