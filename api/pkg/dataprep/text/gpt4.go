package text

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lukemarsden/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
)

// If there is not enough context to generate %d questions, you can generate fewer questions.

// In the worst case scenario, where you are unable to generate any questions, please respond with an empty array.

// It's VERY important that you don't include any additional text in your response, otherwise the system will be unable to parse your response.

// ONLY include the JSON array of questions and answers.

const GPT4_CONCURRENCY = 20
const GPT4_CHUNK_SIZE = 4096

func NewDataPrepTextGPT4(options DataPrepTextOptions) (*DataOpenAIGPT, error) {
	getSystemPromptFn := func(chunk string, options DataPrepTextOptions) string {
		return fmt.Sprintf(`
You are a Teacher/ Professor. Your task is to setup a quiz/examination.
Using the provided context, formulate %d questions that captures an important fact from the context.
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
`, options.QuestionsPerChunk, options.QuestionsPerChunk)
	}

	getUserPromptFn := func(chunk string, options DataPrepTextOptions) string {
		return fmt.Sprintf(`
Given the following context - please summarize it into %d question and answer pairs. Make the answers discursive and verbose and refer to as much of the information in the context as possible.

ONLY include a question if you know the answer.

If there is not enough context to generate %d questions, you can generate fewer questions.

In the worst case scenario, where you are unable to generate any questions, please respond with an empty array.

It's VERY important that you don't include any additional text in your response, otherwise the system will be unable to parse your response.

ONLY include the JSON array of questions and answers.

Based on the context, guess a reasonable name for the document and refer to that document name in the questions. For example, if the document appears to be Bob Anderson's CV, refer to it as "Bob Anderson's CV" rather than using generic terms like "the author".

Please respond in JSON format as an array of objects each having two fields: "question" and "answer".

%s`, options.QuestionsPerChunk, options.QuestionsPerChunk, chunk)
	}

	parseResponseFn := func(answer string, options DataPrepTextOptions) ([]types.DataPrepTextQuestion, error) {
		answer = strings.TrimPrefix(answer, "```json")
		// sometimes GPT4 in it's wisdom puts a message after the enclosing ```json``` block
		parts := strings.Split(answer, "```")
		answer = parts[0]
		var resRaw []types.DataPrepTextQuestionRaw
		err := json.Unmarshal([]byte(answer), &resRaw)
		if err != nil {
			return nil, fmt.Errorf("error parsing JSON:\n\n%s", answer)
		}

		res := []types.DataPrepTextQuestion{}
		for _, q := range resRaw {
			res = append(res, types.DataPrepTextQuestion{
				Conversations: []types.DataPrepTextQuestionPart{
					{
						From:  "human",
						Value: q.Question,
					},
					{
						From:  "gpt",
						Value: q.Answer,
					},
				},
			})
		}

		return res, nil
	}

	return NewDataOpenAIGPT(
		options,
		openai.GPT4TurboPreview,
		GPT4_CONCURRENCY,
		GPT4_CHUNK_SIZE,
		getSystemPromptFn,
		getUserPromptFn,
		parseResponseFn,
	)

}
