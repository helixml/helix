// sessions are the higher level ChatGPT like UI concept

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"

	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

type RunSessionRequest struct {
	OrganizationID string
	App            *types.App
	Session        *types.Session
	User           *types.User

	InteractionID string // Optional, will generate a new interaction ID if not provided

	PromptMessage types.MessageContent

	HistoryLimit int // Do -1 to not include any interactions
}

const DefaultHistoryLimit = 6

// RunSessionBlocking - creates the interaction, runs the chat completion and returns the updated integration (already updated in the database)
func (c *Controller) RunBlockingSession(ctx context.Context, req *RunSessionRequest) (*types.Interaction, error) {
	if req.User.Deactivated {
		return nil, fmt.Errorf("user is deactivated")
	}

	if len(req.PromptMessage.Parts) == 0 {
		return nil, fmt.Errorf("prompt message is required")
	}

	if req.HistoryLimit == 0 {
		req.HistoryLimit = DefaultHistoryLimit
	}

	var (
		interactions []*types.Interaction
		err          error
	)

	if req.HistoryLimit != -1 {
		interactions, _, err = c.Options.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
			SessionID:    req.Session.ID,
			GenerationID: req.Session.GenerationID,
			PerPage:      req.HistoryLimit,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list interactions for session '%s': %w", req.Session.ID, err)
		}
	}

	systemPrompt := req.Session.Metadata.SystemPrompt

	if req.App != nil {
		systemPrompt = req.App.Config.Helix.Assistants[0].SystemPrompt
	}

	var interactionID string

	if req.InteractionID != "" {
		interactionID = req.InteractionID
	} else {
		interactionID = system.GenerateInteractionID()
	}

	// Add the new message to the existing session
	interaction := &types.Interaction{
		ID:                   interactionID,
		Trigger:              req.Session.Trigger,
		GenerationID:         req.Session.GenerationID,
		AppID:                req.App.ID,
		Created:              time.Now(),
		Updated:              time.Now(),
		SessionID:            req.Session.ID,
		UserID:               req.User.ID,
		SystemPrompt:         systemPrompt,
		Mode:                 types.SessionModeInference,
		State:                types.InteractionStateWaiting,
		PromptMessage:        req.PromptMessage.Parts[0].(string),
		PromptMessageContent: req.PromptMessage,
	}

	interactions = append(interactions, interaction)

	err = c.WriteInteractions(ctx, interaction)
	if err != nil {
		log.Error().Any("interaction", interaction).Err(err).Msg("failed to create interaction")
		return nil, fmt.Errorf("failed to create interaction: %w", err)
	}

	messages := types.InteractionsToOpenAIMessages(systemPrompt, interactions)

	request := openai.ChatCompletionRequest{
		Stream:   false,
		Messages: messages,
	}

	bts, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		log.Error().
			Err(err).
			Str("app_id", req.App.ID).
			Msg("failed to marshal request")
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx = oai.SetContextOrganizationID(ctx, req.OrganizationID)
	ctx = oai.SetContextSessionID(ctx, req.Session.ID)
	ctx = oai.SetContextValues(ctx, &oai.ContextValues{
		OwnerID:         req.User.ID,
		SessionID:       req.Session.ID,
		InteractionID:   interactionID,
		ProjectID:       req.User.ProjectID,
		SpecTaskID:      req.User.SpecTaskID,
		OriginalRequest: bts,
	})

	ctx = oai.SetContextAppID(ctx, req.App.ID)

	start := time.Now()

	resp, _, err := c.ChatCompletion(ctx, req.User, request, &ChatCompletionOptions{
		OrganizationID: req.Session.OrganizationID,
		AppID:          req.App.ID,
		Conversational: true,
	})
	if err != nil {
		interaction.Error = err.Error()
		interaction.State = types.InteractionStateError
		interaction.Completed = time.Now()
		interaction.DurationMs = int(time.Since(start).Milliseconds())

		updateErr := c.UpdateInteraction(ctx, req.Session, interaction)
		if updateErr != nil {
			log.Error().
				Err(updateErr).
				Str("app_id", req.App.ID).
				Str("user_id", req.User.ID).
				Str("session_id", req.Session.ID).
				Msg("failed to update interactions")
		}

		return nil, fmt.Errorf("failed to get response from inference API: %w", err)
	}

	interaction.ResponseMessage = resp.Choices[0].Message.Content
	interaction.State = types.InteractionStateComplete
	interaction.Completed = time.Now()
	interaction.DurationMs = int(time.Since(start).Milliseconds())
	interaction.Usage = types.Usage{
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
	}

	err = c.UpdateInteraction(ctx, req.Session, interaction)
	if err != nil {
		log.Error().
			Err(err).
			Str("app_id", req.App.ID).
			Str("user_id", req.User.ID).
			Str("session_id", req.Session.ID).
			Msg("failed to update session")
	}

	return interaction, nil
}

