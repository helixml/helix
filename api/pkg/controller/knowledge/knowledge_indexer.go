package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"

	"github.com/helixml/helix/api/pkg/extract"
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

		go func(knowledge *types.Knowledge) {
			log.
				Info().
				Str("knowledge_id", knowledge.ID).
				Msg("indexing knowledge")

			err := r.indexKnowledge(ctx, knowledge)
			if err != nil {
				log.
					Warn().
					Err(err).
					Str("knowledge_id", knowledge.ID).
					Msg("failed to index knowledge")
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

	return nil
}

func (r *Reconciler) getIndexingData(ctx context.Context, k *types.Knowledge) ([]*indexerData, error) {
	switch {
	case k.Source.Web != nil:
		return r.extractDataFromWeb(ctx, k)
	case k.Source.HelixDrive != nil:
		return r.extractDataFromHelixDrive(ctx, k)
	default:
		return nil, fmt.Errorf("unknown source")
	}
}

func (r *Reconciler) extractDataFromWeb(ctx context.Context, k *types.Knowledge) ([]*indexerData, error) {
	if k.Source.Web == nil {
		return nil, fmt.Errorf("no web source defined")
	}

	var result []*indexerData

	if len(k.Source.Web.URLs) == 0 {
		return result, nil
	}

	// Optional mode to disable text extractor and chunking,
	// useful when the indexing server will know how to handle
	// raw data directly
	extractorEnabled := true

	if k.RAGSettings.DisableChunking {
		extractorEnabled = false
	}

	// TODO: add concurrency
	for _, u := range k.Source.Web.URLs {
		if extractorEnabled {
			extracted, err := r.extractor.Extract(ctx, &extract.ExtractRequest{
				URL: u,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to extract data from %s, error: %w", u, err)
			}

			result = append(result, &indexerData{
				Data:   []byte(extracted),
				Source: u,
			})

			continue
		}

		bts, err := r.downloadDirectly(ctx, k, u)
		if err != nil {
			return nil, fmt.Errorf("failed to download data from %s, error: %w", u, err)
		}

		result = append(result, &indexerData{
			Data:   bts,
			Source: u,
		})
	}

	return result, nil
}

func (r *Reconciler) downloadDirectly(ctx context.Context, k *types.Knowledge, u string) ([]byte, error) {
	// Extractor and indexer disabled, downloading directly
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to get %s, error: %w", u, err)
	}

	// If username and password are specified, use them for basic auth
	if k.Source.Web.Auth.Username != "" || k.Source.Web.Auth.Password != "" {
		req.SetBasicAuth(k.Source.Web.Auth.Username, k.Source.Web.Auth.Password)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download, error: %w", err)
	}
	defer resp.Body.Close()

	bts, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body, error: %w", err)
	}

	return bts, nil
}

func (r *Reconciler) extractDataFromHelixDrive(ctx context.Context, k *types.Knowledge) ([]*indexerData, error) {
	return nil, fmt.Errorf("TODO")
}

func (r *Reconciler) getRagClient(k *types.Knowledge) rag.Rag {
	if k.RAGSettings.IndexURL != "" && k.RAGSettings.QueryURL != "" {
		return r.newRagClient(k.RAGSettings.IndexURL, k.RAGSettings.QueryURL)
	}
	return r.ragClient
}

func (r *Reconciler) indexData(ctx context.Context, k *types.Knowledge, data []*indexerData) error {
	if k.RAGSettings.DisableChunking {
		return r.indexDataDirectly(ctx, k, data)
	}

	return nil
}

func (r *Reconciler) indexDataDirectly(ctx context.Context, k *types.Knowledge, data []*indexerData) error {
	documentGroupID := k.ID

	ragClient := r.getRagClient(k)

	for _, d := range data {
		err := ragClient.Index(context.Background(), &types.SessionRAGIndexChunk{
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

	// All good
	return nil

}

func (r *Reconciler) indexDataWithChunking(ctx context.Context, k *types.Knowledge, data []*indexerData) error {

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
