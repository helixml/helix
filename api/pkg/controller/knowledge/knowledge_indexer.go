package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"sync/atomic"
	"time"

	"github.com/dustin/go-humanize"
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

		// Sanity check the limits
		if k.Source.Web != nil {
			if r.config.RAG.Crawler.MaxPages > 0 && k.Source.Web.Crawler.MaxPages > r.config.RAG.Crawler.MaxPages {
				log.Warn().Msg("knowledge 'max pages' limit is above the server config, updating")
				k.Source.Web.Crawler.MaxPages = r.config.RAG.Crawler.MaxPages
			}

			if r.config.RAG.Crawler.MaxDepth > 0 && k.Source.Web.Crawler.MaxDepth > r.config.RAG.Crawler.MaxDepth {
				log.Warn().Msg("knowledge 'max depth' limit is above the server config, updating")
				k.Source.Web.Crawler.MaxDepth = r.config.RAG.Crawler.MaxDepth
			}
		}

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

	// Sanity check if we have any data
	err = checkContents(data)
	if err != nil {
		return err
	}

	crawledSources := getCrawledSources(data)

	elapsed := time.Since(start)
	log.Info().
		Str("knowledge_id", k.ID).
		Float64("elapsed_seconds", elapsed.Seconds()).
		Msg("indexing data loaded")

	k.Message = "indexing data"
	k.ProgressPercent = 0
	k.CrawledSources = &types.CrawledSources{
		URLs: crawledSources,
	}

	_, err = r.store.UpdateKnowledge(ctx, k)
	if err != nil {
		log.Error().
			Err(err).
			Str("knowledge_id", k.ID).
			Msg("failed to update knowledge state")
	}

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
		KnowledgeID:    k.ID,
		Version:        version,
		Size:           k.Size,
		State:          types.KnowledgeStateReady,
		CrawledSources: k.CrawledSources,
	})
	if err != nil {
		log.Warn().
			Err(err).
			Str("knowledge_id", k.ID).
			Str("version", version).
			Msg("failed to create knowledge version")
		return fmt.Errorf("failed to create knowledge version, error: %w", err)
	}

	log.Info().
		Str("knowledge_id", k.ID).
		Str("new_version", version).
		Msg("knowledge indexed")

	// Delete old versions
	err = r.deleteOldVersions(ctx, k)
	if err != nil {
		return fmt.Errorf("failed to delete old versions, error: %w", err)
	}

	return nil
}

func (r *Reconciler) deleteOldVersions(ctx context.Context, k *types.Knowledge) error {
	versions, err := r.store.ListKnowledgeVersions(ctx, &store.ListKnowledgeVersionQuery{
		KnowledgeID: k.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to list knowledge versions, error: %w", err)
	}

	if len(versions) <= r.config.RAG.MaxVersions {
		log.Info().
			Str("knowledge_id", k.ID).
			Msg("no need to delete any previous versions as there are less than the max allowed")
		return nil
	}

	// Sort by created date, oldest first
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Created.Before(versions[j].Created)
	})

	// Delete the oldest versions
	for _, v := range versions[:len(versions)-r.config.RAG.MaxVersions] {
		err := r.deleteKnowledgeVersion(ctx, k, v)
		if err != nil {
			log.Warn().
				Err(err).
				Str("knowledge_id", k.ID).
				Str("version", v.Version).
				Msg("failed to delete knowledge version")
		} else {
			log.Info().
				Str("knowledge_id", k.ID).
				Str("version", v.Version).
				Str("size", humanize.Bytes(uint64(k.Size))).
				Msg("deleted old knowledge version")
		}
	}

	return nil
}

