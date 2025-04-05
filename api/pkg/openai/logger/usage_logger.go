package logger

import (
	"context"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type UsageLogger struct {
	store store.Store
}

var _ LogStore = &UsageLogger{}

func NewUsageLogger(store store.Store) *UsageLogger {
	return &UsageLogger{store: store}
}

func (l *UsageLogger) CreateLLMCall(ctx context.Context, call *types.LLMCall) (*types.LLMCall, error) {
	metric := &types.UsageMetric{
		AppID:             call.AppID,
		UserID:            call.UserID,
		Model:             call.Model,
		Provider:          call.Provider,
		PromptTokens:      int(call.PromptTokens),
		CompletionTokens:  int(call.CompletionTokens),
		TotalTokens:       int(call.PromptTokens + call.CompletionTokens),
		DurationMs:        int(call.DurationMs),
		RequestSizeBytes:  len(call.Request),
		ResponseSizeBytes: len(call.Response),
	}

	_, err := l.store.CreateUsageMetric(ctx, metric)
	if err != nil {
		log.Error().
			Str("user_id", metric.UserID).
			Str("model", metric.Model).
			Str("provider", metric.Provider).
			Int("prompt_tokens", metric.PromptTokens).
			Int("completion_tokens", metric.CompletionTokens).
			Err(err).Msg("failed to log LLM usage")
		return nil, err
	}

	return call, nil
}
