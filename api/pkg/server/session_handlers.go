package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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

	userContext := getRequestContext(req)

	status, err := s.Controller.GetStatus(userContext)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	// Default to text
	if startReq.Type == "" {
		startReq.Type = types.SessionTypeText
	}

	// Default to inference
	if startReq.Mode == "" {
		startReq.Mode = types.SessionModeInference
	}

	model, err := types.ProcessModelName(startReq.Model, startReq.Mode, startReq.Type, startReq.LoraDir != "")
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	startReq.Model = model.String()

	if startReq.LoraDir != "" {
		// Basic validation on the lora dir path, it should be something like
		// dev/users/9f2a1f87-b3b8-4e58-9176-32b4861c70e2/sessions/974a8bdc-c1d1-42dc-9a49-7bfa6db112d1/lora/e1c11fba-8d49-4a41-8ae7-60532ab67410
		ownerContext := types.OwnerContext{
			Owner:     userContext.User.ID,
			OwnerType: userContext.User.Type,
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
			Stream:           startReq.Stream,
			ModelName:        types.ModelName(startReq.Model),
			Owner:            userContext.User.ID,
			OwnerType:        userContext.User.Type,
			LoraDir:          startReq.LoraDir,
			UserInteractions: interactions,
			Priority:         status.Config.StripeSubscriptionActive,
			ActiveTools:      startReq.Tools,
		}

		cfg = &startSessionConfig{
			sessionID: sessionID,
			modelName: startReq.Model,
			start: func() error {
				_, err := s.Controller.StartSession(userContext, newSession)
				if err != nil {
					return fmt.Errorf("failed to create session: %s", err)
				}
				return nil
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

				_, err := s.Controller.UpdateSession(getRequestContext(req), types.UpdateSessionRequest{
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
