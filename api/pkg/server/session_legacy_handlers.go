package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// startSessionHandler godoc
// @Summary Start new text completion session
// @Description Start new text completion session. Can be used to start or continue a session with the Helix API.
// @Tags    chat

// @Success 200 {object} types.OpenAIResponse
// @Param request    body types.SessionChatRequest true "Request body with the message and model to start chat completion.")
// @Router /api/v1/sessions/chat [post]
// @Security BearerAuth
func (s *HelixAPIServer) startChatSessionLegacyHandler(ctx context.Context, user *types.User, startReq *types.SessionChatRequest, req *http.Request, rw http.ResponseWriter) {

	status, err := s.Controller.GetStatus(ctx, user)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	// Default to text
	if startReq.Type == "" {
		startReq.Type = types.SessionTypeText
	}

	var cfg *startSessionConfig

	if startReq.SessionID == "" {
		if startReq.LoraDir != "" {
			// Basic validation on the lora dir path, it should be something like
			// dev/users/9f2a1f87-b3b8-4e58-9176-32b4861c70e2/sessions/974a8bdc-c1d1-42dc-9a49-7bfa6db112d1/lora/e1c11fba-8d49-4a41-8ae7-60532ab67410
			// this works for both session based file paths and data entity based file paths
			ownerContext := types.OwnerContext{
				Owner:     user.ID,
				OwnerType: user.Type,
			}
			userPath, err := s.Controller.GetFilestoreUserPath(ownerContext, "")
			if err != nil {
				http.Error(rw, err.Error(), http.StatusBadRequest)
				return
			}

			if !strings.HasPrefix(startReq.LoraDir, userPath) {
				http.Error(rw,
					fmt.Sprintf(
						"lora dir path must be within the user's directory (starts with '%s', full path example '%s/sessions/<session_id>/lora/<lora_id>')", userPath, userPath),
					http.StatusBadRequest)
				return
			}
		}

		useModel := startReq.Model

		interactions, err := messagesToInteractions(startReq.Messages)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return
		}

		// this will be assigned if the token being used is an app token
		appID := user.AppID

		if startReq.AppID != "" {
			appID = startReq.AppID
		}

		// or we could be using a normal token and passing the app_id in the query string
		if req.URL.Query().Get("app_id") != "" {
			appID = req.URL.Query().Get("app_id")
		}

		assistantID := "0"

		if startReq.AssistantID != "" {
			assistantID = startReq.AssistantID
		}

		if req.URL.Query().Get("assistant_id") != "" {
			assistantID = req.URL.Query().Get("assistant_id")
		}

		sessionID := system.GenerateSessionID()
		newSession := types.InternalSessionRequest{
			ID:               sessionID,
			Mode:             types.SessionModeInference,
			Type:             startReq.Type,
			ParentApp:        appID,
			AssistantID:      assistantID,
			SystemPrompt:     startReq.SystemPrompt,
			Stream:           startReq.Stream,
			ModelName:        startReq.Model,
			Owner:            user.ID,
			OwnerType:        user.Type,
			LoraDir:          startReq.LoraDir,
			UserInteractions: interactions,
			Priority:         status.Config.StripeSubscriptionActive,
			ActiveTools:      startReq.Tools,
			RAGSourceID:      startReq.RAGSourceID,
		}

		// if we have an app then let's populate the InternalSessionRequest with values from it
		if newSession.ParentApp != "" {
			app, err := s.Store.GetApp(ctx, appID)
			if err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}

			// TODO: support > 1 assistant
			if len(app.Config.Helix.Assistants) <= 0 {
				http.Error(rw, "there are no assistants found in that app", http.StatusBadRequest)
				return
			}

			assistant := data.GetAssistant(app, assistantID)
			if assistant == nil {
				http.Error(rw, fmt.Sprintf("could not find assistant with id %s", assistantID), http.StatusNotFound)
				return
			}

			if assistant.SystemPrompt != "" {
				newSession.SystemPrompt = assistant.SystemPrompt
			}

			if assistant.Model != "" {
				useModel = assistant.Model
			}

			if assistant.RAGSourceID != "" {
				newSession.RAGSourceID = assistant.RAGSourceID
			}

			if assistant.LoraID != "" {
				newSession.LoraID = assistant.LoraID
			}

			if assistant.Type != "" {
				newSession.Type = assistant.Type
			}

			// tools will be assigned by the app inside the controller
			// TODO: refactor so all "get settings from the app" code is in the same place
		}

		// now we add any query params we have gotten
		if req.URL.Query().Get("model") != "" {
			useModel = req.URL.Query().Get("model")
		}

		if req.URL.Query().Get("system_prompt") != "" {
			newSession.SystemPrompt = req.URL.Query().Get("system_prompt")
		}

		if req.URL.Query().Get("rag_source_id") != "" {
			newSession.RAGSourceID = req.URL.Query().Get("rag_source_id")
		}

		if req.URL.Query().Get("lora_id") != "" {
			newSession.LoraID = req.URL.Query().Get("lora_id")
		}

		hasFinetune := startReq.LoraDir != ""
		ragEnabled := newSession.RAGSourceID != ""

		processedModel, err := model.ProcessModelName(string(s.Cfg.Inference.Provider), useModel, types.SessionModeInference, startReq.Type, hasFinetune, ragEnabled)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		newSession.ModelName = processedModel

		// we need to load the rag source and apply the rag settings to the session
		if newSession.RAGSourceID != "" {
			ragSource, err := s.Store.GetDataEntity(ctx, newSession.RAGSourceID)
			if err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}

			newSession.RAGSettings = ragSource.Config.RAGSettings
		}

		// we need to load the lora source and apply the lora settings to the session
		if newSession.LoraID != "" {
			loraSource, err := s.Store.GetDataEntity(ctx, newSession.LoraID)
			if err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}

			newSession.LoraDir = loraSource.Config.FilestorePath
		}

		cfg = &startSessionConfig{
			sessionID: sessionID,
			modelName: newSession.ModelName,
			start: func() error {
				_, err := s.Controller.StartSession(ctx, user, newSession)
				if err != nil {
					return fmt.Errorf("failed to create session: %s", err)
				}
				return nil
			},
		}
	} else {
		existingSession, err := s.Store.GetSession(ctx, startReq.SessionID)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return
		}

		// Existing session
		interactions, err := messagesToInteractions(startReq.Messages)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return
		}

		if len(interactions) != 1 {
			http.Error(rw, "only 1 message is allowed for now", http.StatusBadRequest)
			return
		}

		// Only user interactions are allowed for existing sessions
		if interactions[0].Creator != types.CreatorTypeUser {
			http.Error(rw, "only user interactions are allowed for existing sessions", http.StatusBadRequest)
			return
		}

		cfg = &startSessionConfig{
			sessionID: startReq.SessionID,
			modelName: existingSession.ModelName,
			start: func() error {

				_, err := s.Controller.UpdateSession(ctx, user, types.UpdateSessionRequest{
					SessionID:       startReq.SessionID,
					UserInteraction: interactions[0],
					SessionMode:     types.SessionModeInference,
				})
				if err != nil {
					return fmt.Errorf("failed to update session: %s", err)
				}

				return nil
			},
		}
	}

	if startReq.Stream {
		s.handleStreamingResponse(rw, req, user, cfg)
		return
	}

	s.handleBlockingResponse(rw, req, user, cfg)
}

