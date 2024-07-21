package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/lukemarsden/go-openai2"
)

type HelixClient interface {
	CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
	CreateChatCompletionStream(ctx context.Context, request openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error)
}

var _ HelixClient = &InternalHelixServer{}

var chatCompletionTimeout = 180 * time.Second

func (c *InternalHelixServer) CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, chatCompletionTimeout)
	defer cancel()

	requestID := system.GenerateRequestID()

	doneCh := make(chan struct{})

	ownerID, sessionID, interactionID := GetContextValues(ctx)

	if ownerID == "" {
		return openai.ChatCompletionResponse{}, fmt.Errorf("ownerID not set in context, use 'openai.SetContextValues()' before calling this method")
	}

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

		if runnerResp.Error != "" {
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
		OwnerID:       ownerID,
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

func (c *InternalHelixServer) CreateChatCompletionStream(ctx context.Context, request openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
	ctx, cancel := context.WithTimeout(ctx, chatCompletionTimeout)
	defer cancel()

	request.Stream = true

	requestID := system.GenerateRequestID()

	ownerID, sessionID, interactionID := GetContextValues(ctx)
	if ownerID == "" {
		return nil, fmt.Errorf("ownerID not set in context, use 'openai.SetContextValues()' before calling this method")
	}

	doneCh := make(chan struct{})

	config := openai.DefaultConfig("helix")
	client := openai.NewClientWithConfig(config)

	pr, pw := io.Pipe()

	ht := &helixTransport{
		reader: pr,
		writer: pw,
	}

	// Create a fake HTTP client with a custom transport that will be feeding the stream
	config.HTTPClient = &http.Client{
		Transport: ht,
	}

	// Subscribe to the runner response from the runner
	sub, err := c.pubsub.Subscribe(ctx, pubsub.GetRunnerResponsesQueue(ownerID, requestID), func(payload []byte) error {
		var runnerResp types.RunnerLLMInferenceResponse
		err := json.Unmarshal(payload, &runnerResp)
		if err != nil {
			return fmt.Errorf("error unmarshalling runner response: %w", err)
		}

		if runnerResp.StreamResponse != nil {
			bts, err := json.Marshal(runnerResp.StreamResponse)
			if err != nil {
				return fmt.Errorf("error marshalling stream response: %w", err)
			}

			_, err = pw.Write(bts)
			if err != nil {
				return fmt.Errorf("error writing to stream: %w", err)
			}
		}

		if runnerResp.Done {
			close(doneCh)

			// Ensure the buffer gets EOF so it stops reading
			_ = pw.Close()
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to runner responses: %w", err)
	}

	// Enqueue the request, it will be picked up by the runner
	c.enqueueRequest(&types.RunnerLLMInferenceRequest{
		RequestID:     requestID,
		CreatedAt:     time.Now(),
		OwnerID:       ownerID,
		SessionID:     sessionID,
		InteractionID: interactionID,
		Request:       &request,
	})

	go func() {
		<-ctx.Done()
		sub.Unsubscribe()
	}()

	// Initiate through our client
	return client.CreateChatCompletionStream(ctx, request)
}

type helixTransport struct {
	reader *io.PipeReader
	writer *io.PipeWriter
}

func (t *helixTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	readCloser := io.NopCloser(t.reader)

	return &http.Response{
		StatusCode: 200,
		Body:       readCloser,
	}, nil
}