// deleteKnowledgeVersion deletes the knowledge data from the vector DB and the version record from the
// postgres database
func (r *Reconciler) deleteKnowledgeVersion(ctx context.Context, k *types.Knowledge, v *types.KnowledgeVersion) error {
	ragClient := r.getRagClient(k)

	err := ragClient.Delete(ctx, &types.DeleteIndexRequest{
		DataEntityID: v.GetDataEntityID(),
	})
	if err != nil {
		return fmt.Errorf("failed to delete knowledge version from vector DB, error: %w", err)
	}

	err = r.store.DeleteKnowledgeVersion(ctx, v.ID)
	if err != nil {
		return fmt.Errorf("failed to delete knowledge version, error: %w", err)
	}

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
	totalItems := int32(len(data))

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		var lastProgress int

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				current := int(progress.Load())
				percentage := int(float32(current) / float32(totalItems) * 100)

				// If we have progress, update the progress
				if percentage != lastProgress {
					r.updateProgress(k, types.KnowledgeStateIndexing, fmt.Sprintf("indexing data %d/%d", current, totalItems), percentage)
					lastProgress = percentage
				}
			}
		}
	}()

	for _, d := range data {
		d := d

		pool.Go(func() error {
			defer progress.Add(1)

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

	// Ensure we update to 100% when done
	r.updateProgress(k, types.KnowledgeStateIndexing, "indexing data completed", 100)

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
		Str("size", humanize.Bytes(uint64(getSize(data)))).
		Msg("submitting chunks into the rag server")

	progress := atomic.Int32{}
	totalItems := int32(len(chunks))

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		var lastProgress int

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				current := int(progress.Load())
				percentage := int(float32(current) / float32(len(chunks)) * 100)

				// If we have progress, update the progress
				if percentage != lastProgress {
					r.updateProgress(k, types.KnowledgeStateIndexing, fmt.Sprintf("indexing data %d/%d chunks", current, totalItems), percentage)
					lastProgress = percentage
				}
			}
		}
	}()

	batches := convertChunksIntoBatches(chunks, 100)

	for _, batch := range batches {
		defer progress.Add(int32(len(batch)))

		// Convert the chunks into index chunks
		indexChunks := convertTextSplitterChunks(k, version, batch)

		// Index the chunks batch
		err := ragClient.Index(ctx, indexChunks...)
		if err != nil {
			return fmt.Errorf("failed to index chunks, error: %w", err)
		}
	}

	// Ensure we update to 100% when done
	r.updateProgress(k, types.KnowledgeStateIndexing, "indexing data completed", 100)

	return nil
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
	Source     string
	Data       []byte
	StatusCode int
	DurationMs int64
	Message    string
}

func convertChunksIntoBatches(chunks []*text.DataPrepTextSplitterChunk, batchSize int) [][]*text.DataPrepTextSplitterChunk {
	batches := make([][]*text.DataPrepTextSplitterChunk, 0, (len(chunks)+batchSize-1)/batchSize)

	for batchSize < len(chunks) {
		chunks, batches = chunks[batchSize:], append(batches, chunks[0:batchSize:batchSize])
	}
	batches = append(batches, chunks)

	return batches
}

func convertTextSplitterChunks(k *types.Knowledge, version string, chunks []*text.DataPrepTextSplitterChunk) []*types.SessionRAGIndexChunk {

	var indexChunks []*types.SessionRAGIndexChunk

	for _, chunk := range chunks {
		indexChunks = append(indexChunks, &types.SessionRAGIndexChunk{
			DataEntityID:    types.GetDataEntityID(k.ID, version),
			Filename:        chunk.Filename,
			Source:          chunk.Filename, // For backwards compatibility
			DocumentID:      chunk.DocumentID,
			DocumentGroupID: chunk.DocumentGroupID,
			ContentOffset:   chunk.Index,
			Content:         chunk.Text,
		})
	}

	return indexChunks
}

func checkContents(data []*indexerData) error {
	if len(data) == 0 {
		return fmt.Errorf("couldn't extract any data for indexing, check your data source or configuration")
	}

	for _, d := range data {
		if len(d.Data) > 0 {
			return nil
		}
	}

	return fmt.Errorf("couldn't extract any data for indexing, check your data source or configuration")
}

func getCrawledSources(data []*indexerData) []*types.CrawledURL {
	var crawledSources []*types.CrawledURL

	for _, d := range data {
		crawledSources = append(crawledSources, &types.CrawledURL{
			URL:        d.Source,
			StatusCode: d.StatusCode,
			DurationMs: d.DurationMs,
			Message:    d.Message,
		})
	}

	return crawledSources
}
