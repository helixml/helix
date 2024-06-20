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

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/prompts"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// set to false in production (will log messages to web UI)
const DEBUG = true

func (c *Controller) StartSession(ctx types.RequestContext, req types.InternalSessionRequest) (*types.Session, error) {
	systemInteraction := &types.Interaction{
		ID:             system.GenerateUUID(),
		Created:        time.Now(),
		Updated:        time.Now(),
		Creator:        types.CreatorTypeSystem,
		Mode:           req.Mode,
		Message:        "",
		Files:          []string{},
		State:          types.InteractionStateWaiting,
		Finished:       false,
		Metadata:       map[string]string{},
		DataPrepChunks: map[string][]types.DataPrepChunk{},
		ResponseFormat: req.ResponseFormat,
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
		Interactions:  append(req.UserInteractions, systemInteraction),
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

		sessions, err := c.Options.Store.GetSessions(ctx.Ctx, store.GetSessionsQuery{
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
				lastInteraction, err := data.GetLastSystemInteraction(session.Interactions)
				if err != nil {
					log.Error().Err(err).Msgf("failed to get last system interaction for session: %s", session.ID)
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
	sessionData, err := c.Options.Store.CreateSession(ctx.Ctx, newSession)
	if err != nil {
		return nil, err
	}

	go c.SessionRunner(sessionData)

	err = c.Options.Janitor.WriteSessionEvent(types.SessionEventTypeCreated, ctx, sessionData)
	if err != nil {
		return nil, err
	}

	if newSession.Mode == types.SessionModeFinetune {
		err := c.Options.Notifier.Notify(ctx.Ctx, &notification.Notification{
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

func (c *Controller) UpdateSession(ctx types.RequestContext, req types.UpdateSessionRequest) (*types.Session, error) {
	session, err := c.Options.Store.GetSession(ctx.Ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session %s: %w", req.SessionID, err)
	}

	systemInteraction := &types.Interaction{
		ID:       system.GenerateUUID(),
		Created:  time.Now(),
		Updated:  time.Now(),
		Creator:  types.CreatorTypeSystem,
		Mode:     req.SessionMode,
		Message:  "",
		Files:    []string{},
		State:    types.InteractionStateWaiting,
		Finished: false,
		Metadata: map[string]string{},
	}

	session.Updated = time.Now()
	session.Interactions = append(session.Interactions, req.UserInteraction, systemInteraction)

	log.Debug().Msgf("🟢 update session: %+v", session)

	sessionData, err := c.Options.Store.UpdateSession(ctx.Ctx, *session)
	if err != nil {
		return nil, err
	}

	go c.SessionRunner(sessionData)

	err = c.Options.Janitor.WriteSessionEvent(types.SessionEventTypeUpdated, ctx, sessionData)
	if err != nil {
		return nil, err
	}

	return sessionData, nil
}

func (c *Controller) RestartSession(session *types.Session) (*types.Session, error) {
	// let's see if this session is currently active as far as runners are aware
	activeSessions := map[string]bool{}
	c.activeRunners.Range(func(i string, metrics *types.RunnerState) bool {
		for _, modelInstance := range metrics.ModelInstances {
			if modelInstance.CurrentSession == nil {
				continue
			}
			activeSessions[modelInstance.CurrentSession.SessionID] = true
		}
		return true
	})

	_, ok := activeSessions[session.ID]
	if ok {
		return nil, fmt.Errorf("session is currently active")
	}

	session, err := data.UpdateSystemInteraction(session, func(systemInteraction *types.Interaction) (*types.Interaction, error) {
		systemInteraction.Error = ""
		systemInteraction.Finished = false
		// empty out the previous message so model doesn't think it's already finished
		systemInteraction.Message = ""

		systemInteraction.State = types.InteractionStateWaiting

		// if this is a text inference then don't set the progress to 1 because
		// we don't show progress for text inference
		if session.Mode == types.SessionModeFinetune || session.Type == types.SessionTypeImage {
			systemInteraction.Progress = 1
		} else {
			systemInteraction.Progress = 0
		}

		if session.Mode == types.SessionModeFinetune {
			if systemInteraction.DataPrepStage == types.TextDataPrepStageExtractText || systemInteraction.DataPrepStage == types.TextDataPrepStageGenerateQuestions {
				// in this case we are restarting the data prep
				systemInteraction.Status = ""
			} else if systemInteraction.DataPrepStage == types.TextDataPrepStageFineTune {
				// in this case we are restarting the fine tuning itself
				systemInteraction.Status = "restarted: fine tuning on data..."
			}
		}

		return systemInteraction, nil
	})

	if err != nil {
		return nil, err
	}

	c.WriteSession(session)

	// this will re-run the data prep preparation
	// but that is idempotent so we should be able to
	// not care and just say "start again"
	// if there is more data prep to do, it will carry on
	// if we go staight into the queue then it's a fine tune restart
	go c.SessionRunner(session)

	return session, nil
}

func (c *Controller) AddDocumentsToSession(ctx context.Context, session *types.Session, userInteraction *types.Interaction) (*types.Session, error) {
	// the system interaction is the task we will run on a GPU and update in place
	systemInteraction := &types.Interaction{
		ID:             system.GenerateUUID(),
		Created:        time.Now(),
		Updated:        time.Now(),
		Creator:        types.CreatorTypeSystem,
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
	session.Interactions = append(session.Interactions, userInteraction, systemInteraction)

	c.WriteSession(session)
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
		Msgf("🟢 update session config: %s %+v", sessionData.ID, sessionData.Metadata)

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

	session, err = data.UpdateSystemInteraction(session, func(systemInteraction *types.Interaction) (*types.Interaction, error) {
		systemInteraction.State = types.InteractionStateWaiting
		return systemInteraction, nil
	})
	if err != nil {
		return nil, err
	}

	session.Mode = types.SessionModeFinetune

	c.WriteSession(session)
	go c.SessionRunner(session)

	return session, nil
}

// the idempotent function to "run" the session
// it should work out what this means - i.e. have we prepared the data yet?
func (c *Controller) SessionRunner(sessionData *types.Session) {
	// Wait for that to complete before adding to the queue
	// the model can be adding subsequent child sessions to the queue
	// e.g. in the case of text fine tuning data prep - we need an LLM to convert
	// text into q&a pairs and we want to use our own mistral inference
	preparedSession, err := c.PrepareSession(sessionData)
	if err != nil {
		log.Error().Msgf("error preparing session: %s", err.Error())
		c.ErrorSession(sessionData, err)
		return
	}

	// If last interaction is "action" then we should run the action
	// and not the model
	lastInteraction, err := data.GetLastSystemInteraction(sessionData.Interactions)
	if err == nil && lastInteraction.Mode == types.SessionModeAction {
		_, err := c.runActionInteraction(context.Background(), sessionData, lastInteraction)
		if err != nil {
			log.Error().Msgf("error running action interaction: %s", err.Error())
			c.ErrorSession(sessionData, err)
		}
		return
	}

	// it's ok if we did not get a session back here
	// it means there will be a later action that will add the session to the queue
	// in the case the user needs to edit some data before it can be run for example
	if preparedSession != nil {
		c.AddSessionToQueue(preparedSession)
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
func (c *Controller) PrepareSession(session *types.Session) (*types.Session, error) {
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
			session, convertedTextDocuments, err = c.convertDocumentsToText(session)
			if err != nil {
				return nil, err
			}
		}

		// we put this behind a feature flag because then we can have fine-tune only sessions
		if session.Metadata.RagEnabled {
			session, _, err = c.indexChunksForRag(session)
			if err != nil {
				return nil, err
			}

			// if we are NOT doing fine tuning then we need to mark this as finshed
			if !session.Metadata.TextFinetuneEnabled {
				session, err := data.UpdateSystemInteraction(session, func(systemInteraction *types.Interaction) (*types.Interaction, error) {
					systemInteraction.Finished = true
					systemInteraction.Progress = 0
					systemInteraction.Message = ""
					systemInteraction.Status = "We have indexed all of your documents now you can ask questions..."
					systemInteraction.State = types.InteractionStateComplete
					systemInteraction.DataPrepStage = types.TextDataPrepStageComplete
					systemInteraction.Files = []string{}
					return systemInteraction, nil
				})

				// we need to switch to inference mode now so the user can ask questions
				session.Mode = types.SessionModeInference

				if err != nil {
					return nil, err
				}

				c.WriteSession(session)
				c.BroadcastProgress(session, 0, "")
			}
		}

		// we put this behind a feature flag because then we can have RAG only sessions
		if session.Metadata.TextFinetuneEnabled {
			session, questionChunksGenerated, err := c.convertChunksToQuestions(session)
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
			c.BeginFineTune(session)
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
			session, err = data.UpdateUserInteraction(session, func(userInteraction *types.Interaction) (*types.Interaction, error) {
				userInteraction.DisplayMessage = userInteraction.Message
				injectedUserPrompt, err := prompts.RAGInferencePrompt(session, userInteraction.Message, ragResults)
				if err != nil {
					return nil, err
				}
				userInteraction.Message = injectedUserPrompt
				return userInteraction, nil
			})
			session, err = data.UpdateSystemInteraction(session, func(systemInteraction *types.Interaction) (*types.Interaction, error) {
				systemInteraction.RagResults = ragResults
				return systemInteraction, nil
			})
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

	tools := []*types.Tool{}
	// if this session is spawned from an app then we populate the list of tools from the app rather than the linked
	// database record
	if session.ParentApp != "" {
		app, err := c.Options.Store.GetApp(ctx, session.ParentApp)
		if err != nil {
			return nil, fmt.Errorf("error getting app: %w", err)
		}

		// if the tool exists but the user cannot access it - then something funky is being attempted and we should deny it
		if (!app.Global && !app.Shared) && app.Owner != session.Owner {
			return nil, system.NewHTTPError403(fmt.Sprintf("you do not have access to the app with the id: %s", app.ID))
		}

		if len(app.Config.Helix.Assistants) > 0 {
			assistantID := session.Metadata.AssistantID
			if assistantID == "" {
				assistantID = "0"
			}
			assistant := data.GetAssistant(app, assistantID)
			if assistant == nil {
				return nil, system.NewHTTPError404(fmt.Sprintf("we could not find the assistant with the id: %s", assistantID))
			}
			for _, tool := range assistant.Tools {
				tools = append(tools, &tool)
			}
		}
	} else {
		for _, id := range session.Metadata.ActiveTools {
			tool, err := c.Options.Store.GetTool(context.Background(), id)
			// we might have stale tool ids in our metadata
			// so let's not error here
			if err != nil {
				log.Error().Err(err).Msgf("error loading tool from session config, perhaps stale tool ID found, session: %s, tool: %s", session.ID, id)
				continue
			}

			// if the tool exists but the user cannot access it - then something funky is being attempted and we should deny it
			if !tool.Global && tool.Owner != session.Owner {
				return nil, system.NewHTTPError403(fmt.Sprintf("you do not have access to the tool with the id: %s", tool.ID))
			}

			tools = append(tools, tool)
		}
	}

	if len(tools) == 0 {
		// No tools available, nothing to check
		return session, nil
	}

	userInteraction, err := data.GetLastUserInteraction(session.Interactions)
	if err != nil {
		return nil, fmt.Errorf("failed to get last user interaction: %w", err)
	}

	history := data.GetLastInteractions(session, actionContextHistorySize)

	// If history has more than 2 interactions, remove the last 2 as it's the current user and system interaction
	if len(history) > 2 {
		history = history[:len(history)-2]
	}

	// todo need to be per-assistant configuration (polish assistant, german assistant...)
	isActionable, err := c.ToolsPlanner.IsActionable(ctx, tools, history, userInteraction.Message, session.ParentApp)
	if err != nil {
		log.Error().Err(err).Msg("failed to evaluate of the message is actionable, skipping to general knowledge")
		return session, nil
	}

	log.Info().
		Str("api", isActionable.Api).
		Str("actionable", isActionable.NeedsTool).
		Str("justification", isActionable.Justification).
		Str("message", userInteraction.Message).
		Msg("checked for actionable")

	if !isActionable.Actionable() {
		return session, nil
	}

	// Actionable, converting interaction mode to "action"
	lastInteraction, err := data.GetLastSystemInteraction(session.Interactions)
	if err != nil {
		return nil, fmt.Errorf("failed to get last system interaction: %w", err)
	}

	lastInteraction.Mode = types.SessionModeAction

	lastInteraction.Mode = types.SessionModeAction
	lastInteraction.Metadata["tool_action"] = isActionable.Api
	lastInteraction.Metadata["tool_action_justification"] = isActionable.Justification

	actionTool, ok := getToolFromAction(tools, isActionable.Api)
	if !ok {
		return nil, fmt.Errorf("tool not found for action: %s", isActionable.Api)
	}

	lastInteraction.Metadata["tool_id"] = actionTool.ID
	lastInteraction.Metadata["tool_app_id"] = session.ParentApp

	return session, nil
}

func (c *Controller) BeginFineTune(session *types.Session) error {
	session, err := data.UpdateSystemInteraction(session, func(systemInteraction *types.Interaction) (*types.Interaction, error) {
		systemInteraction.Finished = false
		systemInteraction.Progress = 1
		systemInteraction.Message = ""
		systemInteraction.Status = "fine tuning on data..."
		systemInteraction.State = types.InteractionStateWaiting
		systemInteraction.DataPrepStage = types.TextDataPrepStageFineTune
		systemInteraction.Files = []string{}
		return systemInteraction, nil
	})

	if err != nil {
		return err
	}

	c.WriteSession(session)
	c.AddSessionToQueue(session)
	c.BroadcastProgress(session, 1, "fine tuning on data...")

	return nil
}

// generic "update this session handler"
// this will emit a UserWebsocketEvent with a type of
// WebsocketEventSessionUpdate
func (c *Controller) WriteSession(session *types.Session) {
	log.Trace().
		Msgf("🔵 update session: %s %+v", session.ID, session)

	_, err := c.Options.Store.UpdateSession(context.Background(), *session)
	if err != nil {
		log.Printf("Error adding message: %s", err)
	}

	event := &types.WebsocketEvent{
		Type:      types.WebsocketEventSessionUpdate,
		SessionID: session.ID,
		Owner:     session.Owner,
		Session:   session,
	}

	c.UserWebsocketEventChanWriter <- event
}

func (c *Controller) WriteInteraction(session *types.Session, newInteraction *types.Interaction) *types.Session {
	newInteractions := []*types.Interaction{}
	for _, interaction := range session.Interactions {
		if interaction.ID == newInteraction.ID {
			newInteractions = append(newInteractions, newInteraction)
		} else {
			newInteractions = append(newInteractions, interaction)
		}
	}
	session.Interactions = newInteractions
	c.WriteSession(session)
	return session
}

func (c *Controller) BroadcastWebsocketEvent(ctx context.Context, ev *types.WebsocketEvent) error {
	c.UserWebsocketEventChanWriter <- ev
	return nil
}

func (c *Controller) BroadcastProgress(
	session *types.Session,
	progress int,
	status string,
) {
	ev := &types.WebsocketEvent{
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
	c.UserWebsocketEventChanWriter <- ev
}

func (c *Controller) ErrorSession(session *types.Session, sessionErr error) {
	session, err := data.UpdateUserInteraction(session, func(userInteraction *types.Interaction) (*types.Interaction, error) {
		userInteraction.Finished = true
		userInteraction.State = types.InteractionStateComplete
		return userInteraction, nil
	})
	if err != nil {
		return
	}
	session, err = data.UpdateSystemInteraction(session, func(systemInteraction *types.Interaction) (*types.Interaction, error) {
		systemInteraction.State = types.InteractionStateError
		systemInteraction.Completed = time.Now()
		systemInteraction.Error = sessionErr.Error()
		systemInteraction.Finished = true
		return systemInteraction, nil
	})
	if err != nil {
		return
	}
	c.WriteSession(session)
	c.Options.Janitor.WriteSessionError(session, sessionErr)
}

// add the given session onto the end of the queue
// unless it's already waiting and present in the queue
// in which case let's replace it at it's current position
// we mark the session as "preparing" here to give text fine tuning
// a chance to sort itself out in the background
func (c *Controller) AddSessionToQueue(session *types.Session) {
	sessionSummary, err := data.GetSessionSummary(session)
	if err != nil {
		log.Error().Msgf("error getting session summary: %s", err.Error())
		return
	}

	c.sessionQueueMtx.Lock()
	defer c.sessionQueueMtx.Unlock()

	existing := false
	newQueue := []*types.Session{}
	newSummaryQueue := []*types.SessionSummary{}

	// what is the latest priority session in the queue?
	// if we are a priority session then we will get put after that one
	// if there are no priority sessions in the queue and we are - then we go first
	lastPriorityIndex := -1
	for i, existingSession := range c.sessionQueue {
		if existingSession.ID == session.ID {
			// the session we are updating is already in the queue!
			newQueue = append(newQueue, session)
			newSummaryQueue = append(newSummaryQueue, sessionSummary)
			existing = true
		} else {
			// this is another session we just want to copy it over
			// we use the index to copy so it's the same for the summary and the actual session
			newQueue = append(newQueue, c.sessionQueue[i])
			newSummaryQueue = append(newSummaryQueue, c.sessionSummaryQueue[i])
		}
		if existingSession.Metadata.Priority {
			lastPriorityIndex = i
		}
	}
	if !existing {
		if session.Metadata.Priority {
			if lastPriorityIndex == -1 {
				// prepend the session to the start of the queue
				newQueue = append([]*types.Session{session}, newQueue...)
				newSummaryQueue = append([]*types.SessionSummary{sessionSummary}, newSummaryQueue...)
			} else {
				// insert the session into newQueue just after the lastPriorityIndex
				newQueue = append(newQueue[:lastPriorityIndex+1], append([]*types.Session{session}, newQueue[lastPriorityIndex+1:]...)...)
				newSummaryQueue = append(newSummaryQueue[:lastPriorityIndex+1], append([]*types.SessionSummary{sessionSummary}, newSummaryQueue[lastPriorityIndex+1:]...)...)
			}
		} else {
			// we did not find the session already in the queue
			newQueue = append(newQueue, session)
			newSummaryQueue = append(newSummaryQueue, sessionSummary)
		}
	}

	c.sessionQueue = newQueue
	c.sessionSummaryQueue = newSummaryQueue
}

func (c *Controller) HandleRunnerResponse(ctx context.Context, taskResponse *types.RunnerTaskResponse) (*types.RunnerTaskResponse, error) {
	session, err := c.Options.Store.GetSession(ctx, taskResponse.SessionID)
	if err != nil {
		return nil, err
	}

	if session == nil {
		return nil, fmt.Errorf("session not found: %s", taskResponse.SessionID)
	}

	session, err = data.UpdateSystemInteraction(session, func(targetInteraction *types.Interaction) (*types.Interaction, error) {
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
	c.WriteSession(session)

	if taskResponse.Error != "" {
		c.Options.Janitor.WriteSessionError(session, fmt.Errorf(taskResponse.Error))
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
	ctx types.RequestContext,
	oldSession *types.Session,
	req CloneUntilInteractionRequest,
) (*types.Session, error) {
	// * get a top level session object
	//   * against the correct account
	//   * only include interactions up until the given interaction
	// * loop over each interaction files
	//   * if CopyAllFiles then copy all interaction files into our file store
	//   * otherwise only copy the
	newSession, err := data.CloneSession(*oldSession, req.InteractionID, data.OwnerContextFromRequestContext(ctx))
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

	systemInteraction, err := data.GetLastSystemInteraction(newSession.Interactions)
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

	newPrefix, err := c.GetFilestoreSessionPath(data.OwnerContext(ctx.User.ID), newSession.ID)
	if err != nil {
		return nil, err
	}

	newDocumentIds := map[string]string{}

	for filename, documentID := range newSession.Metadata.DocumentIDs {
		newFile := strings.Replace(filename, oldPrefix, newPrefix, 1)
		newDocumentIds[newFile] = documentID
	}

	newSession.Metadata.DocumentIDs = newDocumentIds

	copyFile := func(filePath string) (string, error) {
		newFile := strings.Replace(filePath, oldPrefix, newPrefix, 1)
		log.Debug().
			Msgf("🔵 clone interaction file: %s -> %s", filePath, newFile)
		err := c.Options.Filestore.CopyFile(ctx.Ctx, filePath, newFile)
		if err != nil {
			return "", err
		}
		return newFile, nil
	}

	copyFolder := func(folderPath string) (string, error) {
		newFolder := strings.Replace(folderPath, oldPrefix, newPrefix, 1)
		log.Debug().
			Msgf("🔵 clone folder: %s -> %s", folderPath, newFolder)
		reader, err := c.Options.Filestore.DownloadFolder(ctx.Ctx, folderPath)
		if err != nil {
			return "", err
		}
		err = c.Options.Filestore.UploadFolder(ctx.Ctx, newFolder, reader)
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
			if path.Base(file) == types.TEXT_DATA_PREP_QUESTIONS_FILE && req.Mode == types.CloneInteractionModeJustData && interaction.ID == userInteraction.ID {
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
			if interaction.ID == systemInteraction.ID {
				// we are on the latest system interaction
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
		} else if interaction.ID == userInteraction.ID || interaction.ID == systemInteraction.ID {
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

	newSession, err = data.UpdateSystemInteraction(newSession, func(systemInteraction *types.Interaction) (*types.Interaction, error) {
		// only touch if we are not cloning as is
		if req.Mode != types.CloneInteractionModeAll {
			systemInteraction.Created = time.Now()
			systemInteraction.Updated = time.Now()
			systemInteraction.Message = ""
			systemInteraction.Status = ""
			systemInteraction.Progress = 0
		}

		if req.Mode == types.CloneInteractionModeJustData {
			// remove the fine tune file
			systemInteraction.DataPrepStage = types.TextDataPrepStageEditFiles
			systemInteraction.State = types.InteractionStateEditing
			systemInteraction.Finished = false
			// remove the metadata that keeps track of processed questions
			// (because we have deleted the questions file)
			systemInteraction.DataPrepChunks = map[string][]types.DataPrepChunk{}
		} else if req.Mode == types.CloneInteractionModeWithQuestions {
			// remove the fine tune file
			systemInteraction.DataPrepStage = types.TextDataPrepStageEditQuestions
			systemInteraction.State = types.InteractionStateEditing
			systemInteraction.Finished = false
		}
		return systemInteraction, nil
	})
	if err != nil {
		return nil, err
	}

	createdSession, err := c.Options.Store.CreateSession(ctx.Ctx, *newSession)
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
