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
	"github.com/rs/zerolog/log"
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

	// Get available models from runners to filter dead models
	availableModels := c.getAvailableModelsFromRunners()

	var models []types.OpenAIModel
	for _, model := range helixModels {
		// Skip embedding models as they should not appear in chat model pickers
		if model.Type == types.ModelTypeEmbed {
			continue
		}

		// Only include models that are actually available on connected runners
		// For VLLM models, we are more permissive since they're started dynamically
		isAvailable := availableModels[model.ID]
		if !isAvailable && model.Runtime != types.RuntimeVLLM {
			log.Debug().
				Str("model_id", model.ID).
				Str("runtime", string(model.Runtime)).
				Msg("Filtering out model not available on any runner")
			continue
		}

		// For VLLM models, log if they're not available (for debugging) but still include them
		if !isAvailable && model.Runtime == types.RuntimeVLLM {
			log.Debug().
				Str("model_id", model.ID).
				Str("runtime", string(model.Runtime)).
				Msg("VLLM model not currently reported as available, but including anyway (will be started dynamically)")
		}

		openAIModel := types.OpenAIModel{
			ID:            model.ID,
			Object:        "model",
			OwnedBy:       "helix",
			Name:          model.Name,
			Description:   model.Description,
			Hide:          model.Hide,
			Type:          string(model.Type),
			ContextLength: int(model.ContextLength),
			Enabled:       model.Enabled,
		}

		log.Debug().
			Str("model_id", model.ID).
			Str("model_name", model.Name).
			Str("database_type", string(model.Type)).
			Str("api_type", openAIModel.Type).
			Str("runtime", string(model.Runtime)).
			Bool("enabled", model.Enabled).
			Bool("hide", model.Hide).
			Msg("Serving model to API")

		models = append(models, openAIModel)
	}
	return models, nil
}

// getAvailableModelsFromRunners returns a map of model IDs that are actually available on connected runners
func (c *InternalHelixServer) getAvailableModelsFromRunners() map[string]bool {
	availableModels := make(map[string]bool)

	// Get all runner statuses from the scheduler
	runnerStatuses, err := c.scheduler.RunnerStatus()
	if err != nil {
		// If we can't get runner status, return empty map (no models available)
		// This is safer than returning all models when we can't verify availability
		return availableModels
	}

	// Process each runner's model status
	for _, status := range runnerStatuses {
		// Add models that are available (not downloading and no error)
		for _, modelStatus := range status.Models {
			if !modelStatus.DownloadInProgress && modelStatus.Error == "" {
				availableModels[modelStatus.ModelID] = true
			}
		}
	}

	return availableModels
}

func (c *InternalHelixServer) APIKey() string {
	return ""
}

func (c *InternalHelixServer) BaseURL() string {
	return ""
}

func (c *InternalHelixServer) BillingEnabled() bool {
	return c.cfg.Stripe.BillingEnabled
}

func (c *InternalHelixServer) enqueueRequest(req *types.RunnerLLMInferenceRequest) error {
	// External agents don't use traditional LLM models - they launch containers instead
	// So we skip model lookup for external_agent model name
	var model *types.Model
	if req.Request.Model == "external_agent" {
		// Create a dummy model for external agents - not actually used for inference
		model = &types.Model{
			ID:   "external_agent",
			Name: "External Agent",
			Type: types.ModelTypeChat,
		}
	} else {
		// Normal model lookup for traditional LLM requests
		var err error
		model, err = c.store.GetModel(context.Background(), req.Request.Model)
		if err != nil {
			return fmt.Errorf("model '%s' not found in helix provider (local scheduler) - check if this model exists in your configured models or if you meant to route to a different provider: %w", req.Request.Model, err)
		}
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
