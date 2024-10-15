// sessions are the higher level ChatGPT like UI concept

package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// this function expects the sessionQueueMtx to be locked when it is run
func (c *Controller) getMatchingSessionFilterIndex(_ context.Context, filter types.SessionFilter) int {
	for i, session := range c.sessionQueue {
		// include sessions that are older than filter.Older
		// so - filter out ones that are too new
		if filter.Older != types.Duration(0) {
			now := time.Now()
			tooNewThreshold := now.Add(-time.Duration(filter.Older))
			if session.Updated.After(tooNewThreshold) { // too new
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

		if filter.Runtime != "" {
			// Filter by runtime
			if model.NewModel(session.ModelName).InferenceRuntime() != filter.Runtime {
				continue
			}
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
			model, err := model.GetModel(session.ModelName)
			if err != nil {
				log.Error().Msgf("unable to look up model %s, possible programming error in adding model to models map (%s)", session.ModelName, err)
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
		if latest.Creator == types.CreatorTypeAssistant {
			// we've already given a response, don't need to do anything
			continue
		}

		if latest.Runner != "" {
			// this session is already being worked on
			continue
		}

		summary, err := data.GetSessionSummary(session)
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

// TODO: remove
func (c *Controller) ShiftSessionQueue(ctx context.Context, filter types.SessionFilter, runnerID string) (*types.Session, error) {
	c.sessionQueueMtx.Lock()
	defer c.sessionQueueMtx.Unlock()

	// Schedule all new sessions in the queue, until we run out of runners
	taken := 0
	for _, session := range c.sessionQueue {
		log.Info().Str("session_id", session.ID).Msg("scheduling session")
		work, err := scheduler.NewSessonWorkload(session)
		if err != nil {
			return nil, fmt.Errorf("creating session workload: %w", err)
		}

		err = c.scheduler.Schedule(work)
		if err != nil {
			retry, err := scheduler.ErrorHandlingStrategy(err, work)

			// If we can retry, break out of the loop and try again later
			if retry {
				break
			}

			// If we can't retry, write an error to the request and continue so it takes it off
			// the queue
			errSession := work.Session()
			errSession.Interactions = append(errSession.Interactions, &types.Interaction{
				Creator: types.CreatorTypeSystem,
				Error:   err.Error(),
				Message: "Error scheduling session",
			})
			_, err = c.Options.Store.UpdateSession(ctx, *errSession)
			if err != nil {
				log.Error().Err(err).Msg("error updating session")
			}
		}
		taken++
	}
	c.sessionQueue = c.sessionQueue[taken:]
	c.sessionSummaryQueue = c.sessionSummaryQueue[taken:]

	// Default to requesting warm work
	newWorkOnly := false

	// Only get new work if the filter has a memory requirement (see runner/controller.go)
	if filter.Memory != 0 {
		newWorkOnly = true
	}

	// Now for this runner, get work
	req, err := c.scheduler.WorkForRunner(runnerID, scheduler.WorkloadTypeSession, newWorkOnly, filter.ModelName)
	if err != nil {
		return nil, fmt.Errorf("error getting work for runner: %w", err)
	}

	if req == nil {
		return nil, nil
	}

	c.addSchedulingDecision(filter, runnerID, req.Session())
	log.Info().Str("runnerID", runnerID).Interface("filter", filter).Interface("req", req).Int("len(sessionQueue)", len(c.sessionQueue)).Msgf("🟠 helix_openai_server GetNextLLMInferenceRequest END")
	return req.Session(), nil
}

// TODO: remove
func (c *Controller) addSchedulingDecision(filter types.SessionFilter, runnerID string, session *types.Session) {
	assistantInteraction, err := data.GetAssistantInteraction(session)
	if err != nil {
		log.Error().Msgf("error adding scheduling decision: %s", err)
		return
	}
	decision := &types.GlobalSchedulingDecision{
		Created:       time.Now(),
		RunnerID:      runnerID,
		SessionID:     session.ID,
		InteractionID: assistantInteraction.ID,
		Filter:        filter,
		ModelName:     session.ModelName,
		Mode:          session.Mode,
	}

	c.schedulingDecisions = append([]*types.GlobalSchedulingDecision{decision}, c.schedulingDecisions...)

	if len(c.schedulingDecisions) > c.Options.Config.Controller.SchedulingDecisionBufferSize {
		c.schedulingDecisions = c.schedulingDecisions[:len(c.schedulingDecisions)-1]
	}
}
