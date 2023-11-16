package text

// generic options - api key need not be defined
// the chunk sizes applies to all interfaces because
// we just call out to our unstructured service for all things
type DataPrepTextOptions struct {
	APIKey            string
	ChunkSize         int
	OverflowSize      int
	QuestionsPerChunk int
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
