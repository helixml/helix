package logger

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	openai "github.com/lukemarsden/go-openai2"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
	oai "github.com/helixml/helix/api/pkg/openai"
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
	cfg      *config.ServerConfig
	logStore LogStore
	client   *openai.Client
	wg       sync.WaitGroup
}

func New(cfg *config.ServerConfig, logStore LogStore, client *openai.Client) *LoggingMiddleware {
	return &LoggingMiddleware{
		cfg:      cfg,
		logStore: logStore,
		client:   client,
		wg:       sync.WaitGroup{},
	}
}

func (m *LoggingMiddleware) CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	start := time.Now()
	resp, err := m.client.CreateChatCompletion(ctx, request)
	if err != nil {
		return resp, err
	}

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.logLLMCall(ctx, &request, &resp, time.Since(start).Milliseconds())
	}()

	return resp, nil
}

func (m *LoggingMiddleware) CreateChatCompletionStream(ctx context.Context, request openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
	return m.client.CreateChatCompletionStream(ctx, request)
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
		log.Error().Msg("failed to get context values")
		vals = &oai.ContextValues{}
	}

	llmCall := &types.LLMCall{
		SessionID:        vals.SessionID,
		InteractionID:    vals.InteractionID,
		Model:            req.Model,
		Step:             vals.Step,
		Request:          reqBts,
		Response:         respBts,
		Provider:         string(m.cfg.Inference.Provider),
		DurationMs:       durationMs,
		PromptTokens:     int64(resp.Usage.PromptTokens),
		CompletionTokens: int64(resp.Usage.CompletionTokens),
		TotalTokens:      int64(resp.Usage.TotalTokens),
	}
	ctx, cancel := context.WithTimeout(context.Background(), logCallTimeout)
	defer cancel()

	_, err = m.logStore.CreateLLMCall(ctx, llmCall)
	if err != nil {
		log.Error().Err(err).Msg("failed to log LLM call")
	}
}
