package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

const schedulingDecisionHistorySize = 10

type HelixServer interface {
	// GetNextLLMInferenceRequest is called by the HTTP handler  to get the next LLM inference request to process for the runner
	GetNextLLMInferenceRequest(ctx context.Context, filter types.InferenceRequestFilter, runnerID string) (*types.RunnerLLMInferenceRequest, error)
	// ProcessRunnerResponse is called by the HTTP handler when the runner sends a response over the websocket
	ProcessRunnerResponse(ctx context.Context, resp *types.RunnerLLMInferenceResponse) error
	// GetSchedulingDecision returns the last scheduling decisions made by the server, used for the dashboar
	GetSchedulingDecision() []*types.GlobalSchedulingDecision
}

var _ HelixServer = &InternalHelixServer{}

// InternalHelixClient utilizes Helix runners to complete chat requests. Primary
// purpose is to power internal tools
type InternalHelixServer struct {
	cfg *config.ServerConfig

	pubsub pubsub.PubSub // Used to get responses from the runners
	// controller Controller    // Used to create sessions

	queueMu sync.Mutex
	queue   []*types.RunnerLLMInferenceRequest

	schedulingDecisionsMu sync.Mutex
	schedulingDecisions   []*types.GlobalSchedulingDecision
	scheduler             scheduler.Scheduler
}

func NewInternalHelixServer(cfg *config.ServerConfig, pubsub pubsub.PubSub, scheduler scheduler.Scheduler) *InternalHelixServer {
	return &InternalHelixServer{
		cfg:       cfg,
		pubsub:    pubsub,
		scheduler: scheduler,
	}
}

// TODO: move logic from controller and other places. This method would be called directly from the runner
// handler to get the next session. Pubsub is handled internally within this package
func (c *InternalHelixServer) GetNextLLMInferenceRequest(ctx context.Context, filter types.InferenceRequestFilter, runnerID string) (*types.RunnerLLMInferenceRequest, error) {
	c.queueMu.Lock()
	defer c.queueMu.Unlock()

	// Doing all the scheduling work here to avoid making too many changes at once. Schedule any
	// requests that are currently in the queue.
	taken := 0
	for _, req := range c.queue {
		work, err := scheduler.NewLLMWorkload(req)
		if err != nil {
			log.Warn().Err(err).Str("id", req.RequestID).Msg("creating workload")
			continue
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
			// TODO(Phil): Not sure how to write an error back as a response
			log.Error().Err(err).Str("id", work.ID()).Msg("error scheduling")
		}
		taken++
	}
	// Clear processed queue
	c.queue = c.queue[taken:]

	// Default to requesting warm work
	newWorkOnly := false

	// Only get new work if the filter has a memory requirement (see runner/controller.go)
	if filter.Memory != 0 {
		newWorkOnly = true
	}

	// Now for this runner, get work
	req, err := c.scheduler.WorkForRunner(runnerID, scheduler.WorkloadTypeLLMInferenceRequest, newWorkOnly)
	if err != nil {
		return nil, fmt.Errorf("error getting work for runner: %w", err)
	}

	if req != nil {
		c.addSchedulingDecision(filter, runnerID, runnerID, req.LLMInferenceRequest().SessionID, req.LLMInferenceRequest().InteractionID)
		log.Info().Str("runnerID", runnerID).Interface("filter", filter).Interface("req", req).Int("len(queue)", len(c.queue)).Msgf("🟠 helix_openai_server GetNextLLMInferenceRequest END")
		return req.LLMInferenceRequest(), nil
	}
	return nil, nil

}

func (c *InternalHelixServer) enqueueRequest(req *types.RunnerLLMInferenceRequest) {
	c.queueMu.Lock()
	defer c.queueMu.Unlock()

	c.queue = append(c.queue, req)
}

// ProcessRunnerResponse is called on both partial streaming and full responses coming from the runner
func (c *InternalHelixServer) ProcessRunnerResponse(ctx context.Context, resp *types.RunnerLLMInferenceResponse) error {
	if resp.Done || resp.Error != "" {
		err := c.scheduler.Release(
			resp.RequestID,
		)
		if err != nil {
			return fmt.Errorf("error releasing allocation: %w", err)
		}
	}
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

func (c *InternalHelixServer) GetSchedulingDecision() []*types.GlobalSchedulingDecision {
	c.schedulingDecisionsMu.Lock()
	defer c.schedulingDecisionsMu.Unlock()

	// Copy scheduling decisions
	queue := make([]*types.GlobalSchedulingDecision, len(c.schedulingDecisions))
	copy(queue, c.schedulingDecisions)

	return queue
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
