package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// startSessionHandler godoc
// @Summary Start new text completion session
// @Description Start new text completion session. Can be used to start or continue a session with the Helix API.
// @Tags    chat

// @Success 200 {object} types.OpenAIResponse
// @Param request    body types.SessionChatRequest true "Request body with the message and model to start chat completion.")
// @Router /api/v1/sessions/chat [post]
// @Security BearerAuth
func (s *HelixAPIServer) startSessionHandler(rw http.ResponseWriter, req *http.Request) {

	var startReq types.SessionChatRequest
	err := json.NewDecoder(io.LimitReader(req.Body, 10*MEGABYTE)).Decode(&startReq)
	if err != nil {
		http.Error(rw, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
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

	userContext := s.getRequestContext(req)

	status, err := s.Controller.GetStatus(userContext)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	if startReq.Model == "" {
		startReq.Model = string(types.Model_Axolotl_Mistral7b)
	}

	// Default to text
	if startReq.Type == "" {
		startReq.Type = types.SessionTypeText
	}

	var cfg *startSessionConfig

	if startReq.SessionID == "" {
		interactions, err := messagesToInteractions(startReq.Messages)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return
		}

		sessionID := system.GenerateSessionID()
		newSession := types.CreateSessionRequest{
			SessionID:        sessionID,
			SessionMode:      types.SessionModeInference,
			SessionType:      startReq.Type,
			SystemPrompt:     startReq.SystemPrompt,
			ModelName:        types.ModelName(startReq.Model),
			Owner:            userContext.Owner,
			OwnerType:        userContext.OwnerType,
			LoraDir:          startReq.LoraDir,
			UserInteractions: interactions,
			Priority:         status.Config.StripeSubscriptionActive,
			ActiveTools:      startReq.Tools,
		}

		cfg = &startSessionConfig{
			sessionID: sessionID,
			modelName: startReq.Model,
			start: func() error {
				_, err := s.Controller.CreateSession(userContext, newSession)
				return fmt.Errorf("failed to create session: %s", err)
			},
		}
	} else {
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
			modelName: startReq.Model,
			start: func() error {

				_, err := s.Controller.UpdateSession(s.getRequestContext(req), types.UpdateSessionRequest{
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
		s.handleStreamingResponse(rw, req, userContext, cfg)
		return
	}

	s.handleBlockingResponse(rw, req, userContext, cfg)
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
