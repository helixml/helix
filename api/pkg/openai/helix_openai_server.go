package openai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/types"
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
	cfg       *config.ServerConfig
	pubsub    pubsub.PubSub // Used to get responses from the runners
	scheduler *scheduler.Scheduler
}

func NewInternalHelixServer(cfg *config.ServerConfig, pubsub pubsub.PubSub, scheduler *scheduler.Scheduler) *InternalHelixServer {
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
