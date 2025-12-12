package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"runtime/debug"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
	"github.com/tiktoken-go/tokenizer"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/model"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/transport"
	"github.com/helixml/helix/api/pkg/pricing"
	"github.com/helixml/helix/api/pkg/types"
)

var logCallTimeout = 15 * time.Second

// LogStore is a store for LLM calls. By default this is a Postgres database
// but it can be other storage backends like S3, BigQuery, etc.
type LogStore interface {
	CreateLLMCall(ctx context.Context, call *types.LLMCall) (*types.LLMCall, error)
}

var _ oai.Client = &LoggingMiddleware{}

type LoggingMiddleware struct {
	cfg               *config.ServerConfig
	client            oai.Client
	logStores         []LogStore
	billingLogger     LogStore
	wg                sync.WaitGroup
	provider          types.Provider
	modelInfoProvider model.ModelInfoProvider
	defaultCodec      tokenizer.Codec
}

func Wrap(cfg *config.ServerConfig, provider types.Provider, client oai.Client, modelInfoProvider model.ModelInfoProvider, billingLogger LogStore, logStores ...LogStore) *LoggingMiddleware {
	enc, err := tokenizer.Get(tokenizer.Cl100kBase)
	if err != nil {
		panic("failed to initialize tokenizer")
	}

	return &LoggingMiddleware{
		cfg:               cfg,
		logStores:         logStores,
		billingLogger:     billingLogger,
		client:            client,
		wg:                sync.WaitGroup{},
		provider:          provider,
		modelInfoProvider: modelInfoProvider,
		defaultCodec:      enc,
	}
}

func (m *LoggingMiddleware) APIKey() string {
	return m.client.APIKey()
}

func (m *LoggingMiddleware) BaseURL() string {
	return m.client.BaseURL()
}

func (m *LoggingMiddleware) ListModels(ctx context.Context) ([]types.OpenAIModel, error) {
	return m.client.ListModels(ctx)
}

func (m *LoggingMiddleware) BillingEnabled() bool {
	return m.client.BillingEnabled()
}

// BillingLogger used for testing
func (m *LoggingMiddleware) BillingLogger() LogStore {
	return m.billingLogger
}

// UsageLogStores used for testing
func (m *LoggingMiddleware) UsageLogStores() []LogStore {
	return m.logStores
}

func (m *LoggingMiddleware) CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	start := time.Now()
	resp, err := m.client.CreateChatCompletion(ctx, request)
	if err != nil {
		m.logLLMCall(ctx, start, &request, &resp, err, false, time.Since(start).Milliseconds())
		return resp, err
	}

	m.wg.Add(1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().Msgf("Recovered from panic: %v", r)
			}
		}()

		defer m.wg.Done()

		m.logLLMCall(ctx, start, &request, &resp, nil, false, time.Since(start).Milliseconds())
	}()

	return resp, nil
}

func (m *LoggingMiddleware) CreateChatCompletionStream(ctx context.Context, request openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
	start := time.Now()

	upstream, err := m.client.CreateChatCompletionStream(ctx, request)
	if err != nil {
		m.logLLMCall(ctx, start, &request, nil, err, true, time.Since(start).Milliseconds())
		return nil, err
	}

	downstream, downstreamWriter, err := transport.NewOpenAIStreamingAdapter(request)
	if err != nil {
		return nil, fmt.Errorf("failed to create streaming adapter: %w", err)
	}

	m.wg.Add(1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().Msgf("Recovered from panic: %v\n%s", r, debug.Stack())
			}
		}()

		defer m.wg.Done()
		// Once done, close the writer
		defer downstreamWriter.Close()

		var resp = openai.ChatCompletionResponse{}

		// Read from the upstream stream and write to the downstream stream
		for {
			msg, err := upstream.Recv()
			if err != nil {
				if err == io.EOF {
					break
				}
				log.Error().Err(err).Msg("failed to receive message from upstream stream")
				break
			}

			// Add the message to the response
			appendChunk(&resp, &msg)

			if err := transport.WriteChatCompletionStream(downstreamWriter, &msg); err != nil {
				// TODO: should we return here? For now we just log and continue
				log.Error().Err(err).Msg("failed to  write completion")
			}
		}

		// Once the stream is done, close the downstream writer
		m.logLLMCall(ctx, start, &request, &resp, nil, true, time.Since(start).Milliseconds())
	}()

	return downstream, nil
}

