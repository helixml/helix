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
	"github.com/helixml/helix/api/pkg/sharepoint"
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
	case k.Source.SharePoint != nil:
		return r.extractDataFromSharePoint(ctx, k)
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

	// Ensure we have an app ID for app-scoped paths
	if k.AppID == "" {
		return nil, fmt.Errorf("knowledge must be associated with an app")
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

		// Create new data item with preserved metadata from the original file
		extractedItem := &indexerData{
			Data:            []byte(extractedText),
			Source:          d.Source,
			DocumentGroupID: getDocumentGroupID(d.Source),
			// Preserve metadata from the original file
			Metadata: d.Metadata,
		}

		// Add the extracted data with metadata preserved
		extractedData = append(extractedData, extractedItem)
	}

	return extractedData, nil
}

func (r *Reconciler) getFilestoreFiles(ctx context.Context, fs filestore.FileStore, k *types.Knowledge) ([]*indexerData, error) {
	var result []*indexerData

	log.Debug().
		Str("knowledge_id", k.ID).
		Str("path", k.Source.Filestore.Path).
		Msgf("Getting files from filestore")

	// Determine the physical storage path based on the knowledge path
	var path string

	// If the path already has the app prefix, use it directly
	if strings.HasPrefix(k.Source.Filestore.Path, fmt.Sprintf("apps/%s/", k.AppID)) {
		// The path already includes the apps/app_id prefix
		appPrefix := filestore.GetAppPrefix(r.config.Controller.FilePrefixGlobal, k.AppID)
		relativePath := strings.TrimPrefix(k.Source.Filestore.Path, fmt.Sprintf("apps/%s/", k.AppID))
		path = filepath.Join(appPrefix, relativePath)

		log.Debug().
			Str("knowledge_id", k.ID).
			Str("app_id", k.AppID).
			Str("path_type", "already_prefixed").
			Str("physical_path", path).
			Msgf("Using already prefixed path")
	} else {
		// Simple path (like "pdfs") - construct the app-scoped path
		appPrefix := filestore.GetAppPrefix(r.config.Controller.FilePrefixGlobal, k.AppID)
		path = filepath.Join(appPrefix, k.Source.Filestore.Path)

		log.Debug().
			Str("knowledge_id", k.ID).
			Str("app_id", k.AppID).
			Str("path_type", "simple").
			Str("logical_path", k.Source.Filestore.Path).
			Str("physical_path", path).
			Msgf("Using inferred app-scoped path")
	}

	log.Info().
		Str("knowledge_id", k.ID).
		Str("original_path", k.Source.Filestore.Path).
		Str("physical_path", path).
		Msgf("Starting recursive file listing")

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

				// Skip metadata YAML files from being indexed directly
				if strings.HasSuffix(item.Path, ".metadata.yaml") {
					log.Debug().
						Str("knowledge_id", k.ID).
						Str("file", item.Path).
						Msgf("Skipping metadata file from direct indexing")
					continue
				}

				fileReader, err := fs.OpenFile(ctx, item.Path)
				if err != nil {
					log.Warn().
						Err(err).
						Str("knowledge_id", k.ID).
						Str("file", item.Path).
						Msgf("Failed to open file")
					return fmt.Errorf("failed to open file at %s, error: %w", item.Path, err)
				}

				defer fileReader.Close()

				data, err := io.ReadAll(fileReader)
				if err != nil {
					log.Warn().
						Err(err).
						Str("knowledge_id", k.ID).
						Str("file", item.Path).
						Msgf("Failed to read file")
					return fmt.Errorf("failed to read file at %s, error: %w", item.Path, err)
				}

				// Create the indexer data item
				indexerItem := &indexerData{
					Source:          item.Path,
					Data:            data,
					DocumentGroupID: getDocumentGroupID(item.Path),
				}

				// Try to load metadata for this file
				metadataFilePath := item.Path + ".metadata.yaml"
				metadata, metadataErr := r.getMetadataFromFilestore(ctx, metadataFilePath)
				if metadataErr == nil && metadata != nil {
					// Convert map[string]string to map[string]interface{}
					metadataMap := make(map[string]interface{})
					for k, v := range metadata {
						metadataMap[k] = v
					}
					indexerItem.Metadata = metadataMap

					log.Debug().
						Str("knowledge_id", k.ID).
						Str("file", item.Path).
						Interface("metadata", metadata).
						Msgf("Added metadata to file")
				}

				result = append(result, indexerItem)

				log.Debug().
					Str("knowledge_id", k.ID).
					Str("file", item.Path).
					Int("data_size", len(data)).
					Bool("has_metadata", indexerItem.Metadata != nil).
					Msgf("Successfully read file")
			}
		}
		return nil
	}

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

