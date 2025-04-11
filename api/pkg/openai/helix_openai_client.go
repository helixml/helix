package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

type HelixClient interface {
	CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
	CreateChatCompletionStream(ctx context.Context, request openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error)
	CreateEmbeddings(ctx context.Context, request openai.EmbeddingRequest) (openai.EmbeddingResponse, error)
}

var _ HelixClient = &InternalHelixServer{}

var chatCompletionTimeout = 180 * time.Second

func ListModels(_ context.Context) ([]types.OpenAIModel, error) {
	ollamaModels, err := model.GetDefaultOllamaModels()
	if err != nil {
		return nil, fmt.Errorf("failed to get Ollama models: %w", err)
	}

	helixModels := make([]types.OpenAIModel, 0, len(ollamaModels))

	for _, m := range ollamaModels {
		helixModels = append(helixModels, types.OpenAIModel{
			ID:            m.ModelName().String(),
			Object:        "model",
			OwnedBy:       "helix",
			Name:          m.GetHumanReadableName(),
			Description:   m.GetDescription(),
			Hide:          m.GetHidden(),
			Type:          "chat",
			ContextLength: int(m.GetContextLength()),
		})
	}

	diffusersModels, err := model.GetDefaultDiffusersModels()
	if err != nil {
		return nil, fmt.Errorf("failed to get Diffusers models: %w", err)
	}
	for _, m := range diffusersModels {
		helixModels = append(helixModels, types.OpenAIModel{
			ID:          m.ModelName().String(),
			Object:      "model",
			OwnedBy:     "helix",
			Name:        m.GetHumanReadableName(),
			Description: m.GetDescription(),
			Hide:        m.GetHidden(),
			Type:        "image",
		})
	}

	// Add VLLM models
	vllmModels, err := model.GetDefaultVLLMModels()
	if err != nil {
		return nil, fmt.Errorf("failed to get VLLM models: %w", err)
	}
	for _, m := range vllmModels {
		helixModels = append(helixModels, types.OpenAIModel{
			ID:            m.ModelName().String(),
			Object:        "model",
			OwnedBy:       "helix",
			Name:          m.GetHumanReadableName(),
			Description:   m.GetDescription(),
			Hide:          m.GetHidden(),
			Type:          "chat",
			ContextLength: int(m.GetContextLength()),
		})
	}

	return helixModels, nil
}

