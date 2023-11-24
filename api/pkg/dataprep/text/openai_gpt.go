package text

import (
	"fmt"
	"io"

	"github.com/davecgh/go-spew/spew"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/lukemarsden/helix/api/pkg/system"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

// a generic openai based data prep module that can use any model
type DataOpenAIGPT struct {
	Options           DataPrepTextOptions
	client            *openai.Client
	docs              *dataPrepDocuments
	model             string
	getSystemPromptFn func(chunk string, options DataPrepTextOptions) string
	getUserPromptFn   func(chunk string, options DataPrepTextOptions) string
	parseResponseFn   func(answer string, options DataPrepTextOptions) ([]DataPrepTextConversation, error)
}

func NewDataOpenAIGPT(
	options DataPrepTextOptions,
	model string,
	getSystemPromptFn func(chunk string, options DataPrepTextOptions) string,
	getUserPromptFn func(chunk string, options DataPrepTextOptions) string,
	parseResponseFn func(answer string, options DataPrepTextOptions) ([]DataPrepTextConversation, error),
) (*DataOpenAIGPT, error) {
	return &DataOpenAIGPT{
		Options:           options,
		client:            openai.NewClient(options.APIKey),
		docs:              newDataPrepDocuments(),
		model:             model,
		getUserPromptFn:   getUserPromptFn,
		getSystemPromptFn: getSystemPromptFn,
		parseResponseFn:   parseResponseFn,
	}, nil
}

func (gpt *DataOpenAIGPT) AddDocument(content string) error {
	return gpt.docs.AddDocument(content)
}

func (gpt *DataOpenAIGPT) GetChunks() ([]string, error) {
	return gpt.docs.GetChunks(gpt.Options.ChunkSize, gpt.Options.OverflowSize)
}

func (gpt *DataOpenAIGPT) ConvertChunk(chunk string) ([]DataPrepTextConversation, error) {
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

	postData := openai.ChatCompletionRequest{
		Model:       gpt.model,
		Messages:    messages,
		Temperature: gpt.Options.Temperature,
	}

	log.Debug().
		Msgf("🔴🔴🔴 GPT Question: %+v", postData)

	clientOptions := system.ClientOptions{
		Host:  "https://api.openai.com",
		Token: gpt.Options.APIKey,
	}

	req, err := retryablehttp.NewRequest("POST", system.URL(clientOptions, "/v1/chat/completions"), postData)
	if err != nil {
		return nil, err
	}
	err = system.AddAuthHeadersRetryable(req, clientOptions.Token)
	if err != nil {
		return nil, err
	}

	client := system.NewRetryClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf(string(body))
	}

	if err != nil {
		return nil, err
	}
	fmt.Printf("resp --------------------------------------\n")
	spew.Dump(resp)
	conversation, err := gpt.parseResponseFn(resp.Choices[0].Message.Content, gpt.Options)
	if err != nil {
		return nil, err
	}

	return conversation, nil
}

// Compile-time interface check:
var _ DataPrepText = (*DataOpenAIGPT)(nil)
