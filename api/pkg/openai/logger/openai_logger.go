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

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/model"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/transport"
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
	cfg       *config.ServerConfig
	client    oai.Client
	logStores []LogStore
	wg        sync.WaitGroup
	provider  types.Provider
}

func Wrap(cfg *config.ServerConfig, provider types.Provider, client oai.Client, logStores ...LogStore) *LoggingMiddleware {
	return &LoggingMiddleware{
		cfg:       cfg,
		logStores: logStores,
		client:    client,
		wg:        sync.WaitGroup{},
		provider:  provider,
	}
}

func (m *LoggingMiddleware) APIKey() string {
	return m.client.APIKey()
}

func (m *LoggingMiddleware) ListModels(ctx context.Context) ([]model.OpenAIModel, error) {
	return m.client.ListModels(ctx)
}

func (m *LoggingMiddleware) CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	start := time.Now()
	resp, err := m.client.CreateChatCompletion(ctx, request)
	if err != nil {
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

		m.logLLMCall(ctx, &request, &resp, time.Since(start).Milliseconds())
	}()

	return resp, nil
}

func (m *LoggingMiddleware) CreateChatCompletionStream(ctx context.Context, request openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
	upstream, err := m.client.CreateChatCompletionStream(ctx, request)
	if err != nil {
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

		start := time.Now()

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
		m.logLLMCall(ctx, &request, &resp, time.Since(start).Milliseconds())
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
		resp.Choices[0].Message.Content += chunk.Choices[0].Delta.Content

		if chunk.Choices[0].Delta.FunctionCall != nil {
			resp.Choices[0].Message.FunctionCall = chunk.Choices[0].Delta.FunctionCall
		}
	}

	// Append the usage
	if chunk.Usage != nil {
		resp.Usage.PromptTokens += chunk.Usage.PromptTokens
		resp.Usage.CompletionTokens += chunk.Usage.CompletionTokens
		resp.Usage.TotalTokens += chunk.Usage.TotalTokens
	}
}

func (m *LoggingMiddleware) logLLMCall(ctx context.Context, req *openai.ChatCompletionRequest, resp *openai.ChatCompletionResponse, durationMs int64) {
	reqBts, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		log.Error().Err(err).Msg("failed to marshal LLM request")
		return
	}

	respBts, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		log.Error().Err(err).Msg("failed to marshal LLM response")
		return
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

	log.Debug().
		Str("owner_id", vals.OwnerID).
		Str("app_id", appID).
		Str("model", req.Model).
		Str("provider", string(m.provider)).
		Str("step", string(step.Step)).
		Int("prompt_tokens", resp.Usage.PromptTokens).
		Int("completion_tokens", resp.Usage.CompletionTokens).
		Int("total_tokens", resp.Usage.TotalTokens).
		Msg("logging LLM call")

	llmCall := &types.LLMCall{
		AppID:            appID,
		SessionID:        vals.SessionID,
		InteractionID:    vals.InteractionID,
		Model:            req.Model,
		Step:             step.Step,
		OriginalRequest:  vals.OriginalRequest,
		Request:          reqBts,
		Response:         respBts,
		Provider:         string(m.provider),
		DurationMs:       durationMs,
		PromptTokens:     int64(resp.Usage.PromptTokens),
		CompletionTokens: int64(resp.Usage.CompletionTokens),
		TotalTokens:      int64(resp.Usage.TotalTokens),
		UserID:           vals.OwnerID,
	}
	ctx, cancel := context.WithTimeout(context.Background(), logCallTimeout)
	defer cancel()

	for _, logStore := range m.logStores {
		_, err = logStore.CreateLLMCall(ctx, llmCall)
		if err != nil {
			log.Error().Err(err).Msg("failed to log LLM call")
		}
	}
}

// No-op, not logging embeddings calls
func (m *LoggingMiddleware) CreateEmbeddings(ctx context.Context, request openai.EmbeddingRequest) (resp openai.EmbeddingResponse, err error) {
	return m.client.CreateEmbeddings(ctx, request)
}
