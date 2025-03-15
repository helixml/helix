package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"github.com/helixml/helix/api/pkg/dataprep/text"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

const (
	knowledgeUploadWaitTime = 10 * time.Minute
)

func (r *Reconciler) index(ctx context.Context) error {
	data, err := r.store.ListKnowledge(ctx, &store.ListKnowledgeQuery{
		State: types.KnowledgeStatePending,
	})
	if err != nil {
		return fmt.Errorf("failed to get knowledge entries, error: %w", err)
	}

	// Note: We only process knowledge sources in "Pending" state
	// Knowledge sources in "Preparing" state are ignored by the reconciler
	// and will only be processed when explicitly moved to "Pending" state
	for _, k := range data {
		r.wg.Add(1)

		k.State = types.KnowledgeStateIndexing
		k.Message = ""

		// Sanity check the limits
		if k.Source.Web != nil && k.Source.Web.Crawler != nil {
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

				// If it's just recently created record, we can retry. If it's older than 10 minutes
				// then leave it as error.
				// Note: only applicable to filestore sources!
				if errors.Is(err, ErrNoFilesFound) && knowledge.Source.Filestore != nil && time.Since(knowledge.Created) < knowledgeUploadWaitTime {
					k.State = types.KnowledgeStatePending
					k.Message = "waiting for files to be uploaded"
					_, _ = r.store.UpdateKnowledge(ctx, k)

					// Create a pending version for logs and test expectations
					_, _ = r.store.CreateKnowledgeVersion(ctx, &types.KnowledgeVersion{
						KnowledgeID:     k.ID,
						Version:         version,
						Size:            k.Size,
						State:           types.KnowledgeStatePending,
						Message:         "waiting for files to be uploaded",
						EmbeddingsModel: r.config.RAG.PGVector.EmbeddingsModel,
						Provider:        string(r.config.RAG.DefaultRagProvider),
					})
					return
				}

				k.State = types.KnowledgeStateError
				k.Message = err.Error()
				_, _ = r.store.UpdateKnowledge(ctx, k)

				// Create a failed version too just for logs
				_, _ = r.store.CreateKnowledgeVersion(ctx, &types.KnowledgeVersion{
					KnowledgeID:     k.ID,
					Version:         version,
					Size:            k.Size,
					State:           types.KnowledgeStateError,
					Message:         err.Error(),
					EmbeddingsModel: r.config.RAG.PGVector.EmbeddingsModel,
					Provider:        string(r.config.RAG.DefaultRagProvider),
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

	if err := r.updateProgress(k, types.KnowledgeStateIndexing, "retrieving data for indexing"); err != nil {
		return fmt.Errorf("failed to update progress when retrieving data: %v", err)
	}

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
		Int("crawled_sources", len(crawledSources)).
		Msg("indexing data loaded")

	k.Message = "indexing data"
	k.CrawledSources = &types.CrawledSources{
		URLs: crawledSources,
	}

	r.updateKnowledgeProgress(k.ID, types.KnowledgeProgress{
		Step:           "Indexing",
		Progress:       0,
		ElapsedSeconds: int(elapsed.Seconds()),
		Message:        fmt.Sprintf("indexing data loaded in %s", elapsed),
		StartedAt:      start,
	})

	_, err = r.store.UpdateKnowledge(ctx, k)
	if err != nil {
		log.Error().
			Err(err).
			Str("knowledge_id", k.ID).
			Msg("failed to update knowledge state")
	}

	start = time.Now()

	err = r.indexData(ctx, k, version, data, start)
	if err != nil {
		return fmt.Errorf("indexing failed, error: %w", err)
	}
	elapsed = time.Since(start)
	log.Info().
		Str("knowledge_id", k.ID).
		Str("new_version", version).
		Float64("elapsed_seconds", elapsed.Seconds()).
		Msg("data indexed")

	// Reset the progress
	r.resetKnowledgeProgress(k.ID)

	k.State = types.KnowledgeStateReady
	k.Size = getSize(data)
	k.Version = version // Set latest version
	k.Message = ""

	_, err = r.store.UpdateKnowledge(ctx, k)
	if err != nil {
		return fmt.Errorf("failed to update knowledge, error: %w", err)
	}

	_, err = r.store.CreateKnowledgeVersion(ctx, &types.KnowledgeVersion{
		KnowledgeID:     k.ID,
		Version:         version,
		Size:            k.Size,
		State:           types.KnowledgeStateReady,
		CrawledSources:  k.CrawledSources,
		EmbeddingsModel: r.config.RAG.PGVector.EmbeddingsModel,
		Provider:        string(r.config.RAG.DefaultRagProvider),
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

func (r *Reconciler) indexData(ctx context.Context, k *types.Knowledge, version string, data []*indexerData, startedAt time.Time) error {
	if k.RAGSettings.DisableChunking {
		return r.indexDataDirectly(ctx, k, version, data, startedAt)
	}
	return r.indexDataWithChunking(ctx, k, version, data, startedAt)
}

func (r *Reconciler) indexDataDirectly(ctx context.Context, k *types.Knowledge, version string, data []*indexerData, startedAt time.Time) error {
	ragClient := r.getRagClient(k)

	log.Info().
		Str("knowledge_id", k.ID).
		Int("payloads", len(data)).
		Msg("submitting raw data into the rag server")

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// TODO: we probably want some parallelism here, up to whatever the pdf parser + embeddings server can manage
	// experiment with some values to see what gets is 100, 1K, 15K PDFs handled fastest.
	for idx, d := range data {
		d := d

		percentage := int(float32(idx) / float32(len(data)) * 100)

		r.updateKnowledgeProgress(k.ID, types.KnowledgeProgress{
			Step:           "Indexing",
			Progress:       percentage,
			ElapsedSeconds: int(time.Since(startedAt).Seconds()),
			Message:        fmt.Sprintf("indexing data %d/%d", idx+1, len(data)),
			StartedAt:      startedAt,
		})

		err := ragClient.Index(ctx, &types.SessionRAGIndexChunk{
			DataEntityID:    types.GetDataEntityID(k.ID, version),
			Filename:        d.Source,
			Source:          d.Source,
			DocumentID:      getDocumentID(d.Data),
			DocumentGroupID: getDocumentGroupID(d.Source),
			ContentOffset:   0,
			Content:         string(d.Data),
		})
		if err != nil {
			// return fmt.Errorf("failed to index data from source %s, error: %w", d.Source, err)
			log.Warn().
				Err(err).
				Str("knowledge_id", k.ID).
				Str("source", d.Source).
				Msg("failed to index data chunk")
		}
	}

	// Ensure we update to 100% when done
	if err := r.updateProgress(k, types.KnowledgeStateIndexing, "indexing data completed"); err != nil {
		return fmt.Errorf("failed to update progress when completed data retrieval: %v", err)
	}

	// All good, nothing else to do
	return nil
}

// indexDataWithChunking we expect to be operating on text data, first we split,
// then index with the rag server
func (r *Reconciler) indexDataWithChunking(ctx context.Context, k *types.Knowledge, version string, data []*indexerData, startedAt time.Time) error {
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

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	batches := convertChunksIntoBatches(chunks, 50)

	for idx, batch := range batches {
		// Convert the chunks into index chunks
		indexChunks := r.convertTextSplitterChunks(ctx, k, version, batch)

		percentage := int(float32(idx) / float32(len(batches)) * 100)

		r.updateKnowledgeProgress(k.ID, types.KnowledgeProgress{
			Step:           "Indexing",
			Progress:       percentage,
			ElapsedSeconds: int(time.Since(startedAt).Seconds()),
			Message:        fmt.Sprintf("indexing data %d/%d chunks", idx+1, len(batches)),
			StartedAt:      startedAt,
		})

		// Index the chunks batch
		err := ragClient.Index(ctx, indexChunks...)
		if err != nil {
			return fmt.Errorf("failed to index chunks, error: %w", err)
		}
	}

	// Ensure we update to 100% when done
	if err := r.updateProgress(k, types.KnowledgeStateIndexing, "indexing data completed"); err != nil {
		return fmt.Errorf("failed to update progress when completed data indexing: %v", err)
	}

	return nil
}

func (r *Reconciler) updateProgress(k *types.Knowledge, state types.KnowledgeState, message string) error {
	return r.store.UpdateKnowledgeState(context.Background(), k.ID, state, message)
}

func getDocumentID(contents []byte) string {
	hash := sha256.Sum256(contents)
	hashString := hex.EncodeToString(hash[:])

	return hashString[:10]
}

func getDocumentGroupID(sourceURL string) string {
	hash := sha256.Sum256([]byte(sourceURL))
	hashString := hex.EncodeToString(hash[:])

	return hashString[:10]
}

// indexerData contains the raw contents of a website, file, etc.
// This might be a text/html/pdf but it could also be something else
// for example an sqlite database.
type indexerData struct {
	Source          string
	DocumentGroupID string
	Data            []byte
	StatusCode      int
	DurationMs      int64
	Message         string
}

func convertChunksIntoBatches(chunks []*text.DataPrepTextSplitterChunk, batchSize int) [][]*text.DataPrepTextSplitterChunk {
	batches := make([][]*text.DataPrepTextSplitterChunk, 0, (len(chunks)+batchSize-1)/batchSize)

	for batchSize < len(chunks) {
		chunks, batches = chunks[batchSize:], append(batches, chunks[0:batchSize:batchSize])
	}
	batches = append(batches, chunks)

	return batches
}

func (r *Reconciler) convertTextSplitterChunks(ctx context.Context, k *types.Knowledge, version string, chunks []*text.DataPrepTextSplitterChunk) []*types.SessionRAGIndexChunk {
	indexChunks := make([]*types.SessionRAGIndexChunk, 0, len(chunks))

	// Keep a cache of metadata for files
	metadataCache := make(map[string]map[string]string)

	for _, chunk := range chunks {
		// Check if we already have metadata for this file in the cache
		metadata, ok := metadataCache[chunk.Filename]
		if !ok {
			// Try to find and load metadata file from the filestore
			metadataFilePath := chunk.Filename + ".metadata.yaml"

			// Check if the metadata file exists in the filestore
			// This would need to be implemented based on how we access the filestore
			// For now, we're just assuming a function that checks and loads metadata
			var err error
			metadata, err = r.getMetadataFromFilestore(ctx, metadataFilePath)
			if err != nil {
				// Log but continue - metadata is optional
				log.Warn().
					Err(err).
					Str("knowledge_id", k.ID).
					Str("metadata_file", metadataFilePath).
					Msg("Failed to load metadata file")
			}

			// Cache the metadata (even if nil) to avoid repeated lookups
			metadataCache[chunk.Filename] = metadata
		}

		indexChunks = append(indexChunks, &types.SessionRAGIndexChunk{
			DataEntityID:    types.GetDataEntityID(k.ID, version),
			Filename:        chunk.Filename,
			Source:          chunk.Filename, // For backwards compatibility
			DocumentID:      chunk.DocumentID,
			DocumentGroupID: chunk.DocumentGroupID,
			ContentOffset:   chunk.Index,
			Content:         chunk.Text,
			Metadata:        metadata,
		})
	}

	return indexChunks
}

// getMetadataFromFilestore attempts to retrieve and parse a metadata.yaml file from the filestore
func (r *Reconciler) getMetadataFromFilestore(ctx context.Context, metadataFilePath string) (map[string]string, error) {
	// Check if the metadata file exists
	_, err := r.filestore.Get(ctx, metadataFilePath)
	if err != nil {
		// If the file doesn't exist, just return nil with no error
		return nil, nil
	}

	// Open the metadata file
	reader, err := r.filestore.OpenFile(ctx, metadataFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open metadata file: %w", err)
	}
	defer reader.Close()

	// Read the content
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata file: %w", err)
	}

	// Parse the YAML
	var metadataFile struct {
		Metadata map[string]string `yaml:"metadata"`
	}

	if err := yaml.Unmarshal(data, &metadataFile); err != nil {
		return nil, fmt.Errorf("failed to parse metadata file: %w", err)
	}

	return metadataFile.Metadata, nil
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
			DocumentID: getDocumentID(d.Data),
		})
	}

	return crawledSources
}
