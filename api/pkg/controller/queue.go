// sessions are the higher level ChatGPT like UI concept

package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/lukemarsden/helix/api/pkg/model"
	"github.com/lukemarsden/helix/api/pkg/store"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// this function expects the sessionQueueMtx to be locked when it is run
func (c *Controller) getMatchingSessionFilterIndex(ctx context.Context, filter types.SessionFilter) int {
	for i, session := range c.sessionQueue {
		// include sessions that are older than filter.Older
		// so - filter out ones that are too new
		if filter.Older != types.Duration(0) {
			now := time.Now()
			tooNewThreshold := now.Add(-time.Duration(filter.Older))
			if session.Created.After(tooNewThreshold) { // too new
				log.Trace().Msgf(
					"skipping session %s because it is too new (session created at %s which is after threshold %s)",
					session.ID, session.Created, tooNewThreshold,
				)
				continue
			}
		}

		if filter.Mode != "" && session.Mode != filter.Mode {
			continue
		}
		if filter.Type != "" && session.Type != filter.Type {
			continue
		}
		if filter.ModelName != "" && session.ModelName != filter.ModelName {
			continue
		}

		if filter.LoraDir == types.LORA_DIR_NONE {
			// the filter is NONE - we cannot have a finetune file
			if session.LoraDir != "" {
				continue
			}
		} else if filter.LoraDir != "" {
			// the filter is a SPECIFIC file - we must have that file
			if session.LoraDir != filter.LoraDir {
				continue
			}
		} else if filter.LoraDir == "" {
			// the filter is ANY file - so anything goes
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
		reject := false
		for _, rejectEntry := range filter.Reject {
			if rejectEntry.ModelName == session.ModelName && rejectEntry.Mode == session.Mode &&
				((rejectEntry.LoraDir == types.LORA_DIR_NONE && session.LoraDir == "") ||
					(rejectEntry.LoraDir != "" && rejectEntry.LoraDir == session.LoraDir)) {
				reject = true
			}
		}
		if reject {
			continue
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
	sessionSummaryQueue := []*types.SessionSummary{}

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

		summary, err := model.GetSessionSummary(session)
		if err != nil {
			return err
		}

		sessionQueue = append(sessionQueue, session)
		sessionSummaryQueue = append(sessionSummaryQueue, summary)
	}

	// now we have the queue in oldest first order
	c.sessionQueue = sessionQueue
	c.sessionSummaryQueue = sessionSummaryQueue
	return nil
}

func (c *Controller) ShiftSessionQueue(ctx context.Context, filter types.SessionFilter, runnerID string) (*types.Session, error) {
	c.sessionQueueMtx.Lock()
	defer c.sessionQueueMtx.Unlock()

	sessionIndex := c.getMatchingSessionFilterIndex(ctx, filter)

	if sessionIndex >= 0 {
		session := c.sessionQueue[sessionIndex]

		log.Debug().
			Msgf("🔵 scheduler hit query: %+v", filter)
		log.Debug().
			Msgf("🔵 scheduler hit session: %+v", session)

		c.sessionQueue = append(c.sessionQueue[:sessionIndex], c.sessionQueue[sessionIndex+1:]...)
		c.sessionSummaryQueue = append(c.sessionSummaryQueue[:sessionIndex], c.sessionSummaryQueue[sessionIndex+1:]...)

		if len(session.Interactions) == 0 {
			return nil, fmt.Errorf("no interactions found")
		}

		session, err := model.UpdateSystemInteraction(session, func(targetInteraction *types.Interaction) (*types.Interaction, error) {
			targetInteraction.Scheduled = time.Now()
			return targetInteraction, nil
		})

		if err != nil {
			return nil, err
		}

		c.addSchedulingDecision(filter, runnerID, session)
		c.WriteSession(session)
		return session, nil
	}

	return nil, nil
}

func (c *Controller) addSchedulingDecision(filter types.SessionFilter, runnerID string, session *types.Session) {
	systemInteraction, err := model.GetSystemInteraction(session)
	if err != nil {
		log.Error().Msgf("error adding scheduling decision: %s", err)
		return
	}
	decision := &types.GlobalSchedulingDecision{
		Created:       time.Now(),
		RunnerID:      runnerID,
		SessionID:     session.ID,
		InteractionID: systemInteraction.ID,
		Filter:        filter,
		ModelName:     session.ModelName,
		Mode:          session.Mode,
	}

	c.schedulingDecisions = append([]*types.GlobalSchedulingDecision{decision}, c.schedulingDecisions...)

	if len(c.schedulingDecisions) > c.Options.SchedulingDecisionBufferSize {
		c.schedulingDecisions = c.schedulingDecisions[:len(c.schedulingDecisions)-1]
	}
}
