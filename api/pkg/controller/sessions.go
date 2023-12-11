// sessions are the higher level ChatGPT like UI concept

package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/lukemarsden/helix/api/pkg/model"
	"github.com/lukemarsden/helix/api/pkg/system"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// set to false in production (will log messages to web UI)
const DEBUG = true

func (c *Controller) CreateSession(ctx context.Context, req types.CreateSessionRequest) (*types.Session, error) {
	// the system interaction is the task we will run on a GPU and update in place
	systemInteraction := types.Interaction{
		ID:             system.GenerateUUID(),
		Created:        time.Now(),
		Updated:        time.Now(),
		Creator:        types.CreatorTypeSystem,
		Mode:           req.SessionMode,
		Message:        "",
		Files:          []string{},
		State:          types.InteractionStateWaiting,
		Finished:       false,
		Metadata:       map[string]string{},
		DataPrepChunks: map[string][]types.DataPrepChunk{},
	}

	session := types.Session{
		ID:            req.SessionID,
		Name:          system.GenerateAmusingName(),
		ModelName:     req.ModelName,
		Type:          req.SessionType,
		Mode:          req.SessionMode,
		ParentSession: req.ParentSession,
		Owner:         req.Owner,
		OwnerType:     req.OwnerType,
		Created:       time.Now(),
		Updated:       time.Now(),
		Interactions: []types.Interaction{
			req.UserInteraction,
			systemInteraction,
		},
		Config: types.SessionConfig{
			OriginalMode: req.SessionMode,
		},
	}

	// create session in database
	sessionData, err := c.Options.Store.CreateSession(ctx, session)
	if err != nil {
		return nil, err
	}

	go c.SessionRunner(sessionData)

	return sessionData, nil
}

func (c *Controller) UpdateSession(ctx context.Context, req types.UpdateSessionRequest) (*types.Session, error) {
	systemInteraction := types.Interaction{
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
	session, err := c.Options.Store.GetSession(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}
	session.Updated = time.Now()
	session.Interactions = append(session.Interactions, req.UserInteraction, systemInteraction)

	log.Debug().
		Msgf("🟢 update session: %+v", session)

	sessionData, err := c.Options.Store.UpdateSession(ctx, *session)
	if err != nil {
		return nil, err
	}

	go c.SessionRunner(sessionData)

	return sessionData, nil
}

func (c *Controller) AddDocumentsToSession(ctx context.Context, session *types.Session, userInteraction types.Interaction) (*types.Session, error) {
	// the system interaction is the task we will run on a GPU and update in place
	systemInteraction := types.Interaction{
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

func (c *Controller) CloneTextFinetuneInteraction(
	ctx context.Context,
	session *types.Session,
	userInteraction *types.Interaction,
	mode types.CloneTextType,
) (*types.Session, error) {
	// sessionID := system.GenerateUUID()

	return session, nil
}

// called once we've done the pre-processing for both create and update calls to sessions
func (c *Controller) SessionRunner(sessionData *types.Session) {
	// first we prepare the seession - which could mean whatever the model implementation wants
	// so we have to wait for that to complete before adding to the queue
	// the model can be adding subsequent child sessions to the queue
	// e.g. in the case of text fine tuning data prep - we need an LLM to convert
	// text into q&a pairs and we want to use our own mistral inference
	preparedSession, err := c.PrepareSession(sessionData)
	if err != nil {
		log.Error().Msgf("error preparing session: %s", err.Error())
		c.ErrorSession(sessionData, err)
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
	var err error
	// load the model
	// call it's
	// here we need to turn all of the uploaded files into text files
	// so we ping our handy python server that will do that for us
	if session.Type == types.SessionTypeText && session.Mode == types.SessionModeFinetune {
		session, err = c.convertDocumentsToText(session)
		if err != nil {
			return nil, err
		}
		session, err = c.convertChunksToQuestions(session)
		if err != nil {
			return nil, err
		}

		// we DON'T want the session in the queue yet
		// the user has to confirm the questions are correct
		// or there might have been errors that we want to give the user
		// a chance to decide what to do
		return nil, nil
	}
	return session, nil
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
	newInteractions := []types.Interaction{}
	for _, interaction := range session.Interactions {
		if interaction.ID == newInteraction.ID {
			newInteractions = append(newInteractions, *newInteraction)
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
	userInteraction, err := model.GetUserInteraction(session)
	if err != nil {
		return
	}

	userInteraction.Finished = true
	userInteraction.State = types.InteractionStateComplete

	errorInteraction, err := model.GetSystemInteraction(session)
	if err != nil {
		return
	}
	errorInteraction.State = types.InteractionStateError
	errorInteraction.Completed = time.Now()
	errorInteraction.Error = sessionErr.Error()
	errorInteraction.Finished = true

	session = updateSessionInteractions(session, []types.Interaction{
		*userInteraction,
		*errorInteraction,
	})

	c.WriteSession(session)
}

// add the given session onto the end of the queue
// unless it's already waiting and present in the queue
// in which case let's replace it at it's current position
// we mark the session as "preparing" here to give text fine tuning
// a chance to sort itself out in the background
func (c *Controller) AddSessionToQueue(session *types.Session) {
	sessionSummary, err := model.GetSessionSummary(session)
	if err != nil {
		log.Error().Msgf("error getting session summary: %s", err.Error())
		return
	}

	c.sessionQueueMtx.Lock()
	defer c.sessionQueueMtx.Unlock()

	existing := false
	newQueue := []*types.Session{}
	newSummaryQueue := []*types.SessionSummary{}
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
	}
	if !existing {
		// we did not find the session already in the queue
		newQueue = append(newQueue, session)
		newSummaryQueue = append(newSummaryQueue, sessionSummary)
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

	session, err = model.UpdateSystemInteraction(session, func(targetInteraction *types.Interaction) (*types.Interaction, error) {
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
		}

		return targetInteraction, nil
	})

	c.WriteSession(session)

	return taskResponse, nil
}
