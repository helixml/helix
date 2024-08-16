package controller

import (
	"reflect"
	"testing"

	openai "github.com/lukemarsden/go-openai2"
)

func Test_setSystemPrompt(t *testing.T) {
	type args struct {
		req          *openai.ChatCompletionRequest
		systemPrompt string
	}
	tests := []struct {
		name string
		args args
		want openai.ChatCompletionRequest
	}{
		{
			name: "No system prompt set and no messages",
			args: args{
				req:          &openai.ChatCompletionRequest{},
				systemPrompt: "",
			},
			want: openai.ChatCompletionRequest{},
		},
		{
			name: "System prompt set and message user only",
			args: args{
				req: &openai.ChatCompletionRequest{
					Messages: []openai.ChatCompletionMessage{
						{
							Role:    openai.ChatMessageRoleUser,
							Content: "Hello",
						},
					},
				},
				systemPrompt: "You are a helpful assistant.",
			},
			want: openai.ChatCompletionRequest{
				Messages: []openai.ChatCompletionMessage{
					{
						Role:    openai.ChatMessageRoleSystem,
						Content: "You are a helpful assistant.",
					},
					{
						Role:    openai.ChatMessageRoleUser,
						Content: "Hello",
					},
				},
			},
		},
		{
			name: "System prompt is set and request messages has system prompt",
			args: args{
				req: &openai.ChatCompletionRequest{
					Messages: []openai.ChatCompletionMessage{
						{
							Role:    openai.ChatMessageRoleSystem,
							Content: "Original system prompt",
						},
						{
							Role:    openai.ChatMessageRoleUser,
							Content: "Hello",
						},
					},
				},
				systemPrompt: "New system prompt",
			},
			want: openai.ChatCompletionRequest{
				Messages: []openai.ChatCompletionMessage{
					{
						Role:    openai.ChatMessageRoleSystem,
						Content: "New system prompt",
					},
					{
						Role:    openai.ChatMessageRoleUser,
						Content: "Hello",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := setSystemPrompt(tt.args.req, tt.args.systemPrompt); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("setSystemPrompt() = %v, want %v", got, tt.want)
			}
		})
	}
}
