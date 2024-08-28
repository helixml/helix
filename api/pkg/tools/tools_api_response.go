package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/lukemarsden/go-openai2"
	"github.com/rs/zerolog/log"
)

func (c *ChainStrategy) interpretResponse(ctx context.Context, sessionID, interactionID string, tool *types.Tool, currentMessage string, resp *http.Response) (*RunActionResponse, error) {
	bts, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return c.handleErrorResponse(ctx, sessionID, interactionID, tool, resp.StatusCode, bts)
	}

	return c.handleSuccessResponse(ctx, sessionID, interactionID, tool, currentMessage, resp.StatusCode, bts)
}

func (c *ChainStrategy) handleSuccessResponse(ctx context.Context, sessionID, interactionID string, tool *types.Tool, currentMessage string, _ int, body []byte) (*RunActionResponse, error) {
	systemPrompt := successResponsePrompt
	if tool.Config.API.ResponseSuccessTemplate != "" {
		systemPrompt = tool.Config.API.ResponseSuccessTemplate
	}

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemPrompt,
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: fmt.Sprintf("%s\nInput: %s", currentMessage, string(body)),
		},
	}

	req := openai.ChatCompletionRequest{
		Stream:   false,
		Model:    c.cfg.Tools.Model,
		Messages: messages,
	}

	ctx = oai.SetContextValues(ctx, &oai.ContextValues{
		OwnerID:       "system",
		SessionID:     sessionID,
		InteractionID: interactionID,
	})

	ctx = oai.SetStep(ctx, &oai.Step{
		Step: types.LLMCallStepInterpretResponse,
	})

	resp, err := c.apiClient.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get response from inference API: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from inference API")
	}

	return &RunActionResponse{
		Message:    resp.Choices[0].Message.Content,
		RawMessage: string(body),
	}, nil
}

func (c *ChainStrategy) handleErrorResponse(ctx context.Context, sessionID, interactionID string, tool *types.Tool, statusCode int, body []byte) (*RunActionResponse, error) {
	systemPrompt := errorResponsePrompt
	if tool.Config.API.ResponseErrorTemplate != "" {
		systemPrompt = tool.Config.API.ResponseErrorTemplate
	}

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemPrompt,
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: fmt.Sprintf("Got this error while processing your request: (status code %d), response body:\n\n%s", statusCode, string(body)),
		},
	}

	req := openai.ChatCompletionRequest{
		Stream:   false,
		Model:    c.cfg.Tools.Model,
		Messages: messages,
	}

	ctx = oai.SetContextValues(ctx, &oai.ContextValues{
		OwnerID:       "system",
		SessionID:     sessionID,
		InteractionID: interactionID,
	})

	ctx = oai.SetStep(ctx, &oai.Step{
		Step: types.LLMCallStepInterpretResponse,
	})

	resp, err := c.apiClient.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get response from inference API: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from inference API")
	}

	return &RunActionResponse{
		Message:    resp.Choices[0].Message.Content,
		RawMessage: string(body),
	}, nil
}

func (c *ChainStrategy) interpretResponseStream(ctx context.Context, sessionID, interactionID string, tool *types.Tool, currentMessage string, resp *http.Response) (*openai.ChatCompletionStream, error) {
	bts, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return c.handleSuccessResponseStream(ctx, sessionID, interactionID, tool, currentMessage, resp.StatusCode, bts)
	}

	return c.handleSuccessResponseStream(ctx, sessionID, interactionID, tool, currentMessage, resp.StatusCode, bts)
}

func (c *ChainStrategy) handleSuccessResponseStream(ctx context.Context, sessionID, interactionID string, tool *types.Tool, currentMessage string, _ int, body []byte) (*openai.ChatCompletionStream, error) {
	systemPrompt := successResponsePrompt
	if tool.Config.API.ResponseSuccessTemplate != "" {
		systemPrompt = tool.Config.API.ResponseSuccessTemplate
	}

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemPrompt,
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: fmt.Sprintf("%s\nInput: %s", currentMessage, string(body)),
		},
	}

	req := openai.ChatCompletionRequest{
		Stream:   true,
		Model:    c.cfg.Tools.Model,
		Messages: messages,
	}

	started := time.Now()

	resp, err := c.apiClient.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get response from inference API: %w", err)
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		log.Info().
			Str("session_id", sessionID).
			Str("interaction_id", interactionID).
			Dur("duration", time.Since(started)).
			Msg("LLM call")
		// c.logLLMCall(sessionID, interactionID, types.LLMCallStepInterpretResponse, &req, &resp, time.Since(started).Milliseconds())
	}()

	return resp, nil
}

func (c *ChainStrategy) handleErrorResponseStream(ctx context.Context, sessionID, interactionID string, tool *types.Tool, statusCode int, body []byte) (*openai.ChatCompletionStream, error) {
	systemPrompt := errorResponsePrompt
	if tool.Config.API.ResponseErrorTemplate != "" {
		systemPrompt = tool.Config.API.ResponseErrorTemplate
	}

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemPrompt,
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: fmt.Sprintf("Got this error while processing your request: (status code %d), response body:\n\n%s", statusCode, string(body)),
		},
	}

	req := openai.ChatCompletionRequest{
		Stream:   true,
		Model:    c.cfg.Tools.Model,
		Messages: messages,
	}

	started := time.Now()

	resp, err := c.apiClient.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get response from inference API: %w", err)
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		log.Info().
			Str("session_id", sessionID).
			Str("interaction_id", interactionID).
			Dur("duration", time.Since(started)).
			Msg("LLM call")
		// c.logLLMCall(sessionID, interactionID, types.LLMCallStepInterpretResponse, &req, &resp, time.Since(started).Milliseconds())
	}()

	return resp, nil
}

const successResponsePrompt = `Always assist with care, respect, and truth. Respond with utmost utility yet securely. Avoid harmful, unethical, prejudiced, or negative content. Ensure replies promote fairness and positivity. Be concise.`

const errorResponsePrompt = `As an ai chat assistant, your job is to help the user understand and resolve API error messages.
When offering solutions, You will clarify without going into unnecessary detail. You must respond in less than 100 words. 
You should commence by saying "An error occurred while trying to process your request ..." also, if you think it's auth error, ask the user to read this doc https://docs.helix.ml/helix/develop/helix-tools/ (format as markdown)`
