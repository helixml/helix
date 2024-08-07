package tools

import (
	"context"
	"encoding/json"

	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/lukemarsden/go-openai2"
	"github.com/rs/zerolog/log"
)

func (c *ChainStrategy) logLLMCall(ctx context.Context, sessionID, interactionID string, step types.LLMCallStep, req *openai.ChatCompletionRequest, resp *openai.ChatCompletionResponse, durationMs int64) {
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

	llmCall := &types.LLMCall{
		SessionID:        sessionID,
		InteractionID:    interactionID,
		Model:            req.Model,
		Step:             step,
		Request:          reqBts,
		Response:         respBts,
		Provider:         string(c.cfg.Tools.Provider),
		DurationMs:       durationMs,
		PromptTokens:     int64(resp.Usage.PromptTokens),
		CompletionTokens: int64(resp.Usage.CompletionTokens),
		TotalTokens:      int64(resp.Usage.TotalTokens),
	}

	_, err = c.store.CreateLLMCall(ctx, llmCall)
	if err != nil {
		log.Error().Err(err).Msg("failed to log LLM call")
	}
}
