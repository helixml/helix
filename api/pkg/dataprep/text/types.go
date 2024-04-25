package text

import (
	"github.com/helixml/helix/api/pkg/types"
)

type DataPrepTextQuestionGenerator interface {
	ExpandChunks(chunks []*DataPrepTextSplitterChunk) ([]*DataPrepTextSplitterChunk, error)
	ConvertChunk(chunk string, index int, documentID, documentGroupID, promptName string) ([]types.DataPrepTextQuestion, error)
	GetConcurrency() int
	GetChunkSize() int
}
