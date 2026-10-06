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
		OrganizationID:    call.OrganizationID,
		AppID:             call.AppID,
		UserID:            call.UserID,
		InteractionID:     call.InteractionID,
		SessionID:         call.SessionID,
		CodeAgentRuntime:  call.CodeAgentRuntime,
		Model:             call.Model,
		Provider:          call.Provider,
		Source:            types.UsageMetricSourceHelixProxy,
		UsageKnown:        true,
		PromptTokens:      int(call.PromptTokens),
		CompletionTokens:  int(call.CompletionTokens),
		TotalTokens:       int(call.PromptTokens + call.CompletionTokens),
		CacheReadTokens:   int(call.CacheReadTokens),
		CacheWriteTokens:  int(call.CacheWriteTokens),
		PromptCost:        call.PromptCost,
		CompletionCost:    call.CompletionCost,
		CacheReadCost:     call.CacheReadCost,
		CacheWriteCost:    call.CacheWriteCost,
		TotalCost:         call.TotalCost,
		DurationMs:        int(call.DurationMs),
		RequestSizeBytes:  len(call.Request),
		ResponseSizeBytes: len(call.Response),
		SpecTaskID:        call.SpecTaskID,
		ProjectID:         call.ProjectID,
	}

	// Only requests that actually produced tool calls belong in the tool call
	// error rate. Counting every request would bury the signal under ordinary
	// prose turns, and counting requests that merely offered tools would
	// penalise a model for correctly declining to call one.
	if call.ToolCallsReturned > 0 {
		metric.ToolCallRequests = 1
		if call.ToolCallErrors > 0 {
			metric.ToolCallErrorRequests = 1
		}
	}

	_, err := l.CreateUsageMetric(ctx, metric)
	if err != nil {
		return nil, err
	}

	return call, nil
}

func (l *UsageLogger) CreateUsageMetric(ctx context.Context, metric *types.UsageMetric) (*types.UsageMetric, error) {
	created, err := l.store.CreateUsageMetric(ctx, metric)
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

	return created, nil
}
