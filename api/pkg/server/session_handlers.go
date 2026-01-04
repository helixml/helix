package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/model"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/store"
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

	var (
		generateSessionNameProvider string
		generateSessionNameModel    string
	)

	ctx = oai.SetContextAppID(ctx, startReq.AppID)

	if assistantID := req.URL.Query().Get("assistant_id"); assistantID != "" {
		startReq.AssistantID = assistantID
	}

	// messageContextLimit - how many messages to keep in the context,
	// configured by the app
	var messageContextLimit int
	var agentType string

	if startReq.AppID == "" {
		// If organization ID is set, check if user is a member of the organization
		if startReq.OrganizationID != "" {
			_, err := s.authorizeOrgMember(req.Context(), user, startReq.OrganizationID)
			if err != nil {
				http.Error(rw, "You do not have access to the organization with the id: "+startReq.OrganizationID, http.StatusForbidden)
				return
			}
		}

		// Setting default message context limit
		messageContextLimit = s.Cfg.Inference.DefaultContextLimit
		agentType = startReq.AgentType
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

		// Determine agent type from assistant configuration
		agentType = getAgentTypeFromApp(app, startReq)
		log.Debug().
			Str("app_id", startReq.AppID).
			Str("determined_agent_type", agentType).
			Msg("Determined agent type from app configuration")

		// Load external agent config from app if agent type is external
		if agentType == "zed_external" && startReq.ExternalAgentConfig == nil {
			if app.Config.Helix.ExternalAgentConfig != nil {
				// Check if the config has meaningful values (display settings)
				appConfig := app.Config.Helix.ExternalAgentConfig
				if appConfig.Resolution != "" || appConfig.DesktopType != "" || appConfig.DisplayWidth > 0 {
					startReq.ExternalAgentConfig = appConfig
					log.Debug().
						Str("app_id", startReq.AppID).
						Msg("Loaded external agent config from app configuration")
				}
			}
		}

		// Get the assistant from the app (use AssistantID if specified, otherwise default to first assistant)
		{
			var assistant *types.AssistantConfig
			assistantID := startReq.AssistantID
			if assistantID == "" {
				assistantID = "0" // Default to first assistant
			}
			assistant = data.GetAssistant(app, assistantID)

			// Update the model if the assistant has one
			// Prefer GenerationModel over Model (new field vs legacy field)
			if assistant.GenerationModel != "" {
				startReq.Model = assistant.GenerationModel
				if assistant.GenerationModelProvider != "" {
					startReq.Provider = types.Provider(assistant.GenerationModelProvider)
				}
			} else if assistant.Model != "" {
				startReq.Model = assistant.Model
			}

			// Override provider if explicitly set on assistant
			if assistant.Provider != "" {
				startReq.Provider = types.Provider(assistant.Provider)
			}

			if assistant.ContextLimit > 0 {
				messageContextLimit = assistant.ContextLimit
			}

			// Set model for session name generation based on agent type
			if assistant.IsAgentMode() {
				// For agent mode, use small generation model
				generateSessionNameProvider = assistant.SmallGenerationModelProvider
				generateSessionNameModel = assistant.SmallGenerationModel
			} else {
				// For basic mode, use generation model (or fall back to Model field)
				if assistant.GenerationModel != "" {
					generateSessionNameProvider = assistant.GenerationModelProvider
					generateSessionNameModel = assistant.GenerationModel
				} else if assistant.Model != "" {
					generateSessionNameProvider = assistant.Provider
					generateSessionNameModel = assistant.Model
				}
			}
		}
	}

	if len(startReq.Messages) == 0 {
		http.Error(rw, "messages must not be empty", http.StatusBadRequest)
		return
	}

	// If more than one message - session regeneration
	if len(startReq.Messages) > 1 {
		log.Info().Msg("session regeneration requested")
	}

	modelName, err := model.ProcessModelName(s.Cfg.Inference.Provider, startReq.Model, types.SessionTypeText)
	if err != nil {
		http.Error(rw, "invalid model name: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Default to text
	if startReq.Type == "" {
		startReq.Type = types.SessionTypeText
	}

	// If there's no app and no system prompt, set the default system prompt
	if startReq.AppID == "" && startReq.SystemPrompt == "" {
		startReq.SystemPrompt = `You are a helpful assistant called Helix, built on a platform called HelixML enabling private deployment of GenAI models enabling privacy, security and compliance. If the user's query includes sections in square brackets [like this], indicating that some values are missing, you should ask for the missing values, but DO NOT include the square brackets in your response - instead make the response seem natural and extremely concise - only asking the required questions asking for the values to be filled in. To reiterate, do NOT include square brackets in the response.

EXAMPLE:
If the query includes "prepare a pitch for [a specific topic]", ask "What topic would you like to prepare a pitch for?" instead of "please specify the [specific topic]"

If the user asks for information about Helix or installing Helix, refer them to the Helix website at https://helix.ml or the docs at https://docs.helixml.tech, using markdown links. Only offer the links if the user asks for information about Helix or installing Helix.`
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

		// Load interactions for the session
		interactions, _, err := s.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
			SessionID:    session.ID,
			GenerationID: session.GenerationID,
		})
		if err != nil {
			http.Error(rw, fmt.Sprintf("failed to get interactions for session %s, error: %s", startReq.SessionID, err), http.StatusInternalServerError)
			return
		}

		log.Info().
			Int("interactions", len(interactions)).
			Str("session_id", session.ID).
			Int("generation_id", session.GenerationID).
			Msg("session loaded")

		// Updating session interactions
		session.Interactions = interactions

		startReq.OrganizationID = session.OrganizationID

		// If the session has an AppID, use it as the next interaction
		if session.ParentApp != "" {
			startReq.AppID = session.ParentApp
		}

		// If the provider is not set, use the provider from the session
		if startReq.Provider == "" {
			startReq.Provider = types.Provider(session.Provider)
		} else {
			// Update provider for the session
			session.Provider = string(startReq.Provider)
		}

		// Updating session model and provider
		session.ModelName = startReq.Model

	} else {
		// Create session
		newSession = true

		// Set default agent type if not specified
		if startReq.AgentType == "" {
			startReq.AgentType = "helix"
		}

		session = &types.Session{
			ID:             system.GenerateSessionID(),
			Name:           s.getTemporarySessionName(message),
			Created:        time.Now(),
			Updated:        time.Now(),
			Mode:           types.SessionModeInference,
			Type:           types.SessionTypeText,
			Provider:       string(startReq.Provider),
			ModelName:      startReq.Model,
			ParentApp:      startReq.AppID,
			OrganizationID: startReq.OrganizationID,
			Owner:          user.ID,
			OwnerType:      user.Type,
			Metadata: types.SessionMetadata{
				Stream:              startReq.Stream,
				SystemPrompt:        startReq.SystemPrompt,
				AssistantID:         startReq.AssistantID,
				HelixVersion:        data.GetHelixVersion(),
				AgentType:           agentType,
				ExternalAgentConfig: startReq.ExternalAgentConfig,
			},
		}

		// Set the session ID in the context to enable document ID tracking
		ctx = oai.SetContextSessionID(ctx, session.ID)
		log.Debug().
			Str("session_id", session.ID).
			Str("app_id", startReq.AppID).
			Msg("new session: set session ID in context for document tracking")
	}

	session, err = appendOrOverwrite(session, &startReq)
	if err != nil {
		http.Error(rw, "failed to process session messages: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Set the organization ID in the context for OAuth token retrieval
	ctx = oai.SetContextOrganizationID(ctx, session.OrganizationID)

	// Write the initial session
	err = s.Controller.WriteSession(req.Context(), session)
	if err != nil {
		http.Error(rw, "failed to write session: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Track which interactions are new (for external agent notification)
	// For existing sessions, only the last interaction(s) are new (from current generation)
	// For new sessions, all interactions are new
	newInteractionsStartIndex := 0
	if startReq.SessionID != "" {
		// Existing session - find where old interactions end by checking GenerationID
		// Only notify about interactions from the current generation
		for i := len(session.Interactions) - 1; i >= 0; i-- {
			if session.Interactions[i].GenerationID < session.GenerationID {
				// This interaction is from a previous generation
				newInteractionsStartIndex = i + 1
				break
			}
		}
		log.Debug().
			Int("total_interactions", len(session.Interactions)).
			Int("new_start_index", newInteractionsStartIndex).
			Msg("Tracking new interactions for external agent notification")
	}

	// Write the initial interactions
	err = s.Controller.WriteInteractions(req.Context(), session.Interactions...)
	if err != nil {
		http.Error(rw, "failed to write interactions: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Notify external agents ONLY of NEW interactions (not replaying history)
	for i := newInteractionsStartIndex; i < len(session.Interactions); i++ {
		interaction := session.Interactions[i]
		if err := s.NotifyExternalAgentOfNewInteraction(session.ID, interaction); err != nil {
			log.Warn().Err(err).
				Str("session_id", session.ID).
				Str("interaction_id", interaction.ID).
				Msg("Failed to notify external agent of new interaction")
		}
	}

	// Validate external agent configuration if specified
	if agentType == "zed_external" {
		if startReq.ExternalAgentConfig == nil {
			log.Info().
				Str("session_id", session.ID).
				Str("user_id", user.ID).
				Msg("External agent type specified with no configuration, using defaults")

			// Create empty configuration - wolf_executor handles workspace paths
			startReq.ExternalAgentConfig = &types.ExternalAgentConfig{}
		}

		log.Info().
			Str("session_id", session.ID).
			Str("user_id", user.ID).
			Msg("Launching external agent for Zed session")
	}

	// Handle external agent routing if agent type is zed_external
	if newSession && agentType == "zed_external" {
		// Register the session in the external agent executor before launching
		if s.externalAgentExecutor != nil {
			// Create a ZedAgent struct with session info for registration
			zedAgent := &types.ZedAgent{
				SessionID:   session.ID,
				UserID:      user.ID,
				Input:       "Initialize Zed development environment",
				ProjectPath: "workspace", // Use relative path
			}

			// Apply display settings from external agent configuration
			if startReq.ExternalAgentConfig != nil {
				if startReq.ExternalAgentConfig.DisplayWidth > 0 {
					zedAgent.DisplayWidth = startReq.ExternalAgentConfig.DisplayWidth
				}
				if startReq.ExternalAgentConfig.DisplayHeight > 0 {
					zedAgent.DisplayHeight = startReq.ExternalAgentConfig.DisplayHeight
				}
				if startReq.ExternalAgentConfig.DisplayRefreshRate > 0 {
					zedAgent.DisplayRefreshRate = startReq.ExternalAgentConfig.DisplayRefreshRate
				}
				// CRITICAL: Also copy resolution preset, zoom level, and desktop type
				// These are needed for proper GNOME 2x HiDPI scaling
				if startReq.ExternalAgentConfig.Resolution != "" {
					zedAgent.Resolution = startReq.ExternalAgentConfig.Resolution
				}
				if startReq.ExternalAgentConfig.ZoomLevel > 0 {
					zedAgent.ZoomLevel = startReq.ExternalAgentConfig.ZoomLevel
				}
				if startReq.ExternalAgentConfig.DesktopType != "" {
					zedAgent.DesktopType = startReq.ExternalAgentConfig.DesktopType
				}
			}

			// Add user's API token for git operations (merges with any custom env vars)
			if addErr := s.addUserAPITokenToAgent(req.Context(), zedAgent, session.Owner); addErr != nil {
				log.Warn().Err(addErr).Str("user_id", session.Owner).Msg("Failed to add user API token (continuing without git)")
				// Don't fail - external agents can work without git
			}

			// Register session in executor so RDP endpoint can find it
			agentResp, regErr := s.externalAgentExecutor.StartDesktop(req.Context(), zedAgent)
			if regErr != nil {
				log.Error().Err(regErr).Str("session_id", session.ID).Msg("Failed to register session in external agent executor")
				http.Error(rw, fmt.Sprintf("failed to initialize external agent: %s", regErr.Error()), http.StatusInternalServerError)
				return
			}

			// Store lobby ID, PIN, and Wolf instance ID in session (Phase 3: Multi-tenancy + Streaming)
			if agentResp.WolfLobbyID != "" || agentResp.WolfLobbyPIN != "" || agentResp.WolfInstanceID != "" {
				session.Metadata.WolfLobbyID = agentResp.WolfLobbyID
				session.Metadata.WolfLobbyPIN = agentResp.WolfLobbyPIN
				// CRITICAL: Store WolfInstanceID on session record - required for wolf-app-state endpoint
				// Without this, the frontend gets stuck at "Starting Desktop" forever
				session.WolfInstanceID = agentResp.WolfInstanceID
				_, err := s.Controller.Options.Store.UpdateSession(req.Context(), *session)
				if err != nil {
					log.Error().Err(err).Str("session_id", session.ID).Msg("Failed to store lobby data in session")
				} else {
					log.Info().
						Str("session_id", session.ID).
						Str("lobby_id", agentResp.WolfLobbyID).
						Str("lobby_pin", agentResp.WolfLobbyPIN).
						Str("wolf_instance_id", agentResp.WolfInstanceID).
						Msg("✅ Stored lobby ID, PIN, and Wolf instance ID in session (chat endpoint)")
				}
			}

			log.Info().Str("session_id", session.ID).Msg("External agent session registered successfully")
		}

		// External agent session created successfully
		// Message routing will be handled by handleExternalAgentStreaming when messages are sent
		log.Info().Str("session_id", session.ID).Msg("External agent session created, ready for WebSocket communication")
	}

	if newSession {
		go func(sessionAgentType string) {
			// Use default name for external agent sessions instead of generating via LLM
			if sessionAgentType == "zed_external" {
				defaultName := "External Agent Session"
				log.Debug().
					Str("session_id", session.ID).
					Str("agent_type", startReq.AgentType).
					Str("default_name", defaultName).
					Msg("Using default name for external agent session")

				session.Name = defaultName
				err := s.Controller.UpdateSessionName(req.Context(), user.ID, session.ID, defaultName)
				if err != nil {
					log.Error().Err(err).Msg("error updating session name for external agent")
				}
				return
			}

			var (
				provider string
				model    string
			)

			if generateSessionNameProvider != "" && generateSessionNameModel != "" {
				provider = generateSessionNameProvider
				model = generateSessionNameModel
			} else {
				provider = string(startReq.Provider)
				model = modelName
			}

			name, err := s.generateSessionName(ctx, user, startReq.OrganizationID, session, provider, model, message)
			if err != nil {
				log.Error().Err(err).Msg("error generating session name")
				return
			}

			session.Name = name

			err = s.Controller.UpdateSessionName(req.Context(), user.ID, session.ID, name)
			if err != nil {
				log.Error().Err(err).Msg("error updating session name")
			}
		}(agentType)
	}

	ownerID := user.ID
	if user.TokenType == types.TokenTypeRunner {
		ownerID = oai.RunnerID
	}

	lastInteraction := session.Interactions[len(session.Interactions)-1]

	ctx = oai.SetContextValues(ctx, &oai.ContextValues{
		OwnerID:         ownerID,
		SessionID:       session.ID,
		InteractionID:   lastInteraction.ID,
		ProjectID:       user.ProjectID,
		SpecTaskID:      user.SpecTaskID,
		OriginalRequest: body,
	})

	var (
		chatCompletionRequest = openai.ChatCompletionRequest{
			Model:    modelName,
			Messages: []openai.ChatCompletionMessage{},
		}
		options = &controller.ChatCompletionOptions{
			OrganizationID: startReq.OrganizationID,
			AppID:          startReq.AppID,
			AssistantID:    startReq.AssistantID,
			Provider:       string(startReq.Provider),
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

	// TODO: replace with summary
	messagesToInclude := limitInteractions(session.Interactions, messageContextLimit)

	chatCompletionRequest.Messages = types.InteractionsToOpenAIMessages(startReq.SystemPrompt, messagesToInclude)

	if !startReq.Stream {
		err := s.handleBlockingSession(ctx, user, session, lastInteraction, chatCompletionRequest, options, rw)
		if err != nil {
			log.Err(err).Msg("error handling blocking session")
		}
		return
	}

	err = s.handleStreamingSession(ctx, user, session, lastInteraction, chatCompletionRequest, options, rw)
	if err != nil {
		log.Err(err).Msg("error handling streaming session")
	}
}

// appendOrOverwrite appends the new message to the session or overwrites the existing messages
//   - If only a single message is provided, it will be appended to the session
//   - If multiple messages are provided, it will overwrite the existing messages from the beginning of the session,
//     this allows user to regenerate responses from any point in the session conversation. If user overwrites
//     the message in the middle of the conversation, all messages after it will be removed.
//   - If multiple messages are provided, the last message is always from the user. This allows to always correctly regenerate
//     the response
func appendOrOverwrite(session *types.Session, req *types.SessionChatRequest) (*types.Session, error) {
	if len(req.Messages) == 0 {
		return session, fmt.Errorf("no messages provided")
	}

	if len(req.Messages) == 1 {
		// If regenerate is true, remove all existing interactions
		if req.Regenerate {
			session.Interactions = []*types.Interaction{}
			session.GenerationID++ // Increment generation ID to start a new generation
		}

		// Append the message
		message, _ := req.Message()
		messageContent := req.MessageContent()

		// Each alternating between user and assistant messages
		// must create a new single interaction

		interaction := &types.Interaction{
			ID:                   system.GenerateInteractionID(),
			AppID:                session.ParentApp,
			SessionID:            session.ID,
			GenerationID:         session.GenerationID,
			UserID:               session.Owner,
			Created:              time.Now(),
			Updated:              time.Now(),
			Scheduled:            time.Now(),
			Completed:            time.Now(),
			Mode:                 types.SessionModeInference,
			SystemPrompt:         session.Metadata.SystemPrompt,
			PromptMessage:        message,
			PromptMessageContent: messageContent,
			State:                types.InteractionStateWaiting, // Will be updated once inference is complete
		}

		session.Interactions = append(session.Interactions, interaction)

		return session, nil
	}

	// Multiple messages, we are in "regenerate" mode
	if req.InteractionID == "" {
		return session, fmt.Errorf("interaction_id is required for multiple messages when regenerating")
	}

	// Last message must be from the user
	if req.Messages[len(req.Messages)-1].Role != openai.ChatMessageRoleUser {
		return session, fmt.Errorf("last message must be from the user")
	}

	session.GenerationID++ // Increment generation ID to start a new generation

	// Cut existing interactions to this index
	session.Interactions = session.Interactions[:getInteractionIndex(session.Interactions, req)]

	// Update all existing interactions to have the new generation ID
	for idx := range session.Interactions {
		session.Interactions[idx].GenerationID = session.GenerationID
	}

	message, ok := req.Message()
	if !ok {
		return session, fmt.Errorf("invalid message")
	}

	messageContent := req.MessageContent()

	// Append the new message
	session.Interactions = append(session.Interactions,
		&types.Interaction{
			ID:                   system.GenerateInteractionID(),
			AppID:                session.ParentApp,
			Created:              time.Now(),
			Updated:              time.Now(),
			Scheduled:            time.Now(),
			Completed:            time.Now(),
			SessionID:            session.ID,
			GenerationID:         session.GenerationID,
			UserID:               session.Owner,
			Mode:                 types.SessionModeInference,
			State:                types.InteractionStateWaiting,
			SystemPrompt:         session.Metadata.SystemPrompt,
			PromptMessage:        message,
			PromptMessageContent: messageContent,
		},
	)

	return session, nil
}

// getAgentTypeFromApp determines the agent type from the app's assistant configuration
func getAgentTypeFromApp(app *types.App, startReq types.SessionChatRequest) string {
	// Default to request agent type if no app
	if app == nil {
		return startReq.AgentType
	}

	// Get the assistant configuration
	var assistant *types.AssistantConfig
	if startReq.AssistantID != "" {
		assistant = data.GetAssistant(app, startReq.AssistantID)
	} else if len(app.Config.Helix.Assistants) > 0 {
		assistant = &app.Config.Helix.Assistants[0]
	}

	// Use assistant agent type if available, otherwise fall back to request agent type
	if assistant != nil {
		return string(assistant.AgentType)
	}

	return startReq.AgentType
}

func getInteractionIndex(interactions []*types.Interaction, req *types.SessionChatRequest) int {
	// Get last message interaction ID
	if len(req.Messages) == 0 {
		return 0
	}

	// Cut interactions until the last message interaction
	for i, interaction := range interactions {
		if interaction.ID == req.InteractionID {
			return i
		}
	}

	return 0
}

// limitInteractions returns the interactions except the last one, limited by the limit.
// If limit is 3 but there are 10 interactions, last one will be excluded and only the next 3 before it
// will be returned.
func limitInteractions(interactions []*types.Interaction, limit int) []*types.Interaction {
	if limit > 0 && len(interactions) > limit {
		// Add all interactions except the last one, limited by messageContextLimit
		// +1 because we're not counting the last interaction which is the pending response
		startIdx := len(interactions) - limit
		return interactions[startIdx : len(interactions)-1]
	}
	return interactions
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

func (s *HelixAPIServer) generateSessionName(ctx context.Context, user *types.User, orgID string, session *types.Session, provider, model, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ownerID := user.ID
	if user.TokenType == types.TokenTypeRunner {
		ownerID = oai.RunnerID
	}

	// Get last interaction ID
	lastInteractionID := "n/a"
	if len(session.Interactions) > 0 {
		lastInteractionID = session.Interactions[len(session.Interactions)-1].ID
	}

	ctx = oai.SetContextValues(ctx, &oai.ContextValues{
		OwnerID:       ownerID,
		SessionID:     session.ID,
		InteractionID: lastInteractionID,
		ProjectID:     user.ProjectID,
		SpecTaskID:    user.SpecTaskID,
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
		OrganizationID: orgID,
		Provider:       provider,
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
	interaction *types.Interaction,
	chatCompletionRequest openai.ChatCompletionRequest,
	options *controller.ChatCompletionOptions, rw http.ResponseWriter,
) error {
	// Ensure request is not streaming
	chatCompletionRequest.Stream = false

	// Set the session ID in the context to enable document ID tracking
	ctx = oai.SetContextSessionID(ctx, session.ID)
	// Also set the app ID in the context for OAuth token retrieval
	ctx = oai.SetContextAppID(ctx, session.ParentApp)
	// Make sure the app ID is also set in the options
	if session.ParentApp != "" {
		options.AppID = session.ParentApp
	}
	log.Info().
		Str("session_id", session.ID).
		Str("interaction_id", interaction.ID).
		Str("app_id", session.ParentApp).
		Msg("handleBlockingSession: set session ID in context for document tracking")

	start := time.Now()

	// Check if this is an external agent session
	if session.Metadata.AgentType == "zed_external" {
		log.Info().
			Str("session_id", session.ID).
			Str("agent_type", session.Metadata.AgentType).
			Msg("Routing non-streaming session to external agent")

		return s.handleExternalAgentStreaming(ctx, user, session, interaction, chatCompletionRequest, rw, start)
	}

	// Call the LLM
	chatCompletionResponse, _, err := s.Controller.ChatCompletion(ctx, user, chatCompletionRequest, options)
	if err != nil {
		// Update the session with the response
		interaction.Error = err.Error()
		interaction.State = types.InteractionStateError
		interaction.Completed = time.Now()
		interaction.DurationMs = int(time.Since(start).Milliseconds())

		// Create new context with a timeout for persisting session to the database.
		// Do not inherit the context from the caller, as it may be cancelled.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		writeErr := s.Controller.UpdateInteraction(ctx, session, interaction)
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
	interaction.ResponseMessage = chatCompletionResponse.Choices[0].Message.Content
	interaction.Completed = time.Now()
	interaction.State = types.InteractionStateComplete
	interaction.DurationMs = int(time.Since(start).Milliseconds())

	// Create new context with a timeout for persisting session to the database.
	// Do not inherit the context from the caller, as it may be cancelled.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = s.Controller.UpdateInteraction(ctx, session, interaction)
	if err != nil {
		return err
	}

	// Trigger async summary generation for this interaction and session title update
	s.triggerSummaryGeneration(session, interaction, user.ID)

	chatCompletionResponse.ID = session.ID

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	err = json.NewEncoder(rw).Encode(chatCompletionResponse)
	if err != nil {
		log.Err(err).Msg("error writing response")
	}

	return nil
}

func (s *HelixAPIServer) handleStreamingSession(ctx context.Context, user *types.User, session *types.Session, interaction *types.Interaction, chatCompletionRequest openai.ChatCompletionRequest, options *controller.ChatCompletionOptions, rw http.ResponseWriter) error {
	// Ensure request is streaming
	chatCompletionRequest.Stream = true

	// Set the session ID in the context to enable document ID tracking
	ctx = oai.SetContextSessionID(ctx, session.ID)
	// Also set the app ID in the context for OAuth token retrieval
	ctx = oai.SetContextAppID(ctx, session.ParentApp)
	// Make sure the app ID is also set in the options
	if session.ParentApp != "" {
		options.AppID = session.ParentApp
	}
	log.Info().
		Str("session_id", session.ID).
		Str("interaction_id", interaction.ID).
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

	start := time.Now()

	// Check if this is an external agent session
	if session.Metadata.AgentType == "zed_external" {
		log.Info().
			Str("session_id", session.ID).
			Str("agent_type", session.Metadata.AgentType).
			Msg("Routing session to external agent")

		return s.handleExternalAgentStreaming(ctx, user, session, interaction, chatCompletionRequest, rw, start)
	}

	// Instruct the agent to send thoughts about tools and decisions
	options.Conversational = true

	// Call the LLM
	stream, _, err := s.Controller.ChatCompletionStream(ctx, user, chatCompletionRequest, options)
	if err != nil {
		log.Error().
			Str("app_id", options.AppID).
			Str("session_id", session.ID).
			Err(err).
			Msg("error running controller chat completion stream")

		// Update the session with the response
		interaction.Error = err.Error()
		interaction.Completed = time.Now()
		interaction.State = types.InteractionStateError
		interaction.DurationMs = int(time.Since(start).Milliseconds())

		// Create new context with a timeout for persisting session to the database.
		// Do not inherit the context from the caller, as it may be cancelled.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Reload session to get any metadata updates that happened during streaming
		// (e.g., document IDs from knowledge/RAG results). This prevents the WebSocket
		// event from UpdateInteraction overriding the correct metadata sent earlier.
		if updatedSession, err := s.Store.GetSession(ctx, session.ID); err != nil {
			log.Warn().Err(err).Str("session_id", session.ID).Msg("failed to reload session metadata for initial error update")
		} else {
			// Preserve the interactions array from the original session object
			// but use the updated metadata from the database
			updatedSession.Interactions = session.Interactions
			session = updatedSession
			log.Debug().
				Str("session_id", session.ID).
				Interface("document_ids", session.Metadata.DocumentIDs).
				Msg("reloaded session metadata before initial error WebSocket update")
		}

		writeErr := s.Controller.UpdateInteraction(ctx, session, interaction)
		if writeErr != nil {
			return fmt.Errorf("error writing session: %w", writeErr)
		}

		// Write error message
		errorMsg := fmt.Sprintf("data: {\"error\":{\"message\":\"%s\"}}\n\n", err.Error())
		if _, err := rw.Write([]byte(errorMsg)); err != nil {
			log.Error().Err(err).Msg("failed to write error chunk")
		}

		if f, ok := rw.(http.Flusher); ok {
			f.Flush()
		}
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
			log.Error().
				Str("app_id", options.AppID).
				Str("session_id", session.ID).
				Any("response", response).
				Err(err).
				Msg("error receiving stream")

			// Update the interaction with what we have got so far
			interaction.ResponseMessage = fullResponse
			interaction.Completed = time.Now()
			interaction.State = types.InteractionStateError
			interaction.Error = err.Error()
			interaction.DurationMs = int(time.Since(start).Milliseconds())

			// Create new context with a timeout for persisting session to the database.
			// Do not inherit the context from the caller, as it may be cancelled.
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// Reload session to get any metadata updates that happened during streaming
			// (e.g., document IDs from knowledge/RAG results). This prevents the WebSocket
			// event from UpdateInteraction overriding the correct metadata sent earlier.
			if updatedSession, err := s.Store.GetSession(ctx, session.ID); err != nil {
				log.Warn().Err(err).Str("session_id", session.ID).Msg("failed to reload session metadata for error update")
			} else {
				// Preserve the interactions array from the original session object
				// but use the updated metadata from the database
				updatedSession.Interactions = session.Interactions
				session = updatedSession
				log.Debug().
					Str("session_id", session.ID).
					Interface("document_ids", session.Metadata.DocumentIDs).
					Msg("reloaded session metadata before error WebSocket update")
			}

			return s.Controller.UpdateInteraction(ctx, session, interaction)
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

	// Write last data: [DONE] chunk
	_ = writeChunk(rw, []byte("[DONE]"))

	// Update last interaction
	interaction.ResponseMessage = fullResponse
	interaction.Completed = time.Now()
	interaction.State = types.InteractionStateComplete
	interaction.DurationMs = int(time.Since(start).Milliseconds())

	// Create new context with a timeout for persisting session to the database.
	// Do not inherit the context from the caller, as it may be cancelled.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Reload session to get any metadata updates that happened during streaming
	// (e.g., document IDs from knowledge/RAG results). This prevents the WebSocket
	// event from UpdateInteraction overriding the correct metadata sent earlier.
	if updatedSession, err := s.Store.GetSession(ctx, session.ID); err != nil {
		log.Warn().Err(err).Str("session_id", session.ID).Msg("failed to reload session metadata for final update")
	} else {
		// Preserve the interactions array from the original session object
		// but use the updated metadata from the database
		updatedSession.Interactions = session.Interactions
		session = updatedSession
		log.Debug().
			Str("session_id", session.ID).
			Interface("document_ids", session.Metadata.DocumentIDs).
			Msg("reloaded session metadata before final WebSocket update")
	}

	err = s.Controller.UpdateInteraction(ctx, session, interaction)
	if err != nil {
		return fmt.Errorf("error updating interaction: %w", err)
	}

	// Trigger async summary generation for this interaction and session title update
	s.triggerSummaryGeneration(session, interaction, session.Owner)

	return nil
}

// handleExternalAgentStreaming routes chat messages to external Zed agent via WebSocket
func (s *HelixAPIServer) handleExternalAgentStreaming(ctx context.Context, user *types.User, session *types.Session, interaction *types.Interaction, chatCompletionRequest openai.ChatCompletionRequest, rw http.ResponseWriter, start time.Time) error {
	// Check if external agent executor is available
	if s.externalAgentExecutor == nil {
		log.Error().Str("session_id", session.ID).Msg("External agent executor not available")
		return s.writeErrorResponse(rw, "External agent not available")
	}

	// Get the external agent session
	agentSession, err := s.externalAgentExecutor.GetSession(session.ID)
	if err != nil {
		log.Error().Err(err).Str("session_id", session.ID).Msg("External agent session not found")
		return s.writeErrorResponse(rw, "External agent session not found")
	}

	// Ensure model parameter is set in chatCompletionRequest
	if chatCompletionRequest.Model == "" {
		// Use a default model for external agents or get from session metadata
		chatCompletionRequest.Model = "gpt-4" // Default model for external agents
		log.Debug().Str("session_id", session.ID).Str("model", chatCompletionRequest.Model).Msg("Set default model for external agent")
	}

	// Extract the user's message from the chat completion request
	var userMessage string
	if len(chatCompletionRequest.Messages) > 0 {
		lastMessage := chatCompletionRequest.Messages[len(chatCompletionRequest.Messages)-1]
		if lastMessage.Role == "user" {
			// Handle both simple content (string) and multi-content (complex structure)
			if lastMessage.Content != "" {
				userMessage = lastMessage.Content
			} else if len(lastMessage.MultiContent) > 0 {
				// Extract text from multi-content parts
				for _, part := range lastMessage.MultiContent {
					if part.Type == openai.ChatMessagePartTypeText {
						userMessage += part.Text
					}
				}
			}
		}
	}

	if userMessage == "" {
		log.Error().Str("session_id", session.ID).Msg("No user message found in chat completion request")
		return s.writeErrorResponse(rw, "No user message found")
	}

	log.Info().
		Str("session_id", session.ID).
		Str("user_message", userMessage).
		Str("agent_session_status", agentSession.Status).
		Msg("Sending message to external Zed agent")

	// Send message to external agent via WebSocket and stream response back
	return s.streamFromExternalAgent(ctx, session, userMessage, chatCompletionRequest, rw)
}

// streamFromExternalAgent sends a message to the external agent and streams the response
func (s *HelixAPIServer) streamFromExternalAgent(ctx context.Context, session *types.Session, userMessage string, chatCompletionRequest openai.ChatCompletionRequest, rw http.ResponseWriter) error {
	// Get the last interaction to update with the response
	if len(session.Interactions) == 0 {
		log.Error().Str("session_id", session.ID).Msg("No interactions found in session")
		return fmt.Errorf("no interactions found in session")
	}

	interaction := session.Interactions[len(session.Interactions)-1]
	start := time.Now()

	// Wait for external agent to be ready (WebSocket connection established)
	// Extended timeout to allow time for manual Moonlight client connection to kickstart container
	if err := s.waitForExternalAgentReady(ctx, session.ID, 300*time.Second); err != nil {
		log.Error().Err(err).Str("session_id", session.ID).Msg("External agent not ready")

		// Update interaction with error
		interaction.Error = fmt.Sprintf("External agent not ready: %s", err.Error())
		interaction.State = types.InteractionStateError
		interaction.Completed = time.Now()
		interaction.DurationMs = int(time.Since(start).Milliseconds())
		s.Controller.UpdateInteraction(ctx, session, interaction)

		http.Error(rw, fmt.Sprintf("External agent not ready: %s", err.Error()), http.StatusServiceUnavailable)
		return err
	}

	// Generate unique request ID for tracking
	requestID := fmt.Sprintf("req_%d", time.Now().UnixNano())

	// Determine which agent to use based on the spec task's code agent config
	agentName := s.getAgentNameForSession(ctx, session)

	// Send chat message to external agent
	// NEW PROTOCOL: Use acp_thread_id instead of zed_context_id
	command := types.ExternalAgentCommand{
		Type: "chat_message",
		Data: map[string]interface{}{
			"acp_thread_id": session.Metadata.ZedThreadID, // ACP thread ID (null on first message, triggers thread creation)
			"message":       userMessage,
			"request_id":    requestID, // For correlation
			"agent_name":    agentName, // Which agent to use (zed-agent or qwen)
			// NOTE: helix_session_id is sent via SyncMessage.SessionID, not in Data
		},
	}

	// Set up legacy channel handling for external agent communication
	responseChan := make(chan string, 100)
	doneChan := make(chan bool, 1)
	errorChan := make(chan error, 1)

	// Store response channel with request ID (would use proper storage in production)
	s.storeResponseChannel(session.ID, requestID, responseChan, doneChan, errorChan)
	defer s.cleanupResponseChannel(session.ID, requestID)

	// CRITICAL: Store session->interaction mapping so message_added can find the right interaction
	if s.sessionToWaitingInteraction == nil {
		s.sessionToWaitingInteraction = make(map[string]string)
	}
	s.sessionToWaitingInteraction[session.ID] = interaction.ID
	log.Info().
		Str("session_id", session.ID).
		Str("interaction_id", interaction.ID).
		Msg("🔗 [HELIX] Stored session->interaction mapping for streaming request")

	// CRITICAL: Store request_id->session mapping so thread_created can find the right session
	if s.requestToSessionMapping == nil {
		s.requestToSessionMapping = make(map[string]string)
	}
	s.requestToSessionMapping[requestID] = session.ID
	log.Info().
		Str("request_id", requestID).
		Str("session_id", session.ID).
		Msg("🔗 [HELIX] Stored request_id->session mapping for thread creation")

	log.Info().
		Str("session_id", session.ID).
		Str("request_id", requestID).
		Interface("command", command).
		Msg("🔴 [HELIX] SENDING CHAT_MESSAGE COMMAND TO EXTERNAL AGENT")

	log.Info().
		Str("helix_session_id", session.ID).
		Str("request_id", requestID).
		Str("user_message", userMessage).
		Msg("🗂️  [HELIX] SESSION MAPPING: Chat request details")

	// Send command to external agent
	if err := s.sendCommandToExternalAgent(session.ID, command); err != nil {
		log.Error().Err(err).Str("session_id", session.ID).Msg("Failed to send command to external agent")
		http.Error(rw, fmt.Sprintf("Failed to send command to external agent: %s", err.Error()), http.StatusInternalServerError)
		return err
	}

	log.Info().
		Str("session_id", session.ID).
		Str("request_id", requestID).
		Str("message", userMessage).
		Msg("Sent message to external agent")

	// Accumulate the full response for updating the interaction
	var fullResponse string

	// Create timeout timer ONCE before loop (not on each iteration)
	// This prevents multiple timers from being created
	timeout := time.NewTimer(90 * time.Second)
	defer timeout.Stop()

	// Stream response chunks as they arrive
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case chunk := <-responseChan:
			// Accumulate the response
			fullResponse += chunk

			response := &openai.ChatCompletionStreamResponse{
				Object: "chat.completion.chunk",
				ID:     session.ID,
				Model:  chatCompletionRequest.Model,
				Choices: []openai.ChatCompletionStreamChoice{
					{
						Index: 0,
						Delta: openai.ChatCompletionStreamChoiceDelta{
							Content: chunk,
						},
					},
				},
			}

			bts, err := json.Marshal(response)
			if err != nil {
				log.Error().Err(err).Msg("failed to marshal response")
				continue
			}

			if err := writeChunk(rw, bts); err != nil {
				log.Error().Err(err).Msg("failed to write chunk")
				return err
			}

		case <-doneChan:
			// CRITICAL: Stop the timeout timer to prevent it from firing after completion
			timeout.Stop()
			// CRITICAL: For external agent flow, the response was already saved to DB by handleMessageAdded
			// We need to RELOAD the interaction from DB to get the final response, not use fullResponse
			// which is only accumulated from responseChan (unused in external agent flow)

			// Reload the interaction from database to get the final response
			reloadCtx, cancelReload := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelReload()

			reloadedInteraction, err := s.Controller.Options.Store.GetInteraction(reloadCtx, interaction.ID)
			if err != nil {
				log.Error().Err(err).
					Str("session_id", session.ID).
					Str("interaction_id", interaction.ID).
					Msg("Failed to reload interaction from database")
			} else {
				// Use the response from database (which was saved by handleMessageAdded)
				interaction.ResponseMessage = reloadedInteraction.ResponseMessage
				log.Info().
					Str("session_id", session.ID).
					Str("interaction_id", interaction.ID).
					Int("response_length", len(reloadedInteraction.ResponseMessage)).
					Msg("🔄 [HELIX] Reloaded interaction response from database for doneChan")
			}

			// Mark as complete
			interaction.Completed = time.Now()
			interaction.State = types.InteractionStateComplete
			interaction.DurationMs = int(time.Since(start).Milliseconds())

			// Create new context with a timeout for persisting session to the database
			updateCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			err = s.Controller.UpdateInteraction(updateCtx, session, interaction)
			if err != nil {
				log.Error().Err(err).Str("session_id", session.ID).Msg("Failed to update interaction")
			}

			// Send completion signal
			response := &openai.ChatCompletionStreamResponse{
				Object: "chat.completion.chunk",
				ID:     session.ID,
				Model:  chatCompletionRequest.Model,
				Choices: []openai.ChatCompletionStreamChoice{
					{
						Index:        0,
						Delta:        openai.ChatCompletionStreamChoiceDelta{},
						FinishReason: "stop",
					},
				},
			}

			bts, err := json.Marshal(response)
			if err == nil {
				writeChunk(rw, bts)
			}

			// Send [DONE] signal
			if err := writeChunk(rw, []byte("[DONE]")); err != nil {
				log.Error().Err(err).Msg("failed to write done signal")
			}

			log.Info().
				Str("session_id", session.ID).
				Str("request_id", requestID).
				Str("response_message", interaction.ResponseMessage).
				Int("response_length", len(interaction.ResponseMessage)).
				Int("duration_ms", interaction.DurationMs).
				Msg("External agent response completed and interaction updated")
			return nil

		case err := <-errorChan:
			// Update interaction with error
			interaction.Error = err.Error()
			interaction.State = types.InteractionStateError
			interaction.Completed = time.Now()
			interaction.DurationMs = int(time.Since(start).Milliseconds())

			updateCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			s.Controller.UpdateInteraction(updateCtx, session, interaction)

			log.Error().Err(err).Str("session_id", session.ID).Str("request_id", requestID).Msg("External agent response error")
			return s.writeErrorResponse(rw, "External agent error: "+err.Error())

		case <-timeout.C:
			// Update interaction with timeout error
			interaction.Error = "External agent response timeout"
			interaction.State = types.InteractionStateError
			interaction.Completed = time.Now()
			interaction.DurationMs = int(time.Since(start).Milliseconds())

			updateCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			s.Controller.UpdateInteraction(updateCtx, session, interaction)

			log.Warn().Str("session_id", session.ID).Str("request_id", requestID).Msg("External agent response timeout")
			return s.writeErrorResponse(rw, "External agent response timeout")
		}
	}
}

// waitForExternalAgentReady waits for the external agent WebSocket connection to be established
func (s *HelixAPIServer) waitForExternalAgentReady(ctx context.Context, sessionID string, timeout time.Duration) error {
	log.Info().
		Str("session_id", sessionID).
		Dur("timeout", timeout).
		Msg("Waiting for external agent to be ready")

	startCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	startTime := time.Now()
	attemptCount := 0

	for {
		select {
		case <-startCtx.Done():
			elapsed := time.Since(startTime)
			log.Error().
				Str("session_id", sessionID).
				Dur("elapsed", elapsed).
				Int("attempts", attemptCount).
				Msg("Timeout waiting for external agent to be ready")
			return fmt.Errorf("timeout waiting for external agent to be ready after %v (%d attempts)", elapsed, attemptCount)
		case <-ticker.C:
			attemptCount++

			// Check if any external Zed agent WebSocket connections exist
			connections := s.externalAgentWSManager.listConnections()
			log.Info().
				Int("connection_count", len(connections)).
				Interface("connections", connections).
				Msg("🔍 [HELIX] Checking external agent connections")
			if len(connections) > 0 {
				elapsed := time.Since(startTime)
				log.Info().
					Str("session_id", sessionID).
					Dur("elapsed", elapsed).
					Int("attempts", attemptCount).
					Msg("External agent is ready")
				return nil
			}

			// Log periodic status updates
			elapsed := time.Since(startTime)
			if attemptCount%10 == 0 || elapsed > 10*time.Second {
				timeLeft := timeout - elapsed
				log.Debug().
					Str("session_id", sessionID).
					Dur("elapsed", elapsed).
					Dur("time_left", timeLeft).
					Int("attempt", attemptCount).
					Msg("Still waiting for external agent to connect")
			}
		}
	}
}

// Response channel management for external agent requests
var responseChannels = make(map[string]map[string]chan string)
var doneChannels = make(map[string]map[string]chan bool)
var errorChannels = make(map[string]map[string]chan error)
var channelMutex sync.RWMutex

// storeResponseChannel stores response channels for a request
func (s *HelixAPIServer) storeResponseChannel(sessionID, requestID string, responseChan chan string, doneChan chan bool, errorChan chan error) {
	channelMutex.Lock()
	defer channelMutex.Unlock()

	if responseChannels[sessionID] == nil {
		responseChannels[sessionID] = make(map[string]chan string)
		doneChannels[sessionID] = make(map[string]chan bool)
		errorChannels[sessionID] = make(map[string]chan error)
	}

	responseChannels[sessionID][requestID] = responseChan
	doneChannels[sessionID][requestID] = doneChan
	errorChannels[sessionID][requestID] = errorChan
}

// cleanupResponseChannel cleans up channels for a request
func (s *HelixAPIServer) cleanupResponseChannel(sessionID, requestID string) {
	channelMutex.Lock()
	defer channelMutex.Unlock()

	if responseChannels[sessionID] != nil {
		delete(responseChannels[sessionID], requestID)
		delete(doneChannels[sessionID], requestID)
		delete(errorChannels[sessionID], requestID)

		if len(responseChannels[sessionID]) == 0 {
			delete(responseChannels, sessionID)
			delete(doneChannels, sessionID)
			delete(errorChannels, sessionID)
		}
	}
}

// getResponseChannel gets response channels for a request
func (s *HelixAPIServer) getResponseChannel(sessionID, requestID string) (chan string, chan bool, chan error, bool) {
	channelMutex.RLock()
	defer channelMutex.RUnlock()

	if responseChannels[sessionID] != nil {
		responseChan, responseExists := responseChannels[sessionID][requestID]
		doneChan, doneExists := doneChannels[sessionID][requestID]
		errorChan, errorExists := errorChannels[sessionID][requestID]

		if responseExists && doneExists && errorExists {
			return responseChan, doneChan, errorChan, true
		}
	}

	return nil, nil, nil, false
}

// writeErrorResponse writes an error response in SSE format
func (s *HelixAPIServer) writeErrorResponse(rw http.ResponseWriter, errorMsg string) error {
	// Write error message in SSE format
	errorMsg = fmt.Sprintf("data: {\"error\":{\"message\":\"%s\"}}\n\n", errorMsg)
	_, err := rw.Write([]byte(errorMsg))
	if err != nil {
		return err
	}

	if f, ok := rw.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}

// getSessionStepInfo godoc
// @Summary Get session step info
// @Description Get session step info
// @Tags    session

// @Success 200 {array} types.StepInfo
// @Router /api/v1/sessions/{id}/step-info [get]
// @Security BearerAuth
func (s *HelixAPIServer) getSessionStepInfo(_ http.ResponseWriter, req *http.Request) ([]*types.StepInfo, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	id := mux.Vars(req)["id"]

	session, err := s.Store.GetSession(ctx, id)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get session %s, error: %s", id, err))
	}

	if session.Owner != user.ID {
		return nil, system.NewHTTPError403("you are not allowed to access this session")
	}

	stepInfos, err := s.Store.ListStepInfos(ctx, &store.ListStepInfosQuery{
		SessionID: id,
	})
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get step infos for session %s, error: %s", id, err))
	}

	return stepInfos, nil
}

// getSessionRDPConnection godoc
// @Summary Get Wolf connection info for a session
// @Description Get Wolf streaming connection details for accessing a session (replaces RDP)
// @Tags    sessions
// @Success 200 {object} map[string]interface{}
// @Param id path string true "Session ID"
// @Router /api/v1/sessions/{id}/rdp-connection [get]
// @Security BearerAuth
func (s *HelixAPIServer) getSessionRDPConnection(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	user := getRequestUser(req)
	if user == nil {
		http.Error(rw, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	id := vars["id"]

	// Get the session to validate ownership
	session, err := s.Store.GetSession(ctx, id)
	if err != nil {
		http.Error(rw, fmt.Sprintf("session %s not found", id), http.StatusNotFound)
		return
	}

	if session.Owner != user.ID {
		http.Error(rw, "you are not allowed to access this session", http.StatusForbidden)
		return
	}

	// Check if this is an external agent session
	if session.Metadata.AgentType != "zed_external" {
		log.Error().
			Str("session_id", id).
			Str("agent_type", session.Metadata.AgentType).
			Msg("Session is not an external agent session - Wolf access not available")
		http.Error(rw, "Wolf access not available for this session", http.StatusNotFound)
		return
	}

	if s.externalAgentExecutor == nil {
		http.Error(rw, "external agent executor not available", http.StatusServiceUnavailable)
		return
	}

	// Get the external agent session info
	agentSession, err := s.externalAgentExecutor.GetSession(id)
	if err != nil {
		log.Error().
			Err(err).
			Str("session_id", id).
			Msg("Failed to find external agent session")
		http.Error(rw, fmt.Sprintf("session %s not found: %s", id, err.Error()), http.StatusNotFound)
		return
	}

	log.Info().
		Str("session_id", id).
		Str("status", agentSession.Status).
		Msg("Found external agent session (Wolf-based)")

	// Return Wolf-based connection details with WebSocket info
	connectionInfo := types.ExternalAgentConnectionInfo{
		SessionID:          agentSession.SessionID,
		ScreenshotURL:      fmt.Sprintf("/api/v1/external-agents/%s/screenshot", agentSession.SessionID),
		StreamURL:          "moonlight://localhost:47989",
		Status:             agentSession.Status,
		WebsocketURL:       fmt.Sprintf("wss://%s/api/v1/external-agents/sync?session_id=%s", req.Host, agentSession.SessionID),
		WebsocketConnected: s.isExternalAgentConnected(agentSession.SessionID),
	}

	log.Info().
		Str("session_id", agentSession.SessionID).
		Str("status", agentSession.Status).
		Bool("websocket_connected", connectionInfo.WebsocketConnected).
		Msg("Returning Wolf connection info")

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(connectionInfo)
}

// resumeSession godoc
// @Summary Resume a paused external agent session
// @Description Restarts the external agent container for a session that has been stopped
// @Tags    sessions
// @Success 200 {object} map[string]interface{}
// @Param id path string true "Session ID"
// @Router /api/v1/sessions/{id}/resume [post]
// @Security BearerAuth
func (s *HelixAPIServer) resumeSession(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	user := getRequestUser(req)
	if user == nil {
		http.Error(rw, "unauthorized", http.StatusUnauthorized)
		return
	}

	id := mux.Vars(req)["id"]

	log.Info().
		Str("session_id", id).
		Str("user_id", user.ID).
		Msg("Resume session request")

	// Get the session to check if it's an external agent session
	session, err := s.Controller.Options.Store.GetSession(ctx, id)
	if err != nil {
		log.Error().
			Err(err).
			Str("session_id", id).
			Msg("Failed to get session for resume")
		http.Error(rw, fmt.Sprintf("session not found: %s", err.Error()), http.StatusNotFound)
		return
	}

	// Check if user owns this session
	if session.Owner != user.ID {
		log.Warn().
			Str("session_id", id).
			Str("user_id", user.ID).
			Str("owner_id", session.Owner).
			Msg("User not authorized to resume session")
		http.Error(rw, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Check if this is an external agent session
	if session.Metadata.AgentType != "zed_external" {
		log.Warn().
			Str("session_id", id).
			Str("agent_type", session.Metadata.AgentType).
			Msg("Session is not an external agent session - cannot resume")
		http.Error(rw, "only external agent sessions can be resumed", http.StatusBadRequest)
		return
	}

	if s.externalAgentExecutor == nil {
		http.Error(rw, "external agent executor not available", http.StatusServiceUnavailable)
		return
	}

	// Create a new ZedAgent request to restart the agent
	// We need to get the spec task ID if this was a spec task session
	specTaskID := session.Metadata.SpecTaskID

	// Build the ZedAgent config for resume
	agent := &types.ZedAgent{
		SessionID:      id,
		UserID:         user.ID,
		HelixSessionID: id, // This session already exists
		Input:          "Resume session",
		ProjectPath:    "workspace",
		SpecTaskID:     specTaskID,
		// WorkDir left empty - wolf_executor will use filestore path for persistence
	}

	// Load project context for both SpecTask AND exploratory sessions
	// This ensures startup scripts run on resume for all project-scoped sessions
	var projectID string

	if specTaskID != "" {
		// SpecTask session - get project from task
		specTask, err := s.Controller.Options.Store.GetSpecTask(ctx, specTaskID)
		if err != nil {
			log.Warn().
				Err(err).
				Str("spec_task_id", specTaskID).
				Msg("Failed to load spec task for resume, continuing without repository info")
		} else if specTask.ProjectID != "" {
			projectID = specTask.ProjectID
		}
	} else if session.Metadata.ProjectID != "" {
		// Exploratory session - get project from session metadata
		projectID = session.Metadata.ProjectID
		log.Info().
			Str("session_id", id).
			Str("project_id", projectID).
			Msg("Loading project context for exploratory session resume")
	}

	// If we have a project, load repositories and startup script
	if projectID != "" {
		agent.ProjectID = projectID

		projectRepos, err := s.Controller.Options.Store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{
			ProjectID: projectID,
		})
		if err == nil && len(projectRepos) > 0 {
			agent.RepositoryIDs = make([]string, 0, len(projectRepos))
			for _, repo := range projectRepos {
				if repo.ID != "" {
					agent.RepositoryIDs = append(agent.RepositoryIDs, repo.ID)
				}
			}

			// Set primary repository from project (repos are now managed at project level)
			project, err := s.Controller.Options.Store.GetProject(ctx, projectID)
			if err == nil && project.DefaultRepoID != "" {
				agent.PrimaryRepositoryID = project.DefaultRepoID
			} else if len(projectRepos) > 0 {
				// Use first repo as fallback if no default set
				agent.PrimaryRepositoryID = projectRepos[0].ID
			}
		}
	}

	// Get display settings from app's ExternalAgentConfig (or use defaults)
	// The app ID comes from session.ParentApp (the agent assigned to the session)
	if session.ParentApp != "" {
		app, err := s.Controller.Options.Store.GetApp(ctx, session.ParentApp)
		if err == nil && app != nil && app.Config.Helix.ExternalAgentConfig != nil {
			width, height := app.Config.Helix.ExternalAgentConfig.GetEffectiveResolution()
			agent.DisplayWidth = width
			agent.DisplayHeight = height
			if app.Config.Helix.ExternalAgentConfig.DisplayRefreshRate > 0 {
				agent.DisplayRefreshRate = app.Config.Helix.ExternalAgentConfig.DisplayRefreshRate
			}
			// CRITICAL: Also get resolution preset, zoom level, and desktop type for proper HiDPI scaling
			agent.Resolution = app.Config.Helix.ExternalAgentConfig.Resolution
			agent.ZoomLevel = app.Config.Helix.ExternalAgentConfig.GetEffectiveZoomLevel()
			agent.DesktopType = app.Config.Helix.ExternalAgentConfig.GetEffectiveDesktopType()
			log.Debug().
				Str("session_id", id).
				Str("app_id", session.ParentApp).
				Int("display_width", width).
				Int("display_height", height).
				Int("display_refresh_rate", agent.DisplayRefreshRate).
				Str("resolution", agent.Resolution).
				Int("zoom_level", agent.ZoomLevel).
				Str("desktop_type", agent.DesktopType).
				Msg("Using display settings from app's ExternalAgentConfig for session resume")
		}
	}

	// Add user's API token to agent environment for git operations
	if err := s.addUserAPITokenToAgent(ctx, agent, session.Owner); err != nil {
		log.Error().Err(err).Str("user_id", session.Owner).Msg("Failed to add user API token to agent")
		http.Error(rw, fmt.Sprintf("failed to get user API keys: %v", err), http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("session_id", id).
		Str("spec_task_id", specTaskID).
		Msg("Resuming external agent session")

	// Start the external agent (this will create a new Wolf lobby)
	response, err := s.externalAgentExecutor.StartDesktop(ctx, agent)
	if err != nil {
		log.Error().
			Err(err).
			Str("session_id", id).
			Msg("Failed to resume external agent")
		http.Error(rw, fmt.Sprintf("failed to resume agent: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Update session metadata with new Wolf lobby info (only if non-empty)
	// Don't overwrite existing metadata with empty values from lobby reuse
	if response.WolfLobbyID != "" {
		session.Metadata.WolfLobbyID = response.WolfLobbyID
	}
	if response.WolfLobbyPIN != "" {
		session.Metadata.WolfLobbyPIN = response.WolfLobbyPIN
	}
	// CRITICAL: Clear PausedScreenshotPath when resuming
	// Otherwise the screenshot API returns the old saved screenshot instead of live RevDial fetch
	session.Metadata.PausedScreenshotPath = ""
	_, err = s.Controller.Options.Store.UpdateSession(ctx, *session)
	if err != nil {
		log.Error().
			Err(err).
			Str("session_id", id).
			Msg("Failed to update session metadata with new lobby info")
		// Don't fail the request - the agent is running
	}

	log.Info().
		Str("session_id", id).
		Str("wolf_lobby_id", response.WolfLobbyID).
		Msg("External agent session resumed successfully")

	// If session has a ZedThreadID, send open_thread command to Zed
	// This tells Zed to open the last thread in the AgentPanel UI
	if session.Metadata.ZedThreadID != "" {
		// Get the agent name from the session's code agent runtime
		agentName := session.Metadata.CodeAgentRuntime.ZedAgentName()

		go func() {
			// Wait for Zed WebSocket to connect (typically takes 3-4 seconds)
			// Retry mechanism: try multiple times with delays
			maxRetries := 5
			retryDelay := 2 * time.Second

			for attempt := 1; attempt <= maxRetries; attempt++ {
				time.Sleep(retryDelay)

				err := s.sendOpenThreadCommand(id, session.Metadata.ZedThreadID, agentName)
				if err == nil {
					// Success - command sent
					log.Info().
						Str("session_id", id).
						Str("thread_id", session.Metadata.ZedThreadID).
						Str("agent_name", agentName).
						Int("attempt", attempt).
						Msg("✅ Sent open_thread command to Zed")
					return
				}

				// Log retry attempt
				log.Warn().
					Err(err).
					Str("session_id", id).
					Str("thread_id", session.Metadata.ZedThreadID).
					Int("attempt", attempt).
					Int("max_retries", maxRetries).
					Msg("Retrying open_thread command (WebSocket not connected yet)")
			}

			// All retries exhausted - log final failure
			log.Error().
				Str("session_id", id).
				Str("thread_id", session.Metadata.ZedThreadID).
				Int("retries", maxRetries).
				Msg("❌ Failed to send open_thread command after all retries - WebSocket never connected")
		}()
	}

	// Return success response
	result := types.SessionResumeResponse{
		SessionID:     id,
		Status:        "resumed",
		WolfLobbyID:   response.WolfLobbyID,
		WolfLobbyPIN:  response.WolfLobbyPIN,
		ScreenshotURL: response.ScreenshotURL,
	}

	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(result)
}

// sendOpenThreadCommand sends an open_thread command to Zed via WebSocket
// to tell it to open a specific thread in the AgentPanel UI.
// agentName specifies which ACP agent to use (e.g., "qwen", "claude", "gemini", "codex")
// or empty string for NativeAgent (Zed's built-in agent).
func (s *HelixAPIServer) sendOpenThreadCommand(sessionID string, acpThreadID string, agentName string) error {
	data := map[string]interface{}{
		"acp_thread_id": acpThreadID,
	}
	// Only include agent_name if it's set (ACP agents need this, NativeAgent doesn't)
	if agentName != "" {
		data["agent_name"] = agentName
	}

	command := types.ExternalAgentCommand{
		Type: "open_thread",
		Data: data,
	}

	// Use the unified sendCommandToExternalAgent which handles connection lookup and routing
	return s.sendCommandToExternalAgent(sessionID, command)
}

// getSessionIdleStatus godoc
// @Summary Get idle status for external agent session
// @Description Returns idle timeout information for a session with an external agent
// @Tags    sessions
// @Success 200 {object} types.SessionIdleStatus
// @Failure 400 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Param id path string true "Session ID"
// @Router /api/v1/sessions/{id}/idle-status [get]
// @Security BearerAuth
func (s *HelixAPIServer) getSessionIdleStatus(_ http.ResponseWriter, req *http.Request) (*types.SessionIdleStatus, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	vars := mux.Vars(req)
	sessionID := vars["id"]

	// Get session to verify access
	session, err := s.Store.GetSession(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session")
		return nil, system.NewHTTPError404("session not found")
	}

	// Verify user owns this session
	if user == nil || session.Owner != user.ID {
		return nil, system.NewHTTPError403("access denied")
	}

	// Check if this is an external agent session
	if session.Metadata.AgentType != "zed_external" {
		return &types.SessionIdleStatus{
			HasExternalAgent: false,
		}, nil
	}

	// Try to find the external agent activity record
	// External agent ID is the same as the session ID for external agent sessions
	activity, err := s.Store.GetExternalAgentActivity(ctx, sessionID)
	if err != nil {
		// No activity record found - might be a very new session
		log.Debug().
			Str("session_id", sessionID).
			Msg("No external agent activity found for session")

		return &types.SessionIdleStatus{
			HasExternalAgent: true,
			IdleMinutes:      0,
			WillTerminateIn:  30,
			WarningThreshold: false,
		}, nil
	}

	// Calculate idle minutes
	idleMinutes := int(time.Since(activity.LastInteraction).Minutes())

	// Idle threshold is 30 minutes
	const idleThreshold = 30
	willTerminateIn := idleThreshold - idleMinutes
	if willTerminateIn < 0 {
		willTerminateIn = 0
	}

	// Show warning when 25 minutes or less remaining (for easy testing visibility)
	warningThreshold := willTerminateIn <= 25 && willTerminateIn > 0

	return &types.SessionIdleStatus{
		HasExternalAgent: true,
		IdleMinutes:      idleMinutes,
		WillTerminateIn:  willTerminateIn,
		WarningThreshold: warningThreshold,
	}, nil
}

// stopExternalAgentSession godoc
// @Summary Stop external Zed agent session
// @Description Stop the external Zed agent for any session (stops container, keeps session record)
// @Tags Sessions
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} map[string]string
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/sessions/{id}/stop-external-agent [delete]
func (s *HelixAPIServer) stopExternalAgentSession(_ http.ResponseWriter, r *http.Request) (map[string]string, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	vars := mux.Vars(r)
	sessionID := vars["id"]

	// Get session to verify access and check if it's an external agent session
	session, err := s.Store.GetSession(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session")
		return nil, system.NewHTTPError404("session not found")
	}

	// Verify user owns this session
	if user == nil || session.Owner != user.ID {
		return nil, system.NewHTTPError403("access denied")
	}

	// Check if this is an external agent session
	if session.Metadata.AgentType != "zed_external" {
		return nil, system.NewHTTPError400("session does not have an external Zed agent")
	}

	// Stop the Zed agent (Wolf container)
	err = s.externalAgentExecutor.StopDesktop(ctx, sessionID)
	if err != nil {
		log.Error().
			Err(err).
			Str("session_id", sessionID).
			Msg("Failed to stop external Zed agent")
		return nil, system.NewHTTPError500("failed to stop external Zed agent")
	}

	log.Info().
		Str("session_id", sessionID).
		Str("user_id", user.ID).
		Msg("External Zed agent stopped successfully")

	return map[string]string{
		"message":    "external Zed agent stopped",
		"session_id": sessionID,
	}, nil
}

// triggerSummaryGeneration triggers async summary generation for an interaction
// and session title update. This is called when an interaction completes.
func (s *HelixAPIServer) triggerSummaryGeneration(session *types.Session, interaction *types.Interaction, ownerID string) {
	if s.summaryService == nil {
		return
	}

	// Skip summary generation for internal requests (avoid infinite loops)
	// These are identified by special session metadata or missing user context
	if session.Metadata.SystemSession {
		log.Debug().
			Str("session_id", session.ID).
			Msg("Skipping summary generation for system session")
		return
	}

	// Use a fresh context (not tied to request context which may be cancelled)
	ctx := context.Background()

	// Generate summary for the interaction
	s.summaryService.GenerateInteractionSummaryAsync(ctx, interaction, ownerID)

	// Update session title based on TOC
	s.summaryService.UpdateSessionTitleAsync(ctx, session.ID, ownerID)
}
