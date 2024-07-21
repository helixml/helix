package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

const schedulingDecisionHistorySize = 10

// InternalHelixClient utilizes Helix runners to complete chat requests. Primary
// purpose is to power internal tools
type InternalHelixServer struct {
	cfg *config.ServerConfig

	pubsub     pubsub.PubSub // Used to get responses from the runners
	controller Controller    // Used to create sessions

	runnersMu sync.Mutex
	runners   map[string]*types.RunnerState

	queueMu sync.Mutex
	queue   []*types.RunnerLLMInferenceRequest

	schedulingDecisions []*types.GlobalSchedulingDecision
}

func NewInternalHelixServer(cfg *config.ServerConfig, pubsub pubsub.PubSub, controller Controller) *InternalHelixServer {
	return &InternalHelixServer{
		cfg:        cfg,
		pubsub:     pubsub,
		controller: controller,
	}
}

// TODO: move logic from controller and other places. This method would be called directly from the runner
// handler to get the next session. Pubsub is handled internally within this package
func (c *InternalHelixServer) GetNextLLMInferenceRequest(ctx context.Context, filter types.InferenceRequestFilter, runnerID string) (*types.RunnerLLMInferenceRequest, error) {

}

func (c *InternalHelixServer) enqueueRequest(req *types.RunnerLLMInferenceRequest) {
	c.queueMu.Lock()
	defer c.queueMu.Unlock()

	c.queue = append(c.queue, req)
}

func filterLLMInferenceRequest(reqs []*types.RunnerLLMInferenceRequest, filter types.InferenceRequestFilter) ([]*types.RunnerLLMInferenceRequest, error) {
	var filteredReqs []*types.RunnerLLMInferenceRequest

	modelName := types.ModelName(filter.ModelName)

	model, err := model.GetModel(modelName)
	if err != nil {
		return nil, fmt.Errorf("error getting model: %w", err)
	}

	for _, req := range reqs {
		if filter.ModelName != "" && types.ModelName(req.Request.Model) != filter.ModelName {
			continue
		}

		if filter.Memory != 0 && model.GetMemoryRequirements(types.SessionModeInference) > filter.Memory {
			continue
		}

		if filter.Older != 0 && req.CreatedAt.After(time.Now().Add(-filter.Older)) {
			continue
		}

		filteredReqs = append(filteredReqs, req)
	}

	return filteredReqs, nil

}

// ProcessRunnerResponse is called on both partial streaming and full responses coming from the runner
func (c *InternalHelixClient) ProcessRunnerResponse(ctx context.Context, resp *types.RunnerLLMInferenceResponse) error {
	bts, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("error marshalling runner response: %w", err)
	}

	err = c.pubsub.Publish(ctx, pubsub.GetRunnerResponsesQueue(resp.UserID, resp.RequestID), bts)
	if err != nil {
		return fmt.Errorf("error publishing runner response: %w", err)
	}

	return nil
}

func (c *InternalHelixServer) addSchedulingDecision(filter types.SessionFilter, runnerID string, session *types.Session) {
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
