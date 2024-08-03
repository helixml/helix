package server

import (
	"encoding/json"
	"io"
	"net/http"

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

	ctx := req.Context()
	user := getRequestUser(req)

	status, err := s.Controller.GetStatus(req.Context(), user)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	// For finetunes, legacy route
	if startReq.LoraDir != "" {
		s.startChatSessionLegacyHandler(req.Context(), user, &startReq, req, rw)
		return
	}

	// Default to text
	if startReq.Type == "" {
		startReq.Type = types.SessionTypeText
	}

	if startReq.SessionID == "" {

	}
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

	user := getRequestUser(req)
	ctx := req.Context()

	ownerContext := getOwnerContext(req)

	status, err := s.Controller.GetStatus(ctx, user)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	// Default to text
	if startReq.Type == "" {
		startReq.Type = types.SessionTypeText
	}

	dataEntity, err := s.Store.GetDataEntity(ctx, startReq.DataEntityID)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	if dataEntity.Owner != user.ID {
		http.Error(rw, "you must own the data entity", http.StatusBadRequest)
		return
	}

	// TODO: data entity pipelines where we don't even need a session
	userInteraction, err := s.getUserInteractionFromDataEntity(dataEntity, ownerContext)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	model, err := types.ProcessModelName("", types.SessionModeFinetune, startReq.Type, true, startReq.RagEnabled)
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
		Owner:               user.ID,
		OwnerType:           user.Type,
		UserInteractions:    []*types.Interaction{userInteraction},
		Priority:            status.Config.StripeSubscriptionActive,
		UploadedDataID:      dataEntity.ID,
		RAGEnabled:          startReq.RagEnabled,
		TextFinetuneEnabled: startReq.TextFinetuneEnabled,
		RAGSettings:         startReq.RagSettings,
	}

	sessionData, err := s.Controller.StartSession(ctx, user, createRequest)
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
