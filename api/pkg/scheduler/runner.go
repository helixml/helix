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
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

type RunnerController struct {
	runners []string
	mu      *sync.RWMutex
	ps      pubsub.PubSub
	ctx     context.Context
	fs      filestore.FileStore
}

type RunnerControllerConfig struct {
	PubSub pubsub.PubSub
	FS     filestore.FileStore
}

func NewRunnerController(ctx context.Context, cfg *RunnerControllerConfig) (*RunnerController, error) {
	controller := &RunnerController{
		ctx:     ctx,
		ps:      cfg.PubSub,
		fs:      cfg.FS,
		runners: []string{},
		mu:      &sync.RWMutex{},
	}

	sub, err := cfg.PubSub.SubscribeWithCtx(controller.ctx, pubsub.GetRunnerConnectedQueue("*"), func(ctx context.Context, msg *nats.Msg) error {
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
		sub.Unsubscribe()
	}()

	return controller, nil
}

func (r *RunnerController) Send(ctx context.Context, runnerId string, headers map[string]string, req *types.Request) (*types.Response, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("error marshalling request: %w", err)
	}

	// Publish the task to the "tasks" subject
	response, err := r.ps.Request(ctx, pubsub.GetRunnerQueue(runnerId), headers, data, 5*time.Minute) // TODO(phil): some requests are long running, so we need to make this configurable
	if err != nil {
		return nil, fmt.Errorf("error sending request to runner: %w", err)
	}

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

func (c *RunnerController) RunnerIDs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.runners
}

func (c *RunnerController) TotalMemory(runnerID string) uint64 {
	status, err := c.getStatus(runnerID)
	if err != nil {
		log.Error().Err(err).Msg("error getting runner status")
		return 0
	}
	return uint64(status.TotalMemory)
}

func (c *RunnerController) FreeMemory(runnerID string) uint64 {
	status, err := c.getStatus(runnerID)
	if err != nil {
		log.Error().Err(err).Msg("error getting runner status")
		return 0
	}
	return uint64(status.FreeMemory)
}

func (c *RunnerController) Version(runnerID string) string {
	status, err := c.getStatus(runnerID)
	if err != nil {
		log.Error().Err(err).Msg("error getting runner status")
		return ""
	}
	return status.Version
}

func (c *RunnerController) Slots(runnerID string) ([]*types.RunnerSlot, error) {
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
	})
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
	})
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error submitting image generation request: %s", resp.Body)
	}

	return nil
}

func (c *RunnerController) CreateSlot(slot *Slot) error {
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
	})
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("error creating slot: %s", resp.Body)
	}
	return nil
}

func (c *RunnerController) DeleteSlot(runnerID string, slotID uuid.UUID) error {
	log.Info().
		Str("runner_id", runnerID).
		Str("slot_id", slotID.String()).
		Msg("deleting slot")
	resp, err := c.Send(c.ctx, runnerID, nil, &types.Request{
		Method: "DELETE",
		URL:    fmt.Sprintf("/api/v1/slots/%s", slotID.String()),
	})
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("error deleting slot: %s", resp.Body)
	}
	return nil
}

func (c *RunnerController) getStatus(runnerID string) (*types.RunnerStatus, error) {
	resp, err := c.Send(c.ctx, runnerID, nil, &types.Request{
		Method: "GET",
		URL:    "/api/v1/status",
	})
	if err != nil {
		return nil, err
	}
	var status types.RunnerStatus
	if err := json.Unmarshal([]byte(resp.Body), &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func (c *RunnerController) getSlots(runnerID string) (*types.ListRunnerSlotsResponse, error) {
	resp, err := c.Send(c.ctx, runnerID, nil, &types.Request{
		Method: "GET",
		URL:    "/api/v1/slots",
	})
	if err != nil {
		return nil, err
	}
	var slots types.ListRunnerSlotsResponse
	if err := json.Unmarshal([]byte(resp.Body), &slots); err != nil {
		return nil, err
	}
	return &slots, nil
}

func (c *RunnerController) SubmitFinetuningRequest(slot *Slot, session *types.Session) error {
	log.Info().Str("session_id", session.ID).Msg("processing fine-tuning interaction")

	headers := map[string]string{}
	headers[types.SessionIDHeader] = session.ID
	headers[pubsub.HelixNatsReplyHeader] = pubsub.GetRunnerResponsesQueue(session.Owner, session.ID)

	// TODO(Phil): the old code had some complicated logic around merging multiple jsonl files.
	// I'll just use the jsonl files from the last interaction for now.

	// and append them to one large JSONL file
	userInteractions := data.FilterUserInteractions(session.Interactions)
	finetuneInteractions := data.FilterFinetuneInteractions(userInteractions)
	if len(finetuneInteractions) == 0 {
		return fmt.Errorf("no finetune interactions found")
	}
	lastInteraction := finetuneInteractions[len(finetuneInteractions)-1]
	if len(lastInteraction.Files) != 1 {
		return fmt.Errorf("last interaction should have exactly one file")
	}
	combinedFile := lastInteraction.Files[0]

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
			Epochs:                 20, // TODO: connect this up to the finetuning API when it is ready
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
	})
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error submitting finetuning request: %s", resp.Body)
	}

	return nil
}
