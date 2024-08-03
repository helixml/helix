package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/pubsub"
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

	// Default to text
	if startReq.Type == "" {
		startReq.Type = types.SessionTypeText
	}

	message, ok := startReq.Message()
	if !ok {
		http.Error(rw, "invalid message", http.StatusBadRequest)
		return
	}

	var (
		chatCompletionRequest = openai.ChatCompletionRequest{
			Model: startReq.Model,
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
	)

	if startReq.SessionID != "" {
		session, err := s.Store.GetSession(ctx, startReq.SessionID)
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
	}

	if startReq.Legacy {
		stream, err := s.Controller.ChatCompletionStream(req.Context(), user, chatCompletionRequest, options)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		go func() {
			s.streamUpdates(user, startReq.SessionID, stream)
		}()

		return
	}

	if !startReq.Stream {
		resp, err := s.Controller.ChatCompletion(req.Context(), user, chatCompletionRequest, options)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		return
	}
}

// streamUpdates writes the event to pubsub so user's browser can pick them
// up and update the session in the UI
func (s *HelixAPIServer) streamUpdates(user *types.User, sessionID string, stream *openai.ChatCompletionStream) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			log.Err(err).Msg("error receiving stream")
			return
		}

		bts, err := json.Marshal(&types.WebsocketEvent{
			Type: "worker_task_response",
			Session: &types.Session{
				ID: sessionID,
			},
			WorkerTaskResponse: &types.RunnerTaskResponse{
				Type:    types.WorkerTaskResponseTypeStream,
				Message: response.Choices[0].Delta.Content,
				Done:    false,
			},
		})

		err = s.pubsub.Publish(ctx, pubsub.GetSessionQueue(user.ID, sessionID), bts)
		if err != nil {
			log.Error().Err(err).Msg("failed to publish message")
		}
	}

	// Send the final message that it's done
	bts, err := json.Marshal(&types.WebsocketEvent{
		Type: "worker_task_response",
		Session: &types.Session{
			ID: sessionID,
		},
		WorkerTaskResponse: &types.RunnerTaskResponse{
			Type:    types.WorkerTaskResponseTypeStream,
			Message: "",
			Done:    true,
		},
	})

	err = s.pubsub.Publish(ctx, pubsub.GetSessionQueue(user.ID, sessionID), bts)
	if err != nil {
		log.Error().Err(err).Msg("failed to publish message")
	}

	// Now send the updated session
	bts, err = json.Marshal(&types.WebsocketEvent{
		Type: "session_update",
		Session: &types.Session{
			ID: sessionID,
		},
	})

	err = s.pubsub.Publish(ctx, pubsub.GetSessionQueue(user.ID, sessionID), bts)
	if err != nil {
		log.Error().Err(err).Msg("failed to publish message")
	}

}
