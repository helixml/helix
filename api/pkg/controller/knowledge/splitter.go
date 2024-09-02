package knowledge

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/dataprep/text"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"

	"github.com/tmc/langchaingo/textsplitter"
)

func splitData(k *types.Knowledge, data []*indexerData) ([]*text.DataPrepTextSplitterChunk, error) {
	var chunks []*text.DataPrepTextSplitterChunk

	switch k.RAGSettings.TextSplitter {
	case types.TextSplitterTypeText:
		log.Info().
			Str("knowledge_id", k.ID).
			Msgf("splitting data with text splitter")

		splitter, err := text.NewDataPrepSplitter(text.DataPrepTextSplitterOptions{
			ChunkSize: k.RAGSettings.ChunkSize,
			Overflow:  k.RAGSettings.ChunkOverflow,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create data prep splitter, error %w", err)
		}

		documentGroupID := k.ID

		for _, d := range data {
			_, err := splitter.AddDocument(d.Source, string(d.Data), documentGroupID)
			if err != nil {
				return nil, fmt.Errorf("failed to split %s, error %w", d.Source, err)
			}
		}

		return splitter.Chunks, nil
	default:
		log.Info().
			Str("knowledge_id", k.ID).
			Msgf("splitting data with markdown text splitter")

		splitter := textsplitter.NewMarkdownTextSplitter(
			textsplitter.WithChunkSize(k.RAGSettings.ChunkSize),
			textsplitter.WithChunkOverlap(k.RAGSettings.ChunkOverflow),
		)

		for _, d := range data {
			parts, err := splitter.SplitText(string(d.Data))
			if err != nil {
				return nil, fmt.Errorf("failed to split %s, error %w", d.Source, err)
			}

			for idx, part := range parts {
				chunks = append(chunks, &text.DataPrepTextSplitterChunk{
					Filename:        d.Source,
					Index:           idx,
					Text:            string(part),
					DocumentID:      getDocumentID(d.Data),
					DocumentGroupID: k.ID,
				})
			}
		}
	}

	return chunks, nil
}