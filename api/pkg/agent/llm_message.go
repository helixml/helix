// Start of Selection
package agent

import (
	"fmt"

	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

// TODO Remove all three and use openai functions directly
func UserMessage(content string) *openai.ChatCompletionMessage {
	return &openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: content,
	}
}

func AssistantMessage(content string) *openai.ChatCompletionMessage {
	return &openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: content,
	}
}

func SystemMessage(content string) *openai.ChatCompletionMessage {
	return &openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: content,
	}
}

func DeveloperMessage(content string) *openai.ChatCompletionMessage {
	return &openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleDeveloper,
		Content: content,
	}
}

// MessageList holds an ordered collection of LLMMessage to preserve the history.
type MessageList struct {
	Messages []*openai.ChatCompletionMessage
}

func NewMessageList() *MessageList {
	return &MessageList{
		Messages: []*openai.ChatCompletionMessage{},
	}
}

func (ml *MessageList) Len() int {
	return len(ml.Messages)
}

// Add appends one or more new messages to the MessageList in a FIFO order.
func (ml *MessageList) Add(msgs ...*openai.ChatCompletionMessage) {
	ml.Messages = append(ml.Messages, msgs...)
}

func (ml *MessageList) AddFirst(prompt string) {

	ml.Messages = append([]*openai.ChatCompletionMessage{DeveloperMessage(prompt)}, ml.Messages...)
}

func (ml *MessageList) ReplaceAt(index int, newMsg *openai.ChatCompletionMessage) error {
	if index < 0 || index >= len(ml.Messages) {
		return fmt.Errorf("index out of range")
	}
	ml.Messages[index] = newMsg
	return nil
}

func (ml *MessageList) All() []openai.ChatCompletionMessage {
	var result []openai.ChatCompletionMessage
	for _, msg := range ml.Messages {
		result = append(result, *msg)
	}
	return result
}

func (ml *MessageList) Clone() *MessageList {
	return &MessageList{
		Messages: append([]*openai.ChatCompletionMessage{}, ml.Messages...),
	}
}

func (ml *MessageList) Clear() {
	ml.Messages = []*openai.ChatCompletionMessage{}
}

// PrintMessages is for debugging purposes
func (ml *MessageList) PrintMessages() {
	for _, msg := range ml.Messages {
		role := msg.Role
		content := msg.Content
		multiContent := msg.MultiContent

		// Print tool calls if they exist
		if len(msg.ToolCalls) > 0 {
			content += "\nTool Calls:"
			for _, toolCall := range msg.ToolCalls {
				content += fmt.Sprintf("\n- Function: %s", toolCall.Function.Name)
				content += fmt.Sprintf("\n  Arguments: %s", toolCall.Function.Arguments)
			}
		}

		log.Info().Msgf("Role: %s\nContent: %s\nMultiContent: %v\n\n", role, content, multiContent)
	}
}