func appendChunk(resp *openai.ChatCompletionResponse, chunk *openai.ChatCompletionStreamResponse) {
	if chunk == nil {
		return
	}

	if len(resp.Choices) == 0 {
		resp.Choices = []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{},
			},
		}
	}

	if chunk.Model != "" {
		resp.Model = chunk.Model
	}

	if chunk.ID != "" {
		resp.ID = chunk.ID
	}

	if chunk.Created != 0 {
		resp.Created = chunk.Created
	}

	// Append the chunk to the response
	if len(chunk.Choices) > 0 {
		// Role
		if chunk.Choices[0].Delta.Role != "" {
			resp.Choices[0].Message.Role = chunk.Choices[0].Delta.Role
		}

		// Content
		resp.Choices[0].Message.Content += chunk.Choices[0].Delta.Content

		// Function calls
		if chunk.Choices[0].Delta.FunctionCall != nil {
			resp.Choices[0].Message.FunctionCall = chunk.Choices[0].Delta.FunctionCall
		}

		// Tool calls
		if len(chunk.Choices[0].Delta.ToolCalls) > 0 {
			resp.Choices[0].Message.ToolCalls = append(resp.Choices[0].Message.ToolCalls, chunk.Choices[0].Delta.ToolCalls...)
		}
	}

	// Append the usage
	if chunk.Usage != nil {
		resp.Usage.PromptTokens += chunk.Usage.PromptTokens
		resp.Usage.CompletionTokens += chunk.Usage.CompletionTokens
		resp.Usage.TotalTokens += chunk.Usage.TotalTokens
	}
}

func (m *LoggingMiddleware) logLLMCall(ctx context.Context, createdAt time.Time, req *openai.ChatCompletionRequest, resp *openai.ChatCompletionResponse, apiError error, stream bool, durationMs int64) {
	// Remove the cancel function from the context
	ctx = context.WithoutCancel(ctx)

	reqBts, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		log.Error().Err(err).Msg("failed to marshal LLM request")
		return
	}

	var respBts []byte

	if resp != nil {
		respBts, err = json.MarshalIndent(resp, "", "  ")
		if err != nil {
			log.Error().Err(err).Msg("failed to marshal LLM response")
			return
		}
	}

	vals, ok := oai.GetContextValues(ctx)
	if !ok {
		// Session data will be missing for Discord, Slack, etc.
		log.Debug().Msg("failed to get context values")
		vals = &oai.ContextValues{}
	}

	step, ok := oai.GetStep(ctx)
	if !ok {
		// It's normal to not have the step in the context (if it's not a tool)
		step = &oai.Step{}
	}

	appID, ok := oai.GetContextAppID(ctx)
	if !ok {
		log.Debug().Msg("failed to get app_id")
	}

	orgID, ok := oai.GetContextOrganizationID(ctx)
	if !ok {
		log.Debug().Msg("failed to get organization_id")
	}

	// Initialize token counts to 0 in case resp is nil
	var promptTokens, completionTokens, totalTokens int

	if resp != nil {
		if resp.Usage.PromptTokens == 0 && resp.Usage.CompletionTokens == 0 {
			// Compute the token usage
			promptTokens, completionTokens, totalTokens = m.computeTokenUsage(req, resp)
			resp.Usage.PromptTokens = promptTokens
			resp.Usage.CompletionTokens = completionTokens
			resp.Usage.TotalTokens = totalTokens
		} else {
			promptTokens = resp.Usage.PromptTokens
			completionTokens = resp.Usage.CompletionTokens
			totalTokens = resp.Usage.TotalTokens
		}
	}

	var (
		promptCost     float64
		completionCost float64
		totalCost      float64
	)

	// Get pricing info for the model
	modelInfo, err := m.modelInfoProvider.GetModelInfo(ctx, &model.ModelInfoRequest{
		BaseURL:  m.BaseURL(),
		Provider: string(m.provider),
		Model:    req.Model,
	})
	if err != nil {
		log.Warn().
			Err(err).
			Str("user_id", vals.OwnerID).
			Str("model", req.Model).
			Str("provider", string(m.provider)).
			Err(err).Msg("failed to get model info")
	} else {
		// Calculate the cost for the call and persist it
		promptCost, completionCost, err = pricing.CalculateTokenPrice(modelInfo, int64(promptTokens), int64(completionTokens))
		if err != nil {
			log.Error().
				Err(err).
				Str("user_id", vals.OwnerID).
				Str("model", req.Model).
				Str("provider", string(m.provider)).
				Err(err).Msg("failed to calculate token price")
		}
		totalCost = promptCost + completionCost
	}

	log.Debug().
		Str("owner_id", vals.OwnerID).
		Str("app_id", appID).
		Str("organization_id", orgID).
		Str("model", req.Model).
		Str("provider", string(m.provider)).
		Str("step", string(step.Step)).
		Int("prompt_tokens", promptTokens).
		Int("completion_tokens", completionTokens).
		Int("total_tokens", totalTokens).
		Float64("prompt_cost", promptCost).
		Float64("completion_cost", completionCost).
		Float64("total_cost", totalCost).
		Msg("logging LLM call")

	llmCall := &types.LLMCall{
		Created:          createdAt,
		AppID:            appID,
		SessionID:        vals.SessionID,
		InteractionID:    vals.InteractionID,
		OrganizationID:   orgID,
		Model:            req.Model,
		Step:             step.Step,
		OriginalRequest:  vals.OriginalRequest,
		Request:          reqBts,
		Response:         respBts,
		Provider:         string(m.provider),
		DurationMs:       durationMs,
		PromptTokens:     int64(promptTokens),
		CompletionTokens: int64(completionTokens),
		TotalTokens:      int64(totalTokens),
		PromptCost:       promptCost,
		CompletionCost:   completionCost,
		TotalCost:        totalCost,
		UserID:           vals.OwnerID,
		Stream:           stream,
		ProjectID:        vals.ProjectID,
		SpecTaskID:       vals.SpecTaskID,
	}

	if apiError != nil {
		llmCall.Error = apiError.Error()
	}

	ctx, cancel := context.WithTimeout(context.Background(), logCallTimeout)
	defer cancel()

	for _, logStore := range m.logStores {
		_, err = logStore.CreateLLMCall(ctx, llmCall)
		if err != nil {
			log.Error().Err(err).Msg("failed to log LLM call")
		}
	}

	if m.billingLogger != nil {
		_, err = m.billingLogger.CreateLLMCall(ctx, llmCall)
		if err != nil {
			log.Error().Err(err).Msg("failed to log LLM call to billing logger")
		}
	}
}

