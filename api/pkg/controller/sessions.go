// sessions are the higher level ChatGPT like UI concept

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/lukemarsden/helix/api/pkg/model"
	"github.com/lukemarsden/helix/api/pkg/store"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// set to false in production (will log messages to web UI)
const DEBUG = true

// this function expects the sessionQueueMtx to be locked when it is run
func (c *Controller) getMatchingSessionFilterIndex(ctx context.Context, filter types.SessionFilter) int {
	for i, session := range c.sessionQueue {
		if filter.Mode != "" && session.Mode != filter.Mode {
			continue
		}
		if filter.Type != "" && session.Type != filter.Type {
			continue
		}
		if filter.ModelName != "" && session.ModelName != filter.ModelName {
			continue
		}

		if filter.FinetuneFile != "" && session.FinetuneFile != filter.FinetuneFile {
			// in this case - the filter is asking for a session with a finetune file
			// and so we can only reply with a session that has that exact finetune file
			continue
		} else if filter.FinetuneFile == types.FINETUNE_FILE_NONE && session.FinetuneFile != "" {
			// in this case - the runner is asking specifically for a session
			// that does not have a finetune file
			// this cannot be empty string because that means "I don't care"
			continue
		}

		// we are asking for sessions that will fit in an amount of RAM
		// so we need to ask the associated model instance what the memory
		// requirements are for this session
		if filter.Memory > 0 {
			model, ok := c.models[session.ModelName]
			if !ok {
				continue
			}
			if model.GetMemoryRequirements(session.Mode) > filter.Memory {
				continue
			}
		}

		// look to see if we have any rejection matches that we should not include
		for _, rejectEntry := range filter.Reject {
			if rejectEntry.ModelName == session.ModelName && rejectEntry.Mode == session.Mode {
				continue
			}
		}

		// if we've made it this far we've got a session!
		return i
	}

	return -1
}

// load the session queues from the database in case of restart
func (c *Controller) loadSessionQueues(ctx context.Context) error {
	c.sessionQueueMtx.Lock()
	defer c.sessionQueueMtx.Unlock()

	sessionQueue := []*types.Session{}

	st := c.Options.Store

	// fetch all sessions - this is in DESC order so we need to reverse the array
	sessions, err := st.GetSessions(ctx, store.GetSessionsQuery{})
	if err != nil {
		return err
	}

	for i := len(sessions) - 1; i >= 0; i-- {
		session := sessions[i]

		interactions := session.Interactions
		if interactions == nil || len(interactions) == 0 {
			// should never happen, sessions are always initiated by the user
			// creating an initial message
			continue
		}

		latest := interactions[len(interactions)-1]
		if latest.Creator == types.CreatorTypeSystem {
			// we've already given a response, don't need to do anything
			continue
		}

		if latest.Runner != "" {
			// this session is already being worked on
			continue
		}

		sessionQueue = append(sessionQueue, session)
	}

	// now we have the queue in oldest first order
	c.sessionQueue = sessionQueue
	return nil
}

// the core function - decide which task to give to a worker
// TODO: keep track of the previous tasks run by this worker (and therefore we know which weights are loaded into RAM)
// try to send similar tasks to the same worker
func (c *Controller) ShiftSessionQueue(ctx context.Context, filter types.SessionFilter, runnerID string) (*types.Session, error) {
	c.sessionQueueMtx.Lock()
	defer c.sessionQueueMtx.Unlock()

	sessionIndex := c.getMatchingSessionFilterIndex(ctx, filter)

	if sessionIndex >= 0 {
		session := c.sessionQueue[sessionIndex]

		log.Debug().
			Msgf("🔵 scheduler hit query")
		spew.Dump(filter)
		log.Debug().
			Msgf("🔵 scheduler hit session")
		spew.Dump(session)

		c.sessionQueue = append(c.sessionQueue[:sessionIndex], c.sessionQueue[sessionIndex+1:]...)

		if len(session.Interactions) == 0 {
			return nil, fmt.Errorf("no interactions found")
		}

		return session, nil
	}

	return nil, nil
}

func (c *Controller) RemoveSessionFromQueue(ctx context.Context, id string) error {
	c.sessionQueueMtx.Lock()
	defer c.sessionQueueMtx.Unlock()

	sessionQueue := []*types.Session{}

	for _, session := range c.sessionQueue {
		if session.ID == id {
			continue
		}
		sessionQueue = append(sessionQueue, session)
	}

	c.sessionQueue = sessionQueue

	return nil
}

// generic "update this session handler"
func (c *Controller) WriteSession(session *types.Session) {
	log.Debug().
		Msgf("🔵 update session: %s", session.ID)
	spew.Dump(session)

	_, err := c.Options.Store.UpdateSession(context.Background(), *session)
	if err != nil {
		log.Printf("Error adding message: %s", err)
	}

	c.SessionUpdatesChan <- session
}

func (c *Controller) ErrorSession(session *types.Session, sessionErr error) {
	userInteraction, err := model.GetUserInteraction(session)
	if err != nil {
		return
	}

	userInteraction.Finished = true
	userInteraction.State = types.InteractionStateReady

	errorInteraction, err := model.GetSystemInteraction(session)
	if err != nil {
		return
	}
	errorInteraction.State = types.InteractionStateError
	errorInteraction.Error = sessionErr.Error()
	errorInteraction.Finished = true

	newInteractions := []types.Interaction{}
	for _, interaction := range session.Interactions {
		if interaction.ID == errorInteraction.ID {
			newInteractions = append(newInteractions, *errorInteraction)
		} else if interaction.ID == userInteraction.ID {
			newInteractions = append(newInteractions, *userInteraction)
		} else {
			newInteractions = append(newInteractions, interaction)
		}
	}

	session.Interactions = newInteractions

	c.WriteSession(session)
}

