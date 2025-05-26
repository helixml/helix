package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

const (
	retries             = 3
	delayBetweenRetries = time.Second

	embeddingRetries = 5
	embeddingDelay   = 3 * time.Second
)

//go:generate mockgen -source $GOFILE -destination openai_client_mocks.go -package $GOPACKAGE

type Client interface {
	CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
	CreateChatCompletionStream(ctx context.Context, request openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error)

	ListModels(ctx context.Context) ([]types.OpenAIModel, error)

	CreateEmbeddings(ctx context.Context, request openai.EmbeddingRequest) (resp openai.EmbeddingResponse, err error)

	APIKey() string
}

// New creates a new OpenAI client with the given API key and base URL.
// If models are provided, models will be filtered to only include the provided models.
func New(apiKey string, baseURL string, models ...string) *RetryableClient {
	config := openai.DefaultConfig(apiKey)
	config.BaseURL = baseURL

	// Create a custom HTTP client with increased timeout
	httpClient := &http.Client{
		Timeout: 5 * time.Minute, // 5 minute timeout for embedding requests
	}

	// Use our interceptor with the custom timeout
	config.HTTPClient = &openAIClientInterceptor{
		Client: *httpClient,
	}

	client := openai.NewClientWithConfig(config)

	return &RetryableClient{
		apiClient:  client,
		httpClient: httpClient,
		baseURL:    baseURL,
		apiKey:     apiKey,
		models:     models,
	}
}

type RetryableClient struct {
	apiClient *openai.Client

	httpClient *http.Client
	baseURL    string
	apiKey     string
	models     []string
}

// APIKey - returns the API key used by the client, used for testing
func (c *RetryableClient) APIKey() string {
	return c.apiKey
}

func (c *RetryableClient) CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (resp openai.ChatCompletionResponse, err error) {
	if err := c.validateModel(request.Model); err != nil {
		return openai.ChatCompletionResponse{}, err
	}

	// Perform request with retries
	err = retry.Do(func() error {
		resp, err = c.apiClient.CreateChatCompletion(ctx, request)
		if err != nil {
			if strings.Contains(err.Error(), "401 Unauthorized") || strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "400") {
				return retry.Unrecoverable(err)
			}

			return err
		}

		return nil
	},
		retry.Attempts(retries),
		retry.Delay(delayBetweenRetries),
		retry.LastErrorOnly(true),
		retry.Context(ctx),
	)

	return
}

func (c *RetryableClient) CreateChatCompletionStream(ctx context.Context, request openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
	if err := c.validateModel(request.Model); err != nil {
		return nil, err
	}

	return c.apiClient.CreateChatCompletionStream(ctx, request)
}

func (c *RetryableClient) validateModel(model string) error {
	if len(c.models) > 0 {
		if !slices.Contains(c.models, model) {
			return fmt.Errorf("model %s is not in the list of allowed models", model)
		}
	}

	return nil
}

