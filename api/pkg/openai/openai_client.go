package openai

import (
	"context"
	"time"

	"github.com/avast/retry-go/v4"
	openai "github.com/lukemarsden/go-openai2"
)

const (
	retries             = 3
	delayBetweenRetries = time.Second
)

//go:generate mockgen -source $GOFILE -destination openai_client_mocks.go -package $GOPACKAGE

type Client interface {
	CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
	CreateChatCompletionStream(ctx context.Context, request openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error)
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
			// Cast into openai.RequestError
			if apiErr, ok := err.(*openai.RequestError); ok {
				if apiErr.HTTPStatusCode == 401 {
					// Do not retry on auth failures
					return retry.Unrecoverable(err)
				}
			}

			return err
		}

		return nil
	},
		retry.Attempts(retries),
		retry.Delay(delayBetweenRetries),
		retry.Context(ctx),
	)

	return
}

func (c *RetryableClient) CreateChatCompletionStream(ctx context.Context, request openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
	return c.apiClient.CreateChatCompletionStream(ctx, request)
}
