package text

import (
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

const HELIX_MISTRAL_CONCURRENCY = 20
const HELIX_MISTRAL_CHUNK_SIZE = 4096

type DataPrepTextHelixMistralSessionCreate func(req types.CreateSessionRequest) (*types.Session, error)
type DataPrepTextHelixMistralSessionGet func(id string) (*types.Session, error)

// knows how to turn an array of raw documents
// and make them into a finetune dataset for an LLM
// we try to do alpaca_chat.load_qa: question and answer for alpaca chat
// {"question": "...", "answer": "..."}
type DataPrepTextHelixMistral struct {
	Options  DataPrepTextOptions
	session  *types.Session
	createFn DataPrepTextHelixMistralSessionCreate
	getFn    DataPrepTextHelixMistralSessionGet
}

func NewDataPrepTextHelixMistral(
	options DataPrepTextOptions,
	session *types.Session,
	createFn DataPrepTextHelixMistralSessionCreate,
	getFn DataPrepTextHelixMistralSessionGet,
) (*DataPrepTextHelixMistral, error) {
	return &DataPrepTextHelixMistral{
		Options:  options,
		session:  session,
		createFn: createFn,
		getFn:    getFn,
	}, nil
}

func (helixMistral *DataPrepTextHelixMistral) GetConcurrency() int {
	return HELIX_MISTRAL_CONCURRENCY
}

func (helixMistral *DataPrepTextHelixMistral) GetChunkSize() int {
	return HELIX_MISTRAL_CHUNK_SIZE
}

func (helixMistral *DataPrepTextHelixMistral) ExpandChunks(chunks []*DataPrepTextSplitterChunk) ([]*DataPrepTextSplitterChunk, error) {
	// no expansion
	return chunks, nil
}

// TODO: getting a consistent output format that we can parse reliably is really hard
func (helixMistral *DataPrepTextHelixMistral) ConvertChunk(chunk string, index int, documentID, documentGroupID, promptName string) ([]types.DataPrepTextQuestion, error) {
	prompt := fmt.Sprintf(`
You are a Teacher/ Professor. Your task is to setup a quiz/examination.
Using the provided context, formulate exactly %d question and answer pairs that captures an important fact from the context.
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

Please write exactly %d question and answer pairs.

Each question should be on a single line and start with the word "[QUESTION]".
Each answer should be on a single line and start with the word "[ANSWER]".

Do not number the questions or answers.

%s

	`, helixMistral.Options.QuestionsPerChunk, helixMistral.Options.QuestionsPerChunk, chunk)

	log.Debug().
		Msgf("🔴 Mistral Question: %s", prompt)

	session, err := helixMistral.createFn(types.CreateSessionRequest{
		SessionID:     system.GenerateUUID(),
		SessionMode:   types.SessionModeInference,
		SessionType:   types.SessionTypeText,
		ModelName:     types.Model_Mistral7b,
		Owner:         helixMistral.session.Owner,
		OwnerType:     helixMistral.session.OwnerType,
		ParentSession: helixMistral.session.ID,
		UserInteractions: []*types.Interaction{
			{
				ID:             system.GenerateUUID(),
				Created:        time.Now(),
				Creator:        types.CreatorTypeUser,
				Message:        prompt,
				Files:          []string{},
				State:          types.InteractionStateWaiting,
				Finished:       false,
				Metadata:       map[string]string{},
				DataPrepChunks: map[string][]types.DataPrepChunk{},
			},
		},
	})

	if err != nil {
		return nil, err
	}

	// we are now waiting for session to complete so we can get the answer
	// how long are we willing to wait?

	sanity := 0
	result := ""

	for {
		session, err = helixMistral.getFn(session.ID)
		if err != nil {
			return nil, err
		}

		lastInteraction := session.Interactions[len(session.Interactions)-1]
		if lastInteraction.Finished {
			result = lastInteraction.Message
			break
		}

		sanity++
		if sanity > 100000 {
			return nil, fmt.Errorf("sanity check failed after %d iterations", sanity)
		}
		time.Sleep(1 * time.Second)
	}

	var res []types.DataPrepTextQuestion

	log.Debug().
		Msgf("🔴 Mistral Answer: %+v", result)

	// r := csv.NewReader(strings.NewReader(lastInteraction.Message))
	// // var conversations []DataPrepTextConversation

	// for {
	// 	record, err := r.Read()
	// 	if err != nil {
	// 		if err == io.EOF {
	// 			break
	// 		}
	// 		return nil, err
	// 	}

	// 	fmt.Printf("record --------------------------------------\n")
	// 	spew.Dump(record)

	// 	// // Assuming the CSV format is correct and each row has 2 fields
	// 	// conversations = append(conversations, DataPrepTextConversation{
	// 	// 		Question: record[0],
	// 	// 		Answer:   record[1],
	// 	// })
	// }
	// break

	// // parse body as json into result
	// err = json.Unmarshal([]byte(result), &res)
	// if err != nil {
	// 	return nil, err
	// }

	// fmt.Printf("[]DataPrepTextConversation --------------------------------------\n")
	// spew.Dump(res)

	return res, nil
}

// Compile-time interface check:
var _ DataPrepTextQuestionGenerator = (*DataPrepTextHelixMistral)(nil)
