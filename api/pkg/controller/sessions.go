// sessions are the higher level ChatGPT like UI concept

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/jinzhu/copier"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/prompts"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"
)

// set to false in production (will log messages to web UI)
const DEBUG = true

var runnerResponseTimeout = 180 * time.Second

func (c *Controller) StartSession(ctx context.Context, user *types.User, req types.InternalSessionRequest) (*types.Session, error) {
	assistantInteraction := &types.Interaction{
		ID:             system.GenerateUUID(),
		Created:        time.Now(),
		Updated:        time.Now(),
		Creator:        types.CreatorTypeAssistant,
		Mode:           req.Mode,
		Message:        "",
		Files:          []string{},
		State:          types.InteractionStateWaiting,
		Finished:       false,
		Metadata:       map[string]string{},
		DataPrepChunks: map[string][]types.DataPrepChunk{},
		ResponseFormat: req.ResponseFormat,
		Tools:          req.Tools,
	}

	activeTools := req.ActiveTools
	if activeTools == nil {
		activeTools = []string{}
	}

	newSession := types.Session{
		ID:            req.ID,
		Name:          system.GenerateAmusingName(),
		ModelName:     req.ModelName,
		Type:          req.Type,
		Mode:          req.Mode,
		ParentSession: req.ParentSession,
		ParentApp:     req.ParentApp,
		LoraDir:       req.LoraDir,
		Owner:         req.Owner,
		OwnerType:     req.OwnerType,
		Created:       time.Now(),
		Updated:       time.Now(),
		Interactions:  append(req.UserInteractions, assistantInteraction),
		Metadata: types.SessionMetadata{
			Stream:       req.Stream,
			OriginalMode: req.Mode,
			SystemPrompt: req.SystemPrompt,
			Origin: types.SessionOrigin{
				Type: types.SessionOriginTypeUserCreated,
			},
			Priority:                req.Priority,
			ManuallyReviewQuestions: req.ManuallyReviewQuestions,
			HelixVersion:            data.GetHelixVersion(),
			RagEnabled:              req.RAGEnabled,
			TextFinetuneEnabled:     req.TextFinetuneEnabled,
			RagSettings:             req.RAGSettings,
			ActiveTools:             activeTools,
			UploadedDataID:          req.UploadedDataID,
			RAGSourceID:             req.RAGSourceID,
			LoraID:                  req.LoraID,
			AssistantID:             req.AssistantID,
			AppQueryParams:          req.AppQueryParams,
		},
	}

	// if we have a rag source ID then it also means rag is enabled (for inference)
	if newSession.Metadata.RAGSourceID != "" {
		newSession.Metadata.RagEnabled = true
	}

	if c.Options.Config.SubscriptionQuotas.Enabled && newSession.Mode == types.SessionModeFinetune {
		// Check for max concurrent finetuning sessions
		var currentlyRunningFinetuneSessions int

		sessions, err := c.Options.Store.GetSessions(ctx, store.GetSessionsQuery{
			Owner: newSession.Owner,
		})
		if err != nil {
			log.
				Err(err).
				Str("session_id", req.ID).
				Msg("failed to get sessions")
			return nil, fmt.Errorf("failed to get sessions: %w", err)
		}

		for _, session := range sessions {
			if session.Mode == types.SessionModeFinetune {
				// Check if the last interaction is still running
				lastInteraction, err := data.GetLastAssistantInteraction(session.Interactions)
				if err != nil {
					log.Error().Err(err).Msgf("failed to get last assistant interaction for session: %s", session.ID)
					continue
				}

				if lastInteraction.State == types.InteractionStateWaiting {
					currentlyRunningFinetuneSessions++
				}
			}
		}

		pro, err := c.isUserProTier(context.Background(), req.Owner)
		if err != nil {
			return nil, fmt.Errorf("error getting user '%s' meta: %s", req.Owner, err.Error())
		}

		if pro {
			// Pro plan
			if currentlyRunningFinetuneSessions >= c.Options.Config.SubscriptionQuotas.Finetuning.Pro.MaxConcurrent {
				return nil, fmt.Errorf(
					"you have reached the maximum number of concurrent finetuning sessions (%d/%d) allowed for your subscription plan",
					currentlyRunningFinetuneSessions,
					c.Options.Config.SubscriptionQuotas.Finetuning.Pro.MaxChunks,
				)
			}
		} else {
			// Free plan
			if currentlyRunningFinetuneSessions >= c.Options.Config.SubscriptionQuotas.Finetuning.Free.MaxConcurrent {
				return nil, fmt.Errorf(
					"you have reached the maximum number of concurrent finetuning sessions (%d/%d) allowed for your subscription plan, upgrade to increase your limits",
					currentlyRunningFinetuneSessions,
					c.Options.Config.SubscriptionQuotas.Finetuning.Pro.MaxChunks,
				)
			}
		}
	}

	// create session in database
	sessionData, err := c.Options.Store.CreateSession(ctx, newSession)
	if err != nil {
		return nil, err
	}

	go c.SessionRunner(sessionData)

	err = c.Options.Janitor.WriteSessionEvent(types.SessionEventTypeCreated, user, sessionData)
	if err != nil {
		return nil, err
	}

	if newSession.Mode == types.SessionModeFinetune && !newSession.Metadata.RagEnabled {
		err := c.Options.Notifier.Notify(ctx, &notification.Notification{
			Event:   notification.EventFinetuningStarted,
			Session: &newSession,
		})
		if err != nil {
			log.Error().Msgf("error notifying finetuning started: %s", err.Error())
		}
	}

	return sessionData, nil
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

func (c *Controller) UpdateSession(ctx context.Context, user *types.User, req types.UpdateSessionRequest) (*types.Session, error) {
	session, err := c.Options.Store.GetSession(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session %s: %w", req.SessionID, err)
	}

	assistantInteraction := &types.Interaction{
		ID:       system.GenerateUUID(),
		Created:  time.Now(),
		Updated:  time.Now(),
		Creator:  types.CreatorTypeAssistant,
		Mode:     req.SessionMode,
		Message:  "",
		Files:    []string{},
		State:    types.InteractionStateWaiting,
		Finished: false,
		Metadata: map[string]string{},
	}

	session.Updated = time.Now()
	session.Interactions = append(session.Interactions, req.UserInteraction, assistantInteraction)

	log.Debug().Msgf("🟢 update session: %+v", session)

	sessionData, err := c.Options.Store.UpdateSession(ctx, *session)
	if err != nil {
		return nil, err
	}

	go c.SessionRunner(sessionData)

	err = c.Options.Janitor.WriteSessionEvent(types.SessionEventTypeUpdated, user, sessionData)
	if err != nil {
		return nil, err
	}

	return sessionData, nil
}

func (c *Controller) RestartSession(ctx context.Context, session *types.Session) (*types.Session, error) {
	session, err := data.UpdateAssistantInteraction(session, func(assistantInteraction *types.Interaction) (*types.Interaction, error) {
		assistantInteraction.Error = ""
		assistantInteraction.Finished = false
		// empty out the previous message so model doesn't think it's already finished
		assistantInteraction.Message = ""

		assistantInteraction.State = types.InteractionStateWaiting

		// if this is a text inference then don't set the progress to 1 because
		// we don't show progress for text inference
		if session.Mode == types.SessionModeFinetune || session.Type == types.SessionTypeImage {
			assistantInteraction.Progress = 1
		} else {
			assistantInteraction.Progress = 0
		}

		if session.Mode == types.SessionModeFinetune {
			if assistantInteraction.DataPrepStage == types.TextDataPrepStageExtractText ||
				assistantInteraction.DataPrepStage == types.TextDataPrepStageGenerateQuestions {
				// in this case we are restarting the data prep
				assistantInteraction.Status = ""
			} else if assistantInteraction.DataPrepStage == types.TextDataPrepStageFineTune {
				// in this case we are restarting the fine tuning itself
				assistantInteraction.Status = "restarted: fine tuning on data..."
			}
		}

		return assistantInteraction, nil
	})

	if err != nil {
		return nil, err
	}

	if err := c.WriteSession(ctx, session); err != nil {
		// NOTE: we dont return here as this "only" emits WS events
		log.Error().Err(err).Msg("failed writing session")
	}

	// this will re-run the data prep preparation
	// but that is idempotent so we should be able to
	// not care and just say "start again"
	// if there is more data prep to do, it will carry on
	// if we go staight into the queue then it's a fine tune restart
	c.SessionRunner(session)

	return session, nil
}

func (c *Controller) AddDocumentsToSession(ctx context.Context, session *types.Session, userInteraction *types.Interaction) (*types.Session, error) {
	// the assistant interaction is the task we will run on a GPU and update in place
	assistantInteraction := &types.Interaction{
		ID:             system.GenerateUUID(),
		Created:        time.Now(),
		Updated:        time.Now(),
		Creator:        types.CreatorTypeAssistant,
		Mode:           userInteraction.Mode,
		Message:        "",
		Files:          []string{},
		State:          types.InteractionStateWaiting,
		Finished:       false,
		Metadata:       map[string]string{},
		DataPrepChunks: map[string][]types.DataPrepChunk{},
	}

	// we switch back to finetune mode - the session has been in inference mode
	// so the user can ask questions
	session.Mode = types.SessionModeFinetune
	session.Updated = time.Now()
	session.Interactions = append(session.Interactions, userInteraction, assistantInteraction)

	if err := c.WriteSession(ctx, session); err != nil {
		// NOTE: we dont return here as this "only" emits WS events
		log.Err(err).Msg("failed writing session")
	}

	go c.SessionRunner(session)

	return session, nil
}

func (c *Controller) UpdateSessionMetadata(ctx context.Context, session *types.Session, meta *types.SessionMetadata) (*types.SessionMetadata, error) {
	session.Updated = time.Now()
	session.Metadata = *meta

	sessionData, err := c.Options.Store.UpdateSession(ctx, *session)
	if err != nil {
		return nil, err
	}

	log.Debug().
		Str("session_id", sessionData.ID).
		Msg("🟢 update session config")

	return &sessionData.Metadata, nil
}

func (c *Controller) AddDocumentsToInteraction(ctx context.Context, session *types.Session, newFiles []string) (*types.Session, error) {
	session, err := data.UpdateUserInteraction(session, func(userInteraction *types.Interaction) (*types.Interaction, error) {
		userInteraction.Files = append(userInteraction.Files, newFiles...)
		return userInteraction, nil
	})
	if err != nil {
		return nil, err
	}

	session, err = data.UpdateAssistantInteraction(session, func(assistantInteraction *types.Interaction) (*types.Interaction, error) {
		assistantInteraction.State = types.InteractionStateWaiting
		return assistantInteraction, nil
	})
	if err != nil {
		return nil, err
	}

	session.Mode = types.SessionModeFinetune

	if err := c.WriteSession(ctx, session); err != nil {
		// NOTE: we dont return here as this "only" emits WS events
		log.Err(err).Msg("failed writing session")
	}

	go c.SessionRunner(session)

	return session, nil
}

// the idempotent function to "run" the session
// it should work out what this means - i.e. have we prepared the data yet?
func (c *Controller) SessionRunner(sessionData *types.Session) {
	ctx := context.Background()
	// Wait for that to complete before adding to the queue
	// the model can be adding subsequent child sessions to the queue
	// e.g. in the case of text fine tuning data prep - we need an LLM to convert
	// text into q&a pairs and we want to use our own mistral inference
	preparedSession, err := c.PrepareSession(ctx, sessionData)
	if err != nil {
		log.Error().Msgf("error preparing session: %s", err.Error())
		c.ErrorSession(ctx, sessionData, err)
		return
	}

	// If last interaction is "action" then we should run the action
	// and not the model
	lastInteraction, err := data.GetLastAssistantInteraction(sessionData.Interactions)
	if err == nil && lastInteraction.Mode == types.SessionModeAction {
		_, err := c.runActionInteraction(context.Background(), sessionData, lastInteraction)
		if err != nil {
			log.Error().Msgf("error running action interaction: %s", err.Error())
			c.ErrorSession(ctx, sessionData, err)
		}
		return
	}

	// it's ok if we did not get a session back here
	// it means there will be a later action that will add the session to the queue
	// in the case the user needs to edit some data before it can be run for example
	if preparedSession != nil {
		err = c.AddSessionToQueue(preparedSession)
		if err != nil {
			log.Error().Msgf("error adding session to queue: %s", err.Error())
			c.ErrorSession(ctx, sessionData, err)
		}
	}
}

// this is called in a go routine from the main api handler
// this is blocking the session being added to the queue
// so we get the chance to do some async pre-processing
// before the session joins the queue
// in some cases - we need the user to interact with our pre-processing
// in this case - let's return nil here and let the user interaction add the session to the queue
// once they have completed their editing
// e.g. for text fine-tuning we need to prepare the input files
//   - convert pdf, docx, etc to txt
//   - chunk the text based on buffer and overflow config
//   - feed each chunk into an LLM implementation to extract q&a pairs
//   - append the q&a pairs to a jsonl file
//
// so - that is all auto handled by the system
// the user then needs to view and edit the resuting JSONL file in the browser
// so now we are in a state where the session is still preparing but we are waiting
// for the user - so, we return nil here with no error which
// TODO: this should be adding jobs to a queue
func (c *Controller) PrepareSession(ctx context.Context, session *types.Session) (*types.Session, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var convertedTextDocuments int
	var err error

	if session.Type == types.SessionTypeText && session.Mode == types.SessionModeInference {
		// Check if this is actionable
		session, err = c.checkForActions(session)
		if err != nil {
			return nil, err
		}
	}

	// load the model
	// call it's
	// here we need to turn all of the uploaded files into text files
	// so we ping our handy python server that will do that for us
	if session.Type == types.SessionTypeText && session.Mode == types.SessionModeFinetune {

		// if either rag or finetuning is enabled then we need to convert the files to text
		if session.Metadata.TextFinetuneEnabled || session.Metadata.RagEnabled {
			session, convertedTextDocuments, err = c.convertDocumentsToText(ctx, session)
			if err != nil {
				return nil, err
			}
		}

		// we put this behind a feature flag because then we can have fine-tune only sessions
		if session.Metadata.RagEnabled {
			session, _, err = c.indexChunksForRag(ctx, session)
			if err != nil {
				return nil, err
			}

			// if we are NOT doing fine tuning then we need to mark this as finshed
			if !session.Metadata.TextFinetuneEnabled {
				session, err := data.UpdateAssistantInteraction(session, func(assistantInteraction *types.Interaction) (*types.Interaction, error) {
					assistantInteraction.Finished = true
					assistantInteraction.Progress = 0
					assistantInteraction.Message = ""
					assistantInteraction.Status = "We have indexed all of your documents now you can ask questions..."
					assistantInteraction.State = types.InteractionStateComplete
					assistantInteraction.DataPrepStage = types.TextDataPrepStageComplete
					assistantInteraction.Files = []string{}
					return assistantInteraction, nil
				})

				// we need to switch to inference mode now so the user can ask questions
				session.Mode = types.SessionModeInference

				if err != nil {
					return nil, err
				}

				if err := c.WriteSession(ctx, session); err != nil {
					log.Error().Err(err).Msg("failed writing session")
				}
				c.BroadcastProgress(ctx, session, 0, "")
			}
		}

		// we put this behind a feature flag because then we can have RAG only sessions
		if session.Metadata.TextFinetuneEnabled {
			session, questionChunksGenerated, err := c.convertChunksToQuestions(ctx, session)
			if err != nil {
				return nil, err
			}

			// if we have checked the ManuallyReviewQuestions setting
			// then we DON'T want the session in the queue yet
			// the user has to confirm the questions are correct
			// or there might have been errors that we want to give the user
			// a chance to decide what to do
			if session.Metadata.ManuallyReviewQuestions {
				if convertedTextDocuments > 0 || questionChunksGenerated > 0 {
					return nil, nil
				}
			}

			// if there are any errors in the data prep then we should not auto-progress
			// and give the user the choice
			qaPairErrorCount, err := c.convertChunksToQuestionsErrorCount(session)
			if err != nil {
				return nil, err
			}

			if qaPairErrorCount > 0 {
				return nil, nil
			}

			// otherwise lets kick off the fine tune
			if err := c.BeginFineTune(ctx, session); err != nil {
				return nil, err
			}
		}

		return nil, nil
	} else if session.Type == types.SessionTypeText && session.Mode == types.SessionModeInference {
		// we need to check if we are doing RAG and if yes, we need to augment the prompt
		// with the results from the RAGStore
		if session.Metadata.RagEnabled {
			ragResults, err := c.getRAGResults(session)
			if err != nil {
				return nil, err
			}

			var ragContent []*prompts.RagContent
			for _, result := range ragResults {
				ragContent = append(ragContent, &prompts.RagContent{
					DocumentID: result.DocumentID,
					Content:    result.Content,
				})
			}

			session, err = data.UpdateUserInteraction(session, func(userInteraction *types.Interaction) (*types.Interaction, error) {
				userInteraction.DisplayMessage = userInteraction.Message
				injectedUserPrompt, err := prompts.RAGInferencePrompt(userInteraction.Message, ragContent)
				if err != nil {
					return nil, err
				}
				userInteraction.Message = injectedUserPrompt
				return userInteraction, nil
			})
			if err != nil {
				return nil, err
			}
			session, err = data.UpdateAssistantInteraction(session, func(assistantInteraction *types.Interaction) (*types.Interaction, error) {
				assistantInteraction.RagResults = ragResults
				return assistantInteraction, nil
			})
			if err != nil {
				return nil, err
			}

			// Update session metadata with document IDs from RAG results
			if err := c.UpdateSessionWithKnowledgeResults(ctx, session, ragResults); err != nil {
				log.Error().Err(err).Str("session_id", session.ID).Msg("failed to update session with knowledge results")
				// We don't return an error here to allow the session to continue
			}
		}
	}

	return session, nil
}

func (c *Controller) checkForActions(session *types.Session) (*types.Session, error) {
	if !c.Options.Config.Tools.Enabled {
		// Tools not enabled for the server
		return session, nil
	}

	ctx := context.Background()

	var (
		activeTools []*types.Tool
		assistant   *types.AssistantConfig
	)
	// if this session is spawned from an app then we populate the list of tools from the app rather than the linked
	// database record
	if session.ParentApp != "" {
		app, err := c.Options.Store.GetAppWithTools(ctx, session.ParentApp)
		if err != nil {
			return nil, fmt.Errorf("error getting app: %w", err)
		}

		// if the tool exists but the user cannot access it - then something funky is being attempted and we should deny it
		if !app.Global && app.Owner != session.Owner {
			return nil, system.NewHTTPError403(fmt.Sprintf("you do not have access to the app with the id: %s", app.ID))
		}

		if len(app.Config.Helix.Assistants) > 0 {
			assistantID := session.Metadata.AssistantID
			if assistantID == "" {
				assistantID = "0"
			}
			assistant = data.GetAssistant(app, assistantID)
			if assistant == nil {
				return nil, system.NewHTTPError404(fmt.Sprintf("we could not find the assistant with the id: %s", assistantID))
			}
			activeTools = append(activeTools, assistant.Tools...)
		}
	}

	if len(activeTools) == 0 {
		// No tools available, nothing to check
		return session, nil
	}

	history := data.GetLastInteractions(session, actionContextHistorySize)

	for i, interaction := range history {
		log.Info().
			Int("index", i).
			Str("creator", string(interaction.Creator)).
			Str("message", interaction.Message).
			Msg("History item")
	}

	messageHistory := types.HistoryFromInteractions(history)

	// Actionable, converting interaction mode to "action"
	lastInteraction, err := data.GetLastAssistantInteraction(session.Interactions)
	if err != nil {
		return nil, fmt.Errorf("failed to get last assistant interaction: %w", err)
	}

	var options []tools.Option

	// If assistant has configured an actionable template, use it
	if assistant != nil && assistant.IsActionableTemplate != "" {
		options = append(options, tools.WithIsActionableTemplate(assistant.IsActionableTemplate))
	}
	// If assistant has configured a model, use it
	if assistant != nil && assistant.Model != "" {
		options = append(options, tools.WithModel(assistant.Model))
	}

	isActionable, err := c.ToolsPlanner.IsActionable(ctx, session.ID, lastInteraction.ID, activeTools, messageHistory, options...)
	if err != nil {
		log.Error().Err(err).Msg("failed to evaluate if the message is actionable, skipping to general knowledge")
		return session, nil
	}

	log.Info().
		Str("api", isActionable.API).
		Str("actionable", isActionable.NeedsTool).
		Str("justification", isActionable.Justification).
		Str("history", fmt.Sprintf("%+v", messageHistory)).
		Msg("checked for actionable")

	if !isActionable.Actionable() {
		return session, nil
	}

	lastInteraction.Mode = types.SessionModeAction

	lastInteraction.Mode = types.SessionModeAction
	lastInteraction.Metadata["tool_action"] = isActionable.API
	lastInteraction.Metadata["tool_action_justification"] = isActionable.Justification

	actionTool, ok := tools.GetToolFromAction(activeTools, isActionable.API)
	if !ok {
		return nil, fmt.Errorf("tool not found for action: %s", isActionable.API)
	}

	lastInteraction.Metadata["tool_id"] = actionTool.ID
	lastInteraction.Metadata["tool_app_id"] = session.ParentApp

	return session, nil
}

func (c *Controller) BeginFineTune(ctx context.Context, session *types.Session) error {
	session, err := data.UpdateAssistantInteraction(session, func(assistantInteraction *types.Interaction) (*types.Interaction, error) {
		assistantInteraction.Finished = false
		assistantInteraction.Progress = 1
		assistantInteraction.Message = ""
		assistantInteraction.Status = "fine tuning on data..."
		assistantInteraction.State = types.InteractionStateWaiting
		assistantInteraction.DataPrepStage = types.TextDataPrepStageFineTune
		assistantInteraction.Files = []string{}
		return assistantInteraction, nil
	})

	if err != nil {
		return err
	}

	err = c.WriteSession(ctx, session)
	if err != nil {
		return err
	}

	err = c.AddSessionToQueue(session)
	if err != nil {
		return err
	}
	c.BroadcastProgress(ctx, session, 1, "fine tuning on data...")

	return nil
}

// generic "update this session handler"
// this will emit a UserWebsocketEvent with a type of
// WebsocketEventSessionUpdate
func (c *Controller) WriteSession(ctx context.Context, session *types.Session) error {
	// First, check if we need to preserve document IDs from the database
	existingSession, err := c.Options.Store.GetSession(ctx, session.ID)
	if err == nil && existingSession != nil {
		// Log the document IDs from the existing session and the new session
		log.Debug().
			Str("session_id", session.ID).
			Interface("existing_document_ids", existingSession.Metadata.DocumentIDs).
			Interface("new_document_ids", session.Metadata.DocumentIDs).
			Msg("WriteSession: comparing document IDs between existing and new session")

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

func (c *Controller) UpdateSessionName(ctx context.Context, owner string, sessionID, name string) error {
	log.Trace().
		Msgf("🔵 update session name: %s %+v", sessionID, name)

	err := c.Options.Store.UpdateSessionName(context.Background(), sessionID, name)
	if err != nil {
		log.Printf("Error adding message: %s", err)
		return err
	}
	session, err := c.Options.Store.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	event := &types.WebsocketEvent{
		Type:      types.WebsocketEventSessionUpdate,
		SessionID: sessionID,
		Owner:     owner,
		Session:   session,
	}

	_ = c.publishEvent(context.Background(), event)

	return nil
}

func (c *Controller) WriteInteraction(ctx context.Context, session *types.Session, newInteraction *types.Interaction) *types.Session {
	newInteractions := []*types.Interaction{}
	for _, interaction := range session.Interactions {
		if interaction.ID == newInteraction.ID {
			newInteractions = append(newInteractions, newInteraction)
		} else {
			newInteractions = append(newInteractions, interaction)
		}
	}
	session.Interactions = newInteractions
	if err := c.WriteSession(ctx, session); err != nil {
		log.Printf("failed to write interaction session: %v", err)
	}
	return session
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

	err = c.Options.PubSub.Publish(ctx, pubsub.GetSessionQueue(event.Owner, event.SessionID), message)
	if err != nil {
		log.Error().Msgf("Error publishing event: %s", err.Error())
	}

	return err
}

func (c *Controller) ErrorSession(ctx context.Context, session *types.Session, sessionErr error) {
	session, err := data.UpdateUserInteraction(session, func(userInteraction *types.Interaction) (*types.Interaction, error) {
		userInteraction.Finished = true
		userInteraction.State = types.InteractionStateComplete
		return userInteraction, nil
	})
	if err != nil {
		return
	}
	session, err = data.UpdateAssistantInteraction(session,
		func(assistantInteraction *types.Interaction) (*types.Interaction, error) {
			assistantInteraction.State = types.InteractionStateError
			assistantInteraction.Completed = time.Now()
			assistantInteraction.Error = sessionErr.Error()
			assistantInteraction.Finished = true
			return assistantInteraction, nil
		})
	if err != nil {
		return
	}
	if err := c.WriteSession(ctx, session); err != nil {
		log.Error().Err(err).Msg("failed to write error session")
	}
	if err := c.Options.Janitor.WriteSessionError(session, sessionErr); err != nil {
		log.Error().Err(err).Msg("failed to write janitor session error")
	}
}

// add the given session onto the end of the queue
// unless it's already waiting and present in the queue
// in which case let's replace it at it's current position
// we mark the session as "preparing" here to give text fine tuning
// a chance to sort itself out in the background
func (c *Controller) AddSessionToQueue(session *types.Session) error {
	work, err := scheduler.NewSessionWorkload(session)
	if err != nil {
		return fmt.Errorf("error creating workload: %w", err)
	}
	lastInteraction, err := data.GetLastInteraction(session)
	if err != nil {
		return fmt.Errorf("error getting last interaction: %w", err)
	}
	// Create a pubsub subscription to listen for responses to this session
	subCtx, cancel := context.WithTimeout(c.Ctx, runnerResponseTimeout)
	log.Debug().Str("owner", session.Owner).Str("request_id", lastInteraction.ID).Msg("subscribing to runner responses queue")
	sub, err := c.Options.PubSub.Subscribe(subCtx, pubsub.GetRunnerResponsesQueue(session.Owner, lastInteraction.ID), func(payload []byte) error {
		err := c.pubsubHandler(session, payload)
		if err != nil {
			log.Error().Err(err).Msg("error handling runner response")
			cancel()
		}
		return err
	})
	if err != nil {
		return fmt.Errorf("error subscribing to runner responses queue: %w", err)
	}

	go func() {
		<-subCtx.Done()
		log.Debug().Str("owner", session.Owner).Str("request_id", lastInteraction.ID).Msg("unsubscribing from runner responses queue")
		_ = sub.Unsubscribe()
	}()

	err = c.scheduler.Enqueue(work)
	if err != nil {
		return fmt.Errorf("error enqueuing work: %w", err)
	}

	return nil
}

func (c *Controller) pubsubHandler(session *types.Session, payload []byte) error {
	lastInteraction, err := data.GetLastInteraction(session)
	if err != nil {
		return fmt.Errorf("error getting last interaction: %w", err)
	}
	log.Debug().Str("owner", session.Owner).Str("request_id", lastInteraction.ID).Msg("received runner response")

	var runnerResp types.RunnerNatsReplyResponse
	err = json.Unmarshal(payload, &runnerResp)
	if err != nil {
		return fmt.Errorf("error unmarshalling runner response: %w", err)
	}

	log.Trace().Interface("runnerResp", runnerResp).Msg("runner response")

	if runnerResp.Error != "" {
		return fmt.Errorf("runner error: %s", runnerResp.Error)
	}

	if session.Mode == types.SessionModeInference && session.Type == types.SessionTypeImage {
		// Remove the SSE "data: " prefix from the response
		response := strings.TrimPrefix(string(runnerResp.Response), "data: ")

		// Parse the openai response
		var imageGenerationResponse types.HelixImageGenerationUpdate
		err = json.Unmarshal([]byte(response), &imageGenerationResponse)
		if err != nil {
			return fmt.Errorf("error unmarshalling openai response: %w", err)
		}

		log.Trace().Interface("imageGenerationResponse", imageGenerationResponse).Msg("image generation response")

		files := []string{}
		for _, image := range imageGenerationResponse.Data {
			files = append(files, image.URL)
		}

		if imageGenerationResponse.Completed {
			c.BroadcastProgress(c.Ctx, session, 100, "done")
			_, err = c.HandleRunnerResponse(c.Ctx, &types.RunnerTaskResponse{
				Type:          types.WorkerTaskResponseTypeResult,
				SessionID:     session.ID,
				InteractionID: lastInteraction.ID,
				Owner:         session.Owner,
				Progress:      imageGenerationResponse.Step,
				Status:        "done",
				Done:          true,
				Files:         files,
			})
			if err != nil {
				return fmt.Errorf("error handling runner response: %w", err)
			}
		} else {
			c.BroadcastProgress(c.Ctx, session, imageGenerationResponse.Step, "generating...")
		}
	} else if session.Mode == types.SessionModeFinetune && session.Type == types.SessionTypeText {
		// Remove the SSE "data: " prefix from the response if it exists
		response := strings.TrimPrefix(string(runnerResp.Response), "data: ")

		// Parse the openai response
		var fineTuningResponse types.HelixFineTuningUpdate
		err = json.Unmarshal([]byte(response), &fineTuningResponse)
		if err != nil {
			return fmt.Errorf("error unmarshalling openai response: %w", err)
		}

		log.Trace().Interface("fineTuningResponse", fineTuningResponse).Msg("fine tuning response")

		if fineTuningResponse.Completed {
			c.BroadcastProgress(c.Ctx, session, 100, "done")
			_, err = c.HandleRunnerResponse(c.Ctx, &types.RunnerTaskResponse{
				Type:          types.WorkerTaskResponseTypeResult,
				SessionID:     session.ID,
				InteractionID: lastInteraction.ID,
				Owner:         session.Owner,
				Progress:      100,
				Status:        "done",
				Done:          true,
				LoraDir:       fineTuningResponse.LoraDir,
			})
			if err != nil {
				return fmt.Errorf("error handling runner response: %w", err)
			}
		} else {
			c.BroadcastProgress(c.Ctx, session, fineTuningResponse.Progress, "fine-tuning...")
		}
	} else if session.Type == types.SessionTypeText && lastInteraction.Mode == types.SessionModeInference {
		// This is an inference session on a fine-tuned model.
		// Republish the result to the old websocket handlers
		// Remove the SSE "data: " prefix from the response if there is any
		response := strings.TrimPrefix(string(runnerResp.Response), "data: ")

		// Parse the openai response
		var completion openai.ChatCompletionResponse
		err = json.Unmarshal([]byte(response), &completion)
		if err != nil {
			return fmt.Errorf("error unmarshalling openai response: %w", err)
		}
		log.Trace().Interface("message", completion.Choices[0].Message.Content).Msg("completion")
		_, err = c.HandleRunnerResponse(c.Ctx, &types.RunnerTaskResponse{
			Type:          types.WorkerTaskResponseTypeResult,
			SessionID:     session.ID,
			InteractionID: lastInteraction.ID,
			Owner:         session.Owner,
			Message:       completion.Choices[0].Message.Content,
			Done:          completion.Choices[0].FinishReason == openai.FinishReasonStop,
		})
		if err != nil {
			return fmt.Errorf("error handling runner response: %w", err)
		}
	} else {
		return fmt.Errorf("unsupported session mode or type: %s %s", session.Mode, session.Type)
	}

	return nil
}

func (c *Controller) HandleRunnerResponse(ctx context.Context, taskResponse *types.RunnerTaskResponse) (*types.RunnerTaskResponse, error) {
	session, err := c.Options.Store.GetSession(ctx, taskResponse.SessionID)
	if err != nil {
		return nil, err
	}

	if session == nil {
		return nil, fmt.Errorf("session not found: %s", taskResponse.SessionID)
	}

	session, err = data.UpdateAssistantInteraction(session, func(targetInteraction *types.Interaction) (*types.Interaction, error) {
		// mark the interaction as complete if we are a fully finished response
		if taskResponse.Type == types.WorkerTaskResponseTypeResult {
			targetInteraction.Finished = true
			targetInteraction.Completed = time.Now()
			targetInteraction.State = types.InteractionStateComplete
		}

		// update the message if we've been given one
		if taskResponse.Message != "" {
			if taskResponse.Type == types.WorkerTaskResponseTypeResult {
				targetInteraction.Message = taskResponse.Message
			} else if taskResponse.Type == types.WorkerTaskResponseTypeStream {
				targetInteraction.Message += taskResponse.Message
			}
		}

		targetInteraction.ToolCallID = taskResponse.ToolCallID
		targetInteraction.ToolCalls = taskResponse.ToolCalls

		targetInteraction.Usage = taskResponse.Usage

		if taskResponse.Progress != 0 {
			targetInteraction.Progress = taskResponse.Progress
		}

		if taskResponse.Status != "" {
			targetInteraction.Status = taskResponse.Status
		}

		// update the files if there are some
		if taskResponse.Files != nil {
			targetInteraction.Files = taskResponse.Files
		}

		if taskResponse.Error != "" {
			targetInteraction.Error = taskResponse.Error
		}

		if taskResponse.Type == types.WorkerTaskResponseTypeResult && session.Mode == types.SessionModeFinetune && taskResponse.LoraDir != "" {
			// we got some files back from a finetune
			// so let's hoist the session into inference mode but with the finetune file attached
			session.Mode = types.SessionModeInference
			session.LoraDir = taskResponse.LoraDir
			targetInteraction.LoraDir = taskResponse.LoraDir
			targetInteraction.DataPrepStage = types.TextDataPrepStageComplete
			targetInteraction.Progress = 0
			targetInteraction.Status = ""

			// only notify the user that the fine tune was completed if there was not an error
			if taskResponse.Error == "" {

				// create a new data entity that is the RAG source
				loraDataEntity, err := c.Options.Store.CreateDataEntity(context.Background(), &types.DataEntity{
					ID:        system.GenerateUUID(),
					Created:   time.Now(),
					Updated:   time.Now(),
					Type:      types.DataEntityTypeLora,
					Owner:     session.Owner,
					OwnerType: session.OwnerType,
					Config: types.DataEntityConfig{
						FilestorePath: taskResponse.LoraDir,
					},
				})
				if err != nil {
					return nil, err
				}
				session.Metadata.LoraID = loraDataEntity.ID

				err = c.Options.Notifier.Notify(ctx, &notification.Notification{
					Event:   notification.EventFinetuningComplete,
					Session: session,
				})
				if err != nil {
					log.Ctx(ctx).Error().Msgf("error notifying finetuning completed: %s", err.Error())
				}
			}
		}

		return targetInteraction, nil
	})

	if err != nil {
		return nil, err
	}

	if err := c.WriteSession(ctx, session); err != nil {
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

type CloneUntilInteractionRequest struct {
	InteractionID string
	Mode          types.CloneInteractionMode
	CopyAllFiles  bool
}

// the user interaction is the thing we are cloning
func (c *Controller) CloneUntilInteraction(
	ctx context.Context,
	user *types.User,
	oldSession *types.Session,
	req CloneUntilInteractionRequest,
) (*types.Session, error) {
	// * get a top level session object
	//   * against the correct account
	//   * only include interactions up until the given interaction
	// * loop over each interaction files
	//   * if CopyAllFiles then copy all interaction files into our file store
	//   * otherwise only copy the
	newSession, err := data.CloneSession(*oldSession, req.InteractionID, data.OwnerContextFromRequestContext(user))
	if err != nil {
		return nil, err
	}

	// for anything other than 'all' mode - we should revert the type to the original type
	// this is for when we are editing a finetune session that has since become an inference session
	// but now we are going back into finetine land by editing the interaction (somehow)
	// put another way - if we are cloning a session in all mode - we can copy the mode as is
	if req.Mode != types.CloneInteractionModeAll {
		newSession.Mode = oldSession.Metadata.OriginalMode
	}

	// these two interactions are the ones we will change based on the clone mode
	userInteraction, err := data.GetLastUserInteraction(newSession.Interactions)
	if err != nil {
		return nil, err
	}

	assistantInteraction, err := data.GetLastAssistantInteraction(newSession.Interactions)
	if err != nil {
		return nil, err
	}

	// the full filestore prefix for old user & old session
	// we can copy all files by just replacing the higher level prefixes
	// e.g. /users/123/sessions/456/inputs/789 -> /users/123/sessions/999/inputs/789
	oldPrefix, err := c.GetFilestoreSessionPath(data.OwnerContext(oldSession.Owner), oldSession.ID)
	if err != nil {
		return nil, err
	}

	newPrefix, err := c.GetFilestoreSessionPath(data.OwnerContext(user.ID), newSession.ID)
	if err != nil {
		return nil, err
	}

	newDocumentIDs := map[string]string{}

	for filename, documentID := range newSession.Metadata.DocumentIDs {
		newFile := strings.Replace(filename, oldPrefix, newPrefix, 1)
		newDocumentIDs[newFile] = documentID
	}

	newSession.Metadata.DocumentIDs = newDocumentIDs

	copyFile := func(filePath string) (string, error) {
		newFile := strings.Replace(filePath, oldPrefix, newPrefix, 1)
		log.Debug().
			Msgf("🔵 clone interaction file: %s -> %s", filePath, newFile)
		err := c.Options.Filestore.CopyFile(ctx, filePath, newFile)
		if err != nil {
			return "", err
		}
		return newFile, nil
	}

	copyFolder := func(folderPath string) (string, error) {
		newFolder := strings.Replace(folderPath, oldPrefix, newPrefix, 1)
		log.Debug().
			Msgf("🔵 clone folder: %s -> %s", folderPath, newFolder)
		reader, err := c.Options.Filestore.DownloadFolder(ctx, folderPath)
		if err != nil {
			return "", err
		}
		err = c.Options.Filestore.UploadFolder(ctx, newFolder, reader)
		if err != nil {
			return "", err
		}
		return newFolder, nil
	}

	// this will actually copy files so only call this if you want to remap
	copyInteractionFiles := func(interaction *types.Interaction) (*types.Interaction, error) {
		newFiles := []string{}
		var newInteraction types.Interaction

		err := copier.Copy(&newInteraction, interaction)
		if err != nil {
			return nil, fmt.Errorf("error copying interaction: %s", err.Error())
		}

		for _, file := range interaction.Files {
			if path.Base(file) == types.TextDataPrepQuestionsFile && req.Mode == types.CloneInteractionModeJustData && interaction.ID == userInteraction.ID {
				// this means we are only copying the data and we've just come across the questions file
				// in the last user interaction so we don't copy it
				continue
			}
			newFile, err := copyFile(file)
			if err != nil {
				return &newInteraction, err
			}
			newFiles = append(newFiles, newFile)
		}
		newInteraction.Files = newFiles
		return &newInteraction, nil
	}

	// this will actually copy files so only call this if you want to remap
	copyLoraDir := func(interaction *types.Interaction) (*types.Interaction, error) {
		var newInteraction types.Interaction

		err := copier.Copy(&newInteraction, interaction)
		if err != nil {
			return nil, fmt.Errorf("error copying interaction: %s", err.Error())
		}

		if interaction.LoraDir != "" {
			shouldCopyLora := false
			if interaction.ID == assistantInteraction.ID {
				// we are on the latest assistant interaction
				// let's check the mode to see if we should bring the lora with us
				if req.Mode == types.CloneInteractionModeAll {
					shouldCopyLora = true
				}
			} else {
				shouldCopyLora = true
			}

			if shouldCopyLora {
				newLoraDir, err := copyFolder(interaction.LoraDir)
				if err != nil {
					return interaction, err
				}
				newInteraction.LoraDir = newLoraDir
			} else {
				newInteraction.LoraDir = ""
			}
		}
		return &newInteraction, nil
	}

	// the result files are always copied if we are in a different user account
	// but not if we are in the same account
	// and the interaction file list will be pointing at a different session folder
	// but that is OK because the file store is immutable
	newInteractions := []*types.Interaction{}
	for _, interaction := range newSession.Interactions {
		if req.CopyAllFiles {
			newInteraction, err := copyInteractionFiles(interaction)
			if err != nil {
				return nil, err
			}
			newInteraction, err = copyLoraDir(newInteraction)
			if err != nil {
				return nil, err
			}
			newInteractions = append(newInteractions, newInteraction)
		} else if interaction.ID == userInteraction.ID || interaction.ID == assistantInteraction.ID {
			// these are the last 2 interactions of a session being cloned within the same account
			newInteraction, err := copyInteractionFiles(interaction)
			if err != nil {
				return nil, err
			}
			newInteraction, err = copyLoraDir(newInteraction)
			if err != nil {
				return nil, err
			}
			newInteractions = append(newInteractions, newInteraction)
		} else {
			newInteractions = append(newInteractions, interaction)
		}
	}

	newSession.Interactions = newInteractions

	// the folder is already copied over
	if req.Mode != types.CloneInteractionModeAll {
		newSession.LoraDir = ""
	} else {
		newSession.LoraDir = strings.Replace(oldSession.LoraDir, oldPrefix, newPrefix, 1)
	}

	// always copy the session results folder otherwise we have split brain on results
	newSession, err = data.UpdateUserInteraction(newSession, func(userInteraction *types.Interaction) (*types.Interaction, error) {
		// only touch if we are not cloning as is
		if req.Mode != types.CloneInteractionModeAll {
			userInteraction.Created = time.Now()
			userInteraction.Updated = time.Now()
			userInteraction.Message = ""
			userInteraction.Status = ""
			userInteraction.Progress = 0
		}

		return userInteraction, nil
	})
	if err != nil {
		return nil, err
	}

	newSession, err = data.UpdateAssistantInteraction(newSession, func(assistantInteraction *types.Interaction) (*types.Interaction, error) {
		// only touch if we are not cloning as is
		if req.Mode != types.CloneInteractionModeAll {
			assistantInteraction.Created = time.Now()
			assistantInteraction.Updated = time.Now()
			assistantInteraction.Message = ""
			assistantInteraction.Status = ""
			assistantInteraction.Progress = 0
		}

		if req.Mode == types.CloneInteractionModeJustData {
			// remove the fine tune file
			assistantInteraction.DataPrepStage = types.TextDataPrepStageEditFiles
			assistantInteraction.State = types.InteractionStateEditing
			assistantInteraction.Finished = false
			// remove the metadata that keeps track of processed questions
			// (because we have deleted the questions file)
			assistantInteraction.DataPrepChunks = map[string][]types.DataPrepChunk{}
		} else if req.Mode == types.CloneInteractionModeWithQuestions {
			// remove the fine tune file
			assistantInteraction.DataPrepStage = types.TextDataPrepStageEditQuestions
			assistantInteraction.State = types.InteractionStateEditing
			assistantInteraction.Finished = false
		}
		return assistantInteraction, nil
	})
	if err != nil {
		return nil, err
	}

	createdSession, err := c.Options.Store.CreateSession(ctx, *newSession)
	if err != nil {
		return nil, err
	}

	return createdSession, nil
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

// return the JSON of some fine tune conversation data
func (c *Controller) ReadTextFineTuneQuestions(filepath string) ([]types.DataPrepTextQuestion, error) {
	data, err := c.FilestoreReadTextFile(filepath)
	if err != nil {
		return nil, err
	}

	var conversations []types.DataPrepTextQuestion
	lines := strings.Split(data, "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		var conversation types.DataPrepTextQuestion
		err := json.Unmarshal([]byte(line), &conversation)
		if err != nil {
			return nil, err
		}
		conversations = append(conversations, conversation)
	}

	return conversations, nil
}

func (c *Controller) WriteTextFineTuneQuestions(filepath string, data []types.DataPrepTextQuestion) error {
	jsonLines := []string{}

	for _, conversationEntry := range data {
		jsonLine, err := json.Marshal(conversationEntry)
		if err != nil {
			return err
		}
		jsonLines = append(jsonLines, string(jsonLine))
	}

	_, err := c.Options.Filestore.WriteFile(c.Ctx, filepath, strings.NewReader(strings.Join(jsonLines, "\n")))
	if err != nil {
		return err
	}

	return nil
}
