package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/lukemarsden/go-openai2"
)

type contextValues struct {
	OwnerID       string
	SessionID     string
	InteractionID string
}

const contextValuesKey = "contextValues"

func SetContextValues(ctx context.Context, ownerID, sessionID, interactionID string) context.Context {
	return context.WithValue(ctx, contextValuesKey, contextValues{
		OwnerID:       ownerID,
		SessionID:     sessionID,
		InteractionID: interactionID,
	})
}

func GetContextValues(ctx context.Context) (ownerID, sessionID, interactionID string) {
	if ctx == nil {
		return "", "", ""
	}

	values, ok := ctx.Value(contextValuesKey).(contextValues)
	if !ok {
		return "", "", ""
	}

	return values.OwnerID, values.SessionID, values.InteractionID
}

var chatCompletionTimeout = 180 * time.Second

func (c *InternalHelixServer) CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, chatCompletionTimeout)
	defer cancel()

	requestID := system.GenerateRequestID()

	doneCh := make(chan struct{})

	ownerID, sessionID, interactionID := GetContextValues(ctx)

	var (
		resp      openai.ChatCompletionResponse
		respError error
	)

	// Subscribe to the runner response from the runner
	sub, err := c.pubsub.Subscribe(ctx, pubsub.GetRunnerResponsesQueue(ownerID, requestID), func(payload []byte) error {
		var runnerResp types.RunnerLLMInferenceResponse
		err := json.Unmarshal(payload, &runnerResp)
		if err != nil {
			return fmt.Errorf("error unmarshalling runner response: %w", err)
		}

		defer close(doneCh)

		if runnerResp.Response != nil {
			resp = *runnerResp.Response
		}

		if runnerResp.Error != "nil" {
			respError = fmt.Errorf("runner error: %s", runnerResp.Error)
		}

		return nil
	})
	if err != nil {
		return openai.ChatCompletionResponse{}, fmt.Errorf("failed to subscribe to runner responses: %w", err)
	}

	defer sub.Unsubscribe()

	// Enqueue the request, it will be picked up by the runner
	c.enqueueRequest(&types.RunnerLLMInferenceRequest{
		RequestID:     requestID,
		CreatedAt:     time.Now(),
		Owner:         ownerID,
		SessionID:     sessionID,
		InteractionID: interactionID,
		Request:       &request,
	})

	// Wait for the response or until the context is done (timeout)
	select {
	case <-doneCh:
	case <-ctx.Done():
		return openai.ChatCompletionResponse{}, fmt.Errorf("timeout waiting for runner response")
	}

	if respError != nil {
		return openai.ChatCompletionResponse{}, respError
	}

	return resp, nil
}
