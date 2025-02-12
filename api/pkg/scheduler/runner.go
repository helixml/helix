package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/nats-io/nats.go"
	"github.com/puzpuzpuz/xsync/v3"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

const (
	submitChatCompletionRequestTimeout = 10 * time.Second
	defaultRequestTimeout              = 10 * time.Second
	cacheUpdateInterval                = 5 * time.Second
)

type RunnerController struct {
	runners     []string
	mu          *sync.RWMutex
	ps          pubsub.PubSub
	ctx         context.Context
	fs          filestore.FileStore
	slotsCache  *xsync.MapOf[string, *Cache[types.ListRunnerSlotsResponse]]
	statusCache *xsync.MapOf[string, *Cache[types.RunnerStatus]]
}

type RunnerControllerConfig struct {
	PubSub pubsub.PubSub
	FS     filestore.FileStore
}

func NewRunnerController(ctx context.Context, cfg *RunnerControllerConfig) (*RunnerController, error) {
	controller := &RunnerController{
		ctx:         ctx,
		ps:          cfg.PubSub,
		fs:          cfg.FS,
		runners:     []string{},
		mu:          &sync.RWMutex{},
		slotsCache:  xsync.NewMapOf[string, *Cache[types.ListRunnerSlotsResponse]](),
		statusCache: xsync.NewMapOf[string, *Cache[types.RunnerStatus]](),
	}

	sub, err := cfg.PubSub.SubscribeWithCtx(controller.ctx, pubsub.GetRunnerConnectedQueue("*"), func(_ context.Context, msg *nats.Msg) error {
		log.Info().Str("subject", msg.Subject).Str("data", string(msg.Data)).Msg("runner connected")
		runnerID, err := pubsub.ParseRunnerID(msg.Subject)
		if err != nil {
			log.Error().Err(err).Str("subject", msg.Subject).Msg("error parsing runner ID")
			return err
		}
		controller.OnConnectedHandler(runnerID)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error subscribing to runner.connected.*: %w", err)
	}
	go func() {
		<-ctx.Done()
		err := sub.Unsubscribe()
		if err != nil {
			log.Error().Err(err).Msg("error unsubscribing from runner.connected.*")
		}
	}()

	go controller.reconcileCaches(ctx)

	return controller, nil
}

func (c *RunnerController) reconcileCaches(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
			// Clean up slots cache
			c.slotsCache.Range(func(runnerID string, cache *Cache[types.ListRunnerSlotsResponse]) bool {
				if !slices.Contains(c.runners, runnerID) {
					log.Trace().Str("runner_id", runnerID).Msg("closing slots cache")
					cache.Close()
					c.slotsCache.Delete(runnerID)
				}
				return true
			})

			// Clean up status cache
			c.statusCache.Range(func(runnerID string, cache *Cache[types.RunnerStatus]) bool {
				if !slices.Contains(c.runners, runnerID) {
					log.Trace().Str("runner_id", runnerID).Msg("closing status cache")
					cache.Close()
					c.statusCache.Delete(runnerID)
				}
				return true
			})
		}
	}
}

func (c *RunnerController) Send(ctx context.Context, runnerID string, headers map[string]string, req *types.Request, timeout time.Duration) (*types.Response, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("error marshalling request: %w", err)
	}

	// Publish the task to the "tasks" subject
	start := time.Now()
	response, err := c.ps.Request(ctx, pubsub.GetRunnerQueue(runnerID), headers, data, timeout)
	if err != nil {
		return nil, fmt.Errorf("error sending request to runner: %w", err)
	}
	duration := time.Since(start)
	log.Trace().Str("runner_id", runnerID).Str("method", req.Method).Str("url", req.URL).Str("duration", duration.String()).Msg("request sent to runner")

	var resp types.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		return nil, fmt.Errorf("error unmarshalling response: %w", err)
	}

	return &resp, nil
}

func (c *RunnerController) OnConnectedHandler(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Add the runner to the cluster if it is not already in the cluster.
	if !slices.Contains(c.runners, id) {
		c.runners = append(c.runners, id)
	}
}

