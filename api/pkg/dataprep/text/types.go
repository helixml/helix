package text

import (
	"fmt"

	"github.com/lukemarsden/helix/api/pkg/types"
)

type DataPrepModule string

const (
	DataPrepModule_None         DataPrepModule = ""
	DataPrepModule_GPT3Point5   DataPrepModule = "gpt3.5"
	DataPrepModule_GPT4         DataPrepModule = "gpt4"
	DataPrepModule_HelixMistral DataPrepModule = "helix_mistral"
)

func ValidateDataPrepModule(moduleName string, acceptEmpty bool) (DataPrepModule, error) {
	switch moduleName {
	case string(DataPrepModule_GPT3Point5):
		return DataPrepModule_GPT3Point5, nil
	case string(DataPrepModule_GPT4):
		return DataPrepModule_GPT4, nil
	case string(DataPrepModule_HelixMistral):
		return DataPrepModule_HelixMistral, nil
	default:
		if acceptEmpty && moduleName == string(DataPrepModule_None) {
			return DataPrepModule_None, nil
		} else {
			return DataPrepModule_None, fmt.Errorf("invalid data prep module name: %s", moduleName)
		}
	}
}

// generic options - api key need not be defined
// the chunk sizes applies to all interfaces because
// we just call out to our unstructured service for all things
type DataPrepTextOptions struct {
	Module            DataPrepModule
	APIKey            string
	ChunkSize         int
	OverflowSize      int
	QuestionsPerChunk int
	Temperature       float32
	Concurrency       int
}

type DataPrepTextQuestionGenerator interface {
	ConvertChunk(chunk string, index int) ([]types.DataPrepTextQuestion, error)
	GetConcurrency() int
}
