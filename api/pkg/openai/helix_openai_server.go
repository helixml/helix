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
)

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
