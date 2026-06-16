package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/inferencerouter"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// RevDialer opens a connection to a sandbox over its outbound RevDial tunnel,
// keyed by device id. Implemented by connman.ConnectionManager. Kept as a
// narrow interface here to avoid importing connman (and to keep this package
// testable). The sandbox is never addressed by network host - only by id -
// so dispatch works regardless of where the sandbox runs (NAT, other network).
type RevDialer interface {
	Dial(ctx context.Context, key string) (net.Conn, error)
}

type HelixServer interface {
	// ProcessRunnerResponse is called by the HTTP handler when the runner sends a response over the websocket
	ProcessRunnerResponse(ctx context.Context, resp *types.RunnerLLMInferenceResponse) error
}

var _ HelixServer = &InternalHelixServer{}

// InternalHelixClient utilizes Helix runners to complete chat requests. Primary
// purpose is to power internal tools
type InternalHelixServer struct {
	cfg             *config.ServerConfig
	pubsub          pubsub.PubSub           // Used to get responses from the runners
	inferenceRouter *inferencerouter.Router // Sandbox-absorbs-runner pivot: replaces scheduler for routing
	dialer          RevDialer               // dispatches inference to a sandbox over its RevDial tunnel
	store           store.Store
}

// NewInternalHelixServer constructs the in-process OpenAI-compatible
// "helix" provider. Scheduler argument removed in the sandbox-absorbs-
// runner pivot; the inference router replaces it (set via
// SetInferenceRouter post-construction so the apiServer literal can wire
// fields without ordering pain).
func NewInternalHelixServer(cfg *config.ServerConfig, store store.Store, pubsub pubsub.PubSub) *InternalHelixServer {
	return &InternalHelixServer{
		cfg:    cfg,
		store:  store,
		pubsub: pubsub,
	}
}

// SetInferenceRouter wires the new sandbox-side router into the server.
// Called once during apiServer construction. Allowed to be nil — that
// disables the new path and keeps everything on the scheduler, which is
// the safe default during the transition.
func (c *InternalHelixServer) SetInferenceRouter(r *inferencerouter.Router) {
	c.inferenceRouter = r
}

// SetDialer wires the RevDial connection manager used to reach sandboxes.
// Called once during apiServer construction.
func (c *InternalHelixServer) SetDialer(d RevDialer) {
	c.dialer = d
}

