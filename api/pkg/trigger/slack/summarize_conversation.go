package slack

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/slack-go/slack"

	"github.com/helixml/helix/api/pkg/controller"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/types"
)

func (s *SlackBot) summarizeConversation(ctx context.Context, user *types.User, session *types.Session, interactionID string, messages []slack.Message) (string, error) {
	summaryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	summaryCtx = oai.SetContextValues(summaryCtx, &oai.ContextValues{
		OwnerID:       session.Owner,
		SessionID:     session.ID,
		InteractionID: interactionID,
	})

	summaryCtx = oai.SetStep(summaryCtx, &oai.Step{
		Step: types.LLMCallStepSummarizeConversation,
	})

	summaryCtx = oai.SetContextAppID(summaryCtx, s.app.ID)
	summaryCtx = oai.SetContextOrganizationID(summaryCtx, session.OrganizationID)

	conversationHistory := buildConversationHistory(messages, s.botUserID, s.botID)
	if strings.TrimSpace(conversationHistory) == "" {
		return "", errors.New("thread has no text messages to summarize")
	}

	if len(s.app.Config.Helix.Assistants) == 0 {
		return "", fmt.Errorf("app %s has no assistants configured", s.app.ID)
	}

	var (
		provider string
		model    string
	)

	if s.app.Config.Helix.Assistants[0].AgentMode && s.app.Config.Helix.Assistants[0].SmallGenerationModel != "" {
		model = s.app.Config.Helix.Assistants[0].SmallGenerationModel
		provider = s.app.Config.Helix.Assistants[0].SmallGenerationModelProvider
	} else {
		model = s.app.Config.Helix.Assistants[0].Model
		provider = s.app.Config.Helix.Assistants[0].Provider
	}

	systemPrompt := `You are a helpful assistant that summarizes a conversation. Be concise and to the point, focus on details. Return the summary in the same language as the conversation
The format should be in markdown:
# Conversation so far
User: <user message>
Human operator: <human message>
Bot: <bot message>`

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

	options := &controller.ChatCompletionOptions{
		OrganizationID: s.app.OrganizationID,
		Provider:       provider,
	}

	resp, _, err := s.controller.ChatCompletion(summaryCtx, user, req, options)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", errors.New("no data in the LLM response")
	}

	return resp.Choices[0].Message.Content, nil
}

func buildConversationHistory(messages []slack.Message, botUserID, botID string) string {
	var conversationHistory strings.Builder

	for _, message := range messages {
		content := strings.TrimSpace(message.Text)
		if content == "" {
			continue
		}

		switch {
		case message.User == botUserID:
			conversationHistory.WriteString(fmt.Sprintf("Bot: %s\n", content))
		case botID != "" && message.BotID == botID:
			conversationHistory.WriteString(fmt.Sprintf("Bot: %s\n", content))
		case message.SubType == "bot_message" || message.BotID != "":
			conversationHistory.WriteString(fmt.Sprintf("Bot: %s\n", content))
		default:
			conversationHistory.WriteString(fmt.Sprintf("User: %s\n", content))
		}
	}

	return conversationHistory.String()
}
