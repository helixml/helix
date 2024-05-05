package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
)

const (
	BYTE = 1 << (10 * iota)
	KILOBYTE
	MEGABYTE
)

// https://platform.openai.com/docs/api-reference/chat/create
// POST https://app.tryhelix.ai//v1/chat/completions
func (apiServer *HelixAPIServer) createChatCompletion(res http.ResponseWriter, req *http.Request) {
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

	var interactions []*types.Interaction

	for _, m := range chatCompletionRequest.Messages {
		// Validating roles
		switch m.Role {
		case "user", "system", "assistant":
			// OK
		default:
			http.Error(res, "invalid role, available roles: 'user', 'system', 'assistant'", http.StatusBadRequest)
			return
		}

		var creator types.CreatorType
		switch m.Role {
		case "user":
			creator = types.CreatorTypeUser
		case "system":
			creator = types.CreatorTypeSystem
		}

		interaction := &types.Interaction{
			ID:             system.GenerateUUID(),
			Created:        time.Now(),
			Updated:        time.Now(),
			Scheduled:      time.Now(),
			Completed:      time.Now(),
			Creator:        creator,
			Mode:           sessionMode,
			Message:        m.Content,
			Files:          []string{},
			State:          types.InteractionStateComplete,
			Finished:       true,
			Metadata:       map[string]string{},
			DataPrepChunks: map[string][]types.DataPrepChunk{},
		}

		interactions = append(interactions, interaction)
	}

	newSession := types.CreateSessionRequest{
		SessionID:        sessionID,
		SessionMode:      sessionMode,
		SessionType:      types.SessionTypeText,
		Stream:           chatCompletionRequest.Stream,
		ModelName:        types.ModelName(chatCompletionRequest.Model),
		Owner:            userContext.Owner,
		OwnerType:        userContext.OwnerType,
		UserInteractions: interactions,
		Priority:         status.Config.StripeSubscriptionActive,
		ActiveTools:      []string{},
	}

	if chatCompletionRequest.ResponseFormat != nil {
		newSession.ResponseFormat = types.ResponseFormat{
			Type:   types.ResponseFormatType(chatCompletionRequest.ResponseFormat.Type),
			Schema: chatCompletionRequest.ResponseFormat.Schema,
		}
	}

	startReq := &startSessionConfig{
		sessionID: sessionID,
		modelName: chatCompletionRequest.Model,
		start: func() error {
			_, err := apiServer.Controller.StartSession(userContext, newSession)
			return err
		},
	}

	if chatCompletionRequest.Stream {
		apiServer.handleStreamingResponse(res, req, userContext, startReq)
		return
	}

	apiServer.handleBlockingResponse(res, req, userContext, startReq)
}

type startSessionConfig struct {
	sessionID string
	modelName string
	start     func() error
}

