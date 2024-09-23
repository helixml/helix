package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"

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

		_, _ = r.store.UpdateKnowledge(ctx, k)

		log.
			Info().
			Str("knowledge_id", k.ID).
			Msg("indexing knowledge")

		go func(knowledge *types.Knowledge) {
			defer r.wg.Done()

			version := system.GenerateVersion()

			err := r.indexKnowledge(ctx, knowledge, version)
			if err != nil {
				log.
					Warn().
					Err(err).
					Str("knowledge_id", knowledge.ID).
					Msg("failed to index knowledge")

				k.State = types.KnowledgeStateError
				k.Message = err.Error()
				_, _ = r.store.UpdateKnowledge(ctx, k)

				// Create a failed version too just for logs
				_, _ = r.store.CreateKnowledgeVersion(ctx, &types.KnowledgeVersion{
					KnowledgeID: k.ID,
					Version:     version,
					Size:        k.Size,
					State:       types.KnowledgeStateError,
					Message:     err.Error(),
				})
				return
			}

		}(k)
	}

	return nil
}

func (r *Reconciler) indexKnowledge(ctx context.Context, k *types.Knowledge, version string) error {
	// If source is plain text, nothing to do
	if k.Source.Content != nil {
		k.State = types.KnowledgeStateReady
		k.Version = version
		_, err := r.store.UpdateKnowledge(ctx, k)
		if err != nil {
			return fmt.Errorf("failed to update knowledge, error: %w", err)
		}
		return nil
	}

	start := time.Now()

	r.updateProgress(k, types.KnowledgeStateIndexing, "retrieving data for indexing", 0)

	data, err := r.getIndexingData(ctx, k)
	if err != nil {
		return fmt.Errorf("failed to get indexing data, error: %w", err)
	}
	elapsed := time.Since(start)
	log.Info().
		Str("knowledge_id", k.ID).
		Float64("elapsed_seconds", elapsed.Seconds()).
		Msg("indexing data loaded")

	r.updateProgress(k, types.KnowledgeStateIndexing, "indexing data", 0)

	start = time.Now()

	err = r.indexData(ctx, k, version, data)
	if err != nil {
		return fmt.Errorf("indexing failed, error: %w", err)
	}
	elapsed = time.Since(start)
	log.Info().
		Str("knowledge_id", k.ID).
		Str("new_version", version).
		Float64("elapsed_seconds", elapsed.Seconds()).
		Msg("data indexed")

	k.State = types.KnowledgeStateReady
	k.Size = getSize(data)
	k.Version = version // Set latest version

	_, err = r.store.UpdateKnowledge(ctx, k)
	if err != nil {
		return fmt.Errorf("failed to update knowledge, error: %w", err)
	}

	_, err = r.store.CreateKnowledgeVersion(ctx, &types.KnowledgeVersion{
		KnowledgeID: k.ID,
		Version:     version,
		Size:        k.Size,
		State:       types.KnowledgeStateReady,
	})
	if err != nil {
		log.Warn().
			Err(err).
			Str("knowledge_id", k.ID).
			Str("version", version).
			Msg("failed to create knowledge version")
	}

	log.Info().
		Str("knowledge_id", k.ID).
		Str("new_version", version).
		Msg("knowledge indexed")

	return nil
}

func getSize(data []*indexerData) int64 {
	size := int64(0)
	for _, d := range data {
		size += int64(len(d.Data))
	}
	return size
}

func (r *Reconciler) getRagClient(k *types.Knowledge) rag.RAG {
	if k.RAGSettings.IndexURL != "" && k.RAGSettings.QueryURL != "" {
		log.Info().
			Str("knowledge_id", k.ID).
			Str("knowledge_name", k.Name).
			Str("index_url", k.RAGSettings.IndexURL).
			Str("query_url", k.RAGSettings.QueryURL).
			Msg("using custom RAG server")

		return r.newRagClient(&k.RAGSettings)
	}
	return r.ragClient
}

func (r *Reconciler) indexData(ctx context.Context, k *types.Knowledge, version string, data []*indexerData) error {
	if k.RAGSettings.DisableChunking {
		return r.indexDataDirectly(ctx, k, version, data)
	}
	return r.indexDataWithChunking(ctx, k, version, data)
}

func (r *Reconciler) indexDataDirectly(ctx context.Context, k *types.Knowledge, version string, data []*indexerData) error {
	documentGroupID := k.ID

	ragClient := r.getRagClient(k)

	log.Info().
		Str("knowledge_id", k.ID).
		Int("payloads", len(data)).
		Msg("submitting raw data into the rag server")

	pool := pool.New().
		WithMaxGoroutines(r.config.RAG.IndexingConcurrency).
		WithErrors()

	progress := atomic.Int32{}

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		var lastProgress int

		for {
			select {
			case <-ticker.C:
				current := int(progress.Load())
				// If we have progress, update the progress
				if current != lastProgress {
					r.updateProgress(k, types.KnowledgeStateIndexing, "indexing data", current)
					lastProgress = int(current)
				}
			}
		}
	}()

	for _, d := range data {
		d := d

		pool.Go(func() error {
			err := ragClient.Index(ctx, &types.SessionRAGIndexChunk{
				DataEntityID:    types.GetDataEntityID(k.ID, version),
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
func (r *Reconciler) indexDataWithChunking(ctx context.Context, k *types.Knowledge, version string, data []*indexerData) error {
	chunks, err := splitData(k, data)
	if err != nil {
		return fmt.Errorf("failed to split data, error: %w", err)
	}

	ragClient := r.getRagClient(k)

	log.Info().
		Str("knowledge_id", k.ID).
		Int("chunks", len(chunks)).
		Msg("submitting chunks into the rag server")

	pool := pool.New().WithContext(ctx).
		WithMaxGoroutines(r.config.RAG.IndexingConcurrency).
		WithCancelOnError()

	for _, chunk := range chunks {
		chunk := chunk
		pool.Go(func(ctx context.Context) error {
			err := ragClient.Index(ctx, &types.SessionRAGIndexChunk{
				DataEntityID:    types.GetDataEntityID(k.ID, version),
				Filename:        chunk.Filename,
				Source:          chunk.Filename, // For backwards compatibility
				DocumentID:      chunk.DocumentID,
				DocumentGroupID: chunk.DocumentGroupID,
				ContentOffset:   chunk.Index,
				Content:         chunk.Text,
			})
			if err != nil {
				return fmt.Errorf("failed to index chunk, error: %w", err)
			}
			return nil
		})
	}

	return pool.Wait()
}

func (r *Reconciler) updateProgress(k *types.Knowledge, state types.KnowledgeState, message string, percent int) error {
	return r.store.UpdateKnowledgeState(context.Background(), k.ID, state, message, percent)
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
