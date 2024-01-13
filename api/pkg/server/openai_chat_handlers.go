package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/lukemarsden/helix/api/pkg/pubsub"
	"github.com/lukemarsden/helix/api/pkg/system"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

const (
	BYTE = 1 << (10 * iota)
	KILOBYTE
	MEGABYTE
)

// https://platform.openai.com/docs/api-reference/chat/create
// POST https://api.openai.com/v1/chat/completions
func (apiServer *HelixAPIServer) createChatCompletion(res http.ResponseWriter, req *http.Request) {
	reqContext := apiServer.getRequestContext(req)

	body, err := io.ReadAll(io.LimitReader(req.Body, 10*MEGABYTE))
	if err != nil {
		http.Error(res, err.Error(), http.StatusBadRequest)
		return
	}

	var chatCompletionRequest types.ChatCompletionRequest
	err = json.Unmarshal(body, &chatCompletionRequest)
	if err != nil {
		http.Error(res, err.Error(), http.StatusBadRequest)
		return
	}

	userContext := apiServer.getRequestContext(req)
	status, err := apiServer.Controller.GetStatus(userContext)
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}

	sessionID := system.GenerateUUID()

	sessionMode := types.SessionModeInference

	var messages []types.InteractionMessage

	for _, m := range chatCompletionRequest.Messages {
		// Validating roles
		switch m.Role {
		case "user", "system", "assistant":
			// OK
		default:
			http.Error(res, "invalid role, available roles: 'user', 'system', 'assistant'", http.StatusBadRequest)
			return

		}

		messages = append(messages, types.InteractionMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	interaction := types.Interaction{
		ID:             system.GenerateUUID(),
		Created:        time.Now(),
		Updated:        time.Now(),
		Scheduled:      time.Now(),
		Completed:      time.Now(),
		Creator:        types.CreatorTypeUser,
		Mode:           sessionMode,
		Messages:       messages,
		Files:          []string{},
		State:          types.InteractionStateComplete,
		Finished:       true,
		Metadata:       map[string]string{},
		DataPrepChunks: map[string][]types.DataPrepChunk{},
	}

	sessionData, err := apiServer.Controller.CreateSession(userContext, types.CreateSessionRequest{
		SessionID:       sessionID,
		SessionMode:     sessionMode,
		SessionType:     types.SessionTypeText,
		ModelName:       types.ModelName(chatCompletionRequest.Model),
		Owner:           reqContext.Owner,
		OwnerType:       reqContext.OwnerType,
		UserInteraction: interaction,
		Priority:        status.Config.StripeSubscriptionActive,
	})
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}

	if chatCompletionRequest.Stream {
		apiServer.handleStreamingResponse(res, req, sessionData)
		return
	}

	apiServer.handleBlockingResponse(res, req, sessionData)
}

