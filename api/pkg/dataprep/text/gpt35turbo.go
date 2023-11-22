package text

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

// knows how to turn an array of raw documents
// and make them into a finetune dataset for an LLM
// we try to do alpaca_chat.load_qa: question and answer for alpaca chat
// {"question": "...", "answer": "..."}
type DataPrepTextGPT35Turbo struct {
	Options DataPrepTextOptions
	client  *openai.Client
	docs    *dataPrepDocuments
}

func NewDataPrepTextGPT35Turbo(options DataPrepTextOptions) (*DataPrepTextGPT35Turbo, error) {
	return &DataPrepTextGPT35Turbo{
		Options: options,
		client:  openai.NewClient(options.APIKey),
		docs:    newDataPrepDocuments(),
	}, nil
}

func (gpt *DataPrepTextGPT35Turbo) AddDocument(content string) error {
	return gpt.docs.AddDocument(content)
}

func (gpt *DataPrepTextGPT35Turbo) GetChunks() ([]string, error) {
	return gpt.docs.GetChunks(gpt.Options.ChunkSize, gpt.Options.OverflowSize)
}

func (gpt *DataPrepTextGPT35Turbo) ConvertChunk(chunk string) ([]DataPrepTextConversation, error) {
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

Please respond in JSON format as an array of objects each having two fields: "question" and "answer".
	`, gpt.Options.QuestionsPerChunk, gpt.Options.QuestionsPerChunk)

	userPrompt := fmt.Sprintf(`
Given the following context - please summarize it into %d question and answer pairs. Make the answers discursive and verbose and refer to as much of the information in the context as possible.

ONLY include a question if you know the answer.

Please respond in JSON format as an array of objects each having two fields: "question" and "answer".

%s
	`, gpt.Options.QuestionsPerChunk, chunk)

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

	log.Debug().
		Msgf("🔴🔴🔴 GPT 3.5 Turbo Question: %+v", messages)

	resp, err := gpt.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model:    openai.GPT3Dot5Turbo,
			Messages: messages,
		},
	)
	if err != nil {
		return nil, err
	}

	answer := resp.Choices[0].Message.Content

	var res []DataPrepTextConversation

	log.Debug().
		Msgf("🔴🔴🔴 GPT 3.5 Turbo Answer: %+v", answer)

	// parse body as json into result
	err = json.Unmarshal([]byte(answer), &res)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// Compile-time interface check:
var _ DataPrepText = (*DataPrepTextGPT35Turbo)(nil)
