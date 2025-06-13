package openai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

type HelixServer interface {
	// ProcessRunnerResponse is called by the HTTP handler when the runner sends a response over the websocket
	ProcessRunnerResponse(ctx context.Context, resp *types.RunnerLLMInferenceResponse) error
}

var _ HelixServer = &InternalHelixServer{}

// InternalHelixClient utilizes Helix runners to complete chat requests. Primary
// purpose is to power internal tools
type InternalHelixServer struct {
	cfg       *config.ServerConfig
	pubsub    pubsub.PubSub // Used to get responses from the runners
	scheduler *scheduler.Scheduler
	store     store.Store
}

func NewInternalHelixServer(cfg *config.ServerConfig, store store.Store, pubsub pubsub.PubSub, scheduler *scheduler.Scheduler) *InternalHelixServer {
	return &InternalHelixServer{
		cfg:       cfg,
		store:     store,
		pubsub:    pubsub,
		scheduler: scheduler,
	}
}

func (c *InternalHelixServer) ListModels(ctx context.Context) ([]types.OpenAIModel, error) {
	helixModels, err := c.store.ListModels(ctx, &store.ListModelsQuery{})
	if err != nil {
		return nil, fmt.Errorf("error listing models: %w", err)
	}
	var models []types.OpenAIModel
	for _, model := range helixModels {
		// Skip embedding models as they should not appear in chat model pickers
		if model.Type == types.ModelTypeEmbed {
			continue
		}

		models = append(models, types.OpenAIModel{
			ID:            model.ID,
			Object:        "model",
			OwnedBy:       "helix",
			Name:          model.Name,
			Description:   model.Description,
			Hide:          model.Hide,
			Type:          string(model.Type),
			ContextLength: int(model.ContextLength),
			Enabled:       model.Enabled,
		})
	}
	return models, nil
}

func (c *InternalHelixServer) APIKey() string {
	return ""
}

func (c *InternalHelixServer) enqueueRequest(req *types.RunnerLLMInferenceRequest) error {
	model, err := c.store.GetModel(context.Background(), req.Request.Model)
	if err != nil {
		return fmt.Errorf("error getting model: %w", err)
	}

	work, err := scheduler.NewLLMWorkload(req, model)
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
