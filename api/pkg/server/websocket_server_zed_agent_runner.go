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

// StartZedAgentRunnerWebSocketServer starts a WebSocket server to which Zed agent runners can connect
// and wait for the tasks to run
func (apiServer *HelixAPIServer) startZedAgentRunnerWebSocketServer(r *mux.Router, path string) {
	r.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		user, err := apiServer.authMiddleware.getUserFromToken(r.Context(), getRequestToken(r))
		if err != nil {
			log.Error().Msgf("Error getting user: %s", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if user == nil || !isRunner(user) {
			log.Error().Msgf("Error authorizing zed agent runner websocket")
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
			Str("action", "🟢 Zed agent runner connected").
			Int("concurrency", concurrency).
			Msgf("connected zed agent runner websocket: %s\n", runnerID)

		// Subscribe to both Zed agent tasks and legacy GPTScript tasks for compatibility
		zedAgentSub, err := apiServer.pubsub.StreamConsume(ctx, pubsub.ZedAgentRunnerStream, pubsub.ZedAgentQueue, func(msg *pubsub.Message) error {
			var messageType types.RunnerEventRequestType

			switch msg.Header.Get("kind") {
			case "zed_agent":
				messageType = types.RunnerEventRequestZedAgent
			case "app":
				messageType = types.RunnerEventRequestApp
			case "tool":
				messageType = types.RunnerEventRequestTool
			default:
				messageType = types.RunnerEventRequestZedAgent
			}

			err := wsConn.WriteJSON(&types.RunnerEventRequestEnvelope{
				RequestID: system.GenerateRequestID(),
				Reply:     msg.Reply, // Runner will need this inbox channel to send messages back to the requestor
				Type:      messageType,
				Payload:   msg.Data, // The actual payload (Zed agent request)
			})
			if err != nil {
				log.Error().Msgf("Error writing to Zed agent runner websocket: %s", err.Error())
				if nakErr := msg.Nak(); nakErr != nil {
					return fmt.Errorf("error writing to Zed agent runner websocket: %v, failed to Nak the message: %v", err, nakErr)
				}
				return err
			}

			if err := msg.Ack(); err != nil {
				return fmt.Errorf("failed to ack the message: %v", err)
			}
			return nil
		})
		if err != nil {
			log.Error().Msgf("Error subscribing to Zed agent queue: %s", err.Error())
			return
		}
		defer func() {
			if err := zedAgentSub.Unsubscribe(); err != nil {
				log.Err(err).Msg("failed to unsubscribe from zed agent queue")
			}
		}()

		// Also subscribe to legacy GPTScript queue for compatibility
		gptscriptSub, err := apiServer.pubsub.StreamConsume(ctx, pubsub.ScriptRunnerStream, pubsub.AppQueue, func(msg *pubsub.Message) error {
			var messageType types.RunnerEventRequestType

			switch msg.Header.Get("kind") {
			case "app":
				messageType = types.RunnerEventRequestApp
			case "tool":
				messageType = types.RunnerEventRequestTool
			}

			err := wsConn.WriteJSON(&types.RunnerEventRequestEnvelope{
				RequestID: system.GenerateRequestID(),
				Reply:     msg.Reply,
				Type:      messageType,
				Payload:   msg.Data,
			})
			if err != nil {
				log.Error().Msgf("Error writing GPTScript task to Zed agent runner websocket: %s", err.Error())
				if nakErr := msg.Nak(); nakErr != nil {
					return fmt.Errorf("error writing GPTScript task to Zed agent runner websocket: %v, failed to Nak the message: %v", err, nakErr)
				}
				return err
			}

			if err := msg.Ack(); err != nil {
				return fmt.Errorf("failed to ack the GPTScript message: %v", err)
			}
			return nil
		})
		if err != nil {
			log.Error().Msgf("Error subscribing to GPTScript app queue for compatibility: %s", err.Error())
			return
		}
		defer func() {
			if err := gptscriptSub.Unsubscribe(); err != nil {
				log.Err(err).Msg("failed to unsubscribe from gptscript queue")
			}
		}()

		// Block reads in order to detect disconnects and handle responses
		for {
			messageType, messageBytes, err := wsConn.ReadMessage()
			log.Trace().Msgf("Zed agent runner websocket event: %s", string(messageBytes))
			if err != nil || messageType == websocket.CloseMessage {
				log.Info().
					Str("action", "🟠 Zed agent runner ws DISCONNECT").
					Msgf("disconnected zed agent runner websocket: %s\n", runnerID)
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
				log.Error().Msgf("Error publishing Zed agent response: %s", err.Error())
			}
		}
	})
}
