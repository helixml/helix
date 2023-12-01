package text

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/lukemarsden/helix/api/pkg/system"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

// a generic openai based data prep module that can use any model
type DataOpenAIGPT struct {
	Options           DataPrepTextOptions
	client            *openai.Client
	model             string
	getSystemPromptFn func(chunk string, options DataPrepTextOptions) string
	getUserPromptFn   func(chunk string, options DataPrepTextOptions) string
	parseResponseFn   func(answer string, options DataPrepTextOptions) ([]types.DataPrepTextQuestion, error)
}

func NewDataOpenAIGPT(
	options DataPrepTextOptions,
	model string,
	getSystemPromptFn func(chunk string, options DataPrepTextOptions) string,
	getUserPromptFn func(chunk string, options DataPrepTextOptions) string,
	parseResponseFn func(answer string, options DataPrepTextOptions) ([]types.DataPrepTextQuestion, error),
) (*DataOpenAIGPT, error) {
	return &DataOpenAIGPT{
		Options:           options,
		client:            openai.NewClient(options.APIKey),
		model:             model,
		getUserPromptFn:   getUserPromptFn,
		getSystemPromptFn: getSystemPromptFn,
		parseResponseFn:   parseResponseFn,
	}, nil
}

func (gpt *DataOpenAIGPT) GetConcurrency() int {
	return 5
}

func (gpt *DataOpenAIGPT) ConvertChunk(chunk string, index int) ([]types.DataPrepTextQuestion, error) {
	// use the data prep module to convert raw text into QA pairs

	// a rough rate limiter
	time.Sleep(2 * time.Second * time.Duration(index%gpt.Options.Concurrency))

	systemPrompt := gpt.getSystemPromptFn(chunk, gpt.Options)
	userPrompt := gpt.getUserPromptFn(chunk, gpt.Options)

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemPrompt,
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: userPrompt,
		},
	}

	clientOptions := system.ClientOptions{
		Host:  "https://api.openai.com",
		Token: gpt.Options.APIKey,
	}

	postData := openai.ChatCompletionRequest{
		Model:       gpt.model,
		Messages:    messages,
		Temperature: gpt.Options.Temperature,
	}

	log.Trace().
		Msgf("🔴🔴🔴 GPT Question: %+v", postData)

	dataBytes, err := json.Marshal(postData)
	if err != nil {
		return nil, fmt.Errorf("error serializing JSON: %s", err.Error())
	}

	req, err := retryablehttp.NewRequest("POST", system.URL(clientOptions, "/v1/chat/completions"), dataBytes)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-type", "application/json")
	err = system.AddAuthHeadersRetryable(req, clientOptions.Token)
	if err != nil {
		return nil, err
	}

	client := system.NewRetryClient()
	client.RequestLogHook = func(logger retryablehttp.Logger, req *http.Request, retry int) {
		log.Error().Msgf("Retrying request: %s (retry #%d)", req.URL, retry)
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Msgf("GPT running request: %s", err.Error())
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Msgf("GPT reading body: %s", err.Error())
		return nil, err
	}

	log.Trace().
		Msgf("🔴🔴🔴 GPT Answer (%d): %+v", resp.StatusCode, string(body))

	if resp.StatusCode >= 400 {
		log.Error().Msgf("GPT bad status code: %d", resp.StatusCode)
		return nil, fmt.Errorf(string(body))
	}

	var openAIResponse openai.ChatCompletionResponse
	err = json.Unmarshal(body, &openAIResponse)
	if err != nil {
		log.Error().Msgf("GPT Error parsing JSON: %s", err.Error())
		return nil, fmt.Errorf("error parsing JSON: %s", err.Error())
	}

	conversation, err := gpt.parseResponseFn(openAIResponse.Choices[0].Message.Content, gpt.Options)
	if err != nil {
		return nil, err
	}

	return conversation, nil
}

// Compile-time interface check:
var _ DataPrepTextQuestionGenerator = (*DataOpenAIGPT)(nil)
