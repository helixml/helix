package text

import (
	"fmt"

	"github.com/davecgh/go-spew/spew"
)

// knows how to turn an array of raw documents
// and make them into a finetune dataset for an LLM
// we try to do alpaca_chat.load_qa: question and answer for alpaca chat
// {"question": "...", "answer": "..."}
type DataPrepTextHelixMistral struct {
	Options DataPrepTextOptions
	docs    *dataPrepDocuments
}

func NewDataPrepTextHelixMistral(options DataPrepTextOptions) (*DataPrepTextHelixMistral, error) {
	return &DataPrepTextHelixMistral{
		Options: options,
		docs:    newDataPrepDocuments(),
	}, nil
}

func (helixMistral *DataPrepTextHelixMistral) AddDocument(content string) error {
	return helixMistral.docs.AddDocument(content)
}

func (helixMistral *DataPrepTextHelixMistral) GetChunks() ([]string, error) {
	return helixMistral.docs.GetChunks(helixMistral.Options.ChunkSize, helixMistral.Options.OverflowSize)
}

func (helixMistral *DataPrepTextHelixMistral) ConvertChunk(chunk string) ([]DataPrepTextConversation, error) {
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
	`, helixMistral.Options.QuestionsPerChunk, helixMistral.Options.QuestionsPerChunk)

	userPrompt := fmt.Sprintf(`
Given the following context - please summarize it into %d question and answer pairs.

Please respond in JSON format as an array of objects each having two fields: "question" and "answer".

%s
	`, helixMistral.Options.QuestionsPerChunk, chunk)

	var res []DataPrepTextConversation

	fmt.Printf("systemPrompt --------------------------------------\n")
	spew.Dump(systemPrompt)

	fmt.Printf("userPrompt --------------------------------------\n")
	spew.Dump(userPrompt)

	return res, nil
}

// Compile-time interface check:
var _ DataPrepText = (*DataPrepTextGPT4)(nil)