func messagesToInteractions(messages []*types.Message) ([]*types.Interaction, error) {
	var interactions []*types.Interaction
	for _, m := range messages {
		// Validating roles
		switch m.Role {
		case types.CreatorTypeUser, types.CreatorTypeAssistant, types.CreatorTypeSystem:
			// OK
		default:
			return nil, fmt.Errorf("invalid role '%s', available roles: 'user', 'system', 'assistant'", m.Role)

		}

		if len(m.Content.Parts) != 1 {
			return nil, fmt.Errorf("invalid message content, should only contain 1 entry and it should be a string")

		}

		switch m.Content.Parts[0].(type) {
		case string:
			// OK
		default:
			return nil, fmt.Errorf("invalid message content %v", m.Content.Parts[0])

		}

		var creator types.CreatorType
		switch m.Role {
		case "user":
			creator = types.CreatorTypeUser
		case "system":
			creator = types.CreatorTypeSystem
		case "assistant":
			creator = types.CreatorTypeAssistant
		}

		interaction := &types.Interaction{
			ID:             system.GenerateUUID(),
			Created:        time.Now(),
			Updated:        time.Now(),
			Scheduled:      time.Now(),
			Completed:      time.Now(),
			Creator:        creator,
			Mode:           types.SessionModeInference,
			Message:        m.Content.Parts[0].(string),
			Files:          []string{},
			State:          types.InteractionStateComplete,
			Finished:       true,
			Metadata:       map[string]string{},
			DataPrepChunks: map[string][]types.DataPrepChunk{},
		}

		interactions = append(interactions, interaction)
	}

	return interactions, nil
}

type startSessionConfig struct {
	sessionID string
	modelName string
	start     func() error
}

