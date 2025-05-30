package controller

import (
	"context"
	"fmt"
	"time"

	agent "github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/agent/skill"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/openai/transport"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

type runAgentRequest struct {
	Assistant *types.AssistantConfig
	User      *types.User
	Request   openai.ChatCompletionRequest
	Options   *ChatCompletionOptions
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

	// Assemble clients and providers

	reasoningModel, err := c.getLLMModelConfig(ctx,
		req.User.ID,
		withFallbackProvider(req.Assistant.ReasoningModelProvider, req.Assistant), // Defaults to top level assistant provider
		req.Assistant.ReasoningModel)
	if err != nil {
		return nil, fmt.Errorf("failed to get reasoning model config: %w", err)
	}
	reasoningModel.ReasoningEffort = req.Assistant.ReasoningModelEffort

	generationModel, err := c.getLLMModelConfig(ctx,
		req.User.ID,
		withFallbackProvider(req.Assistant.GenerationModelProvider, req.Assistant), // Defaults to top level assistant provider
		req.Assistant.GenerationModel)
	if err != nil {
		return nil, fmt.Errorf("failed to get generation model config: %w", err)
	}

	smallReasoningModel, err := c.getLLMModelConfig(ctx,
		req.User.ID,
		withFallbackProvider(req.Assistant.SmallReasoningModelProvider, req.Assistant), // Defaults to top level assistant provider
		req.Assistant.SmallReasoningModel)
	if err != nil {
		return nil, fmt.Errorf("failed to get small reasoning model config: %w", err)
	}
	smallReasoningModel.ReasoningEffort = req.Assistant.SmallReasoningModelEffort

	smallGenerationModel, err := c.getLLMModelConfig(ctx,
		req.User.ID,
		withFallbackProvider(req.Assistant.SmallGenerationModelProvider, req.Assistant), // Defaults to top level assistant provider
		req.Assistant.SmallGenerationModel)
	if err != nil {
		return nil, fmt.Errorf("failed to get small generation model config: %w", err)
	}

	llm := agent.NewLLM(
		reasoningModel,
		generationModel,
		smallReasoningModel,
		smallGenerationModel,
	)

	enriched, err := renderPrompt(req.Assistant.SystemPrompt, systemPromptValues{
		LocalDate: time.Now().Format("2006-01-02"),
		LocalTime: time.Now().Format("15:04:05"),
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to render system prompt")
	}

	var skills []agent.Skill

	// Get API skills
	for _, assistantTool := range req.Assistant.Tools {
		if assistantTool.ToolType == types.ToolTypeAPI {
			skills = append(skills, skill.NewAPICallingSkill(c.ToolsPlanner, assistantTool))
		}
	}

	helixAgent := agent.NewAgent(
		c.stepInfoEmitter,
		enriched,
		skills,
		req.Assistant.MaxIterations,
	)

	messageHistory := agent.NewMessageList()

	// Add request messages except the last user message
	for _, message := range req.Request.Messages[:len(req.Request.Messages)-1] {
		messageText, err := types.GetMessageText(&message)
		if err != nil {
			log.Error().Any("request", req.Request).Err(err).Msg("failed to get message text")
			continue
		}
		// TODO: multi-content messages
		switch message.Role {
		case openai.ChatMessageRoleUser:
			messageHistory.Add(agent.UserMessage(messageText))
		case openai.ChatMessageRoleSystem, openai.ChatMessageRoleAssistant:
			messageHistory.Add(agent.AssistantMessage(messageText))
		}
	}

	session := agent.NewSession(ctx, c.stepInfoEmitter, llm, mem, helixAgent, messageHistory, agent.Meta{
		UserID:        req.User.ID,
		SessionID:     vals.SessionID,
		InteractionID: vals.InteractionID,
		Extra:         map[string]string{},
	})

	// Get user message, could be in the part or content
	session.In(getLastMessage(req.Request))

	return session, nil
}

func withFallbackProvider(provider string, assistant *types.AssistantConfig) string {
	if provider == "" {
		return assistant.Provider
	}
	return provider
}

func (c *Controller) getLLMModelConfig(ctx context.Context, owner, provider, model string) (*agent.LLMModelConfig, error) {
	client, err := c.providerManager.GetClient(ctx, &manager.GetClientRequest{
		Provider: provider,
		Owner:    owner,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	return &agent.LLMModelConfig{
		Client: client,
		Model:  model,
	}, nil
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
			case agent.ResponseTypeThinkingStart:
				_ = transport.WriteChatCompletionStream(writer, &openai.ChatCompletionStreamResponse{
					Choices: []openai.ChatCompletionStreamChoice{
						{
							Delta: openai.ChatCompletionStreamChoiceDelta{Content: "<think>"},
						},
					},
				})
			case agent.ResponseTypeThinking:
				_ = transport.WriteChatCompletionStream(writer, &openai.ChatCompletionStreamResponse{
					Choices: []openai.ChatCompletionStreamChoice{
						{
							Delta: openai.ChatCompletionStreamChoiceDelta{Content: out.Content},
						},
					},
				})
			case agent.ResponseTypeThinkingEnd:
				_ = transport.WriteChatCompletionStream(writer, &openai.ChatCompletionStreamResponse{
					Choices: []openai.ChatCompletionStreamChoice{
						{
							Delta: openai.ChatCompletionStreamChoiceDelta{Content: "</think>"},
						},
					},
				})
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