func (c *Controller) isUserProTier(ctx context.Context, owner string) (bool, error) {
	usermeta, err := c.Options.Store.GetUserMeta(ctx, owner)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return false, nil
		}
		return false, err
	}

	if usermeta.Config.StripeSubscriptionActive {
		return true, nil
	}

	return false, nil
}

// checkInferenceTokenQuota checks if the user has exceeded their monthly token quota for global providers
func (c *Controller) checkInferenceTokenQuota(ctx context.Context, userID string, provider string) error {
	// Skip quota check for runner tokens (system-level access)
	// Runner tokens are used by internal services like Kodit for enrichments
	if userID == "runner-system" {
		return nil
	}

	// Skip quota check if inference quotas are disabled
	if !c.Options.Config.SubscriptionQuotas.Enabled || !c.Options.Config.SubscriptionQuotas.Inference.Enabled {
		return nil
	}

	if !types.IsGlobalProvider(provider) {
		return nil
	}

	// Get user's current monthly usage
	monthlyTokens, err := c.Options.Store.GetUserMonthlyTokenUsage(ctx, userID, types.GlobalProviders)
	if err != nil {
		return fmt.Errorf("failed to get user token usage: %w", err)
	}

	// Check if user is pro tier
	pro, err := c.isUserProTier(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to check user tier: %w", err)
	}

	var limit int
	var strict bool

	if pro {
		limit = c.Options.Config.SubscriptionQuotas.Inference.Pro.MaxMonthlyTokens
		strict = c.Options.Config.SubscriptionQuotas.Inference.Pro.Strict
	} else {
		limit = c.Options.Config.SubscriptionQuotas.Inference.Free.MaxMonthlyTokens
		strict = c.Options.Config.SubscriptionQuotas.Inference.Free.Strict
	}

	if monthlyTokens >= limit {
		if strict {
			tierName := "free"
			if pro {
				tierName = "pro"
			}
			return fmt.Errorf("monthly token limit exceeded for %s tier (%d/%d tokens used). Please upgrade your plan or wait until next month to continue", tierName, monthlyTokens, limit)
		}
		// Log warning for soft limits
		log.Warn().
			Str("user_id", userID).
			Int("usage", monthlyTokens).
			Int("limit", limit).
			Bool("pro_tier", pro).
			Msg("user approaching token limit")
	}

	return nil
}

func (c *Controller) UpdateSessionMetadata(ctx context.Context, session *types.Session, meta *types.SessionMetadata) (*types.SessionMetadata, error) {
	session.Updated = time.Now()
	session.Metadata = *meta

	// Log RAG results before update
	ragResultsCount := 0
	if meta.SessionRAGResults != nil {
		ragResultsCount = len(meta.SessionRAGResults)
	}
	log.Debug().
		Str("session_id", session.ID).
		Int("rag_results_count", ragResultsCount).
		Bool("has_rag_results", meta.SessionRAGResults != nil).
		Msg("🟢 updating session metadata with RAG results")

	sessionData, err := c.Options.Store.UpdateSession(ctx, *session)
	if err != nil {
		return nil, err
	}

	log.Debug().
		Str("session_id", sessionData.ID).
		Msg("🟢 update session config")

	// Send WebSocket notification about the updated session metadata
	if err := c.WriteSession(ctx, session); err != nil {
		log.Error().Err(err).Str("session_id", session.ID).Msg("failed to send WebSocket notification for metadata update")
		// We don't return an error here as the metadata was successfully updated in the database
	}

	// Verify RAG results in updated metadata
	updatedRagResultsCount := 0
	if sessionData.Metadata.SessionRAGResults != nil {
		updatedRagResultsCount = len(sessionData.Metadata.SessionRAGResults)
	}
	log.Debug().
		Str("session_id", sessionData.ID).
		Int("updated_rag_results_count", updatedRagResultsCount).
		Bool("has_updated_rag_results", sessionData.Metadata.SessionRAGResults != nil).
		Msg("🟢 session metadata updated with RAG results")

	return &sessionData.Metadata, nil
}

func (c *Controller) WriteInteractions(ctx context.Context, interactions ...*types.Interaction) error {
	return c.Options.Store.CreateInteractions(ctx, interactions...)
}

func (c *Controller) UpdateInteraction(ctx context.Context, session *types.Session, interaction *types.Interaction) error {
	updated, err := c.Options.Store.UpdateInteraction(ctx, interaction)
	if err != nil {
		return err
	}

	// Update or append
	found := false
	for idx, existingInteraction := range session.Interactions {
		if existingInteraction.ID == interaction.ID {
			session.Interactions[idx] = updated
			found = true
			break
		}
	}

	if !found {
		session.Interactions = append(session.Interactions, updated)
	}

	event := &types.WebsocketEvent{
		Type:      types.WebsocketEventSessionUpdate,
		SessionID: interaction.SessionID,
		Owner:     interaction.UserID,
		Session:   session,
	}

	_ = c.publishEvent(context.Background(), event)

	return nil
}

