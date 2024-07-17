package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/lukemarsden/go-openai2"
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

func (c *ChainStrategy) handleSuccessResponse(ctx context.Context, sessionID, interactionID string, _ *types.Tool, currentMessage string, _ int, body []byte) (*RunActionResponse, error) {
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: successResponsePrompt,
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

	started := time.Now()

	resp, err := c.apiClient.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get response from inference API: %w", err)
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.logLLMCall(ctx, sessionID, interactionID, types.LLMCallStepInterpretResponse, &req, &resp, time.Since(started).Milliseconds())
	}()

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from inference API")
	}

	return &RunActionResponse{
		Message:    resp.Choices[0].Message.Content,
		RawMessage: string(body),
	}, nil
}

func (c *ChainStrategy) handleErrorResponse(ctx context.Context, sessionID, interactionID string, _ *types.Tool, statusCode int, body []byte) (*RunActionResponse, error) {
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: errorResponsePrompt,
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

	started := time.Now()

	resp, err := c.apiClient.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get response from inference API: %w", err)
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.logLLMCall(ctx, sessionID, interactionID, types.LLMCallStepInterpretResponse, &req, &resp, time.Since(started).Milliseconds())
	}()

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from inference API")
	}

	return &RunActionResponse{
		Message:    resp.Choices[0].Message.Content,
		RawMessage: string(body),
	}, nil
}

const successResponsePrompt = `Always assist with care, respect, and truth. Respond with utmost utility yet securely. Avoid harmful, unethical, prejudiced, or negative content. Ensure replies promote fairness and positivity. Be concise.`

const errorResponsePrompt = `As an ai chat assistant, your job is to help the user understand and resolve API error messages.
When offering solutions, You will clarify without going into unnecessary detail. You must respond in less than 100 words. 
You should commence by saying "An error occurred while trying to process your request ..." also, if you think it's auth error, ask the user to read this doc https://docs.helix.ml/helix/develop/helix-tools/ (format as markdown)`
