// sessions are the higher level ChatGPT like UI concept

package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/lukemarsden/helix/api/pkg/store"
	"github.com/lukemarsden/helix/api/pkg/system"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// set to false in production (will log messages to web UI)
const DEBUG = true

// this function expects the sessionQueueMtx to be locked when it is run
func (c *Controller) getMatchingSessionFilterIndex(ctx context.Context, filter types.SessionFilter, deprioritize bool) int {
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

		// if we are in deprioritize mode - it means we will ignore anything
		// that is mentioned in the deprioritize list
		// this function will be run twice - the first time with deprioritize=true
		// and if nothing is returned then again with deprioritize=false
		// TODO: we can probably be more efficient than an inner loop here
		if deprioritize {
			for _, deprioritizeEntry := range filter.Deprioritize {
				if deprioritizeEntry.ModelName == session.ModelName && deprioritizeEntry.Mode == session.Mode {
					continue
				}
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

	// do the 2 phase filter - first applying the deprioritize filter
	// and then if we don't get a match - without it the second time
	sessionIndex := c.getMatchingSessionFilterIndex(ctx, filter, true)
	if sessionIndex == -1 {
		sessionIndex = c.getMatchingSessionFilterIndex(ctx, filter, false)
	}

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

		// update the runner id on the last interaction
		session.Interactions = append(session.Interactions, types.Interaction{
			ID:       system.GenerateUUID(),
			Created:  time.Now(),
			Creator:  types.CreatorTypeSystem,
			Message:  "",
			Files:    []string{},
			Finished: false,
			Runner:   runnerID,
		})

		_, err := c.Options.Store.UpdateSession(ctx, *session)
		if err != nil {
			return nil, err
		}

		return session, nil
	}

	return nil, nil
}

// add the given session onto the end of the queue
// unless it's already waiting and present in the queue
// in which case let's replace it at it's current position
func (c *Controller) PushSessionQueue(ctx context.Context, session *types.Session) error {
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
	return nil
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
	}

	// update the message if we've been given one
	if taskResponse.Message != "" {
		targetInteraction.Message = taskResponse.Message
	}

	// update the files if there are some
	if taskResponse.Files != nil {
		targetInteraction.Files = taskResponse.Files
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

	fmt.Printf("update session --------------------------------------\n")
	spew.Dump(session)

	_, err = c.Options.Store.UpdateSession(ctx, *session)
	if err != nil {
		log.Printf("Error adding message: %s", err)
	}

	c.SessionUpdatesChan <- session

	return taskResponse, nil
}
