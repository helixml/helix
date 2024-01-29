package text

import (
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/dataprep/qapairs"
	"github.com/helixml/helix/api/pkg/types"
)

// Wrapper around qapairs.Query that implements DataPrepTextQuestionGenerator.
// Dynamically generates qapairs based on (baked-in) yaml configuration of
// a suite of named qapair prompts and named target APIs.

type DynamicDataPrep struct {
	Target  string
	Prompts []string
}

func NewDynamicDataPrep(target string, prompts []string) *DynamicDataPrep {
	if target == "" && len(prompts) == 0 {
		allPrompts, err := qapairs.AllPrompts()
		if err != nil {
			panic(err)
		}

		// use sensible defaults
		return &DynamicDataPrep{
			Target:  "together-mixtral",
			Prompts: allPrompts,
		}
	}

	return &DynamicDataPrep{
		Target:  target,
		Prompts: prompts,
	}
}

func (d *DynamicDataPrep) ExpandChunks(chunks []*DataPrepTextSplitterChunk) (
	[]*DataPrepTextSplitterChunk, error,
) {
	result := []*DataPrepTextSplitterChunk{}
	for _, prompt := range d.Prompts {
		for _, chunk := range chunks {
			chunkCopy := *chunk
			chunkCopy.PromptName = prompt
			result = append(result, &chunkCopy)
		}
	}
	return result, nil
}

func (d *DynamicDataPrep) ConvertChunk(
	chunk string, index int, documentID, documentGroupID, promptName string,
) ([]types.DataPrepTextQuestion, error) {
	prompt, err := qapairs.FindPrompt(promptName)
	if err != nil {
		return nil, err
	}
	target, err := qapairs.FindTarget(d.Target)
	if err != nil {
		return nil, err
	}
	text := qapairs.Text{
		Name:     "user-provided",
		Contents: chunk,
	}
	resRaw, err := qapairs.Query(target, prompt, text, documentID, documentGroupID, 0)
	if err != nil {
		return nil, err
	}
	res := []types.DataPrepTextQuestion{}
	qText := fmt.Sprintf("In document %s (document group %s), ", documentID, documentGroupID)
	aText := fmt.Sprintf("[DOC_ID:%s] [DOC_GROUP:%s]\n\n", documentID, documentGroupID)
	for _, q := range resRaw {
		res = append(res, types.DataPrepTextQuestion{
			Conversations: []types.DataPrepTextQuestionPart{
				{
					From: "human",
					// TODO: not perfect utf-8 handling..
					Value: qText + strings.ToLower(string(q.Question[0])) + q.Question[1:],
				},
				{
					From:  "gpt",
					Value: aText + q.Answer,
				},
			},
		})
	}
	return res, nil
}

func (d *DynamicDataPrep) GetConcurrency() int {
	concurrency, err := qapairs.GetConcurrency()
	if err != nil {
		panic(err)
	}
	return concurrency
}

func (d *DynamicDataPrep) GetChunkSize() int {
	chunkSize, err := qapairs.GetChunkSize()
	if err != nil {
		panic(err)
	}
	return chunkSize
}

// Compile-time interface check:
var _ DataPrepTextQuestionGenerator = (*DynamicDataPrep)(nil)
