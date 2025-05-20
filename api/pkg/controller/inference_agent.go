package controller

import (
	"context"
	"fmt"
	"time"

	agent "github.com/helixml/helix/api/pkg/agent"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/transport"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

type runAgentRequest struct {
	Client    oai.Client
	Assistant *types.AssistantConfig
	User      *types.User
	Request   openai.ChatCompletionRequest
	Options   *ChatCompletionOptions
}

func (c *Controller) runAgentBlocking(ctx context.Context, req *runAgentRequest) (*openai.ChatCompletionResponse, error) {
	session, err := c.runAgent(ctx, req)
	if err != nil {
		return nil, err
	}

	var response string
	for {
		out := session.Out()

		if out.Type == agent.ResponseTypePartialText {
			response += out.Content
		}
		if out.Type == agent.ResponseTypeEnd {
			break
		}
	}

	vals, ok := oai.GetContextValues(ctx)
	if !ok {
		vals = &oai.ContextValues{}
	}

	return &openai.ChatCompletionResponse{
		ID:      vals.InteractionID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Content: response,
				},
			},
		},
	}, nil
}

func (c *Controller) runAgentStream(ctx context.Context, req *runAgentRequest) (*openai.ChatCompletionStream, error) {
	session, err := c.runAgent(ctx, req)
	if err != nil {
		return nil, err
	}

	stream, writer, err := transport.NewOpenAIStreamingAdapter(req.Request)
	if err != nil {
		return nil, fmt.Errorf("failed to create openai streaming adapter: %w", err)
	}

	go func() {
		defer writer.Close()
		for {
			out := session.Out()

			switch out.Type {
			case agent.ResponseTypePartialText:
				_ = transport.WriteChatCompletionStream(writer, &openai.ChatCompletionStreamResponse{
					Choices: []openai.ChatCompletionStreamChoice{
						{
							Delta: openai.ChatCompletionStreamChoiceDelta{Content: out.Content},
						},
					},
				})

			case agent.ResponseTypeError:
				_ = transport.WriteChatCompletionStream(writer, &openai.ChatCompletionStreamResponse{
					Choices: []openai.ChatCompletionStreamChoice{
						{
							Delta: openai.ChatCompletionStreamChoiceDelta{Content: fmt.Sprintf("Agent error: %s", out.Content)},
						},
					},
				})
			case agent.ResponseTypeEnd:
				// Write the final chunk with reason stop
				_ = transport.WriteChatCompletionStream(writer, &openai.ChatCompletionStreamResponse{
					Choices: []openai.ChatCompletionStreamChoice{
						{
							FinishReason: openai.FinishReasonStop,
						},
					},
				})
				return
			}
		}
	}()

	return stream, nil
}

func (c *Controller) runAgent(ctx context.Context, req *runAgentRequest) (*agent.Session, error) {
	vals, ok := oai.GetContextValues(ctx)
	if !ok {
		vals = &oai.ContextValues{}
	}

	log.Info().
		Str("session_id", vals.SessionID).
		Str("user_id", req.User.ID).
		Str("interaction_id", vals.InteractionID).
		Msg("Running agent")

	mem := agent.NewDefaultMemory()

	llm := agent.NewLLM(
		req.Client,
		req.Assistant.ReasoningModel,
		req.Assistant.GenerationModel,
		req.Assistant.SmallReasoningModel,
		req.Assistant.SmallGenerationModel,
	)

	enriched, err := renderPrompt(req.Assistant.SystemPrompt, systemPromptValues{
		LocalDate: time.Now().Format("2006-01-02"),
		LocalTime: time.Now().Format("15:04:05"),
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to render system prompt")
	}

	helixAgent := agent.NewAgent(
		enriched,
		[]agent.Skill{},
	)

	messageHistory := agent.NewMessageList()

	// Add request messages
	for _, message := range req.Request.Messages {
		switch message.Role {
		case openai.ChatMessageRoleUser:
			messageHistory.Add(agent.UserMessage(message.Content))
		case openai.ChatMessageRoleSystem, openai.ChatMessageRoleAssistant:
			messageHistory.Add(agent.AssistantMessage(message.Content))
		}
	}

	session := agent.NewSession(ctx, llm, mem, helixAgent, messageHistory, agent.Meta{
		UserID:    req.User.ID,
		SessionID: vals.SessionID,
		Extra:     map[string]string{},
	})

	// Get user message, could be in the part or content
	session.In(getLastMessage(req.Request))

	return session, nil
}