func (s *HelixAPIServer) handleStreamingResponse(res http.ResponseWriter, req *http.Request, user *types.User, startReq *startSessionConfig) {
	// Set chunking headers
	res.Header().Set("Cache-Control", "no-cache")
	res.Header().Set("Connection", "keep-alive")
	res.Header().Set("Transfer-Encoding", "chunked")
	res.Header().Set("Content-Type", "text/event-stream")

	logger := log.With().Str("session_id", startReq.sessionID).Logger()

	doneCh := make(chan struct{})

	sub, err := s.pubsub.Subscribe(req.Context(), pubsub.GetSessionQueue(user.ID, startReq.sessionID), func(payload []byte) error {
		var event types.WebsocketEvent
		err := json.Unmarshal(payload, &event)
		if err != nil {
			return fmt.Errorf("error unmarshalling websocket event '%s': %w", string(payload), err)
		}

		// this is a special case where if we are using tools then they will not stream
		// but the widget only works with streaming responses right now so we have to
		// do this
		// TODO: make tools work with streaming responses
		if event.Session != nil && event.Session.ParentApp != "" && len(event.Session.Interactions) > 0 {
			// we are inside an app - let's check to see if the last interaction was a tools one
			lastInteraction := event.Session.Interactions[len(event.Session.Interactions)-1]
			_, ok := lastInteraction.Metadata["tool_id"]

			// ok we used a tool
			if ok && lastInteraction.Finished {
				logger.Debug().Msgf("session finished")

				lastChunk := createChatCompletionChunk(startReq.sessionID, startReq.modelName, lastInteraction.Message)
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
		}

		// If we get a worker task response with done=true, we need to send a final chunk
		if event.WorkerTaskResponse != nil && event.WorkerTaskResponse.Done {
			logger.Debug().Msgf("session finished")

			lastChunk := createChatCompletionChunk(startReq.sessionID, startReq.modelName, "")
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
		chunk, err := json.Marshal(createChatCompletionChunk(startReq.sessionID, startReq.modelName, event.WorkerTaskResponse.Message))
		if err != nil {
			return fmt.Errorf("error marshalling websocket event '%+v': %w", event, err)
		}

		err = writeChunk(res, chunk)
		if err != nil {
			return err
		}

		// Flush the stream to ensure the client receives the data immediately
		if flusher, ok := res.(http.Flusher); ok {
			flusher.Flush()
		} else {
			log.Warn().Msg("ResponseWriter does not support Flusher interface")
		}

		return nil
	})
	if err != nil {
		log.Error().Msgf("failed to subscribe to session updates: %v", err)
		return
	}

	// Write first chunk where we present the user with the first message
	// from the assistant
	firstChunk := createChatCompletionChunk(startReq.sessionID, startReq.modelName, "")
	firstChunk.Choices[0].Delta.Role = "assistant"

	respData, err := json.Marshal(firstChunk)
	if err != nil {
		log.Error().Err(err).Msgf("error marshalling chunk: %+v", firstChunk)
		return
	}

	err = writeChunk(res, respData)
	if err != nil {
		log.Error().Err(err).Msg("error writing chunk")
		return
	}

	// Flush the stream to ensure the client receives the data immediately
	if flusher, ok := res.(http.Flusher); ok {
		flusher.Flush()
	} else {
		log.Warn().Msg("ResponseWriter does not support Flusher interface")
	}

	// After subscription, start the session, otherwise
	// we can have race-conditions on very fast responses
	// from the runner
	err = startReq.start()
	if err != nil {
		log.Error().Err(err).Msgf("failed to start session")
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
	} else {
		log.Warn().Msg("ResponseWriter does not support Flusher interface")
	}

	return nil
}

func (s *HelixAPIServer) handleBlockingResponse(res http.ResponseWriter, req *http.Request, user *types.User, startReq *startSessionConfig) {
	res.Header().Set("Content-Type", "application/json")

	doneCh := make(chan struct{})

	var updatedSession *types.Session

	// Wait for the results from the session update. Last event will have the interaction with the full
	// response from the model.
	sub, err := s.pubsub.Subscribe(req.Context(), pubsub.GetSessionQueue(user.ID, startReq.sessionID), func(payload []byte) error {
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

	// NOTE: nil check is unnecessary as len(nil_slice) is defined as 0
	// https://go.dev/ref/spec#Length_and_capacity
	if len(updatedSession.Interactions) == 0 {
		http.Error(res, "session update does not contain any interactions", http.StatusInternalServerError)
		return
	}

	var result []types.Choice

	// Take the last interaction
	interaction := updatedSession.Interactions[len(updatedSession.Interactions)-1]

	result = append(result, types.Choice{
		Message: &types.OpenAIMessage{
			Role:       "assistant", // TODO: this might be "tool"
			Content:    interaction.Message,
			ToolCalls:  interaction.ToolCalls,
			ToolCallID: interaction.ToolCallID,
		},
		FinishReason: "stop",
	})

	resp := &types.OpenAIResponse{
		ID:      startReq.sessionID,
		Created: int(time.Now().Unix()),
		Model:   startReq.modelName, // we have to return what the user sent here, due to OpenAI spec.
		Choices: result,
		Object:  "chat.completion",
		Usage: types.OpenAIUsage{
			// TODO: calculate
			PromptTokens:     interaction.Usage.PromptTokens,
			CompletionTokens: interaction.Usage.CompletionTokens,
			TotalTokens:      interaction.Usage.TotalTokens,
		},
	}

	err = json.NewEncoder(res).Encode(resp)
	if err != nil {
		log.Err(err).Msg("error writing response")
	}
}
