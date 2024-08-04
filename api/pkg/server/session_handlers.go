package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

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

	if ragSourceID := req.URL.Query().Get("assistant_id"); ragSourceID != "" {
		startReq.RAGSourceID = ragSourceID
	}

	if assistantID := req.URL.Query().Get("rag_source_id"); assistantID != "" {
		startReq.AssistantID = assistantID
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
	if startReq.LoraDir != "" {
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
		chatCompletionRequest = openai.ChatCompletionRequest{
			Model: modelName.String(),
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: startReq.SystemPrompt,
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: message,
				},
			},
		}

		options = &controller.ChatCompletionOptions{
			AppID:       startReq.AppID,
			AssistantID: startReq.AssistantID,
			RAGSourceID: startReq.RAGSourceID,
		}

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
				RAGSourceID:  options.RAGSourceID,
				AssistantID:  options.AssistantID,
				Origin: types.SessionOrigin{
					Type: types.SessionOriginTypeUserCreated,
				},
				HelixVersion: data.GetHelixVersion(),
			},
		}

		if options.RAGSourceID != "" {
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

	if startReq.Legacy {
		// Always set to streaming for legacy sessions
		chatCompletionRequest.Stream = true

		stream, err := s.Controller.ChatCompletionStream(ctx, user, chatCompletionRequest, options)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		go func() {
			s.streamUpdates(user, session, stream)
		}()

		sessionDataJSON, err := json.Marshal(session)
		if err != nil {
			http.Error(rw, "failed to marshal session data: "+err.Error(), http.StatusInternalServerError)
			return
		}
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		rw.Write(sessionDataJSON)

		return
	}

	if !startReq.Stream {
		err := s.handleBlockingSession(ctx, user, session, chatCompletionRequest, options, rw)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
		}

		return
	}

	http.Error(rw, "not implemented", http.StatusNotImplemented)
}

func (s *HelixAPIServer) handleBlockingSession(ctx context.Context, user *types.User, session *types.Session, chatCompletionRequest openai.ChatCompletionRequest, options *controller.ChatCompletionOptions, rw http.ResponseWriter) error {
	chatCompletionResponse, err := s.Controller.ChatCompletion(ctx, user, chatCompletionRequest, options)
	if err != nil {
		// Update the session with the response
		session.Interactions[len(session.Interactions)-1].Error = err.Error()
	} else {
		if len(chatCompletionResponse.Choices) == 0 {
			return errors.New("no data in the LLM response")
		}
		// Update the session with the response
		session.Interactions[len(session.Interactions)-1].Message = chatCompletionResponse.Choices[0].Message.Content
	}

	err = s.Controller.WriteSession(session)
	if err != nil {
		return err
	}

	var result []types.Choice

	result = append(result, types.Choice{
		Message: &types.OpenAIMessage{
			Role:       chatCompletionResponse.Choices[0].Message.Role,
			Content:    chatCompletionResponse.Choices[0].Message.Content,
			ToolCalls:  chatCompletionResponse.Choices[0].Message.ToolCalls,
			ToolCallID: chatCompletionResponse.Choices[0].Message.ToolCallID,
		},
		FinishReason: "stop",
	})

	resp := &types.OpenAIResponse{
		ID:      session.ID,
		Created: int(time.Now().Unix()),
		Model:   chatCompletionRequest.Model, // we have to return what the user sent here, due to OpenAI spec.
		Choices: result,
		Object:  "chat.completion",
		Usage: types.OpenAIUsage{
			PromptTokens:     chatCompletionResponse.Usage.PromptTokens,
			CompletionTokens: chatCompletionResponse.Usage.CompletionTokens,
			TotalTokens:      chatCompletionResponse.Usage.TotalTokens,
		},
	}

	err = json.NewEncoder(rw).Encode(resp)
	if err != nil {
		log.Err(err).Msg("error writing response")
	}

	return nil
}

// streamUpdates writes the event to pubsub so user's browser can pick them
// up and update the session in the UI
func (s *HelixAPIServer) streamUpdates(user *types.User, session *types.Session, stream *openai.ChatCompletionStream) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

		// Accumulate the response
		responseMessage += response.Choices[0].Delta.Content

		bts, err := json.Marshal(&types.WebsocketEvent{
			Type: "worker_task_response",
			Session: &types.Session{
				ID: session.ID,
			},
			WorkerTaskResponse: &types.RunnerTaskResponse{
				Type:    types.WorkerTaskResponseTypeStream,
				Message: response.Choices[0].Delta.Content,
				Done:    false,
			},
		})

		err = s.pubsub.Publish(ctx, pubsub.GetSessionQueue(user.ID, session.ID), bts)
		if err != nil {
			log.Error().Err(err).Msg("failed to publish message")
		}
	}

	// Send the final message that it's done
	bts, err := json.Marshal(&types.WebsocketEvent{
		Type: "worker_task_response",
		Session: &types.Session{
			ID: session.ID,
		},
		WorkerTaskResponse: &types.RunnerTaskResponse{
			Type:    types.WorkerTaskResponseTypeStream,
			Message: "",
			Done:    true,
		},
	})

	err = s.pubsub.Publish(ctx, pubsub.GetSessionQueue(user.ID, session.ID), bts)
	if err != nil {
		log.Error().Err(err).Msg("failed to publish message")
	}

	// Update last interaction
	session.Interactions[len(session.Interactions)-1].Message = responseMessage

	s.Controller.WriteSession(session)
}
