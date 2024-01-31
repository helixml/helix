package openai

import (
	"context"
	"time"

	"github.com/avast/retry-go/v4"
	openai "github.com/sashabaranov/go-openai"
)

type Client interface {
	CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

func New(apiKey string, baseURL string) *RetryableClient {
	config := openai.DefaultConfig(apiKey)
	config.BaseURL = baseURL

	client := openai.NewClientWithConfig(config)

	return &RetryableClient{
		apiClient: client,
	}
}

type RetryableClient struct {
	apiClient *openai.Client
}

func (c *RetryableClient) CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (resp openai.ChatCompletionResponse, err error) {
	// Perform request with retries
	err = retry.Do(func() error {
		resp, err = c.apiClient.CreateChatCompletion(ctx, request)
		if err != nil {
			return err
		}

		return nil
	},
		retry.Attempts(3),
		retry.Delay(time.Second),
		retry.Context(ctx),
	)

	return
}
