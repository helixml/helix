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
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
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

func (c *InternalHelixServer) ListModels(ctx context.Context) ([]model.OpenAIModel, error) {
	return ListModels(ctx)
}

func (c *InternalHelixServer) APIKey() string {
	return ""
}

func (c *InternalHelixServer) CreateEmbeddings(_ context.Context, _ openai.EmbeddingRequest) (resp openai.EmbeddingResponse, err error) {
	// TODO: implement once we support pass through
	return openai.EmbeddingResponse{}, fmt.Errorf("not implemented")
}

// TODO: move logic from controller and other places. This method would be called directly from the runner
// handler to get the next session. Pubsub is handled internally within this package
func (c *InternalHelixServer) GetNextLLMInferenceRequest(_ context.Context, filter types.InferenceRequestFilter, runnerID string) (*types.RunnerLLMInferenceRequest, error) {
	// Track beginning of request
	runnerReqID := system.GenerateID()

	// Default to requesting warm work
	newWorkOnly := false

	// Only get new work if the filter has a memory requirement (see runner/controller.go)
	if filter.Memory != 0 {
		newWorkOnly = true
	}

	// Now for this runner, get work
	req, err := c.scheduler.WorkForRunner(runnerID, scheduler.WorkloadTypeLLMInferenceRequest, newWorkOnly, filter.ModelName)
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", runnerID).
			Str("runner_request_id", runnerReqID).
			Any("filter", filter).
			Bool("new_work_only", newWorkOnly).
			Msg("error getting work for runner")
		return nil, fmt.Errorf("error getting work for runner: %w", err)
	}

	if req != nil {
		c.addSchedulingDecision(filter, runnerID, runnerID, req.LLMInferenceRequest().SessionID, req.LLMInferenceRequest().InteractionID)
		log.Info().
			Str("runner_id", runnerID).
			Str("runner_request_id", runnerReqID).
			Any("filter", filter).
			Any("req", req).
			Bool("new_work_only", newWorkOnly).
			Msg("returning llm inference request")
		return req.LLMInferenceRequest(), nil
	}
	return nil, nil

}

func (c *InternalHelixServer) enqueueRequest(req *types.RunnerLLMInferenceRequest) error {
	work, err := scheduler.NewLLMWorkload(req)
	if err != nil {
		return fmt.Errorf("error creating workload: %w", err)
	}
	err = c.scheduler.Enqueue(work)
	if err != nil {
		return fmt.Errorf("error enqueuing work: %w", err)
	}
	return nil
}

// ProcessRunnerResponse is called on both partial streaming and full responses coming from the runner
func (c *InternalHelixServer) ProcessRunnerResponse(ctx context.Context, resp *types.RunnerLLMInferenceResponse) error {
	err := c.scheduler.Begin(resp.RequestID)
	if err != nil {
		log.Warn().Err(err).Str("request_id", resp.RequestID).Msg("error beginning allocation, continuing...")
	}
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
		ModelName: model,
		Mode:      types.SessionModeInference,
	}

	c.schedulingDecisions = append([]*types.GlobalSchedulingDecision{decision}, c.schedulingDecisions...)

	if len(c.schedulingDecisions) > schedulingDecisionHistorySize {
		c.schedulingDecisions = c.schedulingDecisions[:len(c.schedulingDecisions)-1]
	}
}
