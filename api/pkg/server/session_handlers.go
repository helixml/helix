package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/helixml/helix/api/pkg/data"
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
func (s *HelixAPIServer) startChatSessionHandler(rw http.ResponseWriter, req *http.Request) {

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

	var cfg *startSessionConfig

	if startReq.SessionID == "" {
		if startReq.LoraDir != "" {
			// Basic validation on the lora dir path, it should be something like
			// dev/users/9f2a1f87-b3b8-4e58-9176-32b4861c70e2/sessions/974a8bdc-c1d1-42dc-9a49-7bfa6db112d1/lora/e1c11fba-8d49-4a41-8ae7-60532ab67410
			// this works for both session based file paths and data entity based file paths
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

		useModel := startReq.Model

		interactions, err := messagesToInteractions(startReq.Messages)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return
		}

		// this will be assigned if the token being used is an app token
		appID := userContext.User.AppID

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
			ModelName:        types.ModelName(startReq.Model),
			Owner:            userContext.User.ID,
			OwnerType:        userContext.User.Type,
			LoraDir:          startReq.LoraDir,
			UserInteractions: interactions,
			Priority:         status.Config.StripeSubscriptionActive,
			ActiveTools:      startReq.Tools,
			RAGSourceID:      startReq.RAGSourceID,
		}

		// if we have an app then let's populate the InternalSessionRequest with values from it
		if newSession.ParentApp != "" {
			app, err := s.Store.GetApp(userContext.Ctx, appID)
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

		processedModel, err := types.ProcessModelName(useModel, types.SessionModeInference, startReq.Type, startReq.LoraDir != "")
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		newSession.ModelName = processedModel

		// we need to load the rag source and apply the rag settings to the session
		if newSession.RAGSourceID != "" {
			ragSource, err := s.Store.GetDataEntity(userContext.Ctx, newSession.RAGSourceID)
			if err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}

			newSession.RAGSettings = ragSource.Config.RAGSettings
		}

		// we need to load the lora source and apply the lora settings to the session
		if newSession.LoraID != "" {
			loraSource, err := s.Store.GetDataEntity(userContext.Ctx, newSession.LoraID)
			if err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}

			newSession.LoraDir = loraSource.Config.FilestorePath
		}

		fmt.Printf("newSession --------------------------------------\n")
		spew.Dump(newSession)
		// we are still in the old frontend mode where it's listening to the websocket
		// TODO: get the frontend to stream using the streaming api below
		if startReq.Legacy {
			sessionData, err := s.Controller.StartSession(userContext, newSession)
			if err != nil {
				http.Error(rw, fmt.Sprintf("failed to start session: %s", err.Error()), http.StatusBadRequest)
				log.Error().Err(err).Msg("failed to start session")
				return
			}

			sessionDataJSON, err := json.Marshal(sessionData)
			if err != nil {
				http.Error(rw, "failed to marshal session data: "+err.Error(), http.StatusInternalServerError)
				return
			}
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusOK)
			rw.Write(sessionDataJSON)
			return
		}

		cfg = &startSessionConfig{
			sessionID: sessionID,
			modelName: string(newSession.ModelName),
			start: func() error {
				_, err := s.Controller.StartSession(userContext, newSession)
				if err != nil {
					return fmt.Errorf("failed to create session: %s", err)
				}
				return nil
			},
		}
	} else {
		existingSession, err := s.Store.GetSession(userContext.Ctx, startReq.SessionID)
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

		// we are still in the old frontend mode where it's listening to the websocket
		// TODO: get the frontend to stream using the streaming api below
		if startReq.Legacy {
			updatedSession, err := s.Controller.UpdateSession(getRequestContext(req), types.UpdateSessionRequest{
				SessionID:       startReq.SessionID,
				UserInteraction: interactions[0],
				SessionMode:     types.SessionModeInference,
			})
			if err != nil {
				http.Error(rw, fmt.Sprintf("failed to start session: %s", err.Error()), http.StatusBadRequest)
				log.Error().Err(err).Msg("failed to start session")
				return
			}

			sessionDataJSON, err := json.Marshal(updatedSession)
			if err != nil {
				http.Error(rw, "failed to marshal session data: "+err.Error(), http.StatusInternalServerError)
				return
			}
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusOK)
			rw.Write(sessionDataJSON)
			return
		}

		cfg = &startSessionConfig{
			sessionID: startReq.SessionID,
			modelName: string(existingSession.ModelName),
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

// startLearnSessionHandler godoc
// @Summary Start new fine tuning and/or rag source generation session
// @Description Start new fine tuning and/or RAG source generation session
// @Tags    learn

// @Success 200 {object} types.Session
// @Param request    body types.SessionLearnRequest true "Request body with settings for the learn session.")
// @Router /api/v1/sessions/learn [post]
// @Security BearerAuth
func (s *HelixAPIServer) startLearnSessionHandler(rw http.ResponseWriter, req *http.Request) {

	var startReq types.SessionLearnRequest
	err := json.NewDecoder(io.LimitReader(req.Body, 10*MEGABYTE)).Decode(&startReq)
	if err != nil {
		http.Error(rw, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if startReq.DataEntityID == "" {
		http.Error(rw, "data entity ID not be empty", http.StatusBadRequest)
		return
	}

	userContext := getRequestContext(req)
	ownerContext := getOwnerContext(req)

	status, err := s.Controller.GetStatus(userContext)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	// Default to text
	if startReq.Type == "" {
		startReq.Type = types.SessionTypeText
	}

	dataEntity, err := s.Store.GetDataEntity(userContext.Ctx, startReq.DataEntityID)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	if dataEntity.Owner != userContext.User.ID {
		http.Error(rw, "you must own the data entity", http.StatusBadRequest)
		return
	}

	// TODO: data entity pipelines where we don't even need a session
	userInteraction, err := s.getUserInteractionFromDataEntity(dataEntity, ownerContext)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	model, err := types.ProcessModelName("", types.SessionModeFinetune, startReq.Type, false)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	sessionID := system.GenerateSessionID()
	createRequest := types.InternalSessionRequest{
		ID:                  sessionID,
		Mode:                types.SessionModeFinetune,
		ModelName:           model,
		Type:                startReq.Type,
		Stream:              true,
		Owner:               userContext.User.ID,
		OwnerType:           userContext.User.Type,
		UserInteractions:    []*types.Interaction{userInteraction},
		Priority:            status.Config.StripeSubscriptionActive,
		UploadedDataID:      dataEntity.ID,
		RAGEnabled:          startReq.RagEnabled,
		TextFinetuneEnabled: startReq.TextFinetuneEnabled,
		RAGSettings:         startReq.RagSettings,
	}

	sessionData, err := s.Controller.StartSession(userContext, createRequest)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	sessionDataJSON, err := json.Marshal(sessionData)
	if err != nil {
		http.Error(rw, "failed to marshal session data: "+err.Error(), http.StatusInternalServerError)
		return
	}
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	rw.Write(sessionDataJSON)
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