// TODO: just use OpenAI client's ListModels function and separate this from TogetherAI
func (c *RetryableClient) ListModels(ctx context.Context) ([]types.OpenAIModel, error) {
	url := c.baseURL + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request to provider's models endpoint: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to provider's models endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get models from provider: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response from provider's models endpoint: %w", err)
	}

	// Log the response body for debugging purposes
	log.Trace().Str("base_url", c.baseURL).Msg("Response from provider's models endpoint")
	log.Trace().RawJSON("response_body", body).Msg("Models response")

	var models []types.OpenAIModel
	var rawResponse struct {
		Data []types.OpenAIModel `json:"data"`
	}
	err = json.Unmarshal(body, &rawResponse)
	if err == nil && len(rawResponse.Data) > 0 {
		models = rawResponse.Data
	} else {
		// If unmarshaling into the struct with "data" field fails, try unmarshaling directly into the slice
		// This is how together.ai returns their models
		err = json.Unmarshal(body, &models)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal response from provider's models endpoint (%s): %w, %s", url, err, string(body))
		}
	}

	// Remove audio, tts models
	models = filterUnsupportedModels(models)

	// Sort models: llama-33-70b-instruct models first, then other llama models, then meta-llama/* models, then the rest
	sort.Slice(models, func(i, j int) bool {
		// First priority: any XXX/llama-33-70b-instruct model
		iIsLlama33_70b := strings.Contains(strings.ToLower(models[i].ID), "llama-33-70b-instruct")
		jIsLlama33_70b := strings.Contains(strings.ToLower(models[j].ID), "llama-33-70b-instruct")

		if iIsLlama33_70b && !jIsLlama33_70b {
			return true
		}
		if !iIsLlama33_70b && jIsLlama33_70b {
			return false
		}

		// Second priority: meta-llama/Llama-3.3-70B-Instruct-Turbo
		if models[i].ID == "meta-llama/Llama-3.3-70B-Instruct-Turbo" {
			return true
		}
		if models[j].ID == "meta-llama/Llama-3.3-70B-Instruct-Turbo" {
			return false
		}

		// Third priority: Meta-Llama 3.1 models
		iIsMetaLlama31 := strings.HasPrefix(models[i].ID, "meta-llama/Meta-Llama-3.1")
		jIsMetaLlama31 := strings.HasPrefix(models[j].ID, "meta-llama/Meta-Llama-3.1")

		if iIsMetaLlama31 && !jIsMetaLlama31 {
			return true
		}
		if !iIsMetaLlama31 && jIsMetaLlama31 {
			return false
		}

		// Fourth priority: Other Meta-Llama models
		iIsMetaLlama := strings.HasPrefix(models[i].ID, "meta-llama/")
		jIsMetaLlama := strings.HasPrefix(models[j].ID, "meta-llama/")

		if iIsMetaLlama && !jIsMetaLlama {
			return true
		}
		if !iIsMetaLlama && jIsMetaLlama {
			return false
		}

		// Fifth priority: any llama prefix model
		iIsLlama := strings.Contains(strings.ToLower(models[i].ID), "llama")
		jIsLlama := strings.Contains(strings.ToLower(models[j].ID), "llama")

		if iIsLlama && !jIsLlama {
			return true
		}
		if !iIsLlama && jIsLlama {
			return false
		}

		// Finally: alphabetical order
		return models[i].ID < models[j].ID
	})

	// Hack to workaround that OpenAI returns models like dall-e-3, which we
	// can't send chat completion requests to. Use a rough heuristic to
	// filter out models we can't use.

	// Check if any model starts with "gpt-"
	hasGPTModel := false
	for _, m := range models {
		if strings.HasPrefix(m.ID, "gpt-") {
			hasGPTModel = true
			break
		}
	}

	// If there's a GPT model, filter out non-GPT models
	if hasGPTModel {
		filteredModels := make([]types.OpenAIModel, 0)
		for _, m := range models {
			// gpt, o3, o1, etc
			if strings.HasPrefix(m.ID, "gpt-") || strings.HasPrefix(m.ID, "o3") || strings.HasPrefix(m.ID, "o1") || strings.HasPrefix(m.ID, "o4") {
				// Add the type chat. This is needed
				// for UI to correctly allow filtering
				m.Type = "chat"

				// Set the context length
				m.ContextLength = getOpenAIModelContextLength(m.ID)

				filteredModels = append(filteredModels, m)
			}
		}
		models = filteredModels
	}

	// Set the enabled field to true if the model is in the list of allowed models
	for i := range models {
		models[i].Enabled = modelEnabled(models[i], c.models)
	}

	return models, nil
}

func (c *RetryableClient) CreateEmbeddings(ctx context.Context, request openai.EmbeddingRequest) (resp openai.EmbeddingResponse, err error) {
	// Perform request with retries
	err = retry.Do(func() error {
		resp, err = c.apiClient.CreateEmbeddings(ctx, request)
		if err != nil {
			if strings.Contains(err.Error(), "401 Unauthorized") {
				return retry.Unrecoverable(err)
			}
			return err
		}
		return nil
	},
		retry.Attempts(embeddingRetries),
		retry.Delay(embeddingDelay),
		retry.Context(ctx),
	)

	return
}

type openAIClientInterceptor struct {
	http.Client
}

// Do intercepts requests to the OpenAI API and modifies the body to be compatible with TogetherAI,
// or others
func (c *openAIClientInterceptor) Do(req *http.Request) (*http.Response, error) {
	if req.URL.Host == "api.together.xyz" && req.URL.Path == "/v1/embeddings" && req.Body != nil {
		// Parse the original embedding request body
		embeddingRequest := openai.EmbeddingRequest{}
		if err := json.NewDecoder(req.Body).Decode(&embeddingRequest); err != nil {
			return nil, fmt.Errorf("failed to decode embedding request body: %w", err)
		}
		req.Body.Close()

		// Create a new request with the modified body
		type togetherEmbeddingRequest struct {
			Model string `json:"model"`
			Input any    `json:"input"`
		}
		togetherRequest := togetherEmbeddingRequest{
			Model: string(embeddingRequest.Model),
			Input: embeddingRequest.Input,
		}

		newBytes, err := json.Marshal(togetherRequest)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal together embedding request: %w", err)
		}

		newReq, err := http.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), bytes.NewBuffer(newBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to create new request: %w", err)
		}
		newReq.Header = req.Header

		return c.Client.Do(newReq)
	}

	return c.Client.Do(req)
}
