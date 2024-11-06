// sessions are the higher level ChatGPT like UI concept

package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// TODO: remove
func (c *Controller) ShiftSessionQueue(ctx context.Context, filter types.SessionFilter, runnerID string) (*types.Session, error) {
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
	log.Info().Str("runnerID", runnerID).Interface("filter", filter).Interface("req", req).Msgf("🟠 helix_openai_server GetNextLLMInferenceRequest END")
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