// add the given session onto the end of the queue
// unless it's already waiting and present in the queue
// in which case let's replace it at it's current position
// we mark the session as "preparing" here to give text fine tuning
// a chance to sort itself out in the background
func (c *Controller) AddSessionToQueue(session *types.Session) {
	c.sessionQueueMtx.Lock()
	defer c.sessionQueueMtx.Unlock()

	existing := false
	newQueue := []*types.Session{}
	for _, existingSession := range c.sessionQueue {
		if existingSession.ID == session.ID {
			newQueue = append(newQueue, session)
			existing = true
		} else {
			newQueue = append(newQueue, existingSession)
		}
	}
	if !existing {
		newQueue = append(newQueue, session)
	}

	c.sessionQueue = newQueue
}

// if the action is "begin" - then we need to ceate a new textstream that is hooked up correctly
// then we stash that in a map
// if the action is "continue" - load the textstream and write to it
// if the action is "end" - unload the text stream
func (c *Controller) HandleWorkerResponse(ctx context.Context, taskResponse *types.WorkerTaskResponse) (*types.WorkerTaskResponse, error) {
	session, err := c.Options.Store.GetSession(ctx, taskResponse.SessionID)
	if err != nil {
		return nil, err
	}

	if session == nil {
		return nil, fmt.Errorf("session not found: %s", taskResponse.SessionID)
	}

	// let's see if we are updating an existing interaction
	// or appending a new one
	var targetInteraction *types.Interaction
	for _, interaction := range session.Interactions {
		if interaction.ID == taskResponse.InteractionID {
			targetInteraction = &interaction
			break
		}
	}

	if targetInteraction == nil {
		return nil, fmt.Errorf("interaction not found: %s -> %s", taskResponse.SessionID, taskResponse.InteractionID)
	}

	if targetInteraction.Creator == types.CreatorTypeUser {
		return nil, fmt.Errorf("interaction is not a system interaction cannot update: %s -> %s", taskResponse.SessionID, taskResponse.InteractionID)
	}

	// mark the interaction as complete if we are a fully finished response
	if taskResponse.Type == types.WorkerTaskResponseTypeResult {
		targetInteraction.Finished = true
		targetInteraction.State = types.InteractionStateReady
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

	// update the files if there are some
	if taskResponse.Files != nil {
		targetInteraction.Files = taskResponse.Files
	}

	if taskResponse.Error != "" {
		targetInteraction.Error = taskResponse.Error
	}

	if taskResponse.Type == types.WorkerTaskResponseTypeResult && session.Mode == types.SessionModeFinetune && len(taskResponse.Files) > 0 {
		// we got some files back from a finetune
		// so let's hoist the session into inference mode but with the finetune file attached
		session.Mode = types.SessionModeInference
		session.FinetuneFile = taskResponse.Files[0]
		targetInteraction.FinetuneFile = taskResponse.Files[0]
	}

	newInteractions := []types.Interaction{}
	for _, interaction := range session.Interactions {
		if interaction.ID == targetInteraction.ID {
			newInteractions = append(newInteractions, *targetInteraction)
		} else {
			newInteractions = append(newInteractions, interaction)
		}
	}

	session.Interactions = newInteractions

	c.WriteSession(session)

	return taskResponse, nil
}

// this is called in a go routine from the main api handler
// it needs to first prepare the session
// and then it can add it to the queue for backend processing
func (c *Controller) PrepareSession(session *types.Session) (*types.Session, error) {
	// here we need to turn all of the uploaded files into text files
	// so we ping our handy python server that will do that for us
	if session.Type == types.SessionTypeText && session.Mode == types.SessionModeFinetune {
		err := c.convertDocumentsToText(session)
		if err != nil {
			return session, err
		}
	}
	return session, nil
}

type convertTextItem struct {
	name    string `json:"name"`
	content string `json:"content"`
}

// in the case of a text fine tune - we need to convert all the documents first
func (c *Controller) convertDocumentsToText(session *types.Session) error {
	userInteraction, err := model.GetUserInteraction(session)
	if err != nil {
		return err
	}

	if userInteraction.State == types.InteractionStateWaiting {
		for _, file := range userInteraction.Files {

			// if file is not a text file
			// then we need to convert it
			if !strings.HasSuffix(file, ".txt") {
				log.Debug().
					Msgf("🔵 converting file: %s", file)
				reader, err := c.Options.Filestore.Download(c.Ctx, file)
				if err != nil {
					return err
				}

				client := newRetryClient()

				req, err := createMultipartRequest(c.Options.TextExtractionURL, "documents", path.Base(file), reader)
				if err != nil {
					return fmt.Errorf("Error creating request: %v\n", err)
				}

				resp, err := client.Do(req)
				if err != nil {
					return err
				}
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return err
				}

				var result []convertTextItem

				err = json.Unmarshal(body, &result)
				if err != nil {
					return err
				}

				if len(result) == 0 {
					return fmt.Errorf("no results found")
				}

				resultItem := result[0]

				newFilepath := path.Join(path.Dir(file), resultItem.name)
			}
		}
	}

	return fmt.Errorf("convert test error")
}
