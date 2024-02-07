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
)

// startSessionHandler can be used to start or continue a session with the Helix API.
func (s *HelixAPIServer) startSessionHandler(rw http.ResponseWriter, req *http.Request) {

	var startSessionRequest types.StartSessionRequest
	err := json.NewDecoder(io.LimitReader(req.Body, 10*MEGABYTE)).Decode(&startSessionRequest)
	if err != nil {
		http.Error(rw, "invalid request body", http.StatusBadRequest)
		return
	}

	if len(startSessionRequest.Messages) == 0 {
		http.Error(rw, "messages must not be empty", http.StatusBadRequest)
		return
	}

	// If more than 1, also not allowed just yet for simplification
	if len(startSessionRequest.Messages) > 1 {
		http.Error(rw, "only 1 message is allowed for now", http.StatusBadRequest)
		return
	}

	// userContext := apiServer.getRequestContext(req)

	// status, err := apiServer.Controller.GetStatus(userContext)
	// if err != nil {
	// 	http.Error(res, err.Error(), http.StatusInternalServerError)
	// 	return
	// }

	// var (
	// 	session *types.Session
	// )

	if startSessionRequest.Model == "" {
		startSessionRequest.Model = string(types.Model_Mistral7b)
	}

	if startSessionRequest.SessionID == "" {
		s.newSessionHandler(rw, req, &startSessionRequest)
		return
	}

	s.existingSessionHandler(rw, req, &startSessionRequest)
}

func (s *HelixAPIServer) newSessionHandler(rw http.ResponseWriter, req *http.Request, newSession *types.StartSessionRequest) {
	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Set("Connection", "keep-alive")
	rw.Header().Set("Transfer-Encoding", "chunked")
	rw.Header().Set("Content-Type", "application/x-ndjson")

	userContext := s.getRequestContext(req)

	status, err := s.Controller.GetStatus(userContext)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	sessionID := system.GenerateSessionID()
	sessionMode := types.SessionModeInference

	// TODO: load messages from the request
	var interactions []*types.Interaction

	for _, m := range newSession.Messages {
		// Validating roles
		switch m.Author.Role {
		case "user", "system", "assistant":
			// OK
		default:
			http.Error(rw, "invalid role, available roles: 'user', 'system', 'assistant'", http.StatusBadRequest)
			return
		}

		if len(m.Content.Parts) != 1 {
			http.Error(rw, "invalid message content, should only contain 1 entry and it should be a string", http.StatusBadRequest)
			return
		}

		switch m.Content.Parts[0].(type) {
		case string:
			// OK
		default:
			http.Error(rw, "invalid message content", http.StatusBadRequest)
			return
		}

		var creator types.CreatorType
		switch m.Author.Role {
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
			Mode:           sessionMode,
			Message:        m.Content.Parts[0].(string),
			Files:          []string{},
			State:          types.InteractionStateComplete,
			Finished:       true,
			Metadata:       map[string]string{},
			DataPrepChunks: map[string][]types.DataPrepChunk{},
		}

		interactions = append(interactions, interaction)
	}

	newSessionRequest := types.CreateSessionRequest{
		SessionID:        sessionID,
		SessionMode:      sessionMode,
		SessionType:      types.SessionTypeText,
		ModelName:        types.ModelName(newSession.Model),
		Owner:            userContext.Owner,
		OwnerType:        userContext.OwnerType,
		UserInteractions: interactions,
		Priority:         status.Config.StripeSubscriptionActive,
	}

	doneCh := make(chan struct{})

	sub, err := s.pubsub.Subscribe(req.Context(), pubsub.GetSessionQueue(newSessionRequest.Owner, newSessionRequest.SessionID), func(payload []byte) error {
		err := s.sessionInferenceUpdates(rw, payload, doneCh)
		if err != nil {
			return fmt.Errorf("error handling session updates: %w", err)
		}
		return nil
	})
	if err != nil {
		system.NewHTTPError500("failed to subscribe to session updates: %s", err)
		return
	}
	// After subscription, start the session, otherwise
	// we can have race-conditions on very fast responses
	// from the runner
	_, err = s.Controller.CreateSession(userContext, newSessionRequest)
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

func (s *HelixAPIServer) sessionInferenceUpdates(rw http.ResponseWriter, payload []byte, doneCh chan struct{}) error {
	var event types.WebsocketEvent
	err := json.Unmarshal(payload, &event)
	if err != nil {
		return fmt.Errorf("error unmarshalling websocket event '%s': %w", string(payload), err)
	}

	// Nothing to do
	if event.WorkerTaskResponse == nil {
		return nil
	}

	if event.WorkerTaskResponse != nil && event.WorkerTaskResponse.Done {
		close(doneCh)
		return nil
	}

	bts, err := json.Marshal(&types.SessionResponse{
		SessionID: event.Session.ID,
		Model:     string(event.Session.ModelName),
		Message: types.Message{
			Author: types.MessageAuthor{
				Role: "assistant",
			},
			Content: types.MessageContent{
				Parts: []interface{}{event.WorkerTaskResponse.Message},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("error marshalling session response: %w", err)
	}

	_, err = fmt.Fprintf(rw, "%s\n", string(bts))

	if err != nil {
		return fmt.Errorf("error writing chunk '%s': %w", string(bts), err)
	}

	return nil
}

func (s *HelixAPIServer) existingSessionHandler(rw http.ResponseWriter, req *http.Request, session *types.StartSessionRequest) {

}
