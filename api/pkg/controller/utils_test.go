package controller

import (
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

func Test_getLastMessage_NormalContentField(t *testing.T) {
	req := openai.ChatCompletionRequest{
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    "user",
				Content: "Hello",
			},
			{
				Role:    "assistant",
				Content: "Hi there",
			},
			{
				Role:    "user",
				Content: "This is the last message",
			},
		},
	}

	result := getLastMessage(req)
	expected := "This is the last message"

	if result != expected {
		t.Errorf("Expected %q but got %q", expected, result)
	}
}

func Test_getLastMessage_MultiContentField(t *testing.T) {
	req := openai.ChatCompletionRequest{
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    "user",
				Content: "Hello",
			},
			{
				Role:    "assistant",
				Content: "Hi there",
			},
			{
				Role: "user",
				MultiContent: []openai.ChatMessagePart{
					{
						Type: openai.ChatMessagePartTypeImageURL,
						ImageURL: &openai.ChatMessageImageURL{
							URL: "http://example.com/image.jpg",
						},
					},
					{
						Type: openai.ChatMessagePartTypeText,
						Text: "This is the text content",
					},
				},
			},
		},
	}

	result := getLastMessage(req)
	expected := "This is the text content"

	if result != expected {
		t.Errorf("Expected %q but got %q", expected, result)
	}
}

func Test_getLastMessage_MultiContentField_NoText(t *testing.T) {
	req := openai.ChatCompletionRequest{
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    "user",
				Content: "Hello",
			},
			{
				Role:    "assistant",
				Content: "Hi there",
			},
			{
				Role:    "user",
				Content: "Fallback content",
				MultiContent: []openai.ChatMessagePart{
					{
						Type: openai.ChatMessagePartTypeImageURL,
						ImageURL: &openai.ChatMessageImageURL{
							URL: "http://example.com/image1.jpg",
						},
					},
					{
						Type: openai.ChatMessagePartTypeImageURL,
						ImageURL: &openai.ChatMessageImageURL{
							URL: "http://example.com/image2.jpg",
						},
					},
				},
			},
		},
	}

	result := getLastMessage(req)
	expected := "Fallback content"

	if result != expected {
		t.Errorf("Expected %q but got %q", expected, result)
	}
}

func Test_getLastMessage_NoMessages(t *testing.T) {
	req := openai.ChatCompletionRequest{
		Messages: []openai.ChatCompletionMessage{},
	}

	result := getLastMessage(req)
	expected := ""

	if result != expected {
		t.Errorf("Expected empty string but got %q", result)
	}
}
