package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"

	"github.com/helixml/helix/api/pkg/dataprep/text"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
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

		// Set version for the indexing process
		// TODO: maybe we should set this only when we have finished indexing
		// so that on failed indexing we can retry without setting a new version
		// and previous version will be used
		k.Version = system.GenerateVersion()

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
				return
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

	start := time.Now()

	data, err := r.getIndexingData(ctx, k)
	if err != nil {
		return fmt.Errorf("failed to get indexing data, error: %w", err)
	}
	elapsed := time.Since(start)
	log.Info().
		Str("knowledge_id", k.ID).
		Float64("elapsed_seconds", elapsed.Seconds()).
		Msg("indexing data loaded")

	start = time.Now()

	err = r.indexData(ctx, k, data)
	if err != nil {
		return fmt.Errorf("indexing failed, error: %w", err)
	}
	elapsed = time.Since(start)
	log.Info().
		Str("knowledge_id", k.ID).
		Float64("elapsed_seconds", elapsed.Seconds()).
		Msg("data indexed")

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
		log.Info().
			Str("knowledge_id", k.ID).
			Str("knowledge_name", k.Name).
			Str("index_url", k.RAGSettings.IndexURL).
			Str("query_url", k.RAGSettings.QueryURL).
			Msg("using custom RAG server")

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

	pool := pool.New().
		WithMaxGoroutines(r.config.RAG.IndexingConcurrency).
		WithErrors()

	for _, d := range data {
		d := d

		pool.Go(func() error {
			err := ragClient.Index(ctx, &types.SessionRAGIndexChunk{
				DataEntityID:    k.GetDataEntityID(),
				Filename:        d.Source,
				Source:          d.Source,
				DocumentID:      getDocumentID(d.Data),
				DocumentGroupID: documentGroupID,
				ContentOffset:   0,
				Content:         string(d.Data),
			})
			if err != nil {
				return fmt.Errorf("failed to index data from source %s, error: %w", d.Source, err)
			}
			return nil
		})
	}

	err := pool.Wait()
	if err != nil {
		return fmt.Errorf("failed to index data, error: %w", err)
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

	pool := pool.New().
		WithMaxGoroutines(r.config.RAG.IndexingConcurrency).
		WithErrors()

	for _, chunk := range splitter.Chunks {
		chunk := chunk
		pool.Go(func() error {
			err := ragClient.Index(ctx, &types.SessionRAGIndexChunk{
				DataEntityID:    k.GetDataEntityID(),
				Filename:        chunk.Filename,
				Source:          chunk.Filename, // For backwards compatibility
				DocumentID:      chunk.DocumentID,
				DocumentGroupID: chunk.DocumentGroupID,
				ContentOffset:   chunk.Index,
				Content:         chunk.Text,
			})
			if err != nil {
				return fmt.Errorf("failed to index chunk '%s', error: %w", chunk.Text, err)
			}
			return nil
		})
	}

	return pool.Wait()
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