func (apiServer *HelixAPIServer) handleStreamingResponse(res http.ResponseWriter, req *http.Request, userContext types.RequestContext, startReq *startSessionConfig) {
	// Set chunking headers
	res.Header().Set("Cache-Control", "no-cache")
	res.Header().Set("Connection", "keep-alive")
	res.Header().Set("Transfer-Encoding", "chunked")
	res.Header().Set("Content-Type", "text/event-stream")

	logger := log.With().Str("session_id", startReq.sessionID).Logger()

	doneCh := make(chan struct{})

	sub, err := apiServer.pubsub.Subscribe(req.Context(), pubsub.GetSessionQueue(userContext.Owner, startReq.sessionID), func(payload []byte) error {
		var event types.WebsocketEvent
		err := json.Unmarshal(payload, &event)
		if err != nil {
			return fmt.Errorf("error unmarshalling websocket event '%s': %w", string(payload), err)
		}

		// If we get a worker task response with done=true, we need to send a final chunk
		if event.WorkerTaskResponse != nil && event.WorkerTaskResponse.Done {
			logger.Debug().Msgf("session finished")

			lastChunk := createChatCompletionChunk(startReq.sessionID, string(startReq.modelName), "")
			lastChunk.Choices[0].FinishReason = "stop"

			respData, err := json.Marshal(lastChunk)
			if err != nil {
				return fmt.Errorf("error marshalling websocket event '%+v': %w", event, err)
			}

			err = writeChunk(res, respData)
			if err != nil {
				return err
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
		chunk, err := json.Marshal(createChatCompletionChunk(startReq.sessionID, string(startReq.modelName), event.WorkerTaskResponse.Message))
		if err != nil {
			return fmt.Errorf("error marshalling websocket event '%+v': %w", event, err)
		}

		err = writeChunk(res, chunk)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		system.NewHTTPError500("failed to subscribe to session updates: %s", err)
		return
	}

	// Write first chunk where we present the user with the first message
	// from the assistant
	firstChunk := createChatCompletionChunk(startReq.sessionID, string(startReq.modelName), "")
	firstChunk.Choices[0].Delta.Role = "assistant"

	respData, err := json.Marshal(firstChunk)
	if err != nil {
		system.NewHTTPError500("error marshalling websocket event '%+v': %s", firstChunk, err)
		return
	}

	err = writeChunk(res, respData)
	if err != nil {
		system.NewHTTPError500("error writing chunk '%s': %s", string(respData), err)
		return
	}

	// After subscription, start the session, otherwise
	// we can have race-conditions on very fast responses
	// from the runner
	err = startReq.start()

	if err != nil {
		system.NewHTTPError500("failed to start session: %s", err)
		return
	}

	select {
	case <-doneCh:
		_ = sub.Unsubscribe()
		return
	case <-req.Context().Done():
		_ = sub.Unsubscribe()
		return
	}
}

// Ref: https://platform.openai.com/docs/api-reference/chat/streaming
// Example:
// {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1694268190,"model":"gpt-3.5-turbo-0613", "system_fingerprint": "fp_44709d6fcb", "choices":[{"index":0,"delta":{"role":"assistant","content":""},"logprobs":null,"finish_reason":null}]}

func createChatCompletionChunk(sessionID, modelName, message string) *types.OpenAIResponse {
	return &types.OpenAIResponse{
		ID:      sessionID,
		Created: int(time.Now().Unix()),
		Model:   modelName, // we have to return what the user sent here, due to OpenAI spec.
		Choices: []types.Choice{
			{
				// Text: message,
				Delta: &types.OpenAIMessage{
					Content: message,
				},
				Index: 0,
			},
		},
		Object: "chat.completion.chunk",
	}
}

func writeChunk(w io.Writer, chunk []byte) error {
	_, err := fmt.Fprintf(w, "data: %s\n\n", string(chunk))
	if err != nil {
		return fmt.Errorf("error writing chunk '%s': %w", string(chunk), err)
	}

	// Flush the ResponseWriter buffer to send the chunk immediately
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	return nil
}

func (apiServer *HelixAPIServer) handleBlockingResponse(res http.ResponseWriter, req *http.Request, userContext types.RequestContext, startReq *startSessionConfig) {
	res.Header().Set("Content-Type", "application/json")

	doneCh := make(chan struct{})

	var updatedSession *types.Session

	// Wait for the results from the session update. Last event will have the interaction with the full
	// response from the model.
	sub, err := apiServer.pubsub.Subscribe(req.Context(), pubsub.GetSessionQueue(userContext.Owner, startReq.sessionID), func(payload []byte) error {
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
	})
	if err != nil {
		log.Err(err).Msg("failed to subscribe to session updates")

		http.Error(res, fmt.Sprintf("failed to subscribe to session updates: %s", err), http.StatusInternalServerError)
		return
	}

	// After subscription, start the session, otherwise
	// we can have race-conditions on very fast responses
	// from the runner
	err = startReq.start()
	if err != nil {
		log.Err(err).Msg("failed to start session")

		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}

	select {
	case <-doneCh:
		_ = sub.Unsubscribe()
		// Continue with response
	case <-req.Context().Done():
		_ = sub.Unsubscribe()
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
		Message: &types.OpenAIMessage{
			Role:    "assistant",
			Content: interaction.Message,
		},
		FinishReason: "stop",
	})

	resp := &types.OpenAIResponse{
		ID:      startReq.sessionID,
		Created: int(time.Now().Unix()),
		Model:   string(startReq.modelName), // we have to return what the user sent here, due to OpenAI spec.
		Choices: result,
		Object:  "chat.completion",
		Usage: types.OpenAIUsage{
			// TODO: calculate
			PromptTokens:     0,
			CompletionTokens: 0,
			TotalTokens:      0,
		},
	}

	err = json.NewEncoder(res).Encode(resp)
	if err != nil {
		log.Err(err).Msg("error writing response")
	}
}
