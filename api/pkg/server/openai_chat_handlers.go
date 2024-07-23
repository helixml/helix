package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"

	openai "github.com/lukemarsden/go-openai2"
	"github.com/rs/zerolog/log"
)

const (
	BYTE = 1 << (10 * iota)
	KILOBYTE
	MEGABYTE
)

// https://platform.openai.com/docs/api-reference/chat/create
// POST https://app.tryhelix.ai//v1/chat/completions
func (apiServer *HelixAPIServer) createChatCompletion(rw http.ResponseWriter, r *http.Request) {
	addCorsHeaders(rw)
	if r.Method == "OPTIONS" {
		return
	}

	user := getRequestUser(r)

	if !hasUser(user) {
		http.Error(rw, "unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10*MEGABYTE))
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	var chatCompletionRequest openai.ChatCompletionRequest
	err = json.Unmarshal(body, &chatCompletionRequest)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	options := &controller.ChatCompletionOptions{}

	if chatCompletionRequest.Stream {
		stream, err := apiServer.Controller.ChatCompletionStream(r.Context(), user, chatCompletionRequest, options)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		// Write the stream into the response

		return
	}

	// Non-streaming request
	resp, err := apiServer.Controller.ChatCompletion(r.Context(), user, chatCompletionRequest, options)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(rw).Encode(resp)
	if err != nil {
		log.Err(err).Msg("error writing response")
	}
}

// https://platform.openai.com/docs/api-reference/chat/create
// POST https://app.tryhelix.ai//v1/chat/completions
func (apiServer *HelixAPIServer) _createChatCompletion(res http.ResponseWriter, req *http.Request) {
	addCorsHeaders(res)
	if req.Method == "OPTIONS" {
		return
	}

	ctx := req.Context()
	user := getRequestUser(req)

	if !hasUser(user) {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

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

	status, err := apiServer.Controller.GetStatus(ctx, user)
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}

	sessionID := system.GenerateUUID()

	sessionMode := types.SessionModeInference

	var interactions []*types.Interaction

	for _, m := range chatCompletionRequest.Messages {
		// Filter out roles that are not user/system/assistant
		switch m.Role {
		case "user", "system", "assistant":
			// OK
		default:
			continue
		}

		var creator types.CreatorType
		switch m.Role {
		case "user":
			creator = types.CreatorTypeUser
		case "system":
			creator = types.CreatorTypeSystem
		case "tool":
			creator = types.CreatorTypeTool
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
			ToolCalls:      m.ToolCalls,
			ToolCallID:     m.ToolCallID,
		}

		interactions = append(interactions, interaction)
	}

	// this will be assigned if the token being used is an app token
	appID := user.AppID

	// or we could be using a normal token and passing the app_id in the query string
	if req.URL.Query().Get("app_id") != "" {
		appID = req.URL.Query().Get("app_id")
	}

	assistantID := "0"
	if req.URL.Query().Get("assistant_id") != "" {
		assistantID = req.URL.Query().Get("assistant_id")
	}

	newSession := types.InternalSessionRequest{
		ID:               sessionID,
		Mode:             sessionMode,
		Type:             types.SessionTypeText,
		ParentApp:        appID,
		AssistantID:      assistantID,
		Stream:           chatCompletionRequest.Stream,
		Owner:            user.ID,
		OwnerType:        user.Type,
		UserInteractions: interactions,
		Priority:         status.Config.StripeSubscriptionActive,
		ActiveTools:      []string{},
		AppQueryParams:   map[string]string{},
		Tools:            chatCompletionRequest.Tools,
	}

	useModel := chatCompletionRequest.Model

	// if we have an app then let's populate the InternalSessionRequest with values from it
	if newSession.ParentApp != "" {
		app, err := apiServer.Store.GetApp(ctx, newSession.ParentApp)
		if err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}

		// TODO: support > 1 assistant
		if len(app.Config.Helix.Assistants) <= 0 {
			http.Error(res, "there are no assistants found in that app", http.StatusBadRequest)
			return
		}

		assistant := data.GetAssistant(app, assistantID)

		if assistant == nil {
			http.Error(res, fmt.Sprintf("could not find assistant with id %s", assistantID), http.StatusNotFound)
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

		// Check to see if the user is passing any app parameters
		for k, v := range req.URL.Query() {
			newSession.AppQueryParams[k] = strings.Join(v, ",")
		}

		// tools will be assigned by the app inside the controller
		// TODO: refactor so all "get settings from the app" code is in the same place
	}

	// now we add any query params we have gotten
	if req.URL.Query().Get("model") != "" {
		useModel = req.URL.Query().Get("model")
	}

	if useModel == "" {
		http.Error(res, "model not specified", http.StatusBadRequest)
		return
	}

	if req.URL.Query().Get("system_prompt") != "" {
		newSession.SystemPrompt = req.URL.Query().Get("system_prompt")
	}

	if req.URL.Query().Get("rag_source_id") != "" {
		newSession.RAGSourceID = req.URL.Query().Get("rag_source_id")
	}

	// we need to load the rag source and apply the rag settings to the session
	if newSession.RAGSourceID != "" {
		ragSource, err := apiServer.Store.GetDataEntity(req.Context(), newSession.RAGSourceID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.Error(res, fmt.Sprintf("RAG source '%s' not found", newSession.RAGSourceID), http.StatusBadRequest)
				return
			}
			http.Error(res, fmt.Sprintf("failed to get RAG source ID '%s', error: %s", newSession.RAGSourceID, err.Error()), http.StatusInternalServerError)
			return
		}
		newSession.RAGSettings = ragSource.Config.RAGSettings
	}

	if req.URL.Query().Get("lora_id") != "" {
		newSession.LoraID = req.URL.Query().Get("lora_id")
	}

	hasFinetune := newSession.LoraID != ""
	ragEnabled := newSession.RAGSourceID != ""

	// this handles all the defaults and alising for the processedModel name
	processedModel, err := types.ProcessModelName(useModel, types.SessionModeInference, types.SessionTypeText, hasFinetune, ragEnabled)
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	newSession.ModelName = processedModel

	if chatCompletionRequest.ResponseFormat != nil {
		newSession.ResponseFormat = types.ResponseFormat{
			Type:   types.ResponseFormatType(chatCompletionRequest.ResponseFormat.Type),
			Schema: chatCompletionRequest.ResponseFormat.Schema,
		}
	}

	if chatCompletionRequest.Tools != nil {
		newSession.Tools = chatCompletionRequest.Tools
	}

	if chatCompletionRequest.ToolChoice != nil {
		newSession.ToolChoice = chatCompletionRequest.ToolChoice
	}

	startReq := &startSessionConfig{
		sessionID: sessionID,
		modelName: chatCompletionRequest.Model,
		start: func() error {
			_, err := apiServer.Controller.StartSession(ctx, user, newSession)
			return err
		},
	}

	if chatCompletionRequest.Stream {
		apiServer.handleStreamingResponse(res, req, user, startReq)
		return
	}

	apiServer.handleBlockingResponse(res, req, user, startReq)
}

type startSessionConfig struct {
	sessionID string
	modelName string
	start     func() error
}

func (apiServer *HelixAPIServer) handleStreamingResponse(res http.ResponseWriter, req *http.Request, user *types.User, startReq *startSessionConfig) {
	// Set chunking headers
	res.Header().Set("Cache-Control", "no-cache")
	res.Header().Set("Connection", "keep-alive")
	res.Header().Set("Transfer-Encoding", "chunked")
	res.Header().Set("Content-Type", "text/event-stream")

	logger := log.With().Str("session_id", startReq.sessionID).Logger()

	doneCh := make(chan struct{})

	sub, err := apiServer.pubsub.Subscribe(req.Context(), pubsub.GetSessionQueue(user.ID, startReq.sessionID), func(payload []byte) error {
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

				lastChunk := createChatCompletionChunk(startReq.sessionID, string(startReq.modelName), lastInteraction.Message)
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

func (apiServer *HelixAPIServer) handleBlockingResponse(res http.ResponseWriter, req *http.Request, user *types.User, startReq *startSessionConfig) {
	res.Header().Set("Content-Type", "application/json")

	doneCh := make(chan struct{})

	var updatedSession *types.Session

	// Wait for the results from the session update. Last event will have the interaction with the full
	// response from the model.
	sub, err := apiServer.pubsub.Subscribe(req.Context(), pubsub.GetSessionQueue(user.ID, startReq.sessionID), func(payload []byte) error {
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
		Model:   string(startReq.modelName), // we have to return what the user sent here, due to OpenAI spec.
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
