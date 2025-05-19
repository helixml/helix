// Start of Selection
package agent

import (
	"fmt"

	"github.com/openai/openai-go"
)

// TODO Remove all three and use openai functions directly
func UserMessage(content string) openai.ChatCompletionMessageParamUnion {
	return openai.UserMessage(content)
}

func AssistantMessage(content string) openai.ChatCompletionMessageParamUnion {
	return openai.AssistantMessage(content)
}

func DeveloperMessage(content string) openai.ChatCompletionMessageParamUnion {
	return openai.DeveloperMessage(content)
}

// MessageList holds an ordered collection of LLMMessage to preserve the history.
type MessageList struct {
	Messages []openai.ChatCompletionMessageParamUnion
}

func NewMessageList() *MessageList {
	return &MessageList{
		Messages: []openai.ChatCompletionMessageParamUnion{},
	}
}

func (ml *MessageList) Len() int {
	return len(ml.Messages)
}

// Add appends one or more new messages to the MessageList in a FIFO order.
func (ml *MessageList) Add(msgs ...openai.ChatCompletionMessageParamUnion) {
	ml.Messages = append(ml.Messages, msgs...)
}

func (ml *MessageList) AddFirst(prompt string) {
	ml.Messages = append([]openai.ChatCompletionMessageParamUnion{DeveloperMessage(prompt)}, ml.Messages...)
}

func (ml *MessageList) ReplaceAt(index int, newMsg openai.ChatCompletionMessageParamUnion) error {
	if index < 0 || index >= len(ml.Messages) {
		return fmt.Errorf("index out of range")
	}
	ml.Messages[index] = newMsg
	return nil
}

func (ml *MessageList) All() []openai.ChatCompletionMessageParamUnion {
	return ml.Messages
}

func (ml *MessageList) Clone() *MessageList {
	return &MessageList{
		Messages: append([]openai.ChatCompletionMessageParamUnion{}, ml.Messages...),
	}
}

func (ml *MessageList) Clear() {
	ml.Messages = []openai.ChatCompletionMessageParamUnion{}
}

// PrintMessages is for debugging purposes
func (ml *MessageList) PrintMessages() {
	for _, msg := range ml.Messages {
		role := "unknown"
		content := ""

		switch {
		case msg.OfUser != nil:
			role = "user"
			if !msg.OfUser.Content.OfString.IsOmitted() {
				content = msg.OfUser.Content.OfString.String()
			}
		case msg.OfAssistant != nil:
			role = "assistant"
			if !msg.OfAssistant.Content.OfString.IsOmitted() {
				content = msg.OfAssistant.Content.OfString.String()
			}
			// Print tool calls if they exist
			if len(msg.OfAssistant.ToolCalls) > 0 {
				content += "\nTool Calls:"
				for _, toolCall := range msg.OfAssistant.ToolCalls {
					content += fmt.Sprintf("\n- Function: %s", toolCall.Function.Name)
					content += fmt.Sprintf("\n  Arguments: %s", toolCall.Function.Arguments)
				}
			}
		case msg.OfDeveloper != nil:
			role = "developer"
			if !msg.OfDeveloper.Content.OfString.IsOmitted() {
				content = msg.OfDeveloper.Content.OfString.String()
			}
		case msg.OfTool != nil:
			role = "tool"
			if !msg.OfTool.Content.OfString.IsOmitted() {
				content = msg.OfTool.Content.OfString.String()
			}
		}

		fmt.Printf("Role: %s\nContent: %s\n\n", role, content)
	}
}