// WriteSession updates a session and emits a WebsocketEventSessionUpdate
func (c *Controller) WriteSession(ctx context.Context, session *types.Session) error {
	// First, check if we need to preserve document IDs from the database
	existingSession, err := c.Options.Store.GetSession(ctx, session.ID)
	if err == nil && existingSession != nil {
		// Log the document IDs and RAG results from the existing session and the new session
		existingRagCount := 0
		newRagCount := 0

		// Update model and provider from the new session
		existingSession.ModelName = session.ModelName
		existingSession.Provider = session.Provider

		if existingSession.Metadata.SessionRAGResults != nil {
			existingRagCount = len(existingSession.Metadata.SessionRAGResults)
		}

		if session.Metadata.SessionRAGResults != nil {
			newRagCount = len(session.Metadata.SessionRAGResults)
		}

		// If the existing session has document IDs and the new session doesn't, preserve them
		if len(existingSession.Metadata.DocumentIDs) > 0 {
			if session.Metadata.DocumentIDs == nil {
				session.Metadata.DocumentIDs = make(map[string]string)
			}

			// Merge document IDs - preserve existing ones that aren't in the new session
			for k, v := range existingSession.Metadata.DocumentIDs {
				if _, exists := session.Metadata.DocumentIDs[k]; !exists {
					session.Metadata.DocumentIDs[k] = v
					log.Debug().
						Str("session_id", session.ID).
						Str("key", k).
						Str("value", v).
						Msg("WriteSession: preserving document ID from database")
				}
			}

			// If document group ID is empty in the new session but exists in the database, preserve it
			if session.Metadata.DocumentGroupID == "" && existingSession.Metadata.DocumentGroupID != "" {
				session.Metadata.DocumentGroupID = existingSession.Metadata.DocumentGroupID
				log.Debug().
					Str("session_id", session.ID).
					Str("document_group_id", existingSession.Metadata.DocumentGroupID).
					Msg("WriteSession: preserving document group ID from database")
			}
		}

		// Properly merge RAG results from existing and new session
		if existingRagCount > 0 || newRagCount > 0 {
			// Create a map to store unique RAG results to avoid duplicates
			existingRagMap := make(map[string]*types.SessionRAGResult)

			// Add existing RAG results to the map if they exist
			if existingSession.Metadata.SessionRAGResults != nil {
				for _, result := range existingSession.Metadata.SessionRAGResults {
					key := createUniqueRagResultKey(result)
					existingRagMap[key] = result
					log.Debug().
						Str("session_id", session.ID).
						Str("document_id", result.DocumentID).
						Str("key", key).
						Msg("WriteSession: preserving existing RAG result")
				}
			}

			// Add new RAG results to the map, replacing any with the same key
			if session.Metadata.SessionRAGResults != nil {
				for _, result := range session.Metadata.SessionRAGResults {
					key := createUniqueRagResultKey(result)
					existingRagMap[key] = result
					log.Debug().
						Str("session_id", session.ID).
						Str("document_id", result.DocumentID).
						Str("key", key).
						Msg("WriteSession: adding new RAG result")
				}
			}

			// Convert map back to array
			mergedResults := make([]*types.SessionRAGResult, 0, len(existingRagMap))
			for _, result := range existingRagMap {
				mergedResults = append(mergedResults, result)
			}

			// Update the session with merged results
			session.Metadata.SessionRAGResults = mergedResults
			log.Debug().
				Str("session_id", session.ID).
				Int("merged_rag_results_count", len(mergedResults)).
				Msg("WriteSession: merged RAG results from database and new session")
		}
	}

	_, err = c.Options.Store.UpdateSession(ctx, *session)
	if err != nil {
		log.Printf("Error adding message: %s", err)
		return err
	}

	event := &types.WebsocketEvent{
		Type:      types.WebsocketEventSessionUpdate,
		SessionID: session.ID,
		Owner:     session.Owner,
		Session:   session,
	}

	_ = c.publishEvent(context.Background(), event)

	return nil
}