// extractDataFromSharePoint fetches files from a SharePoint site/drive using Microsoft Graph API
func (r *Reconciler) extractDataFromSharePoint(ctx context.Context, k *types.Knowledge) ([]*indexerData, error) {
	if k.Source.SharePoint == nil {
		return nil, fmt.Errorf("no SharePoint source defined")
	}

	spConfig := k.Source.SharePoint

	log.Info().
		Str("knowledge_id", k.ID).
		Str("site_id", spConfig.SiteID).
		Str("drive_id", spConfig.DriveID).
		Str("folder_path", spConfig.FolderPath).
		Bool("recursive", spConfig.Recursive).
		Msg("Extracting data from SharePoint")

	// Check if OAuth manager is available
	if r.oauthManager == nil {
		return nil, fmt.Errorf("OAuth manager not configured, cannot access SharePoint")
	}

	// Get the OAuth connection for the knowledge owner
	connection, err := r.oauthManager.GetConnection(ctx, k.Owner, spConfig.OAuthProviderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get OAuth connection for SharePoint: %w", err)
	}

	// Verify the connection is for Microsoft OAuth (either Microsoft type or Custom with Microsoft URLs)
	if !isMicrosoftOAuthProvider(connection.Provider) {
		return nil, fmt.Errorf("OAuth provider must be Microsoft type (or Custom with Microsoft URLs) for SharePoint access, got: %s", connection.Provider.Type)
	}

	// Create SharePoint client with the access token
	spClient := sharepoint.NewClient(connection.AccessToken, r.config.Tools.TLSSkipVerify)

	// List files from SharePoint
	files, err := spClient.ListFiles(ctx, spConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to list SharePoint files: %w", err)
	}

	if len(files) == 0 {
		log.Warn().
			Str("knowledge_id", k.ID).
			Str("site_id", spConfig.SiteID).
			Msg("No files found in SharePoint")
		return nil, ErrNoFilesFound
	}

	log.Info().
		Str("knowledge_id", k.ID).
		Int("file_count", len(files)).
		Msg("Found files in SharePoint")

	// Update progress
	r.updateKnowledgeProgress(k.ID, types.KnowledgeProgress{
		Step:     "downloading",
		Progress: 0,
		Message:  fmt.Sprintf("Downloading %d files from SharePoint", len(files)),
	})

	// Get the drive ID (use default if not specified)
	driveID := spConfig.DriveID
	if driveID == "" {
		drive, err := spClient.GetDefaultDrive(ctx, spConfig.SiteID)
		if err != nil {
			return nil, fmt.Errorf("failed to get default drive: %w", err)
		}
		driveID = drive.ID
	}

	var result []*indexerData

	// Download each file
	for i, file := range files {
		// Update progress
		progress := int(float64(i) / float64(len(files)) * 100)
		r.updateKnowledgeProgress(k.ID, types.KnowledgeProgress{
			Step:     "downloading",
			Progress: progress,
			Message:  fmt.Sprintf("Downloading %s (%d/%d)", file.Name, i+1, len(files)),
		})

		// Download the file
		downloadedFile, err := spClient.DownloadFile(ctx, driveID, file.ID)
		if err != nil {
			log.Warn().
				Err(err).
				Str("knowledge_id", k.ID).
				Str("file_name", file.Name).
				Str("file_id", file.ID).
				Msg("Failed to download file from SharePoint, skipping")
			continue
		}

		log.Debug().
			Str("knowledge_id", k.ID).
			Str("file_name", downloadedFile.Name).
			Str("file_path", downloadedFile.Path).
			Int("content_size", len(downloadedFile.Content)).
			Msg("Downloaded file from SharePoint")

		// If chunking is disabled, add raw content
		if k.RAGSettings.DisableChunking {
			result = append(result, &indexerData{
				Data:            downloadedFile.Content,
				Source:          downloadedFile.WebURL,
				DocumentGroupID: getDocumentGroupID(downloadedFile.Path),
				Metadata: map[string]interface{}{
					"filename":      downloadedFile.Name,
					"path":          downloadedFile.Path,
					"mime_type":     downloadedFile.MimeType,
					"size":          downloadedFile.Size,
					"last_modified": downloadedFile.LastModified,
					"source":        "sharepoint",
				},
			})
			continue
		}

		// Extract text from the file
		extractedText, err := r.extractor.Extract(ctx, &extract.Request{
			Content: downloadedFile.Content,
		})
		if err != nil {
			log.Warn().
				Err(err).
				Str("knowledge_id", k.ID).
				Str("file_name", downloadedFile.Name).
				Msg("Failed to extract text from SharePoint file, skipping")
			continue
		}

		result = append(result, &indexerData{
			Data:            []byte(extractedText),
			Source:          downloadedFile.WebURL,
			DocumentGroupID: getDocumentGroupID(downloadedFile.Path),
			Metadata: map[string]interface{}{
				"filename":      downloadedFile.Name,
				"path":          downloadedFile.Path,
				"mime_type":     downloadedFile.MimeType,
				"size":          downloadedFile.Size,
				"last_modified": downloadedFile.LastModified,
				"source":        "sharepoint",
				"source_url":    downloadedFile.WebURL,
			},
		})
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no files could be processed from SharePoint")
	}

	log.Info().
		Str("knowledge_id", k.ID).
		Int("processed_count", len(result)).
		Msg("Completed SharePoint data extraction")

	return result, nil
}

// isMicrosoftOAuthProvider checks if an OAuth provider is Microsoft-compatible
// This includes both the explicit Microsoft type and Custom providers that use Microsoft OAuth URLs
func isMicrosoftOAuthProvider(provider types.OAuthProvider) bool {
	// Check if it's explicitly a Microsoft provider
	if provider.Type == types.OAuthProviderTypeMicrosoft {
		return true
	}

	// Check if it's a Custom provider with Microsoft OAuth URLs
	if provider.Type == types.OAuthProviderTypeCustom {
		// Check auth URL for Microsoft login endpoint
		if strings.Contains(provider.AuthURL, "login.microsoftonline.com") {
			return true
		}
		// Check token URL for Microsoft login endpoint
		if strings.Contains(provider.TokenURL, "login.microsoftonline.com") {
			return true
		}
	}

	return false
}