// computeTokenUsage - computes the token usage for the request and response if the original response doesn't contain it (openai models, some ollama models, etc.)
// This function is intended to be used as a fallback and provides an estimate.
func (m *LoggingMiddleware) computeTokenUsage(req *openai.ChatCompletionRequest, resp *openai.ChatCompletionResponse) (int, int, int) {
	// Try to get accurate tokenizer
	codec, err := tokenizer.ForModel(tokenizer.Model(req.Model))
	if err != nil {
		log.Debug().Err(err).Str("model", req.Model).Msg("failed to get tokenizer for model, using default codec")
		codec = m.defaultCodec
	}

	// Compute prompt tokens
	promptTokens, err := computeRequestTokens(codec, req)
	if err != nil {
		log.Warn().Err(err).Str("model", req.Model).Msg("failed to count tokens for prompt in computeTokenUsage")
		return 0, 0, 0
	}

	// Compute completion tokens
	completionTokens, err := computeCompletionTokens(codec, resp)
	if err != nil {
		log.Warn().Err(err).Str("model", req.Model).Msg("failed to count tokens for completion in computeTokenUsage")
		return 0, 0, 0
	}

	totalTokens := promptTokens + completionTokens
	return promptTokens, completionTokens, totalTokens
}

func computeRequestTokens(codec tokenizer.Codec, req *openai.ChatCompletionRequest) (int, error) {
	var content string
	for _, message := range req.Messages {
		content += message.Content
		// Note: For some models or complex scenarios, newlines or other separators between messages
		// might be necessary for accurate token counting. This simple concatenation is a common baseline.
	}

	ids, _, err := codec.Encode(content)
	if err != nil {
		return 0, fmt.Errorf("failed to encode request content for token count: %w", err)
	}
	return len(ids), nil
}