func (apiServer *HelixAPIServer) handleStreamingResponse(res http.ResponseWriter, req *http.Request, session *types.Session) {
	// Set chunking headers
	res.Header().Set("Cache-Control", "no-cache")
	res.Header().Set("Connection", "keep-alive")
	res.Header().Set("Transfer-Encoding", "chunked")
	res.Header().Set("Content-Type", "text/event-stream")

	logger := log.With().Str("session_id", session.ID).Logger()

	doneCh := make(chan struct{})

	logger.Debug().Msgf("session streaming started")
	defer logger.Debug().Msgf("session streaming done")

	consumer := apiServer.pubsub.Subscribe(req.Context(), session.ID, func(payload []byte) error {
		var event types.WebsocketEvent
		err := json.Unmarshal(payload, &event)
		if err != nil {
			return fmt.Errorf("error unmarshalling websocket event '%s': %w", string(payload), err)
		}

		// If we get a worker task response with done=true, we need to send a final chunk
		// if event.WorkerTaskResponse != nil && event.WorkerTaskResponse.Done {
		if event.Type == "session_update" && event.Session != nil && event.Session.Interactions[len(event.Session.Interactions)-1].State == types.InteractionStateComplete {
			logger.Debug().Msgf("session finished")

			lastChunk := createChatCompletionChunk(session, "")
			lastChunk.Choices[0].FinishReason = "stop"

			respData, err := json.Marshal(lastChunk)
			if err != nil {
				return fmt.Errorf("error marshalling websocket event '%+v': %w", event, err)
			}

			_, err = fmt.Fprintf(res, "data: %s\n\n", string(respData))
			if err != nil {
				return fmt.Errorf("error writing final chunk '%s': %w", string(respData), err)
			}

			_, _ = res.Write([]byte("data: [DONE]\n\n"))

			// Flush the ResponseWriter buffer to send the chunk immediately
			if flusher, ok := res.(http.Flusher); ok {
				flusher.Flush()
			}

			// Close connection
			close(doneCh)
			return nil
		}

		// Nothing to do
		if event.WorkerTaskResponse == nil {
			return nil
		}

		// Write chunk
		chunk, err := json.Marshal(createChatCompletionChunk(session, event.WorkerTaskResponse.Message))
		if err != nil {
			return fmt.Errorf("error marshalling websocket event '%+v': %w", event, err)
		}

		_, err = fmt.Printf("data: %s\n\n", chunk)

		_, err = fmt.Fprintf(res, "data: %s\n\n", chunk)
		if err != nil {
			return fmt.Errorf("error writing chunk '%s': %w", string(chunk), err)
		}

		// Flush the ResponseWriter buffer to send the chunk immediately
		if flusher, ok := res.(http.Flusher); ok {
			flusher.Flush()
		}

		return nil
	}, pubsub.WithChannelNamespace(session.Owner))

	select {
	case <-doneCh:
		consumer.Unsubscribe(context.Background())
		return
	case <-req.Context().Done():
		consumer.Unsubscribe(context.Background())
		return
	}
}

func createChatCompletionChunk(session *types.Session, message string) *types.OpenAIResponse {
	return &types.OpenAIResponse{
		ID:      session.ID,
		Created: int(time.Now().Unix()),
		Model:   string(session.ModelName), // we have to return what the user sent here, due to OpenAI spec.
		Choices: []types.Choice{
			{
				Text:  message,
				Index: 0,
			},
		},
		Object: "chat.completion.chunk",
	}
}

func (apiServer *HelixAPIServer) handleBlockingResponse(res http.ResponseWriter, req *http.Request, session *types.Session) {
	res.Header().Set("Content-Type", "application/json")

	doneCh := make(chan struct{})

	var updatedSession *types.Session

	consumer := apiServer.pubsub.Subscribe(req.Context(), session.ID, func(payload []byte) error {
		var event types.WebsocketEvent
		err := json.Unmarshal(payload, &event)
		if err != nil {
			return fmt.Errorf("error unmarshalling websocket event '%s': %w", string(payload), err)
		}

		if event.Type != "session_update" || event.Session == nil {
			return nil
		}

		if event.Session.Interactions[len(event.Session.Interactions)-1].State == types.InteractionStateComplete {
			// We are done
			updatedSession = event.Session
			close(doneCh)
			return nil
		}

		// Continue reading
		return nil
	}, pubsub.WithChannelNamespace(session.Owner))

	select {
	case <-doneCh:
		consumer.Unsubscribe(context.Background())
		// Continue with response
	case <-req.Context().Done():
		consumer.Unsubscribe(context.Background())
		return
	}

	if updatedSession == nil {
		http.Error(res, "session update not received", http.StatusInternalServerError)
		return
	}

	if updatedSession.Interactions == nil || len(updatedSession.Interactions) == 0 {
		http.Error(res, "session update does not contain any interactions", http.StatusInternalServerError)
		return
	}

	var result []types.Choice

	// Take the last interaction
	interaction := updatedSession.Interactions[len(updatedSession.Interactions)-1]

	result = append(result, types.Choice{
		Message: &types.Message{
			Role:    "assistant",
			Content: interaction.Message,
		},
		FinishReason: "stop",
	})

	resp := &types.OpenAIResponse{
		ID:      session.ID,
		Created: int(time.Now().Unix()),
		Model:   string(session.ModelName), // we have to return what the user sent here, due to OpenAI spec.
		Choices: result,
		Object:  "chat.completion",
		Usage: types.OpenAIUsage{
			// TODO: calculate
			PromptTokens:     0,
			CompletionTokens: 0,
			TotalTokens:      0,
		},
	}

	err := json.NewEncoder(res).Encode(resp)
	if err != nil {
		log.Err(err).Msg("error writing response")
	}
}
