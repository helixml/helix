package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

func (c *ChainStrategy) interpretResponse(ctx context.Context, client oai.Client, sessionID, interactionID string, tool *types.Tool, history []*types.ToolHistoryMessage, resp *http.Response) (*RunActionResponse, error) {
	bts, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return c.handleErrorResponse(ctx, client, sessionID, interactionID, tool, resp.StatusCode, bts)
	}

	return c.handleSuccessResponse(ctx, client, sessionID, interactionID, tool, history, bts)
}

func (c *ChainStrategy) handleSuccessResponse(ctx context.Context, client oai.Client, sessionID, interactionID string, tool *types.Tool, history []*types.ToolHistoryMessage, body []byte) (*RunActionResponse, error) {
	messages := c.prepareSuccessMessages(tool, history, body)
	req := c.prepareChatCompletionRequest(messages, false, tool.Config.API.Model)

	ctx = c.setContextAndStep(ctx, sessionID, interactionID, types.LLMCallStepInterpretResponse)

	resp, err := client.CreateChatCompletion(ctx, req)
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

func (c *ChainStrategy) handleSuccessResponseStream(ctx context.Context, client oai.Client, sessionID, interactionID string, tool *types.Tool, history []*types.ToolHistoryMessage, body []byte) (*openai.ChatCompletionStream, error) {
	messages := c.prepareSuccessMessages(tool, history, body)
	req := c.prepareChatCompletionRequest(messages, true, tool.Config.API.Model)

	ctx = c.setContextAndStep(ctx, sessionID, interactionID, types.LLMCallStepInterpretResponse)

	started := time.Now()

	resp, err := client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get response from inference API: %w", err)
	}

	c.logLLMCallAsync(sessionID, interactionID, started)

	return resp, nil
}

func (c *ChainStrategy) handleErrorResponse(ctx context.Context, client oai.Client, sessionID, interactionID string, tool *types.Tool, statusCode int, body []byte) (*RunActionResponse, error) {
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
	// override with tool model if specified
	if tool.Config.API.Model != "" {
		req.Model = tool.Config.API.Model
	}

	ctx = oai.SetContextValues(ctx, &oai.ContextValues{
		OwnerID:       oai.SystemID,
		SessionID:     sessionID,
		InteractionID: interactionID,
	})

	ctx = oai.SetStep(ctx, &oai.Step{
		Step: types.LLMCallStepInterpretResponse,
	})

	resp, err := client.CreateChatCompletion(ctx, req)
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

func (c *ChainStrategy) interpretResponseStream(ctx context.Context, client oai.Client, sessionID, interactionID string, tool *types.Tool, history []*types.ToolHistoryMessage, resp *http.Response) (*openai.ChatCompletionStream, error) {
	bts, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return c.handleSuccessResponseStream(ctx, client, sessionID, interactionID, tool, history, bts)
	}

	return c.handleSuccessResponseStream(ctx, client, sessionID, interactionID, tool, history, bts)
}

func (c *ChainStrategy) prepareSuccessMessages(tool *types.Tool, history []*types.ToolHistoryMessage, body []byte) []openai.ChatCompletionMessage {
	systemPrompt := successResponsePrompt
	if tool.Config.API.ResponseSuccessTemplate != "" {
		systemPrompt = tool.Config.API.ResponseSuccessTemplate
	}

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemPrompt,
		},
	}

	// Strip system prompts from history because we're adding our own system prompt
	for _, msg := range history {
		if msg.Role != openai.ChatMessageRoleSystem {
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}
	messages = append(messages,
		openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: fmt.Sprintf("Here is the response from the API:\n%s", string(body)),
		},
		openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: systemPrompt,
		},
	)

	return messages
}

func (c *ChainStrategy) prepareChatCompletionRequest(messages []openai.ChatCompletionMessage, stream bool, overrideModel string) openai.ChatCompletionRequest {
	req := openai.ChatCompletionRequest{
		Stream:   stream,
		Model:    c.cfg.Tools.Model,
		Messages: messages,
	}
	if overrideModel != "" {
		req.Model = overrideModel
	}
	return req
}

func (c *ChainStrategy) setContextAndStep(ctx context.Context, sessionID, interactionID string, step types.LLMCallStep) context.Context {
	ctx = oai.SetContextValues(ctx, &oai.ContextValues{
		OwnerID:       oai.SystemID,
		SessionID:     sessionID,
		InteractionID: interactionID,
	})

	return oai.SetStep(ctx, &oai.Step{
		Step: step,
	})
}

func (c *ChainStrategy) logLLMCallAsync(sessionID, interactionID string, started time.Time) {
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
}

const successResponsePrompt = `Now present the response in a non-tech way. If the API response is empty, say that there's nothing of that type available.
Present the key information in a concise manner.
Include relevant details, references, and links if present. Format the summary in Markdown for clarity and readability where appropriate, but don't mention formatting in your response unless it's relevant to the user's query.
Make sure to NEVER mention technical terms like "APIs, JSON, Request, etc..." and use first person pronoun (say it as if you performed the action)`

const errorResponsePrompt = `As an ai chat assistant, your job is to help the user understand and resolve API error messages.
When offering solutions, You will clarify without going into unnecessary detail. You must respond in less than 100 words.
You should commence by saying "An error occurred while trying to process your request ..." also, if you think it's auth error, ask the user to read this doc https://docs.helixml.tech/helix/develop/helix-tools/ (format as markdown)`
