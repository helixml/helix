package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/inferencerouter"
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
	cfg             *config.ServerConfig
	pubsub          pubsub.PubSub // Used to get responses from the runners
	scheduler       *scheduler.Scheduler
	inferenceRouter *inferencerouter.Router // Sandbox-absorbs-runner pivot: replaces scheduler for routing
	store           store.Store
}

func NewInternalHelixServer(cfg *config.ServerConfig, store store.Store, pubsub pubsub.PubSub, scheduler *scheduler.Scheduler) *InternalHelixServer {
	return &InternalHelixServer{
		cfg:       cfg,
		store:     store,
		pubsub:    pubsub,
		scheduler: scheduler,
	}
}

// SetInferenceRouter wires the new sandbox-side router into the server.
// Called once during apiServer construction. Allowed to be nil — that
// disables the new path and keeps everything on the scheduler, which is
// the safe default during the transition.
func (c *InternalHelixServer) SetInferenceRouter(r *inferencerouter.Router) {
	c.inferenceRouter = r
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
	// Sandbox-absorbs-runner pivot: try the inference router first. If a
	// connected sandbox has the requested model in its active profile, we
	// forward via HTTP — that's the long-term path. Falls back to the
	// scheduler for models the new path doesn't yet know about; the
	// scheduler is removed in a follow-up PR once we've validated the
	// HTTP path against real hardware end-to-end.
	if c.inferenceRouter != nil && req.Request != nil && req.Request.Model != "" {
		if state, err := c.inferenceRouter.PickRunner(req.Request.Model); err == nil && state != nil {
			return c.dispatchHTTPToRunner(req, state.URL)
		}
	}

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

// dispatchHTTPToRunner forwards an inference request over HTTP to a
// sandbox's inference-proxy, then publishes the response back through the
// existing pubsub queue so callers (CreateChatCompletion / Stream /
// CreateEmbeddings) receive it via the same code path they already wait
// on. Returning nil here means "request accepted, response will arrive
// asynchronously via pubsub" — the same contract as the scheduler path.
//
// Streaming is handled by reading the SSE stream from the sandbox and
// publishing each chunk to the same pubsub queue with the same shape the
// runner used to publish.
func (c *InternalHelixServer) dispatchHTTPToRunner(req *types.RunnerLLMInferenceRequest, runnerURL string) error {
	if runnerURL == "" {
		return fmt.Errorf("dispatchHTTPToRunner: runner URL is empty")
	}
	// inference-proxy listens on port 8090 inside the sandbox by default.
	// runnerURL from refreshInferenceRouterFromHeartbeat is "http://<ip>"
	// (no port). Append.
	target := runnerURL + ":8090"

	// Pick the upstream path based on what the request carries.
	var path string
	var bodyStruct any
	switch {
	case req.Embeddings && req.FlexibleEmbeddingRequest != nil:
		path = "/v1/embeddings"
		bodyStruct = req.FlexibleEmbeddingRequest
	case req.Embeddings:
		path = "/v1/embeddings"
		bodyStruct = req.EmbeddingRequest
	default:
		path = "/v1/chat/completions"
		bodyStruct = req.Request
	}

	body, err := json.Marshal(bodyStruct)
	if err != nil {
		return fmt.Errorf("marshal request body: %w", err)
	}

	go c.dispatchAndPublish(req, target+path, body)
	return nil
}

// dispatchAndPublish does the actual HTTP roundtrip in a background
// goroutine and publishes the result(s) into the same pubsub queue the
// scheduler-based path uses. Errors are wrapped into a
// RunnerNatsReplyResponse with the Error field set so callers see them
// through their existing subscribe-and-wait code.
func (c *InternalHelixServer) dispatchAndPublish(req *types.RunnerLLMInferenceRequest, url string, body []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	publishErr := func(msg string) {
		log.Warn().Str("request_id", req.RequestID).Str("url", url).Str("err", msg).Msg("dispatch HTTP -> sandbox failed")
		reply, _ := json.Marshal(&types.RunnerNatsReplyResponse{Error: msg})
		_ = c.pubsub.Publish(ctx, pubsub.GetRunnerResponsesQueue(req.OwnerID, req.RequestID), reply)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		publishErr("build request: " + err.Error())
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(httpReq)
	if err != nil {
		publishErr("HTTP roundtrip: " + err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		publishErr(fmt.Sprintf("upstream %s: %s", resp.Status, strings.TrimSpace(string(respBody))))
		return
	}

	// Streaming responses use SSE. The non-streaming path returns one
	// JSON document. We detect SSE by Content-Type.
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/event-stream") {
		c.streamSSEToPubsub(ctx, req, resp.Body)
		return
	}

	// Non-streaming: read entire body, publish once.
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		publishErr("read response: " + err.Error())
		return
	}
	reply, _ := json.Marshal(&types.RunnerNatsReplyResponse{Response: respBody})
	if err := c.pubsub.Publish(ctx, pubsub.GetRunnerResponsesQueue(req.OwnerID, req.RequestID), reply); err != nil {
		log.Warn().Err(err).Str("request_id", req.RequestID).Msg("publish runner response")
	}
}

// streamSSEToPubsub reads SSE chunks from the sandbox and publishes each
// one as a separate RunnerNatsReplyResponse to the pubsub queue, matching
// the format the existing CreateChatCompletionStream handler expects
// ("data: {...}" lines).
func (c *InternalHelixServer) streamSSEToPubsub(ctx context.Context, req *types.RunnerLLMInferenceRequest, body io.Reader) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024) // 4 MiB max line — generous for big chunks
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line == "data: [DONE]" {
			continue
		}
		// Pass the raw SSE line through; downstream code expects
		// "data: {...}" and trims the prefix itself.
		reply, _ := json.Marshal(&types.RunnerNatsReplyResponse{Response: []byte(line)})
		if err := c.pubsub.Publish(ctx, pubsub.GetRunnerResponsesQueue(req.OwnerID, req.RequestID), reply); err != nil {
			log.Warn().Err(err).Str("request_id", req.RequestID).Msg("publish runner stream chunk")
			return
		}
	}
	if err := scanner.Err(); err != nil {
		log.Warn().Err(err).Str("request_id", req.RequestID).Msg("SSE scan error")
	}
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
