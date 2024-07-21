package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
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
	c.queueMu.Lock()
	defer c.queueMu.Unlock()

	filteredReqs, err := filterLLMInferenceRequest(c.queue, filter)
	if err != nil {
		return nil, fmt.Errorf("error filtering requests: %w", err)
	}

	if len(filteredReqs) == 0 {
		return nil, nil
	}

	req := filteredReqs[0]

	c.queue = append(c.queue[:0], c.queue[1:]...)

	c.addSchedulingDecision(filter, runnerID, runnerID, req.SessionID, req.InteractionID)

	return req, nil
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

	err = c.pubsub.Publish(ctx, pubsub.GetRunnerResponsesQueue(resp.OwnerID, resp.RequestID), bts)
	if err != nil {
		return fmt.Errorf("error publishing runner response: %w", err)
	}

	return nil
}

func (c *InternalHelixServer) addSchedulingDecision(filter types.InferenceRequestFilter, model, runnerID, sessionID, interactionID string) {

	decision := &types.GlobalSchedulingDecision{
		Created:       time.Now(),
		RunnerID:      runnerID,
		SessionID:     sessionID,
		InteractionID: interactionID,
		Filter: types.SessionFilter{
			Mode:  types.SessionModeInference,
			Older: types.Duration(filter.Older),
		},
		ModelName: types.ModelName(model),
		Mode:      types.SessionModeInference,
	}

	c.schedulingDecisions = append([]*types.GlobalSchedulingDecision{decision}, c.schedulingDecisions...)

	if len(c.schedulingDecisions) > schedulingDecisionHistorySize {
		c.schedulingDecisions = c.schedulingDecisions[:len(c.schedulingDecisions)-1]
	}
}
