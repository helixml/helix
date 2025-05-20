package controller

import (
	"context"
	"time"

	agent "github.com/helixml/helix/api/pkg/agent"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
)

type runAgentRequest struct {
	Client    oai.Client
	Assistant *types.AssistantConfig
	User      *types.User
	Request   openai.ChatCompletionRequest
	Options   *ChatCompletionOptions
}

func (c *Controller) runAgent(ctx context.Context, req *runAgentRequest) (*openai.ChatCompletionResponse, error) {
	vals, ok := oai.GetContextValues(ctx)
	if !ok {
		vals = &oai.ContextValues{}
	}

	mem := agent.NewDefaultMemory()

	llm := agent.NewLLM(
		req.Client,
		req.Assistant.ReasoningModel,
		req.Assistant.GenerationModel,
		req.Assistant.SmallReasoningModel,
		req.Assistant.SmallGenerationModel,
	)

	helixAgent := agent.NewAgent(
		req.Assistant.SystemPrompt,
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

	session := agent.NewSession(context.Background(), llm, mem, helixAgent, messageHistory, agent.Meta{
		UserID:    req.User.ID,
		SessionID: vals.SessionID,
		Extra:     map[string]string{},
	})

	// Get user message, could be in the part or content
	session.In(getLastMessage(req.Request))

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
