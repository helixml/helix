package text

import (
	"context"
	"fmt"

	"github.com/davecgh/go-spew/spew"
	openai "github.com/sashabaranov/go-openai"
)

// knows how to turn an array of raw documents
// and make them into a finetune dataset for an LLM
// we try to do alpaca_chat.load_qa: question and answer for alpaca chat
// {"question": "...", "answer": "..."}
type DataPrepTextGPT4 struct {
	Options DataPrepTextOptions
	client  *openai.Client
	docs    *dataPrepDocuments
}

func NewDataPrepTextGPT4(options DataPrepTextOptions) (*DataPrepTextGPT4, error) {
	return &DataPrepTextGPT4{
		Options: options,
		client:  openai.NewClient(options.APIKey),
		docs:    newDataPrepDocuments(),
	}, nil
}

func (gpt *DataPrepTextGPT4) AddDocument(content string) error {
	return gpt.docs.AddDocument(content)
}

func (gpt *DataPrepTextGPT4) GetChunks() ([]string, error) {
	return gpt.docs.GetChunks(gpt.Options.ChunkSize, gpt.Options.OverflowSize)
}

func (gpt *DataPrepTextGPT4) ConvertChunk(chunk string) ([]DataPrepTextConversation, error) {
	fmt.Printf("CONVERT CHUNK YO --------------------------------------\n")
	spew.Dump(chunk)
	resp, err := gpt.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: "Please tell me a joke",
				},
			},
		},
	)
	if err != nil {
		return nil, err
	}

	fmt.Printf("ANSWER --------------------------------------\n")
	fmt.Printf("ANSWER --------------------------------------\n")
	fmt.Printf("ANSWER --------------------------------------\n")
	fmt.Println(resp.Choices[0].Message.Content)

	return []DataPrepTextConversation{}, nil
}

// Compile-time interface check:
var _ DataPrepText = (*DataPrepTextGPT4)(nil)
