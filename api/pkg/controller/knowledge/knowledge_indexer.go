package knowledge

import (
	"context"
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
			return fmt.Errorf("failed to update knowledge")
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

	return nil
}

func (r *Reconciler) getIndexingData(ctx context.Context, k *types.Knowledge) ([]*indexerData, error) {
	switch {
	case k.Source.Web != nil:
		return r.extractDataFromWeb(ctx, k)
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
	req, err := http.NewRequest(http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to get %s, error: %w", u, err)
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

func (r *Reconciler) indexData(ctx context.Context, k *types.Knowledge, data []*indexerData) error {

	return nil
}

// indexerData contains the raw contents of a website, file, etc.
// This might be a text/html/pdf but it could also be something else
// for example an sqlite database.
type indexerData struct {
	Source string
	Data   []byte
}