func (c *InternalHelixServer) ListModels(ctx context.Context) ([]types.OpenAIModel, error) {
	helixModels, err := c.store.ListModels(ctx, &store.ListModelsQuery{})
	if err != nil {
		return nil, fmt.Errorf("error listing models: %w", err)
	}

	// Filter to only models a connected runner can actually serve right now.
	// Pre-pivot we kept VLLM rows in the picker even when no runner had them
	// loaded, on the assumption the scheduler would pull-and-start on demand.
	// Post-sandbox-absorbs-runner that's no longer true: a VLLM model only
	// runs if it's in an active Runner Profile on a connected runner. Showing
	// it in the picker otherwise just leads to "model X is not available"
	// errors when the user picks it (NoRunnerError in inferencerouter).
	availableModels := c.getAvailableModelsFromRunners()

	var models []types.OpenAIModel
	served := make(map[string]bool) // model IDs already in the response
	for _, model := range helixModels {
		// Skip embedding models as they should not appear in chat model pickers
		if model.Type == types.ModelTypeEmbed {
			continue
		}

		if !availableModels[model.ID] {
			log.Debug().
				Str("model_id", model.ID).
				Str("runtime", string(model.Runtime)).
				Msg("Filtering out model not available on any runner")
			continue
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
		served[model.ID] = true
	}

	// Sandbox-absorbs-runner pivot: a model served by a running profile is
	// first-class even when it has no row in the `models` table. The profile's
	// compose YAML is the source of truth for these, so they only exist in the
	// inference router. Union them in so the OpenAI surface (and the model
	// validation in controller.assertProviderServesModel) can see them.
	for id := range availableModels {
		if served[id] {
			continue
		}
		models = append(models, types.OpenAIModel{
			ID:      id,
			Object:  "model",
			OwnedBy: "helix",
			Name:    id,
			Type:    string(types.ModelTypeChat),
			Enabled: true,
		})
		served[id] = true
	}
	return models, nil
}

// getAvailableModelsFromRunners returns a map of model IDs that are
// currently being served by a connected sandbox's active profile. Used
// by ListModels to filter the registered Helix models to those backed
// by a running profile.
func (c *InternalHelixServer) getAvailableModelsFromRunners() map[string]bool {
	availableModels := make(map[string]bool)
	if c.inferenceRouter == nil {
		return availableModels
	}
	for _, m := range c.inferenceRouter.AvailableModels() {
		availableModels[m] = true
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
	// Sandbox-absorbs-runner pivot: the inference router is the only path.
	// Pick a sandbox that has the requested model in its currently-running
	// profile and forward via HTTP. If no sandbox can serve it, return
	// NoRunnerError (which carries the available-models list for a useful
	// 503 to the caller). No scheduler fallback — see AC8 / Decision 11.
	if c.inferenceRouter == nil {
		return fmt.Errorf("inference router not initialised")
	}
	if req.Request == nil || req.Request.Model == "" {
		return fmt.Errorf("inference request missing model name")
	}
	state, err := c.inferenceRouter.PickRunner(req.Request.Model)
	if err != nil {
		return err
	}
	return c.dispatchToSandbox(req, state.ID)
}

// dispatchToSandbox forwards an inference request to a sandbox's inference-proxy
// over the sandbox's outbound RevDial tunnel (keyed by sandbox id), then
// publishes the response back through the existing pubsub queue so callers
// (CreateChatCompletion / Stream / CreateEmbeddings) receive it via the same
// code path they already wait on. Returning nil here means "request accepted,
// response will arrive asynchronously via pubsub" — the same contract as the
// scheduler path.
//
// The sandbox is addressed only by id, never by network host: the GPU sandbox
// may be on a different network / behind NAT and reachable only via the tunnel
// it dialed out to us. hydra (in the sandbox) proxies /api/v1/inference/* to
// the local inference-proxy.
//
// Streaming is handled by reading the SSE stream from the sandbox and
// publishing each chunk to the same pubsub queue with the same shape the
// runner used to publish.
func (c *InternalHelixServer) dispatchToSandbox(req *types.RunnerLLMInferenceRequest, sandboxID string) error {
	if c.dialer == nil {
		return fmt.Errorf("dispatchToSandbox: RevDial dialer not initialised")
	}
	if sandboxID == "" {
		return fmt.Errorf("dispatchToSandbox: sandbox id is empty")
	}

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

	go c.dispatchAndPublish(req, sandboxID, path, body)
	return nil
}

// dispatchAndPublish does the actual HTTP roundtrip in a background
// goroutine and publishes the result(s) into the same pubsub queue the
// scheduler-based path uses. Errors are wrapped into a
// RunnerNatsReplyResponse with the Error field set so callers see them
// through their existing subscribe-and-wait code.
func (c *InternalHelixServer) dispatchAndPublish(req *types.RunnerLLMInferenceRequest, sandboxID, path string, body []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	publishErr := func(msg string) {
		log.Warn().Str("request_id", req.RequestID).Str("sandbox_id", sandboxID).Str("err", msg).Msg("dispatch inference -> sandbox failed")
		reply, _ := json.Marshal(&types.RunnerNatsReplyResponse{Error: msg})
		// Use a fresh context: the dispatch ctx may already be cancelled/expired
		// (that is often *why* we are publishing an error), and the caller must
		// still receive the failure rather than hang until its own timeout.
		pubCtx, pubCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer pubCancel()
		_ = c.pubsub.Publish(pubCtx, pubsub.GetRunnerResponsesQueue(req.OwnerID, req.RequestID), reply)
	}

	// Open a connection back to the sandbox over its RevDial tunnel. The
	// hydra HTTP server in the sandbox serves /api/v1/inference/* and proxies
	// to the local inference-proxy. We speak HTTP/1.1 directly over the conn
	// (no http.Client) so the response body streams as it arrives — required
	// for SSE chat completions.
	conn, err := c.dialer.Dial(ctx, "hydra-"+sandboxID)
	if err != nil {
		publishErr("dial sandbox via RevDial: " + err.Error())
		return
	}
	defer conn.Close()
	// A raw RevDial conn does not honor ctx once dialed, so a sandbox that never
	// finishes the response (hung upstream, half-open tunnel) would block the
	// reads below forever and leak this goroutine + the connection. Close the
	// conn when the 5-minute budget expires so the reads unblock — this restores
	// the bound the previous http.Client{Timeout} gave us.
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://hydra/api/v1/inference"+path, bytes.NewReader(body))
	if err != nil {
		publishErr("build request: " + err.Error())
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	if err := httpReq.Write(conn); err != nil {
		publishErr("write request to tunnel: " + err.Error())
		return
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), httpReq)
	if err != nil {
		publishErr("read response from tunnel: " + err.Error())
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