func (c *RunnerController) deleteRunner(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var newRunners []string
	for _, runner := range c.runners {
		if runner != id {
			newRunners = append(newRunners, runner)
		}
	}
	c.runners = newRunners
}

func (c *RunnerController) RunnerIDs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.runners
}

func (c *RunnerController) TotalMemory(runnerID string) uint64 {
	status, err := c.GetStatus(runnerID)
	if err != nil {
		log.Error().Err(err).Msg("error getting runner status")
		return 0
	}
	return status.TotalMemory
}

func (c *RunnerController) FreeMemory(runnerID string) uint64 {
	status, err := c.GetStatus(runnerID)
	if err != nil {
		log.Error().Err(err).Msg("error getting runner status")
		return 0
	}
	return status.FreeMemory
}

func (c *RunnerController) Version(runnerID string) string {
	status, err := c.GetStatus(runnerID)
	if err != nil {
		log.Error().Err(err).Msg("error getting runner status")
		return ""
	}
	return status.Version
}

func (c *RunnerController) GetSlots(runnerID string) ([]*types.RunnerSlot, error) {
	// Get the slots from the runner.
	slots, err := c.getSlots(runnerID)
	if err != nil {
		return nil, err
	}
	return slots.Slots, nil
}

func (c *RunnerController) SubmitChatCompletionRequest(slot *Slot, req *types.RunnerLLMInferenceRequest) error {
	headers := map[string]string{}
	headers[pubsub.HelixNatsReplyHeader] = pubsub.GetRunnerResponsesQueue(req.OwnerID, req.RequestID)

	chatRequestBytes, err := json.Marshal(req.Request)
	if err != nil {
		return err
	}
	natsReq := types.RunnerNatsReplyRequest{
		RequestID:     req.RequestID,
		CreatedAt:     time.Now(),
		OwnerID:       req.OwnerID,
		SessionID:     req.SessionID,
		InteractionID: req.InteractionID,
		Request:       chatRequestBytes,
	}

	body, err := json.Marshal(natsReq)
	if err != nil {
		return err
	}
	resp, err := c.Send(c.ctx, slot.RunnerID, headers, &types.Request{
		Method: "POST",
		URL:    fmt.Sprintf("/api/v1/slots/%s/v1/chat/completions", slot.ID),
		Body:   body,
	}, submitChatCompletionRequestTimeout)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error submitting chat completion request: %s", resp.Body)
	}

	return nil
}

func (c *RunnerController) SubmitImageGenerationRequest(slot *Slot, session *types.Session) error {
	lastInteraction, err := data.GetLastInteraction(session)
	if err != nil {
		return fmt.Errorf("no last interaction found: %w", err)
	}

	userInteractions := data.FilterUserInteractions(session.Interactions)
	if len(userInteractions) == 0 {
		return fmt.Errorf("no user interaction found")
	}

	// Note that there's no system prompt in the openai api. So I'm just going to merge the previous
	// user interactions into a single prompt. Fingers crossed there's no limits.
	// Merge the user interactions into a single prompt
	prompt := strings.Builder{}
	for _, interaction := range userInteractions[:len(userInteractions)-1] {
		prompt.WriteString(interaction.Message)
		prompt.WriteString("\n")
	}
	prompt.WriteString(userInteractions[len(userInteractions)-1].Message)

	// Convert the session to a valid image generation request
	imageRequest := openai.ImageRequest{
		Prompt: prompt.String(),
		Model:  session.ModelName,
		N:      1,
		User:   session.Owner,
	}
	requestBytes, err := json.Marshal(imageRequest)
	if err != nil {
		return err
	}
	req := &types.RunnerNatsReplyRequest{
		RequestID:     lastInteraction.ID, // Use the last interaction ID as the request ID for sessions, it's important that this kept in sync with the receiver code
		CreatedAt:     time.Now(),
		OwnerID:       session.Owner,
		SessionID:     session.ID,
		InteractionID: lastInteraction.ID,
		Request:       requestBytes,
	}

	headers := map[string]string{}
	headers[pubsub.HelixNatsReplyHeader] = pubsub.GetRunnerResponsesQueue(req.OwnerID, req.RequestID)

	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	resp, err := c.Send(c.ctx, slot.RunnerID, headers, &types.Request{
		Method: "POST",
		URL:    fmt.Sprintf("/api/v1/slots/%s/v1/helix/images/generations", slot.ID),
		Body:   body,
	}, 30*time.Second)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error submitting image generation request: %s", resp.Body)
	}

	return nil
}

