package text

type DataPrepTextOptions struct {
	APIKey       string
	ChunkSize    int
	OverflowSize int
}

type DataPrepTextConversation struct {
	From  string `json:"from"`
	Value string `json:"value"`
}

type DataPrepText interface {
	// add a document to the collection
	AddDocument(content string) error

	// get all chunks across all documents added
	GetChunks() ([]string, error)

	// convert a single chunk
	ConvertChunk(chunk string) ([]DataPrepTextConversation, error)
}