func computeCompletionTokens(codec tokenizer.Codec, resp *openai.ChatCompletionResponse) (int, error) {
	// Compute completion tokens
	var completionTokens int

	if resp != nil && len(resp.Choices) > 0 {
		var contentToEncode string
		choice := resp.Choices[0] // Assuming always at least one choice if len > 0

		if choice.Message.Content != "" {
			contentToEncode = choice.Message.Content
		} else if choice.Message.FunctionCall != nil {
			fc := choice.Message.FunctionCall
			// Basic approximation for function call tokenization: "name arguments"
			if fc.Arguments != "" {
				contentToEncode = fc.Name + " " + fc.Arguments
			} else {
				contentToEncode = fc.Name
			}
		}

		if contentToEncode != "" {
			ids, _, errEncCompletion := codec.Encode(contentToEncode)
			if errEncCompletion != nil {
				log.Debug().Err(errEncCompletion).Msg("failed to count tokens for completion content in computeTokenUsage")
				// completionTokens remains 0
			} else {
				completionTokens = len(ids)
			}
		}
	}

	return completionTokens, nil
}

// TODO: We should actually log the embedding request and response to the llm_calls table
func (m *LoggingMiddleware) CreateEmbeddings(ctx context.Context, request openai.EmbeddingRequest) (resp openai.EmbeddingResponse, err error) {
	startTime := time.Now()

	// Log the request
	log.Info().
		Str("component", "openai_logger").
		Str("provider", string(m.provider)).
		Str("operation", "embedding").
		Str("model", string(request.Model)).
		Int("input_length", len(fmt.Sprintf("%v", request.Input))).
		Msg("🔍 Embedding request")

	// Call the actual embedding API
	resp, err = m.client.CreateEmbeddings(ctx, request)

	// Calculate duration
	durationMs := time.Since(startTime).Milliseconds()

	// Log the response
	if err != nil {
		log.Error().
			Str("component", "openai_logger").
			Str("provider", string(m.provider)).
			Str("operation", "embedding").
			Str("model", string(request.Model)).
			Int64("duration_ms", durationMs).
			Err(err).
			Msg("❌ Embedding failed")
	} else {
		// Build the log entry
		logEntry := log.Info().
			Str("component", "openai_logger").
			Str("provider", string(m.provider)).
			Str("operation", "embedding").
			Str("model", string(request.Model)).
			Int64("duration_ms", durationMs).
			Int("embedding_count", len(resp.Data))

		// Only add dimensions if we have at least one embedding
		if len(resp.Data) > 0 {
			logEntry = logEntry.Int("embedding_dimensions", len(resp.Data[0].Embedding))
		} else {
			logEntry = logEntry.Str("error", "empty_embedding_response")
		}

		logEntry.Msg("✅ Embedding completed")
	}

	return resp, err
}

func (m *LoggingMiddleware) CreateFlexibleEmbeddings(ctx context.Context, request types.FlexibleEmbeddingRequest) (resp types.FlexibleEmbeddingResponse, err error) {
	startTime := time.Now()

	// Log the request
	logEntry := log.Info().
		Str("component", "openai_logger").
		Str("provider", string(m.provider)).
		Str("operation", "flexible_embedding").
		Str("model", request.Model)

	// Add input information based on what's provided
	if request.Input != nil {
		logEntry = logEntry.Int("input_length", len(fmt.Sprintf("%v", request.Input)))
	}
	if len(request.Messages) > 0 {
		logEntry = logEntry.Int("message_count", len(request.Messages))
	}

	logEntry.Msg("🔍 Flexible embedding request")

	// Call the actual embedding API
	resp, err = m.client.CreateFlexibleEmbeddings(ctx, request)

	// Calculate duration
	durationMs := time.Since(startTime).Milliseconds()

	// Log the response
	if err != nil {
		log.Error().
			Str("component", "openai_logger").
			Str("provider", string(m.provider)).
			Str("operation", "flexible_embedding").
			Str("model", request.Model).
			Int64("duration_ms", durationMs).
			Err(err).
			Msg("❌ Flexible embedding failed")
	} else {
		// Build the log entry
		logEntry := log.Info().
			Str("component", "openai_logger").
			Str("provider", string(m.provider)).
			Str("operation", "flexible_embedding").
			Str("model", request.Model).
			Int64("duration_ms", durationMs).
			Int("embedding_count", len(resp.Data))

		// Only add dimensions if we have at least one embedding
		if len(resp.Data) > 0 {
			logEntry = logEntry.Int("embedding_dimensions", len(resp.Data[0].Embedding))
		} else {
			logEntry = logEntry.Str("error", "empty_embedding_response")
		}

		logEntry.Msg("✅ Flexible embedding completed")
	}

	return resp, err
}
