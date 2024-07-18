package openai

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

const schedulingDecisionHistorySize = 10

// TODO: move logic from controller and other places. This method would be called directly from the runner
// handler to get the next session. Pubsub is handled internally within this package
func (c *InternalHelixClient) GetNextLLMInferenceRequest(ctx context.Context, filter types.SessionFilter, runnerID string) (*types.RunnerLLMInferenceRequest, error) {

}

func (c *InternalHelixClient) addSchedulingDecision(filter types.SessionFilter, runnerID string, session *types.Session) {
	systemInteraction, err := data.GetSystemInteraction(session)
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

	if len(c.schedulingDecisions) > schedulingDecisionHistorySize {
		c.schedulingDecisions = c.schedulingDecisions[:len(c.schedulingDecisions)-1]
	}
}

// ProcessRunnerResponse is called on both partial streaming and full responses coming from the runner
func (c *InternalHelixClient) ProcessRunnerResponse(resp *types.RunnerLLMInferenceResponse) error {

}
