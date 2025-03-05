package knowledge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

var ErrNoFilesFound = errors.New("no files found in filestore")

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
				Source:          u,
				DocumentGroupID: getDocumentGroupID(u),
			})
			continue
		}

		if extractorEnabled {
			extracted, err := r.extractor.Extract(ctx, &extract.Request{
				URL: u,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to extract data from %s, error: %w", u, err)
			}

			result = append(result, &indexerData{
				Data:            []byte(extracted),
				Source:          u,
				DocumentGroupID: getDocumentGroupID(u),
			})

			continue
		}

		// Download the file
		bts, err := r.downloadDirectly(ctx, k, u)
		if err != nil {
			return nil, fmt.Errorf("failed to download data from %s, error: %w", u, err)
		}

		result = append(result, &indexerData{
			Data:            bts,
			Source:          u,
			DocumentGroupID: getDocumentGroupID(u),
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
			Data:            []byte(doc.Content),
			Source:          doc.SourceURL,
			DocumentGroupID: getDocumentGroupID(doc.SourceURL),
			StatusCode:      doc.StatusCode,
			DurationMs:      doc.DurationMs,
			Message:         doc.Message,
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
	// Start with detailed logging of inputs
	log.Debug().
		Str("knowledge_id", k.ID).
		Str("app_id", k.AppID).
		Str("original_path", k.Source.Filestore.Path).
		Msgf("Extracting data from Helix filestore")

	// Ensure the knowledge source has a filestore path that is scoped to the app's directory
	if k.Source.Filestore != nil && k.Source.Filestore.Path != "" {
		// Check if the path is already properly scoped to the app's directory
		expectedPrefix := fmt.Sprintf("apps/%s/", k.AppID)

		log.Debug().
			Str("knowledge_id", k.ID).
			Str("original_path", k.Source.Filestore.Path).
			Str("expected_prefix", expectedPrefix).
			Bool("has_prefix", strings.HasPrefix(k.Source.Filestore.Path, expectedPrefix)).
			Msgf("Checking if path has app prefix")

		if !strings.HasPrefix(k.Source.Filestore.Path, expectedPrefix) {
			// If not, construct the appropriate path relative to the app's directory
			// Extract the knowledge name from the path or use the source name
			parts := strings.Split(k.Source.Filestore.Path, "/")
			knowledgeName := parts[len(parts)-1]
			if knowledgeName == "" {
				knowledgeName = k.Name
			}

			// Update the path to be scoped to the app's directory
			originalPath := k.Source.Filestore.Path
			k.Source.Filestore.Path = filepath.Join("apps", k.AppID, knowledgeName)

			log.Info().
				Str("knowledge_id", k.ID).
				Str("original_path", originalPath).
				Str("updated_path", k.Source.Filestore.Path).
				Msgf("Updated knowledge path to include app prefix")

			// Save the updated knowledge
			_, err := r.store.UpdateKnowledge(ctx, k)
			if err != nil {
				log.Warn().Err(err).Msgf("failed to update knowledge path: %s", err)
			}
		}
	}

	data, err := r.getFilestoreFiles(ctx, r.filestore, k)
	if err != nil {
		log.Warn().
			Err(err).
			Str("knowledge_id", k.ID).
			Str("path", k.Source.Filestore.Path).
			Msgf("Failed to get filestore files")
		return nil, fmt.Errorf("failed to get filestore files: %w", err)
	}

	if len(data) == 0 {
		log.Warn().
			Str("knowledge_id", k.ID).
			Str("path", k.Source.Filestore.Path).
			Msgf("No files found in filestore")
		return nil, ErrNoFilesFound
	}

	var totalSize int64

	for _, d := range data {
		totalSize += int64(len(d.Data))
	}

	log.Info().
		Str("knowledge_id", k.ID).
		Int("file_count", len(data)).
		Int64("total_size_bytes", totalSize).
		Str("path", k.Source.Filestore.Path).
		Msgf("Successfully extracted data from filestore")

	// Optional mode to disable text extractor and chunking,
	// useful when the indexing server will know how to handle
	// raw data directly
	if k.RAGSettings.DisableChunking {
		return data, nil
	}

	// Chunking enabled, extracting text
	var extractedData []*indexerData

	for _, d := range data {
		extractedText, err := r.extractor.Extract(ctx, &extract.Request{
			Content: d.Data,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to extract data from %s, error: %w", d.Source, err)
		}

		extractedData = append(extractedData, &indexerData{
			Data:            []byte(extractedText),
			Source:          d.Source,
			DocumentGroupID: getDocumentGroupID(d.Source),
		})
	}

	return extractedData, nil
}

func (r *Reconciler) getFilestoreFiles(ctx context.Context, fs filestore.FileStore, k *types.Knowledge) ([]*indexerData, error) {
	var result []*indexerData

	log.Debug().
		Str("knowledge_id", k.ID).
		Str("path", k.Source.Filestore.Path).
		Msgf("Getting files from filestore")

	var recursiveList func(path string) error

	recursiveList = func(path string) error {
		log.Debug().
			Str("knowledge_id", k.ID).
			Str("listing_path", path).
			Msgf("Listing files at path")

		items, err := fs.List(ctx, path)
		if err != nil {
			log.Warn().
				Err(err).
				Str("knowledge_id", k.ID).
				Str("path", path).
				Msgf("Failed to list files")
			return fmt.Errorf("failed to list files at %s, error: %w", path, err)
		}

		log.Debug().
			Str("knowledge_id", k.ID).
			Str("path", path).
			Int("item_count", len(items)).
			Msgf("Found items in filestore")

		for _, item := range items {
			if item.Directory {
				log.Debug().
					Str("knowledge_id", k.ID).
					Str("directory", item.Path).
					Msgf("Found directory, recursing")
				err := recursiveList(item.Path)
				if err != nil {
					return err
				}
			} else {
				log.Debug().
					Str("knowledge_id", k.ID).
					Str("file", item.Path).
					Int64("size", item.Size).
					Msgf("Found file")

				r, err := fs.OpenFile(ctx, item.Path)
				if err != nil {
					log.Warn().
						Err(err).
						Str("knowledge_id", k.ID).
						Str("file", item.Path).
						Msgf("Failed to open file")
					return fmt.Errorf("failed to open file at %s, error: %w", item.Path, err)
				}

				defer r.Close()

				data, err := io.ReadAll(r)
				if err != nil {
					log.Warn().
						Err(err).
						Str("knowledge_id", k.ID).
						Str("file", item.Path).
						Msgf("Failed to read file")
					return fmt.Errorf("failed to read file at %s, error: %w", item.Path, err)
				}

				result = append(result, &indexerData{
					Source:          item.Path,
					Data:            data,
					DocumentGroupID: getDocumentGroupID(item.Path),
				})

				log.Debug().
					Str("knowledge_id", k.ID).
					Str("file", item.Path).
					Int("data_size", len(data)).
					Msgf("Successfully read file")
			}
		}
		return nil
	}

	userPrefix := filestore.GetUserPrefix(r.config.Controller.FilePrefixGlobal, k.Owner)

	path := filepath.Join(userPrefix, k.Source.Filestore.Path)

	log.Info().
		Str("knowledge_id", k.ID).
		Str("original_path", k.Source.Filestore.Path).
		Str("user_prefix", userPrefix).
		Str("full_path", path).
		Msgf("Starting recursive file listing")

	err := recursiveList(path)
	if err != nil {
		return nil, err
	}

	log.Info().
		Str("knowledge_id", k.ID).
		Int("file_count", len(result)).
		Msgf("Completed file listing")

	return result, nil
}
