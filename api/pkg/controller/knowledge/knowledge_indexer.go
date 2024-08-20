package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/helixml/helix/api/pkg/dataprep/text"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

func (r *Reconciler) index(ctx context.Context) error {
	data, err := r.store.ListKnowledge(ctx, &store.ListKnowledgeQuery{
		State: types.KnowledgeStatePending,
	})
	if err != nil {
		return fmt.Errorf("failed to get knowledge entries, error: %w", err)
	}

	for _, k := range data {
		r.wg.Add(1)

		k.State = types.KnowledgeStateIndexing
		k.Message = ""
		_, _ = r.store.UpdateKnowledge(ctx, k)

		log.
			Info().
			Str("knowledge_id", k.ID).
			Msg("indexing knowledge")

		go func(knowledge *types.Knowledge) {

			err := r.indexKnowledge(ctx, knowledge)
			if err != nil {
				log.
					Warn().
					Err(err).
					Str("knowledge_id", knowledge.ID).
					Msg("failed to index knowledge")

				k.State = types.KnowledgeStateError
				k.Message = err.Error()
				_, _ = r.store.UpdateKnowledge(ctx, k)

			}
		}(k)
	}

	return nil
}

func (r *Reconciler) indexKnowledge(ctx context.Context, k *types.Knowledge) error {
	// If source is plain text, nothing to do
	if k.Source.Content != nil {
		k.State = types.KnowledgeStateReady
		_, err := r.store.UpdateKnowledge(ctx, k)
		if err != nil {
			return fmt.Errorf("failed to update knowledge, error: %w", err)
		}
		return nil
	}

	data, err := r.getIndexingData(ctx, k)
	if err != nil {
		return fmt.Errorf("failed to get indexing data, error: %w", err)
	}

	err = r.indexData(ctx, k, data)
	if err != nil {
		return fmt.Errorf("indexing failed, error: %w", err)
	}

	k.State = types.KnowledgeStateReady
	_, err = r.store.UpdateKnowledge(ctx, k)
	if err != nil {
		return fmt.Errorf("failed to update knowledge, error: %w", err)
	}

	log.Info().
		Str("knowledge_id", k.ID).
		Msg("knowledge indexed")

	return nil
}

func (r *Reconciler) getRagClient(k *types.Knowledge) rag.RAG {
	if k.RAGSettings.IndexURL != "" && k.RAGSettings.QueryURL != "" {
		return r.newRagClient(k.RAGSettings.IndexURL, k.RAGSettings.QueryURL)
	}
	return r.ragClient
}

func (r *Reconciler) indexData(ctx context.Context, k *types.Knowledge, data []*indexerData) error {
	if k.RAGSettings.DisableChunking {
		return r.indexDataDirectly(ctx, k, data)
	}

	return r.indexDataWithChunking(ctx, k, data)
}

func (r *Reconciler) indexDataDirectly(ctx context.Context, k *types.Knowledge, data []*indexerData) error {
	documentGroupID := k.ID

	ragClient := r.getRagClient(k)

	log.Info().
		Str("knowledge_id", k.ID).
		Int("payloads", len(data)).
		Msg("submitting raw data into the rag server")

	for _, d := range data {
		err := ragClient.Index(ctx, &types.SessionRAGIndexChunk{
			DataEntityID:    k.ID,
			Filename:        d.Source,
			DocumentID:      getDocumentID(d.Data),
			DocumentGroupID: documentGroupID,
			ContentOffset:   0,
			Content:         string(d.Data),
		})
		if err != nil {
			return fmt.Errorf("failed to index data from source %s, error: %w", d.Source, err)
		}
	}

	// All good, nothing else to do
	return nil
}

// indexDataWithChunking we expect to be operating on text data, first we split,
// then index with the rag server
func (r *Reconciler) indexDataWithChunking(ctx context.Context, k *types.Knowledge, data []*indexerData) error {
	splitter, err := text.NewDataPrepSplitter(text.DataPrepTextSplitterOptions{
		ChunkSize: k.RAGSettings.ChunkSize,
		Overflow:  k.RAGSettings.ChunkOverflow,
	})
	if err != nil {
		return fmt.Errorf("failed to create text splitter, error: %w", err)
	}
	documentGroupID := k.ID

	for _, d := range data {
		_, err := splitter.AddDocument(d.Source, string(d.Data), documentGroupID)
		if err != nil {
			return fmt.Errorf("failed to split %s, error %w", d.Source, err)
		}
	}

	ragClient := r.getRagClient(k)

	log.Info().
		Str("knowledge_id", k.ID).
		Int("chunks", len(splitter.Chunks)).
		Msg("submitting chunks into the rag server")

	for _, chunk := range splitter.Chunks {
		err := ragClient.Index(context.Background(), &types.SessionRAGIndexChunk{
			DataEntityID:    k.ID,
			Filename:        chunk.Filename,
			DocumentID:      chunk.DocumentID,
			DocumentGroupID: chunk.DocumentGroupID,
			ContentOffset:   chunk.Index,
			Content:         chunk.Text,
		})
		if err != nil {
			return fmt.Errorf("failed to index chunk '%s', error: %w", chunk.Text, err)
		}
	}

	return nil
}

func getDocumentID(contents []byte) string {
	hash := sha256.Sum256(contents)
	hashString := hex.EncodeToString(hash[:])

	return hashString[:10]
}

// indexerData contains the raw contents of a website, file, etc.
// This might be a text/html/pdf but it could also be something else
// for example an sqlite database.
type indexerData struct {
	Source string
	Data   []byte
}
