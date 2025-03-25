package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/model"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/llm"
	openai "github.com/sashabaranov/go-openai"

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
	ctx := req.Context()
	user := getRequestUser(req)

	body, err := io.ReadAll(io.LimitReader(req.Body, 10*MEGABYTE))
	if err != nil {
		log.Error().Err(err).Msg("error reading body")
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	var startReq types.SessionChatRequest
	err = json.Unmarshal(body, &startReq)
	if err != nil {
		http.Error(rw, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Priority is to use the app ID coming from the authentication context,
	// this means that the caller is using app specific API key
	if user.AppID != "" {
		startReq.AppID = user.AppID
	} else {
		// Allow overriding from URL queries
		if appID := req.URL.Query().Get("app_id"); appID != "" {
			startReq.AppID = appID
		}
	}

	ctx = oai.SetContextAppID(ctx, startReq.AppID)

	if ragSourceID := req.URL.Query().Get("rag_source_id"); ragSourceID != "" {
		startReq.RAGSourceID = ragSourceID
	}

	if assistantID := req.URL.Query().Get("assistant_id"); assistantID != "" {
		startReq.AssistantID = assistantID
	}

	if startReq.AppID == "" {
		// If organization ID is set, check if user is a member of the organization
		if startReq.OrganizationID != "" {
			_, err := s.authorizeOrgMember(req.Context(), user, startReq.OrganizationID)
			if err != nil {
				http.Error(rw, "You do not have access to the organization with the id: "+startReq.OrganizationID, http.StatusForbidden)
				return
			}
		}
	} else {
		// If app ID is set, load the app
		app, err := s.Store.GetAppWithTools(req.Context(), startReq.AppID)
		if err != nil {
			log.Error().Err(err).Str("app_id", startReq.AppID).Msg("Failed to load app")
			http.Error(rw, "Failed to load app: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Check if the user has access to the app
		if err := s.authorizeUserToApp(req.Context(), user, app, types.ActionGet); err != nil {
			log.Error().Err(err).Str("app_id", startReq.AppID).Str("user_id", user.ID).Msg("User doesn't have access to app")
			http.Error(rw, "You do not have access to the app with the id: "+startReq.AppID, http.StatusForbidden)
			return
		}

		// Set organization ID if not set yet
		if app.OrganizationID != "" {
			startReq.OrganizationID = app.OrganizationID
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

	// For finetunes, legacy route
	if startReq.LoraDir != "" || startReq.Type == types.SessionTypeImage {
		s.startChatSessionLegacyHandler(ctx, user, &startReq, req, rw)
		return
	}

	modelName, err := model.ProcessModelName(s.Cfg.Inference.Provider, startReq.Model, types.SessionModeInference, types.SessionTypeText, false, false)
	if err != nil {
		http.Error(rw, "invalid model name: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Default to text
	if startReq.Type == "" {
		startReq.Type = types.SessionTypeText
	}

	if startReq.SystemPrompt == "" {
		startReq.SystemPrompt = `You are a helpful assistant called Helix, built on a platform called HelixML enabling private deployment of GenAI models enabling privacy, security and compliance. If the user's query includes sections in square brackets [like this], indicating that some values are missing, you should ask for the missing values, but DO NOT include the square brackets in your response - instead make the response seem natural and extremely concise - only asking the required questions asking for the values to be filled in. To reiterate, do NOT include square brackets in the response.

EXAMPLE:
If the query includes "prepare a pitch for [a specific topic]", ask "What topic would you like to prepare a pitch for?" instead of "please specify the [specific topic]"

If the user asks for information about Helix or installing Helix, refer them to the Helix website at https://tryhelix.ai or the docs at https://docs.helix.ml, using markdown links. Only offer the links if the user asks for information about Helix or installing Helix.`
	}

	message, ok := startReq.Message()
	if !ok {
		http.Error(rw, "invalid message", http.StatusBadRequest)
		return
	}

	var (
		session    *types.Session
		newSession bool
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

		// Set the session ID in the context to enable document ID tracking
		ctx = oai.SetContextSessionID(ctx, session.ID)
		log.Debug().
			Str("session_id", session.ID).
			Str("app_id", startReq.AppID).
			Msg("existing session: set session ID in context for document tracking")
	} else {
		// Create session
		newSession = true

		session = &types.Session{
			ID:             system.GenerateSessionID(),
			Name:           s.getTemporarySessionName(message),
			Created:        time.Now(),
			Updated:        time.Now(),
			Mode:           types.SessionModeInference,
			Type:           types.SessionTypeText,
			ModelName:      startReq.Model,
			ParentApp:      startReq.AppID,
			OrganizationID: startReq.OrganizationID,
			Owner:          user.ID,
			OwnerType:      user.Type,
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

		// Set the session ID in the context to enable document ID tracking
		ctx = oai.SetContextSessionID(ctx, session.ID)
		log.Debug().
			Str("session_id", session.ID).
			Str("app_id", startReq.AppID).
			Msg("new session: set session ID in context for document tracking")
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
			Creator:  types.CreatorTypeAssistant,
			Mode:     types.SessionModeInference,
			Message:  "",
			State:    types.InteractionStateWaiting,
			Finished: false,
			Metadata: map[string]string{},
		},
	)

	// Write the initial session that has the user prompt and also the placeholder interaction
	// for the system response which will be updated later once the response is received
	err = s.Controller.WriteSession(req.Context(), session)
	if err != nil {
		http.Error(rw, "failed to write session: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if newSession {
		go func() {
			name, err := s.generateSessionName(user, session.ID, modelName, message)
			if err != nil {
				log.Error().Err(err).Msg("error generating session name")
				return
			}

			session.Name = name

			err = s.Controller.UpdateSessionName(req.Context(), user.ID, session.ID, name)
			if err != nil {
				log.Error().Err(err).Msg("error updating session name")
			}
		}()
	}

	ownerID := user.ID
	if user.TokenType == types.TokenTypeRunner {
		ownerID = oai.RunnerID
	}

	ctx = oai.SetContextValues(ctx, &oai.ContextValues{
		OwnerID:         ownerID,
		SessionID:       session.ID,
		InteractionID:   session.Interactions[0].ID,
		OriginalRequest: body,
	})

	var (
		chatCompletionRequest = openai.ChatCompletionRequest{
			Model: modelName,
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
			Provider:    string(startReq.Provider),
			QueryParams: func() map[string]string {
				params := make(map[string]string)
				for key, values := range req.URL.Query() {
					if len(values) > 0 {
						params[key] = values[0]
					}
				}
				return params
			}(),
		}
	)

	// Convert interactions (except the last one) to messages
	for _, interaction := range session.Interactions[:len(session.Interactions)-1] {
		chatCompletionRequest.Messages = append(chatCompletionRequest.Messages, openai.ChatCompletionMessage{
			Role:    string(interaction.Creator),
			Content: interaction.Message,
		})
	}

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
}

func (s *HelixAPIServer) restartChatSessionHandler(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	user := getRequestUser(req)
	sessionID := mux.Vars(req)["id"]
	session, err := s.Store.GetSession(ctx, sessionID)
	if err != nil {
		http.Error(rw, fmt.Sprintf("failed to get session %s, error: %s", sessionID, err), http.StatusInternalServerError)
		return
	}

	modelName, err := model.ProcessModelName(s.Cfg.Inference.Provider, session.ModelName, types.SessionModeInference, types.SessionTypeText, false, false)
	if err != nil {
		http.Error(rw, "invalid model name: "+err.Error(), http.StatusBadRequest)
		return
	}
	if modelName != session.ModelName {
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
	err = s.Controller.WriteSession(req.Context(), session)
	if err != nil {
		http.Error(rw, "failed to write session: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert interactions (except the last one) to messages
	var (
		chatCompletionRequest = openai.ChatCompletionRequest{
			Model: modelName,
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

	ctx = oai.SetContextAppID(ctx, session.ParentApp)

	ownerID := user.ID
	if user.TokenType == types.TokenTypeRunner {
		ownerID = oai.RunnerID
	}

	// Set required context values
	ctx = oai.SetContextValues(ctx, &oai.ContextValues{
		OwnerID:       ownerID,
		SessionID:     session.ID,
		InteractionID: session.Interactions[len(session.Interactions)-1].ID,
	})

	err = s.handleStreamingSession(ctx, user, session, chatCompletionRequest, options, rw)
	if err != nil {
		log.Err(err).Msg("error handling blocking session")
	}
}

const titleGenPrompt = `Generate a concise 3-5 word title for the given user input. Follow these rules strictly:

1. Use exactly 3-5 words.
2. Do not use the word "title" in your response.
3. Capture the essence of the user's query or topic.
4. Provide only the title, without any additional commentary.

Examples:

User: "Tell me about the Roman Empire's early days and how it was formed."
Response: Roman Empire's formation

User: "What is the best way to cook a steak?"
Response: Perfect steak cooking techniques

Now, generate a title for the following user input:

%s`

func (s *HelixAPIServer) getTemporarySessionName(prompt string) string {
	// return first few words of the prompt
	words := strings.Split(prompt, " ")
	if len(words) > 5 {
		return strings.Join(words[:5], " ")
	}
	return strings.Join(words, " ")
}

func (s *HelixAPIServer) generateSessionName(user *types.User, sessionID, model, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ownerID := user.ID
	if user.TokenType == types.TokenTypeRunner {
		ownerID = oai.RunnerID
	}

	ctx = oai.SetContextValues(ctx, &oai.ContextValues{
		OwnerID:       ownerID,
		SessionID:     sessionID,
		InteractionID: "n/a",
	})

	ctx = oai.SetStep(ctx, &oai.Step{
		Step: types.LLMCallStepGenerateTitle,
	})

	req := openai.ChatCompletionRequest{
		Model: model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: "You are a helpful assistant that generates a concise title for a given user input.",
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: fmt.Sprintf(titleGenPrompt, prompt),
			},
		},
	}

	options := &controller.ChatCompletionOptions{
		// AppID:       r.URL.Query().Get("app_id"),
		// AssistantID: r.URL.Query().Get("assistant_id"),
		// RAGSourceID: r.URL.Query().Get("rag_source_id"),
	}

	resp, _, err := s.Controller.ChatCompletion(ctx, user, req, options)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", errors.New("no data in the LLM response")
	}

	return llm.StripThinkingTags(resp.Choices[0].Message.Content), nil
}

func (s *HelixAPIServer) handleBlockingSession(
	ctx context.Context,
	user *types.User,
	session *types.Session,
	chatCompletionRequest openai.ChatCompletionRequest,
	options *controller.ChatCompletionOptions, rw http.ResponseWriter,
) error {
	// Ensure request is not streaming
	chatCompletionRequest.Stream = false

	// Set the session ID in the context to enable document ID tracking
	ctx = oai.SetContextSessionID(ctx, session.ID)
	log.Debug().
		Str("session_id", session.ID).
		Str("app_id", session.ParentApp).
		Msg("handleBlockingSession: set session ID in context for document tracking")

	// Call the LLM
	chatCompletionResponse, _, err := s.Controller.ChatCompletion(ctx, user, chatCompletionRequest, options)
	if err != nil {
		// Update the session with the response
		session.Interactions[len(session.Interactions)-1].Error = err.Error()
		session.Interactions[len(session.Interactions)-1].State = types.InteractionStateError
		writeErr := s.Controller.WriteSession(ctx, session)
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

	err = s.Controller.WriteSession(ctx, session)
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

	// Set the session ID in the context to enable document ID tracking
	ctx = oai.SetContextSessionID(ctx, session.ID)
	log.Debug().
		Str("session_id", session.ID).
		Str("app_id", session.ParentApp).
		Msg("handleStreamingSession: set session ID in context for document tracking")

	rw.Header().Set("Content-Type", "text/event-stream")
	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Set("Connection", "keep-alive")

	// Write an empty response to start chunk that contains the session id
	bts, err := json.Marshal(&openai.ChatCompletionStreamResponse{
		Object: "chat.completion.chunk",
		ID:     session.ID,
		Model:  chatCompletionRequest.Model,
	})
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return err
	}

	if err := writeChunk(rw, bts); err != nil {
		log.Error().Err(err).Msg("failed to write chunk")
	}

	// Call the LLM
	stream, _, err := s.Controller.ChatCompletionStream(ctx, user, chatCompletionRequest, options)
	if err != nil {
		// Update last interaction
		session.Interactions[len(session.Interactions)-1].Error = err.Error()
		session.Interactions[len(session.Interactions)-1].Completed = time.Now()
		session.Interactions[len(session.Interactions)-1].State = types.InteractionStateError
		session.Interactions[len(session.Interactions)-1].Finished = true
		if sessErr := s.Controller.WriteSession(ctx, session); sessErr != nil {
			log.Error().Err(err).Msg("failed to write session")
		}

		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return nil
	}
	defer stream.Close()

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

		if err := writeChunk(rw, bts); err != nil {
			log.Error().Err(err).Msg("failed to write chunk")
		}
	}

	// Add a 10-second delay before marking the streaming session as complete
	// This allows more time to observe streaming behavior in the UI
	log.Info().Str("session_id", session.ID).Msg("Delaying completion by 10 seconds to allow UI inspection")
	time.Sleep(10 * time.Second)

	// Update last interaction
	session.Interactions[len(session.Interactions)-1].Message = fullResponse
	session.Interactions[len(session.Interactions)-1].Completed = time.Now()
	session.Interactions[len(session.Interactions)-1].State = types.InteractionStateComplete
	session.Interactions[len(session.Interactions)-1].Finished = true

	return s.Controller.WriteSession(ctx, session)
}
