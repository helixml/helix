package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/data"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/lukemarsden/go-openai2"

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
func (s *HelixAPIServer) startChatSessionHandler(rw http.ResponseWriter, req *http.Request) {

	var startReq types.SessionChatRequest
	err := json.NewDecoder(io.LimitReader(req.Body, 10*MEGABYTE)).Decode(&startReq)
	if err != nil {
		http.Error(rw, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Allow overriding from URL queries
	if appID := req.URL.Query().Get("app_id"); appID != "" {
		startReq.AppID = appID
	}

	if ragSourceID := req.URL.Query().Get("rag_source_id"); ragSourceID != "" {
		startReq.RAGSourceID = ragSourceID
	}

	if assistantID := req.URL.Query().Get("assistant_id"); assistantID != "" {
		startReq.AssistantID = assistantID
	}

	// if the app specifies a model, override startReq.Model so that we display
	// the correct model in the UI (and some things may rely on it)
	if startReq.AppID != "" {
		// load the app
		app, err := s.Store.GetApp(req.Context(), startReq.AppID)
		if err != nil {
			log.Error().Err(err).Str("app_id", startReq.AppID).Msg("Failed to load app")
			http.Error(rw, "Failed to load app: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// If an AssistantID is specified, get the correct assistant from the app
		if startReq.AssistantID != "" {
			var assistant *types.AssistantConfig
			assistantID := startReq.AssistantID
			if assistantID == "" {
				assistantID = "0"
			}
			assistant = data.GetAssistant(app, assistantID)

			// Update the model if the assistant has one
			if assistant.Model != "" {
				startReq.Model = assistant.Model
			}
		}
	}

	if len(startReq.Messages) == 0 {
		http.Error(rw, "messages must not be empty", http.StatusBadRequest)
		return
	}

	// If more than 1, also not allowed just yet for simplification
	if len(startReq.Messages) > 1 {
		http.Error(rw, "only 1 message is allowed for now", http.StatusBadRequest)
		return
	}

	ctx := req.Context()
	user := getRequestUser(req)

	// For finetunes, legacy route
	if startReq.LoraDir != "" || startReq.Type == types.SessionTypeImage {
		s.startChatSessionLegacyHandler(req.Context(), user, &startReq, req, rw)
		return
	}

	modelName, err := types.ProcessModelName(string(s.Cfg.Inference.Provider), startReq.Model, types.SessionModeInference, types.SessionTypeText, false, false)
	if err != nil {
		http.Error(rw, "invalid model name: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Default to text
	if startReq.Type == "" {
		startReq.Type = types.SessionTypeText
	}

	if startReq.SystemPrompt == "" {
		startReq.SystemPrompt = "You are a helpful assistant."
	}

	message, ok := startReq.Message()
	if !ok {
		http.Error(rw, "invalid message", http.StatusBadRequest)
		return
	}

	var (
		session *types.Session
	)

	if startReq.SessionID != "" {
		session, err = s.Store.GetSession(ctx, startReq.SessionID)
		if err != nil {
			http.Error(rw, fmt.Sprintf("failed to get session %s, error: %s", startReq.SessionID, err), http.StatusInternalServerError)
			return
		}

		if session.Owner != user.ID {
			http.Error(rw, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		// If the session has an AppID, use it as the next interaction
		if session.ParentApp != "" {
			startReq.AppID = session.ParentApp
		}
	} else {
		// Create session
		session = &types.Session{
			ID:        system.GenerateSessionID(),
			Name:      system.GenerateAmusingName(),
			Created:   time.Now(),
			Updated:   time.Now(),
			Mode:      types.SessionModeInference,
			Type:      types.SessionTypeText,
			ModelName: types.ModelName(startReq.Model),
			ParentApp: startReq.AppID,
			Owner:     user.ID,
			OwnerType: user.Type,
			Metadata: types.SessionMetadata{
				Stream:       startReq.Stream,
				SystemPrompt: startReq.SystemPrompt,
				RAGSourceID:  startReq.RAGSourceID,
				AssistantID:  startReq.AssistantID,
				Origin: types.SessionOrigin{
					Type: types.SessionOriginTypeUserCreated,
				},
				HelixVersion: data.GetHelixVersion(),
			},
		}

		if startReq.RAGSourceID != "" {
			session.Metadata.RagEnabled = true
		}
	}

	session.Interactions = append(session.Interactions,
		&types.Interaction{
			ID:        system.GenerateUUID(),
			Created:   time.Now(),
			Updated:   time.Now(),
			Scheduled: time.Now(),
			Completed: time.Now(),
			Mode:      types.SessionModeInference,
			Creator:   types.CreatorTypeUser,
			State:     types.InteractionStateComplete,
			Finished:  true,
			Message:   message,
		},
		&types.Interaction{
			ID:       system.GenerateUUID(),
			Created:  time.Now(),
			Updated:  time.Now(),
			Creator:  types.CreatorTypeSystem,
			Mode:     types.SessionModeInference,
			Message:  "",
			State:    types.InteractionStateWaiting,
			Finished: false,
			Metadata: map[string]string{},
		},
	)

	// Write the initial session that has the user prompt and also the placeholder interaction
	// for the system response which will be updated later once the response is received
	err = s.Controller.WriteSession(session)
	if err != nil {
		http.Error(rw, "failed to write session: "+err.Error(), http.StatusInternalServerError)
		return
	}

	ctx = oai.SetContextValues(context.Background(), user.ID, session.ID, session.Interactions[0].ID)

	var (
		chatCompletionRequest = openai.ChatCompletionRequest{
			Model: modelName.String(),
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: startReq.SystemPrompt,
				},
			},
		}

		options = &controller.ChatCompletionOptions{
			AppID:       startReq.AppID,
			AssistantID: startReq.AssistantID,
			RAGSourceID: startReq.RAGSourceID,
		}
	)

	// Convert interactions (except the last one) to messages
	for _, interaction := range session.Interactions[:len(session.Interactions)-1] {
		chatCompletionRequest.Messages = append(chatCompletionRequest.Messages, openai.ChatCompletionMessage{
			Role:    string(interaction.Creator),
			Content: interaction.Message,
		})
	}

	// TODO: remove once frontend is removed
	if startReq.Legacy {
		s.legacyChatCompletionStream(ctx, user, session, chatCompletionRequest, options, rw)
		return
	}
	// End of TODO

	if !startReq.Stream {
		err := s.handleBlockingSession(ctx, user, session, chatCompletionRequest, options, rw)
		if err != nil {
			log.Err(err).Msg("error handling blocking session")
		}
		return
	}

	err = s.handleStreamingSession(ctx, user, session, chatCompletionRequest, options, rw)
	if err != nil {
		log.Err(err).Msg("error handling blocking session")
	}
	return
}

func (s *HelixAPIServer) restartChatSessionHandler(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	user := getRequestUser(req)
	session_id := mux.Vars(req)["id"]
	session, err := s.Store.GetSession(ctx, session_id)
	if err != nil {
		http.Error(rw, fmt.Sprintf("failed to get session %s, error: %s", session_id, err), http.StatusInternalServerError)
		return
	}

	modelName, e := types.ProcessModelName(string(s.Cfg.Inference.Provider), session.ModelName.String(), types.SessionModeInference, types.SessionTypeText, false, false)
	if e != nil {
		http.Error(rw, "invalid model name: "+err.Error(), http.StatusBadRequest)
		return
	}
	if modelName.String() != session.ModelName.String() {
		session.ModelName = modelName
	}

	// Restart the previous interaction
	if len(session.Interactions) > 0 {
		lastInteraction := session.Interactions[len(session.Interactions)-1]
		lastInteraction.State = types.InteractionStateWaiting
		lastInteraction.Completed = time.Time{}
		lastInteraction.Finished = false
		lastInteraction.Error = ""
		lastInteraction.Message = ""
	}

	// Update the session
	err = s.Controller.WriteSession(session)
	if err != nil {
		http.Error(rw, "failed to write session: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert interactions (except the last one) to messages
	var (
		chatCompletionRequest = openai.ChatCompletionRequest{
			Model: modelName.String(),
		}

		options = &controller.ChatCompletionOptions{
			AppID:       session.ParentApp,
			AssistantID: session.Metadata.AssistantID,
			RAGSourceID: session.Metadata.RAGSourceID,
			QueryParams: session.Metadata.AppQueryParams,
		}
	)
	for _, interaction := range session.Interactions[:len(session.Interactions)-1] {
		chatCompletionRequest.Messages = append(chatCompletionRequest.Messages, openai.ChatCompletionMessage{
			Role:    string(interaction.Creator),
			Content: interaction.Message,
		})
	}

	// Set required context values
	ctx = oai.SetContextValues(context.Background(), user.ID, session.ID, session.Interactions[len(session.Interactions)-1].ID)

	// TODO: This uses the "old style" frontend websocket stream, copied from startChatSessionHandler
	s.legacyChatCompletionStream(ctx, user, session, chatCompletionRequest, options, rw)
}

func (s *HelixAPIServer) legacyChatCompletionStream(ctx context.Context, user *types.User, session *types.Session, chatCompletionRequest openai.ChatCompletionRequest, options *controller.ChatCompletionOptions, rw http.ResponseWriter) {
	stream, err := s.Controller.ChatCompletionStream(ctx, user, chatCompletionRequest, options)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	go func() {
		// we log the updated request here because the controller mutates it
		// when doing e.g. tools calls and RAG
		s.legacyStreamUpdates(user, session, stream)
	}()

	sessionDataJSON, err := json.Marshal(session)
	if err != nil {
		http.Error(rw, "failed to marshal session data: "+err.Error(), http.StatusInternalServerError)
		return
	}
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	rw.Write(sessionDataJSON)
}

func (s *HelixAPIServer) handleBlockingSession(ctx context.Context, user *types.User, session *types.Session, chatCompletionRequest openai.ChatCompletionRequest, options *controller.ChatCompletionOptions, rw http.ResponseWriter) error {
	// Ensure request is not streaming
	chatCompletionRequest.Stream = false

	// Call the LLM
	chatCompletionResponse, err := s.Controller.ChatCompletion(ctx, user, chatCompletionRequest, options)
	if err != nil {
		// Update the session with the response
		session.Interactions[len(session.Interactions)-1].Error = err.Error()
		session.Interactions[len(session.Interactions)-1].State = types.InteractionStateError
		writeErr := s.Controller.WriteSession(session)
		if writeErr != nil {
			return fmt.Errorf("error writing session: %w", writeErr)
		}

		http.Error(rw, fmt.Sprintf("error running LLM: %s", err.Error()), http.StatusInternalServerError)
		return nil
	}

	if len(chatCompletionResponse.Choices) == 0 {
		return errors.New("no data in the LLM response")
	}

	// Update the session with the response
	session.Interactions[len(session.Interactions)-1].Message = chatCompletionResponse.Choices[0].Message.Content
	session.Interactions[len(session.Interactions)-1].Completed = time.Now()
	session.Interactions[len(session.Interactions)-1].State = types.InteractionStateComplete
	session.Interactions[len(session.Interactions)-1].Finished = true

	err = s.Controller.WriteSession(session)
	if err != nil {
		return err
	}

	chatCompletionResponse.ID = session.ID

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	err = json.NewEncoder(rw).Encode(chatCompletionResponse)
	if err != nil {
		log.Err(err).Msg("error writing response")
	}

	return nil
}

func (s *HelixAPIServer) handleStreamingSession(ctx context.Context, user *types.User, session *types.Session, chatCompletionRequest openai.ChatCompletionRequest, options *controller.ChatCompletionOptions, rw http.ResponseWriter) error {
	// Ensure request is streaming
	chatCompletionRequest.Stream = true
	// Call the LLM
	stream, err := s.Controller.ChatCompletionStream(ctx, user, chatCompletionRequest, options)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return nil
	}
	defer stream.Close()

	rw.Header().Set("Content-Type", "text/event-stream")
	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Set("Connection", "keep-alive")

	var fullResponse string

	// Write the stream into the response
	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return err
		}

		// Accumulate the response
		if len(response.Choices) > 0 {
			fullResponse += response.Choices[0].Delta.Content
		}
		// Update the response with the interaction ID
		response.ID = session.ID

		// Write the response to the client
		bts, err := json.Marshal(response)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return err
		}

		writeChunk(rw, bts)
	}

	// Update last interaction
	session.Interactions[len(session.Interactions)-1].Message = fullResponse
	session.Interactions[len(session.Interactions)-1].Completed = time.Now()
	session.Interactions[len(session.Interactions)-1].State = types.InteractionStateComplete
	session.Interactions[len(session.Interactions)-1].Finished = true

	return s.Controller.WriteSession(session)
}

// legacyStreamUpdates writes the event to pubsub so user's browser can pick them
// up and update the session in the UI
func (s *HelixAPIServer) legacyStreamUpdates(user *types.User, session *types.Session, stream *controller.StreamLogger) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	interactionID := session.Interactions[len(session.Interactions)-1].ID

	var responseMessage string

	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			log.Err(err).Msg("error receiving stream")
			return
		}

		var messageContent string

		// Accumulate the response
		if len(response.Choices) > 0 {
			messageContent = response.Choices[0].Delta.Content
		}

		responseMessage += messageContent

		bts, err := json.Marshal(&types.WebsocketEvent{
			Type:      "worker_task_response",
			SessionID: session.ID,
			Session: &types.Session{
				ID: session.ID,
			},
			WorkerTaskResponse: &types.RunnerTaskResponse{
				Owner:         user.ID,
				Type:          types.WorkerTaskResponseTypeStream,
				SessionID:     session.ID,
				InteractionID: interactionID,
				Message:       messageContent,
				Done:          false,
			},
		})

		err = s.pubsub.Publish(ctx, pubsub.GetSessionQueue(user.ID, session.ID), bts)
		if err != nil {
			log.Error().Err(err).Msg("failed to publish message")
		}
	}

	// Send the final message that it's done
	bts, err := json.Marshal(&types.WebsocketEvent{
		Type:      "worker_task_response",
		SessionID: session.ID,
		Session: &types.Session{
			ID: session.ID,
		},
		WorkerTaskResponse: &types.RunnerTaskResponse{
			Owner:         user.ID,
			SessionID:     session.ID,
			InteractionID: interactionID,
			Type:          types.WorkerTaskResponseTypeStream,
			Message:       "",
			Done:          true,
		},
	})

	err = s.pubsub.Publish(ctx, pubsub.GetSessionQueue(user.ID, session.ID), bts)
	if err != nil {
		log.Error().Err(err).Msg("failed to publish message")
	}

	// Update last interaction
	session.Interactions[len(session.Interactions)-1].Message = responseMessage
	session.Interactions[len(session.Interactions)-1].Completed = time.Now()
	session.Interactions[len(session.Interactions)-1].State = types.InteractionStateComplete
	session.Interactions[len(session.Interactions)-1].Finished = true

	s.Controller.WriteSession(session)
}
