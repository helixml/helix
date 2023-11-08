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

	systemPrompt := fmt.Sprintf(`
You are a Teacher/ Professor. Your task is to setup a quiz/examination.
Using the provided context, formulate %d questions that
captures an important fact from the context.
You MUST obey the following criteria:
  - Restrict the question to the context information provided.
	- Do NOT create a question that cannot be answered from the context.
	- Phrase the question so that it does NOT refer to specific context. For instance, do NOT put phrases like given provided context or in this work in the question, because if the question is asked elsewhere it wouldn't be provided specific context. Replace these terms with specific details.
	
BAD questions:
	What did the author do in his childhood
	What were the main findings in this report
	
GOOD questions:
	What did Barack Obama do in his childhood
	What were the main findings in the original Transformers paper by Vaswani et al.

The user will provide the context you should summarize into %d questions.
	`, gpt.Options.QuestionsPerChunk, gpt.Options.QuestionsPerChunk)

	userPrompt := fmt.Sprintf(`
Given the following context - please summarize it into %d question and answer pairs.

%s
	`, gpt.Options.QuestionsPerChunk, chunk)

	resp, err := gpt.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: systemPrompt,
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: userPrompt,
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
