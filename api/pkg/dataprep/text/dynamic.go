package text

// import (
// 	"encoding/json"
// 	"fmt"
// 	"strings"

// 	"github.com/helixml/helix/api/pkg/types"
// 	openai "github.com/sashabaranov/go-openai"
// )

// // If there is not enough context to generate %d questions, you can generate fewer questions.

// // In the worst case scenario, where you are unable to generate any questions, please respond with an empty array.

// // It's VERY important that you don't include any additional text in your response, otherwise the system will be unable to parse your response.

// // ONLY include the JSON array of questions and answers.

// const GPT4_CONCURRENCY = 20

// // this is in bytes but the actual limit is in tokens. there's always 1 or more
// // byte per token though. also, subtract a buffer for the user prompt..

// // TODO: we might still choose to make this context window smaller because the
// // gpt4-1106-preview output is max 4k tokens, so on a large document we'll start
// // to lose qapairs that we'd get if we did chunk. The benefit of coherence on
// // smaller articles is probably worth it though? TBD
// const GPT4_CHUNK_SIZE = 128000 - 4000

// func NewDataPrepTextDynamic(options DataPrepTextOptions) (*DataOpenAIGPT, error) {

// 	return &DynamicDataPrep{
// 		Target: "together-mixtral",
// 		Prompts: []string{
// 			"summaries",
// 			"you-are-generating-fine-tuning-data",
// 			"simple-quiz",
// 			"entities-specific-to-broad",
// 			"important-facts",
// 			"entities-relationships",
// 			"origial-prompt",
// 		},
// 	}

// 	getSystemPromptFn := func(chunk string, options DataPrepTextOptions) string {
// 		return fmt.Sprintf(`
// You are a Teacher/ Professor. Your task is to setup a quiz/examination.
// Using the provided context, formulate %d questions that captures an important fact from the context.
// You MUST obey the following criteria:
// 	- Restrict the question to the context information provided.
// 	- Do NOT create a question that cannot be answered from the context.
// 	- Phrase the question so that it does NOT refer to specific context. For instance, do NOT put phrases like given provided context or in this work in the question, because if the question is asked elsewhere it wouldn't be provided specific context. Replace these terms with specific details.

// BAD questions:
// 	What did the author do in his childhood
// 	What were the main findings in this report

// GOOD questions:
// 	What did Barack Obama do in his childhood
// 	What were the main findings in the original Transformers paper by Vaswani et al.

// The user will provide the context you should summarize into %d questions.

// Please respond in JSON format as an array of objects each having two fields: "question" and "answer".
// `, options.QuestionsPerChunk, options.QuestionsPerChunk)
// 	}

// 	// TODO: put filename in
// 	getUserPromptFn := func(chunk, documentID, documentGroupID string, options DataPrepTextOptions) string {
// 		return fmt.Sprintf(`
// Given the following context - please convert it into %d question and answer pairs. Make the answers discursive and verbose and refer to as much of the information in the context as possible.

// ONLY include a question if you know the answer from the context.

// If there is not enough context to generate %d questions, you can generate fewer questions.

// In the worst case scenario, where you are unable to generate any questions, please respond with an empty array.

// It's VERY important that you don't include any additional text in your response, otherwise the system will be unable to parse your response.

// ONLY include the JSON array of questions and answers.

// Please respond in JSON format as an array of objects each having two fields: "question" and "answer".

// This DOCUMENT_ID is %s
// This DOCUMENT_GROUP_ID is %s

// In every question and answer, you MUST reference the DOCUMENT_ID and DOCUMENT_GROUP_ID.

// For example:
// Question: In document A1B2C3 (document group B2C3D4), what has Kier Starmer pointed out regarding NHS waiting lists?
// Answer: In document A1B2C3 (document group B2C3D4), Kier Starmer has pointed out that NHS waiting lists have increased since the prime minister set the goal of reducing them.

// In the first five question-answer pairs, summarize the document, especially based on the document's title. For example:

// Question: In document A1B2C3 (document group B2C3D4), what action are the doctors going to do?
// Answer: Document A1B2C3 (document group B2C3D4) talks about how the junior doctors are going to stage more strikes.

// Here is the context:
// %s`, options.QuestionsPerChunk, options.QuestionsPerChunk, documentID, documentGroupID, chunk)
// 	}

// 	parseResponseFn := func(answer string, options DataPrepTextOptions) ([]types.DataPrepTextQuestion, error) {
// 		answer = strings.TrimPrefix(answer, "```json")
// 		// sometimes GPT4 in it's wisdom puts a message after the enclosing ```json``` block
// 		parts := strings.Split(answer, "```")
// 		answer = parts[0]
// 		var resRaw []types.DataPrepTextQuestionRaw
// 		err := json.Unmarshal([]byte(answer), &resRaw)
// 		if err != nil {
// 			return nil, fmt.Errorf("error parsing JSON:\n\n%s", answer)
// 		}

// 		res := []types.DataPrepTextQuestion{}
// 		for _, q := range resRaw {
// 			res = append(res, types.DataPrepTextQuestion{
// 				Conversations: []types.DataPrepTextQuestionPart{
// 					{
// 						From:  "human",
// 						Value: q.Question,
// 					},
// 					{
// 						From:  "gpt",
// 						Value: q.Answer,
// 					},
// 				},
// 			})
// 		}

// 		return res, nil
// 	}

// 	return NewDataOpenAIGPT(
// 		options,
// 		openai.GPT4TurboPreview,
// 		GPT4_CONCURRENCY,
// 		GPT4_CHUNK_SIZE,
// 		getSystemPromptFn,
// 		getUserPromptFn,
// 		parseResponseFn,
// 	)

// }