func (c *Controller) UpdateSessionName(ctx context.Context, ownerID string, sessionID, name string) error {
	log.Trace().
		Msgf("🔵 update session name: %s %+v", sessionID, name)

	err := c.Options.Store.UpdateSessionName(ctx, sessionID, name)
	if err != nil {
		log.Printf("Error adding message: %s", err)
		return err
	}

	// Publish WebSocket event so clients see the title update
	session, err := c.Options.Store.GetSession(ctx, sessionID)
	if err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("failed to get session for WebSocket notification")
		return nil // Name already updated, just skip notification
	}

	event := &types.WebsocketEvent{
		Type:      types.WebsocketEventSessionUpdate,
		SessionID: sessionID,
		Owner:     session.Owner,
		Session:   session,
	}

	_ = c.publishEvent(ctx, event)

	return nil
}

func (c *Controller) BroadcastProgress(
	ctx context.Context,
	session *types.Session,
	progress int,
	status string,
) {
	event := &types.WebsocketEvent{
		Type:      types.WebsocketEventWorkerTaskResponse,
		SessionID: session.ID,
		Owner:     session.Owner,
		WorkerTaskResponse: &types.RunnerTaskResponse{
			Type:      types.WorkerTaskResponseTypeProgress,
			SessionID: session.ID,
			Owner:     session.Owner,
			Progress:  progress,
			Status:    status,
		},
	}

	_ = c.publishEvent(ctx, event)
}

func (c *Controller) publishEvent(ctx context.Context, event *types.WebsocketEvent) error {
	message, err := json.Marshal(event)
	if err != nil {
		log.Error().Msgf("Error marshalling session update: %s", err.Error())
		return err
	}

	// Check if PubSub is nil (for tests)
	if c.Options.PubSub == nil {
		log.Debug().Msg("PubSub not initialized, skipping event publishing")
		return nil
	}

	err = c.Options.PubSub.Publish(ctx, pubsub.GetSessionQueue(event.Owner, event.SessionID), message)
	if err != nil {
		log.Error().Msgf("Error publishing event: %s", err.Error())
	}

	return err
}

func (c *Controller) ErrorSession(ctx context.Context, session *types.Session, interaction *types.Interaction, sessionErr error) {
	interaction.State = types.InteractionStateError
	interaction.Completed = time.Now()
	interaction.Error = sessionErr.Error()

	_, err := c.Options.Store.UpdateInteraction(ctx, interaction)
	if err != nil {
		log.Error().Err(err).Msg("failed to update interaction")
	}

	if err := c.WriteSession(ctx, session); err != nil {
		log.Error().Err(err).Msg("failed to write error session")
	}
	if err := c.Options.Janitor.WriteSessionError(session, sessionErr); err != nil {
		log.Error().Err(err).Msg("failed to write janitor session error")
	}
}

func (c *Controller) HandleRunnerResponse(ctx context.Context, taskResponse *types.RunnerTaskResponse) (*types.RunnerTaskResponse, error) {
	session, err := c.Options.Store.GetSession(ctx, taskResponse.SessionID)
	if err != nil {
		return nil, err
	}

	if session == nil {
		return nil, fmt.Errorf("session not found: %s", taskResponse.SessionID)
	}

	interaction, err := c.Options.Store.GetInteraction(ctx, taskResponse.InteractionID)
	if err != nil {
		return nil, err
	}

	// session, err = data.UpdateAssistantInteraction(session, func(targetInteraction *types.Interaction) (*types.Interaction, error) {
	// mark the interaction as complete if we are a fully finished response
	if taskResponse.Type == types.WorkerTaskResponseTypeResult {
		interaction.Completed = time.Now()
		interaction.State = types.InteractionStateComplete
	}

	// update the message if we've been given one
	if taskResponse.Message != "" {
		if taskResponse.Type == types.WorkerTaskResponseTypeResult {
			interaction.ResponseMessage = taskResponse.Message
		} else if taskResponse.Type == types.WorkerTaskResponseTypeStream {
			interaction.ResponseMessage += taskResponse.Message
		}
	}

	interaction.ToolCallID = taskResponse.ToolCallID
	interaction.ToolCalls = taskResponse.ToolCalls

	interaction.Usage = taskResponse.Usage

	if taskResponse.Status != "" {
		interaction.Status = taskResponse.Status
	}

	if taskResponse.Error != "" {
		interaction.Error = taskResponse.Error
	}

	_, err = c.Options.Store.UpdateInteraction(ctx, interaction)
	if err != nil {
		return nil, err
	}

	if taskResponse.Error != "" {
		// NOTE: we don't return here as
		if err := c.Options.Janitor.WriteSessionError(session, errors.New(taskResponse.Error)); err != nil {
			return nil, fmt.Errorf("failed writing session error: %s : %v", taskResponse.Error, err)
		}
	}

	return taskResponse, nil
}

// return the contents of a filestore text file
// you must have already applied the users sub-path before calling this
func (c *Controller) FilestoreReadTextFile(filepath string) (string, error) {
	reader, err := c.Options.Filestore.OpenFile(c.Ctx, filepath)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	return string(data), nil
}