func (c *RunnerController) CreateSlot(slot *Slot) error {
	log.Debug().
		Str("runner_id", slot.RunnerID).
		Str("slot_id", slot.ID.String()).
		Str("model", slot.ModelName().String()).
		Str("runtime", string(slot.Runtime())).
		Msg("creating slot")
	req := &types.CreateRunnerSlotRequest{
		ID: slot.ID,
		Attributes: types.CreateRunnerSlotAttributes{
			Runtime: slot.Runtime(),
			Model:   slot.ModelName().String(),
		},
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	resp, err := c.Send(c.ctx, slot.RunnerID, nil, &types.Request{
		Method: "POST",
		URL:    "/api/v1/slots",
		Body:   body,
	}, 1*time.Minute)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("error creating slot: %s", resp.Body)
	}
	return nil
}

func (c *RunnerController) DeleteSlot(runnerID string, slotID uuid.UUID) error {
	log.Debug().
		Str("runner_id", runnerID).
		Str("slot_id", slotID.String()).
		Msg("deleting slot")
	resp, err := c.Send(c.ctx, runnerID, nil, &types.Request{
		Method: "DELETE",
		URL:    fmt.Sprintf("/api/v1/slots/%s", slotID.String()),
	}, defaultRequestTimeout)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("error deleting slot: %s", resp.Body)
	}
	return nil
}

func (c *RunnerController) GetStatus(runnerID string) (*types.RunnerStatus, error) {
	cache, loaded := c.statusCache.LoadOrStore(runnerID, NewCache(
		c.ctx,
		func() (types.RunnerStatus, error) {
			return c.fetchStatus(runnerID)
		},
		CacheConfig{
			updateInterval: cacheUpdateInterval,
		},
	))
	if !loaded {
		log.Trace().Str("runner_id", runnerID).Msg("created new status cache")
	}

	status, err := cache.Get()
	if err != nil {
		return nil, err
	}
	return &status, nil
}

func (c *RunnerController) GetHealthz(runnerID string) error {
	resp, err := c.Send(c.ctx, runnerID, nil, &types.Request{
		Method: "GET",
		URL:    "/api/v1/healthz",
	}, defaultRequestTimeout)
	if err != nil {
		return fmt.Errorf("error getting healthz: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("runner %s is not healthy", runnerID)
	}
	return nil
}

func (c *RunnerController) SubmitFinetuningRequest(slot *Slot, session *types.Session) error {
	log.Info().Str("session_id", session.ID).Msg("processing fine-tuning interaction")

	lastInteraction, err := data.GetLastInteraction(session)
	if err != nil {
		return fmt.Errorf("error getting last interaction: %w", err)
	}
	headers := map[string]string{}
	headers[pubsub.HelixNatsReplyHeader] = pubsub.GetRunnerResponsesQueue(session.Owner, lastInteraction.ID)

	// TODO(Phil): the old code had some complicated logic around merging multiple jsonl files.
	// I'll just use the jsonl files from the last interaction for now.

	// and append them to one large JSONL file
	userInteractions := data.FilterUserInteractions(session.Interactions)
	finetuneInteractions := data.FilterFinetuneInteractions(userInteractions)
	if len(finetuneInteractions) == 0 {
		return fmt.Errorf("no finetune interactions found")
	}
	lastFinetuneInteraction := finetuneInteractions[len(finetuneInteractions)-1]
	if len(lastFinetuneInteraction.Files) != 1 {
		return fmt.Errorf("last interaction should have exactly one file")
	}
	combinedFile := lastFinetuneInteraction.Files[0]

	// Check that combined file size is not zero
	fi, err := c.fs.Get(c.ctx, combinedFile)
	if err != nil {
		return fmt.Errorf("error checking jsonl file: %w", err)
	}
	if fi.Size <= 1 {
		// Check for 1 byte to account for just a newline character
		return fmt.Errorf("training data file is empty")
	}
	log.Debug().Str("session_id", session.ID).Int64("file_size", fi.Size).Msgf("combined file size")

	req := openai.FineTuningJobRequest{
		Model:          session.ModelName,
		TrainingFile:   combinedFile,
		ValidationFile: "",
		Hyperparameters: &openai.Hyperparameters{
			Epochs:                 1,
			LearningRateMultiplier: 0.0002,
			BatchSize:              6,
		},
		Suffix: session.ID, // Use the suffix to identify the session and the final directory for the LORA
	}

	requestBytes, err := json.Marshal(req)
	if err != nil {
		return err
	}
	natsReq := &types.RunnerNatsReplyRequest{
		RequestID:     lastInteraction.ID, // Use the last interaction ID as the request ID for sessions, it's important that this kept in sync with the receiver code
		CreatedAt:     time.Now(),
		OwnerID:       session.Owner,
		SessionID:     session.ID,
		InteractionID: lastInteraction.ID,
		Request:       requestBytes,
	}
	body, err := json.Marshal(natsReq)
	if err != nil {
		return err
	}
	resp, err := c.Send(c.ctx, slot.RunnerID, headers, &types.Request{
		Method: "POST",
		URL:    fmt.Sprintf("/api/v1/slots/%s/v1/helix/fine_tuning/jobs", slot.ID),
		Body:   body,
	}, 10*time.Minute)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error submitting finetuning request: %s", resp.Body)
	}

	return nil
}

