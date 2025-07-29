package controller

import (
	openai "github.com/sashabaranov/go-openai"
)

func getLastMessage(req openai.ChatCompletionRequest) string {
	if len(req.Messages) > 0 {
		lastMessage := req.Messages[len(req.Messages)-1]
		// Prioritize multi-content messages
		if len(lastMessage.MultiContent) > 0 {
			// Find the first text message
			for _, content := range lastMessage.MultiContent {
				if content.Type == openai.ChatMessagePartTypeText {
					return content.Text
				}
			}
		}

		return req.Messages[len(req.Messages)-1].Content
	}

	return ""
}