func (c *InternalHelixServer) CreateChatCompletion(requestCtx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	ctx, cancel := context.WithTimeout(requestCtx, chatCompletionTimeout)
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
		log.Debug().Str("request_id", requestID).Msg("received runner response, closing channel")
		defer close(doneCh)

		var runnerResp types.RunnerNatsReplyResponse
		err := json.Unmarshal(payload, &runnerResp)
		if err != nil {
			return fmt.Errorf("error unmarshalling runner response: %w", err)
		}

		if runnerResp.Error != "" {
			respError = fmt.Errorf("runner error: %s", runnerResp.Error)
		}

		err = json.Unmarshal(runnerResp.Response, &resp)
		if err != nil {
			return fmt.Errorf("error unmarshalling runner response: %w", err)
		}

		return nil
	})
	if err != nil {
		return openai.ChatCompletionResponse{}, fmt.Errorf("failed to subscribe to runner responses: %w", err)
	}

	defer func() {
		if err := sub.Unsubscribe(); err != nil {
			log.Error().Err(err).Msgf("failed to unsubscribe")
		}
	}()

	// Enqueue the request, it will be picked up by the runner
	err = c.enqueueRequest(&types.RunnerLLMInferenceRequest{
		RequestID:     requestID,
		CreatedAt:     time.Now(),
		OwnerID:       vals.OwnerID,
		SessionID:     vals.SessionID,
		InteractionID: vals.InteractionID,
		Request:       &request,
	})
	if err != nil {
		return openai.ChatCompletionResponse{}, fmt.Errorf("error enqueuing request: %w", err)
	}

	// Wait for the response or until the context is done (timeout)
	select {
	case <-doneCh:
	case <-requestCtx.Done():
		return openai.ChatCompletionResponse{}, fmt.Errorf("request was cancelled")
	case <-ctx.Done():
		// If this happens, we have timed out
		return openai.ChatCompletionResponse{}, fmt.Errorf("timeout waiting for runner response")
	}

	if respError != nil {
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
	readyCh := make(chan struct{})
	firstRun := true
	var respError error

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
		var runnerResp types.RunnerNatsReplyResponse
		err := json.Unmarshal(payload, &runnerResp)
		if err != nil {
			return fmt.Errorf("error unmarshalling runner response: %w", err)
		}

		if runnerResp.Error != "" {
			respError = fmt.Errorf("runner error: %s", runnerResp.Error)
		}

		// First chunk received, ready to return the stream or the error
		// This MUST be done before the writeChunk call, otherwise it will block waiting for the
		// reader to start
		if firstRun {
			close(readyCh)
			firstRun = false
		}

		// Remove the SSE "data: " prefix from the response
		response := strings.TrimPrefix(string(runnerResp.Response), "data: ")

		// Parse the streaming response object to make sure it is valid
		var streamResp openai.ChatCompletionStreamResponse
		err = json.Unmarshal([]byte(response), &streamResp)
		if err != nil {
			return fmt.Errorf("error unmarshalling stream response: %w", err)
		}

		// Now write the chunk to the stream
		bts, err := json.Marshal(streamResp)
		if err != nil {
			return fmt.Errorf("error marshalling stream response: %w", err)
		}

		err = writeChunk(pw, bts)
		if err != nil {
			return fmt.Errorf("error writing to stream: %w", err)
		}

		// If it's done, close the stream
		if streamResp.Choices[0].FinishReason != "" || respError != nil {
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
	err = c.enqueueRequest(&types.RunnerLLMInferenceRequest{
		RequestID:     requestID,
		CreatedAt:     time.Now(),
		OwnerID:       vals.OwnerID,
		SessionID:     vals.SessionID,
		InteractionID: vals.InteractionID,
		Request:       &request,
	})
	if err != nil {
		return nil, fmt.Errorf("error enqueuing request: %w", err)
	}

	go func() {
		ctx, cancel := context.WithTimeout(ctx, chatCompletionTimeout)
		defer cancel()

		<-ctx.Done()
		_ = sub.Unsubscribe()
	}()

	// Initiate through our client
	stream, err := client.CreateChatCompletionStream(ctx, request)

	// Wait for the ready signal
	<-readyCh

	if respError != nil {
		return nil, respError
	}

	return stream, err
}

func (c *InternalHelixServer) CreateEmbeddings(ctx context.Context, embeddingRequest openai.EmbeddingRequest) (resp openai.EmbeddingResponse, err error) {
	requestID := system.GenerateRequestID()
	ownerID := "internal"

	// Create a channel to signal when we get a response
	doneCh := make(chan struct{})

	var respData []byte
	var respError error

	// Subscribe to the runner response from the runner
	sub, err := c.pubsub.Subscribe(ctx, pubsub.GetRunnerResponsesQueue(ownerID, requestID), func(payload []byte) error {
		defer close(doneCh)

		var runnerResp types.RunnerNatsReplyResponse
		if err := json.Unmarshal(payload, &runnerResp); err != nil {
			respError = fmt.Errorf("error unmarshalling runner response: %w", err)
			return nil
		}

		if runnerResp.Error != "" {
			respError = fmt.Errorf("embedding error from runner: %s", runnerResp.Error)
			return nil
		}

		respData = runnerResp.Response
		return nil
	})
	if err != nil {
		return resp, fmt.Errorf("error subscribing to runner responses: %w", err)
	}
	defer sub.Unsubscribe()

	// Create a new inference request with embedding request
	req := &types.RunnerLLMInferenceRequest{
		RequestID:        requestID,
		CreatedAt:        time.Now(),
		OwnerID:          ownerID,
		Embeddings:       true,
		EmbeddingRequest: embeddingRequest,
		// Set the request model based on the embedding model
		Request: &openai.ChatCompletionRequest{
			Model: string(embeddingRequest.Model),
		},
	}

	// Enqueue the request for processing
	work, err := scheduler.NewLLMWorkload(req)
	if err != nil {
		return resp, fmt.Errorf("error creating workload: %w", err)
	}

	err = c.scheduler.Enqueue(work)
	if err != nil {
		return resp, fmt.Errorf("error enqueuing work: %w", err)
	}

	// Wait for the response or timeout
	select {
	case <-ctx.Done():
		return resp, ctx.Err()
	case <-doneCh:
		// Response received
		if respError != nil {
			return resp, respError
		}

		// Parse the embedding response
		if err := json.Unmarshal(respData, &resp); err != nil {
			return resp, fmt.Errorf("error unmarshalling embedding response: %w", err)
		}

		return resp, nil
	}
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

func (t *helixTransport) RoundTrip(*http.Request) (*http.Response, error) {
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
