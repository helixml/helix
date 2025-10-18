package crisp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/crisp-im/go-crisp-api/crisp/v3"
	openai "github.com/sashabaranov/go-openai"

	"github.com/helixml/helix/api/pkg/controller"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/ptr"
	"github.com/helixml/helix/api/pkg/types"
)

// TODO: potentially move into shared package if another trigger needs it

// summarizeConversation takes the conversation history and summarizes it into a single interaction
// using a small generation model that will then be used as a context to the main prompt. In Crisp,
// this is done because we might have a conversation between 3 parties: user, helix and human operator
func (c *CrispBot) summarizeConversation(user *types.User, session *types.Session, interactionID string, messages []crisp.ConversationMessage) (string, error) {

	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()

	ctx = oai.SetContextValues(ctx, &oai.ContextValues{
		OwnerID:       session.Owner,
		SessionID:     session.ID,
		InteractionID: interactionID,
	})

	ctx = oai.SetStep(ctx, &oai.Step{
		Step: types.LLMCallStepSummarizeConversation,
	})

	ctx = oai.SetContextAppID(ctx, c.app.ID)
	ctx = oai.SetContextOrganizationID(ctx, session.OrganizationID)

	conversationHistory := buildConversationHistory(messages)

	var (
		provider string
		model    string
	)

	// Use small generation model if agent mode is enabled
	if c.app.Config.Helix.Assistants[0].AgentMode && c.app.Config.Helix.Assistants[0].SmallGenerationModel != "" {
		model = c.app.Config.Helix.Assistants[0].SmallGenerationModel
		provider = c.app.Config.Helix.Assistants[0].SmallGenerationModelProvider
	} else {
		model = c.app.Config.Helix.Assistants[0].Model
		provider = c.app.Config.Helix.Assistants[0].Provider
	}

	systemPrompt := `You are a helpful assistant that summarizes a conversation. Be concise and to the point, focus on details. Return the summary in the same language as the conversation
		The format should be in markdown: 
		# Conversation so far
		User: <user message>
		Human operator: <bot message>
		User: <user message>
		Bot: <bot message>
		`

	promptMessage := fmt.Sprintf("summarize the following conversation:\n\n%s", conversationHistory)

	req := openai.ChatCompletionRequest{
		Model: model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemPrompt,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: promptMessage,
			},
		},
	}

	// Do not set app ID, we are running plain inference
	options := &controller.ChatCompletionOptions{
		OrganizationID: c.app.OrganizationID,
		Provider:       provider,
	}

	resp, _, err := c.controller.ChatCompletion(ctx, user, req, options)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", errors.New("no data in the LLM response")
	}

	return resp.Choices[0].Message.Content, nil
}

func buildConversationHistory(messages []crisp.ConversationMessage) string {
	conversationHistory := ""

	for _, message := range messages {
		if ptr.From(message.Type) != "text" {
			continue
		}

		content, ok := ptr.From(message.Content).(string)
		if !ok {
			continue
		}

		switch {
		case message.Automated != nil && *message.Automated:
			conversationHistory += fmt.Sprintf("Bot: %s\n", content)
		case message.From != nil && *message.From == "operator":
			conversationHistory += fmt.Sprintf("Human operator: %s\n", content)
		default:
			conversationHistory += fmt.Sprintf("User: %s\n", content)
		}
	}

	return conversationHistory
}
