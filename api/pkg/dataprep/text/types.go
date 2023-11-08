package text

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

type DataPrepText interface {
	// add a document to the collection
	AddDocument(content string) error

	// get all chunks across all documents added
	GetChunks() ([]string, error)

	// convert a single chunk
	ConvertChunk(chunk string) ([]DataPrepTextConversation, error)
}
