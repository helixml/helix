package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_SessionChatRequest_PlainString(t *testing.T) {
	request := &SessionChatRequest{
		Messages: []*Message{
			{
				Content: MessageContent{
					Parts: []any{"Hello, world!"},
				},
			},
		},
	}

	message, ok := request.Message()
	require.True(t, ok)
	require.Equal(t, "Hello, world!", message)
}

func Test_SessionChatRequest_TextStructure(t *testing.T) {
	request := &SessionChatRequest{
		Messages: []*Message{
			{
				Content: MessageContent{
					Parts: []any{TextPart{
						Type: "text",
						Text: "Hello, world!",
					}},
				},
			},
		},
	}

	message, ok := request.Message()
	require.True(t, ok)
	require.Equal(t, "Hello, world!", message)
}

func Test_SessionChatRequest_ImageURL_Only(t *testing.T) {
	request := &SessionChatRequest{
		Messages: []*Message{
			{
				Content: MessageContent{
					Parts: []any{
						ImageURLPart{
							Type:     "image_url",
							ImageURL: ImageURLData{URL: "https://example.com/image.png"},
						},
					},
				},
			},
		},
	}

	message, ok := request.Message()
	require.True(t, ok)
	require.Equal(t, "", message)
}

func Test_SessionChatRequest_ImageURL_AndText(t *testing.T) {
	request := &SessionChatRequest{
		Messages: []*Message{
			{
				Content: MessageContent{
					Parts: []any{
						ImageURLPart{
							Type:     "image_url",
							ImageURL: ImageURLData{URL: "https://example.com/image.png"},
						},
						TextPart{
							Type: "text",
							Text: "Hello, world!",
						},
					},
				},
			},
		},
	}

	message, ok := request.Message()
	require.True(t, ok)
	require.Equal(t, "Hello, world!", message)
}

func Test_GetMessageMultiContentPart_TextOnly(t *testing.T) {
	interaction := &Interaction{
		Content: MessageContent{
			Parts: []any{TextPart{
				Type: "text",
				Text: "Hello, world!",
			}},
		},
	}

	parts := interaction.GetMessageMultiContentPart()
	require.Equal(t, 1, len(parts))
	require.Equal(t, "text", string(parts[0].Type))
	require.Equal(t, "Hello, world!", parts[0].Text)
}

func Test_GetMessageMultiContentPart_WithImageURL(t *testing.T) {
	interaction := &Interaction{
		Content: MessageContent{
			Parts: []any{TextPart{
				Type: "text",
				Text: "Hello, world!",
			},
				ImageURLPart{
					Type:     "image_url",
					ImageURL: ImageURLData{URL: "https://example.com/image.png"},
				},
			},
		},
	}

	parts := interaction.GetMessageMultiContentPart()
	require.Equal(t, 2, len(parts))
	require.Equal(t, "text", string(parts[0].Type))
	require.Equal(t, "Hello, world!", parts[0].Text)
	require.Equal(t, "image_url", string(parts[1].Type))
	require.Equal(t, "https://example.com/image.png", parts[1].ImageURL.URL)
}
