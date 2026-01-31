package controller

import (
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

func getLastMessage(req openai.ChatCompletionRequest) string {
	if len(req.Messages) == 0 {
		return ""
	}

	lastMessage := req.Messages[len(req.Messages)-1]

	// Check if the message has MultiContent
	if len(lastMessage.MultiContent) > 0 {
		var textContent strings.Builder
		for _, part := range lastMessage.MultiContent {
			if part.Type == openai.ChatMessagePartTypeText {
				textContent.WriteString(part.Text)
			}
		}
		// If we found text in MultiContent, use it
		if textContent.Len() > 0 {
			return textContent.String()
		}
		// If MultiContent exists but has no text, fall back to Content field
	}

	// Use the regular Content field
	return lastMessage.Content
}
