package text

import "fmt"

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
}

type DataPrepTextConversation struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type ShareGPTConversation struct {
	From  string `json:"from"`
	Value string `json:"value"`
}

type ShareGPTConversations struct {
	Conversations []ShareGPTConversation `json:"conversations"`
}

// an implementation that knows how to add documents,
// chunk into pieces with overflow
// and convert into question answer pairs
type DataPrepText interface {
	// add a document to the collection
	AddDocument(content string) error

	// get all chunks across all documents added
	GetChunks() ([]string, error)

	// convert a single chunk
	ConvertChunk(chunk string) ([]DataPrepTextConversation, error)
}