func (c *RunnerController) fetchSlot(runnerID string, slotID uuid.UUID) (types.RunnerSlot, error) {
	resp, err := c.Send(c.ctx, runnerID, nil, &types.Request{
		Method: "GET",
		URL:    fmt.Sprintf("/api/v1/slots/%s", slotID.String()),
	}, defaultRequestTimeout)
	if err != nil {
		return types.RunnerSlot{}, fmt.Errorf("error getting slot: %w", err)
	}

	var slot types.RunnerSlot
	if err := json.Unmarshal(resp.Body, &slot); err != nil {
		return types.RunnerSlot{}, fmt.Errorf("error unmarshalling slot: %w", err)
	}
	return slot, nil
}

func (c *RunnerController) fetchStatus(runnerID string) (types.RunnerStatus, error) {
	resp, err := c.Send(c.ctx, runnerID, nil, &types.Request{
		Method: "GET",
		URL:    "/api/v1/status",
	}, defaultRequestTimeout)
	if err != nil {
		return types.RunnerStatus{}, err
	}

	var status types.RunnerStatus
	if err := json.Unmarshal(resp.Body, &status); err != nil {
		return types.RunnerStatus{}, err
	}
	return status, nil
}

func (c *RunnerController) getSlots(runnerID string) (*types.ListRunnerSlotsResponse, error) {
	cache, loaded := c.slotsCache.LoadOrStore(runnerID, NewCache(
		c.ctx,
		func() (types.ListRunnerSlotsResponse, error) {
			return c.fetchSlots(runnerID)
		},
		CacheConfig{
			updateInterval: cacheUpdateInterval,
		},
	))
	if !loaded {
		log.Trace().Str("runner_id", runnerID).Msg("created new slots cache")
	}

	slots, err := cache.Get()
	if err != nil {
		return nil, err
	}
	return &slots, nil
}

func (c *RunnerController) fetchSlots(runnerID string) (types.ListRunnerSlotsResponse, error) {
	resp, err := c.Send(c.ctx, runnerID, nil, &types.Request{
		Method: "GET",
		URL:    "/api/v1/slots",
	}, defaultRequestTimeout)
	if err != nil {
		return types.ListRunnerSlotsResponse{}, err
	}
	var slots types.ListRunnerSlotsResponse
	if err := json.Unmarshal(resp.Body, &slots); err != nil {
		return types.ListRunnerSlotsResponse{}, err
	}
	return slots, nil
}
