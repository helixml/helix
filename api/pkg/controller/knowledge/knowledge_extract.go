package knowledge

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"

	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

func (r *Reconciler) getIndexingData(ctx context.Context, k *types.Knowledge) ([]*indexerData, error) {
	switch {
	case k.Source.Web != nil:
		return r.extractDataFromWeb(ctx, k)
	case k.Source.Filestore != nil:
		return r.extractDataFromHelixFilestore(ctx, k)
	default:
		return nil, fmt.Errorf("unknown source: %+v", k.Source)
	}
}

func (r *Reconciler) extractDataFromWeb(ctx context.Context, k *types.Knowledge) ([]*indexerData, error) {
	if k.Source.Web == nil {
		return nil, fmt.Errorf("no web source defined")
	}

	if crawlerEnabled(k) {
		return r.extractDataFromWebWithCrawler(ctx, k)
	}

	// nolint:prealloc
	// NOTE: we don't know the size
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

	for _, u := range k.Source.Web.URLs {
		// If we are not downloading the file, we just send the URL
		if k.RAGSettings.DisableDownloading {
			result = append(result, &indexerData{
				Source: u,
			})
			continue
		}

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

		// Download the file
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

func crawlerEnabled(k *types.Knowledge) bool {
	if k.Source.Web == nil {
		return false
	}

	if k.Source.Web.Crawler == nil {
		return false
	}

	if k.Source.Web.Crawler.Enabled {
		return true
	}

	return false
}

func (r *Reconciler) extractDataFromWebWithCrawler(ctx context.Context, k *types.Knowledge) ([]*indexerData, error) {
	if k.Source.Web == nil {
		return nil, fmt.Errorf("no web source defined")
	}

	if k.Source.Web.Crawler == nil {
		return nil, fmt.Errorf("no crawler defined")
	}

	crawler, err := r.newCrawler(k)
	if err != nil {
		return nil, fmt.Errorf("failed to create crawler: %w", err)
	}

	result, err := crawler.Crawl(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to crawl: %w", err)
	}

	data := make([]*indexerData, 0, len(result))

	for _, doc := range result {
		data = append(data, &indexerData{
			Data:   []byte(doc.Content),
			Source: doc.SourceURL,
		})
	}

	return data, nil
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

func (r *Reconciler) extractDataFromHelixFilestore(ctx context.Context, k *types.Knowledge) ([]*indexerData, error) {
	data, err := r.getFilestoreFiles(ctx, r.filestore, k)
	if err != nil {
		return nil, fmt.Errorf("failed to get filestore files: %w", err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("no data found in filestore")
	}

	var totalSize int64

	for _, d := range data {
		totalSize += int64(len(d.Data))
	}

	log.Info().
		Str("knowledge_id", k.ID).
		Int64("total_size", totalSize).
		Int64("count", int64(len(data))).
		Msg("filestore data found")

	// Optional mode to disable text extractor and chunking,
	// useful when the indexing server will know how to handle
	// raw data directly
	if k.RAGSettings.DisableChunking {
		return data, nil
	}

	// Chunking enabled, extracting text
	// nolint:prealloc
	// NOTE: we don't know the size
	var extractedData []*indexerData

	for _, d := range data {
		extractedText, err := r.extractor.Extract(ctx, &extract.ExtractRequest{
			Content: d.Data,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to extract data from %s, error: %w", d.Source, err)
		}

		extractedData = append(extractedData, &indexerData{
			Data:   []byte(extractedText),
			Source: d.Source,
		})
	}

	return extractedData, nil
}

func (r *Reconciler) getFilestoreFiles(ctx context.Context, fs filestore.FileStore, k *types.Knowledge) ([]*indexerData, error) {
	var result []*indexerData

	var recursiveList func(path string) error

	recursiveList = func(path string) error {
		items, err := fs.List(ctx, path)
		if err != nil {
			return fmt.Errorf("failed to list files at %s, error: %w", path, err)
		}

		for _, item := range items {
			if item.Directory {
				err := recursiveList(item.Path)
				if err != nil {
					return err
				}
			} else {
				r, err := fs.OpenFile(ctx, item.Path)
				if err != nil {
					return fmt.Errorf("failed to open file at %s, error: %w", item.Path, err)
				}

				bts, err := io.ReadAll(r)
				if err != nil {
					return fmt.Errorf("failed to read file at %s, error: %w", item.Path, err)
				}

				result = append(result, &indexerData{
					Data:   bts,
					Source: item.Path,
				})
			}
		}
		return nil
	}

	userPrefix := filestore.GetUserPrefix(r.config.Controller.FilePrefixGlobal, k.Owner)

	path := filepath.Join(userPrefix, k.Source.Filestore.Path)

	err := recursiveList(path)
	if err != nil {
		return nil, err
	}

	return result, nil
}
