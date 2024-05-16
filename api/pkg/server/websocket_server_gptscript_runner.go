package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"

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

		if user == nil || !isRunner(*user) {
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

		log.Info().
			Str("action", "🟢 GPTScript runner connected").
			Msgf("connected runner websocket: %s\n", runnerID)

			// TODO: switch to synchronous
			// subscriptions https://docs.nats.io/using-nats/developer/receiving/sync

		p := pool.New().WithContext(ctx)

		p.Go(func(ctx context.Context) error {
			err = apiServer.pubsub.SynchronousSubscribe(ctx, pubsub.GetGPTScriptAppQueue(), "runner", func(reply string, payload []byte) error {
				err := wsConn.WriteJSON(&types.RunnerEventRequestEnvelope{
					RequestID: system.GenerateRequestID(),
					Reply:     reply, // Runner will need this inbox channel to send messages back to the requestor
					Type:      types.RunnerEventRequestApp,
					Payload:   payload, // The actual payload (GPTScript request)
				})
				if err != nil {
					log.Error().Msgf("Error writing to GPTScript runner websocket: %s", err.Error())
				}
				return err
			})
			if err != nil {
				log.Error().Msgf("Error subscribing to GPTScript app queue: %s", err.Error())
			}
			return err
		})

		p.Go(func(ctx context.Context) error {
			err := apiServer.pubsub.SynchronousSubscribe(ctx, pubsub.GetGPTScriptToolQueue(), "runner", func(reply string, payload []byte) error {
				err := wsConn.WriteJSON(&types.RunnerEventRequestEnvelope{
					RequestID: system.GenerateRequestID(),
					Reply:     reply, // Runner will need this inbox channel to send messages back to the requestor
					Type:      types.RunnerEventRequestTool,
					Payload:   payload, // The actual payload (GPTScript request)
				})
				if err != nil {
					log.Error().Msgf("Error writing to GPTScript runner websocket: %s", err.Error())
				}
				return err
			})
			if err != nil {
				log.Error().Msgf("Error subscribing to GPTScript tools queue: %s", err.Error())
			}
			return err
		})
		defer p.Wait()

		// Block reads in order to detect disconnects
		for {
			messageType, messageBytes, err := wsConn.ReadMessage()
			log.Trace().Msgf("GPTScript runner websocket event: %s", string(messageBytes))
			if err != nil || messageType == websocket.CloseMessage {
				log.Info().
					Str("action", "🟠 runner ws DISCONNECT").
					Msgf("disconnected runner websocket: %s\n", runnerID)
				break
			}
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
