package text

import (
	"github.com/helixml/helix/api/pkg/types"
)

type DataPrepTextQuestionGenerator interface {
	ExpandChunks(chunks []*DataPrepTextSplitterChunk) ([]*DataPrepTextSplitterChunk, error)
	// TODO: consider dropping index, as it's not being used anywhere at the moment
	ConvertChunk(ownerID, sessionID, chunk string, index int, documentID, documentGroupID, promptName string) ([]types.DataPrepTextQuestion, error)
	GetConcurrency() int
	GetChunkSize() int
}
