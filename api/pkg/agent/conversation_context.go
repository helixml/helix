package agent

import (
	"fmt"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

// GetMessageText extracts the plain text content from an OpenAI chat message
// of any type (user, assistant, or developer message)
func GetMessageText(message *openai.ChatCompletionMessage) (string, error) {
	if message.Content == "" {
		return "", fmt.Errorf("message content is empty")
	}

	// For simple string content
	if message.Content != "" {
		return message.Content, nil
	}

	// For array of content parts (if supported in the future)
	if len(message.MultiContent) > 0 {
		var builder strings.Builder
		for _, part := range message.MultiContent {
			if part.Type == "text" {
				builder.WriteString(part.Text)
			}
		}
		return builder.String(), nil
	}

	return "", fmt.Errorf("unsupported message type")
}

// CompileConversationHistory builds the message history for the LLM request
// now it fetches the last 5 messages but in the future, we'lll do smart things here like old message summarization etc
// func CompileConversationHistory(meta Meta, storage Storage) (*MessageList, error) {
// 	return storage.GetConversations(meta, 5, 0)
// }
