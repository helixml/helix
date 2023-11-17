package text

import (
	"fmt"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/lukemarsden/helix/api/pkg/system"
	"github.com/lukemarsden/helix/api/pkg/types"
)

type DataPrepTextHelixMistralSessionCreate func(req types.CreateSessionRequest) (*types.Session, error)
type DataPrepTextHelixMistralSessionGet func(id string) (*types.Session, error)

// knows how to turn an array of raw documents
// and make them into a finetune dataset for an LLM
// we try to do alpaca_chat.load_qa: question and answer for alpaca chat
// {"question": "...", "answer": "..."}
type DataPrepTextHelixMistral struct {
	Options  DataPrepTextOptions
	docs     *dataPrepDocuments
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
		docs:     newDataPrepDocuments(),
		createFn: createFn,
		getFn:    getFn,
	}, nil
}

func (helixMistral *DataPrepTextHelixMistral) AddDocument(content string) error {
	return helixMistral.docs.AddDocument(content)
}

func (helixMistral *DataPrepTextHelixMistral) GetChunks() ([]string, error) {
	return helixMistral.docs.GetChunks(helixMistral.Options.ChunkSize, helixMistral.Options.OverflowSize)
}

func (helixMistral *DataPrepTextHelixMistral) ConvertChunk(chunk string) ([]DataPrepTextConversation, error) {
	prompt := fmt.Sprintf(`
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

Given the following context - please summarize it into %d question and answer pairs.

Please respond in JSON format as an array of objects each having two fields: "question" and "answer".

%s

	`, helixMistral.Options.QuestionsPerChunk, helixMistral.Options.QuestionsPerChunk, helixMistral.Options.QuestionsPerChunk, chunk)

	var res []DataPrepTextConversation

	session, err := helixMistral.createFn(types.CreateSessionRequest{
		SessionMode:   types.SessionModeInference,
		SessionType:   types.SessionTypeText,
		ModelName:     types.Model_Mistral7b,
		Owner:         helixMistral.session.Owner,
		OwnerType:     helixMistral.session.OwnerType,
		ParentSession: helixMistral.session.ID,
		UserInteraction: types.Interaction{
			ID:       system.GenerateUUID(),
			Created:  time.Now(),
			Creator:  types.CreatorTypeUser,
			Message:  prompt,
			Files:    []string{},
			State:    types.InteractionStateWaiting,
			Finished: false,
			Metadata: map[string]string{},
		},
	})

	if err != nil {
		return res, err
	}

	// we are now waiting for session to complete so we can get the answer
	// how long are we willing to wait?

	sanity := 0
	result := ""

	for {
		session, err = helixMistral.getFn(session.ID)
		if err != nil {
			return res, err
		}

		lastInteraction := session.Interactions[len(session.Interactions)-1]
		if lastInteraction.Finished {
			result = lastInteraction.Message
			break
		}

		sanity++
		if sanity > 100000 {
			return res, fmt.Errorf("sanity check failed after %d iterations", sanity)
		}
		time.Sleep(1 * time.Second)
	}
	fmt.Printf("result --------------------------------------\n")
	fmt.Printf("result --------------------------------------\n")
	fmt.Printf("result --------------------------------------\n")
	fmt.Printf("result --------------------------------------\n")
	fmt.Printf("result --------------------------------------\n")
	spew.Dump(result)

	// TODO: parse this output

	return res, nil
}

// Compile-time interface check:
var _ DataPrepText = (*DataPrepTextHelixMistral)(nil)
