package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

type HelixClient interface {
	CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
	CreateChatCompletionStream(ctx context.Context, request openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error)
}

var _ HelixClient = &InternalHelixServer{}

var chatCompletionTimeout = 180 * time.Second

func ListModels(ctx context.Context) ([]model.OpenAIModel, error) {
	ollamaModels, err := model.GetDefaultOllamaModels()
	if err != nil {
		return nil, fmt.Errorf("failed to get Ollama models: %w", err)
	}

	var HelixModels []model.OpenAIModel

	for _, m := range ollamaModels {
		HelixModels = append(HelixModels, model.OpenAIModel{
			ID:          m.ModelName().String(),
			Object:      "model",
			OwnedBy:     "helix",
			Name:        m.GetHumanReadableName(),
			Description: m.GetDescription(),
			Hide:        m.GetHidden(),
		})
	}

	return HelixModels, nil
}

func (c *InternalHelixServer) CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, chatCompletionTimeout)
	defer cancel()

	requestID := system.GenerateRequestID()

	doneCh := make(chan struct{})

	// ownerID, sessionID, interactionID := GetContextValues(ctx)
	vals, ok := GetContextValues(ctx)
	if !ok {
		return openai.ChatCompletionResponse{}, fmt.Errorf("ownerID not set in context, use 'openai.SetContextValues()' before calling this method")
	}

	if vals.OwnerID == "" {
		return openai.ChatCompletionResponse{}, fmt.Errorf("ownerID not set in context, use 'openai.SetContextValues()' before calling this method")
	}

	var (
		resp      openai.ChatCompletionResponse
		respError error
	)

	// Subscribe to the runner response from the runner
	sub, err := c.pubsub.Subscribe(ctx, pubsub.GetRunnerResponsesQueue(vals.OwnerID, requestID), func(payload []byte) error {
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
		OwnerID:       vals.OwnerID,
		SessionID:     vals.SessionID,
		InteractionID: vals.InteractionID,
		Request:       &request,
	})

	// Wait for the response or until the context is done (timeout)
	select {
	case <-doneCh:
	case <-ctx.Done():
		err := c.scheduler.Release(requestID)
		if err != nil {
			log.Error().Err(err).Msg("error releasing allocation")
		}
		return openai.ChatCompletionResponse{}, fmt.Errorf("timeout waiting for runner response")
	}

	if respError != nil {
		err := c.scheduler.Release(requestID)
		if err != nil {
			log.Error().Err(err).Msg("error releasing allocation")
		}
		return openai.ChatCompletionResponse{}, respError
	}

	return resp, nil
}

func (c *InternalHelixServer) CreateChatCompletionStream(ctx context.Context, request openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
	request.Stream = true

	requestID := system.GenerateRequestID()

	// ownerID, sessionID, interactionID := GetContextValues(ctx)
	vals, ok := GetContextValues(ctx)
	if !ok {
		return nil, fmt.Errorf("ownerID not set in context, use 'openai.SetContextValues()' before calling this method")
	}

	if vals.OwnerID == "" {
		return nil, fmt.Errorf("ownerID not set in context, use 'openai.SetContextValues()' before calling this method")
	}

	doneCh := make(chan struct{})

	pr, pw := io.Pipe()

	ht := &helixTransport{
		reader: pr,
		writer: pw,
	}

	// Create a fake HTTP client with a custom transport that will be feeding the stream
	config := openai.DefaultConfig("helix")
	config.HTTPClient = &http.Client{
		Transport: ht,
	}

	client := openai.NewClientWithConfig(config)

	// Subscribe to the runner response from the runner
	sub, err := c.pubsub.Subscribe(ctx, pubsub.GetRunnerResponsesQueue(vals.OwnerID, requestID), func(payload []byte) error {
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

			err = writeChunk(pw, bts)
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
		OwnerID:       vals.OwnerID,
		SessionID:     vals.SessionID,
		InteractionID: vals.InteractionID,
		Request:       &request,
	})

	go func() {
		ctx, cancel := context.WithTimeout(ctx, chatCompletionTimeout)
		defer cancel()

		<-ctx.Done()
		_ = sub.Unsubscribe()
	}()

	// Initiate through our client
	return client.CreateChatCompletionStream(ctx, request)
}

// NewOpenAIStreamingAdapter returns a new OpenAI streaming adapter which allows
// to write into the io.Writer and read from the stream directly
func NewOpenAIStreamingAdapter(req openai.ChatCompletionRequest) (*openai.ChatCompletionStream, *io.PipeWriter, error) {
	pr, pw := io.Pipe()

	ht := &helixTransport{
		reader: pr,
		writer: pw,
	}

	// Create a fake HTTP client with a custom transport that will be feeding the stream
	config := openai.DefaultConfig("helix")
	config.HTTPClient = &http.Client{
		Transport: ht,
	}

	client := openai.NewClientWithConfig(config)

	stream, err := client.CreateChatCompletionStream(context.Background(), req)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating chat completion stream: %w", err)
	}

	return stream, pw, nil
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

func writeChunk(w io.Writer, chunk []byte) error {
	_, err := fmt.Fprintf(w, "data: %s\n\n", string(chunk))
	if err != nil {
		return fmt.Errorf("error writing chunk '%s': %w", string(chunk), err)
	}

	// Flush the ResponseWriter buffer to send the chunk immediately
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	return nil
}
