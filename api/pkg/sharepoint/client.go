package sharepoint

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/types"
)

const (
	graphAPIBaseURL = "https://graph.microsoft.com/v1.0"
)

// Client provides access to SharePoint via Microsoft Graph API
type Client struct {
	httpClient  *http.Client
	accessToken string
}

// DriveItem represents a file or folder in SharePoint/OneDrive
type DriveItem struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Size             int64             `json:"size"`
	WebURL           string            `json:"webUrl"`
	File             *FileInfo         `json:"file,omitempty"`
	Folder           *FolderInfo       `json:"folder,omitempty"`
	ParentReference  *ParentReference  `json:"parentReference,omitempty"`
	DownloadURL      string            `json:"@microsoft.graph.downloadUrl,omitempty"`
	LastModifiedTime string            `json:"lastModifiedDateTime,omitempty"`
}

// FileInfo contains file-specific metadata
type FileInfo struct {
	MimeType string `json:"mimeType"`
	Hashes   *struct {
		QuickXorHash string `json:"quickXorHash"`
		SHA1Hash     string `json:"sha1Hash"`
		SHA256Hash   string `json:"sha256Hash"`
	} `json:"hashes,omitempty"`
}

// FolderInfo contains folder-specific metadata
type FolderInfo struct {
	ChildCount int `json:"childCount"`
}

// ParentReference contains information about the parent item
type ParentReference struct {
	DriveID   string `json:"driveId"`
	DriveType string `json:"driveType"`
	ID        string `json:"id"`
	Path      string `json:"path"`
}

// DriveItemsResponse represents the response from listing drive items
type DriveItemsResponse struct {
	Value    []DriveItem `json:"value"`
	NextLink string      `json:"@odata.nextLink,omitempty"`
}

// Site represents a SharePoint site
type Site struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Name        string `json:"name"`
	WebURL      string `json:"webUrl"`
}

// Drive represents a document library
type Drive struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DriveType   string `json:"driveType"`
	Description string `json:"description"`
	WebURL      string `json:"webUrl"`
}

// DrivesResponse represents the response from listing drives
type DrivesResponse struct {
	Value []Drive `json:"value"`
}

// DownloadedFile represents a downloaded file with its content and metadata
type DownloadedFile struct {
	Name        string
	Path        string
	Content     []byte
	MimeType    string
	Size        int64
	WebURL      string
	LastModified string
}

// NewClient creates a new SharePoint client with the given access token
func NewClient(accessToken string) *Client {
	return &Client{
		httpClient:  &http.Client{},
		accessToken: accessToken,
	}
}

// GetSite retrieves a SharePoint site by ID or URL
func (c *Client) GetSite(ctx context.Context, siteID string) (*Site, error) {
	endpoint := fmt.Sprintf("%s/sites/%s", graphAPIBaseURL, siteID)

	var site Site
	if err := c.doRequest(ctx, "GET", endpoint, nil, &site); err != nil {
		return nil, fmt.Errorf("failed to get site: %w", err)
	}

	return &site, nil
}

// GetSiteByURL retrieves a SharePoint site by its URL
// URL format: https://tenant.sharepoint.com/sites/sitename
// Also handles URLs with extra paths like /SitePages/Home.aspx
func (c *Client) GetSiteByURL(ctx context.Context, siteURL string) (*Site, error) {
	parsedURL, err := url.Parse(siteURL)
	if err != nil {
		return nil, fmt.Errorf("invalid site URL: %w", err)
	}

	hostname := parsedURL.Hostname()
	path := strings.TrimPrefix(parsedURL.Path, "/")

	// Extract just the site path (e.g., "sites/sitename" from "sites/sitename/SitePages/Home.aspx")
	// SharePoint site paths are typically /sites/sitename or /teams/teamname
	pathParts := strings.Split(path, "/")
	if len(pathParts) >= 2 {
		// Keep only the first two parts (e.g., "sites/sitename" or "teams/teamname")
		path = strings.Join(pathParts[:2], "/")
	}

	endpoint := fmt.Sprintf("%s/sites/%s:/%s", graphAPIBaseURL, hostname, path)

	var site Site
	if err := c.doRequest(ctx, "GET", endpoint, nil, &site); err != nil {
		return nil, fmt.Errorf("failed to get site by URL: %w", err)
	}

	return &site, nil
}

// ListDrives lists all document libraries in a site
func (c *Client) ListDrives(ctx context.Context, siteID string) ([]Drive, error) {
	endpoint := fmt.Sprintf("%s/sites/%s/drives", graphAPIBaseURL, siteID)

	var response DrivesResponse
	if err := c.doRequest(ctx, "GET", endpoint, nil, &response); err != nil {
		return nil, fmt.Errorf("failed to list drives: %w", err)
	}

	return response.Value, nil
}

// GetDefaultDrive gets the default document library for a site
func (c *Client) GetDefaultDrive(ctx context.Context, siteID string) (*Drive, error) {
	endpoint := fmt.Sprintf("%s/sites/%s/drive", graphAPIBaseURL, siteID)

	var drive Drive
	if err := c.doRequest(ctx, "GET", endpoint, nil, &drive); err != nil {
		return nil, fmt.Errorf("failed to get default drive: %w", err)
	}

	return &drive, nil
}

// ListFiles lists files in a drive, optionally filtering by folder path and extensions
func (c *Client) ListFiles(ctx context.Context, config *types.KnowledgeSourceSharePoint) ([]DriveItem, error) {
	var endpoint string

	// Determine which drive to use
	driveID := config.DriveID
	if driveID == "" {
		// Get the default drive
		drive, err := c.GetDefaultDrive(ctx, config.SiteID)
		if err != nil {
			return nil, fmt.Errorf("failed to get default drive: %w", err)
		}
		driveID = drive.ID
	}

	// Build the endpoint based on folder path
	if config.FolderPath != "" && config.FolderPath != "/" {
		// URL encode the path, but keep slashes as path separators
		encodedPath := url.PathEscape(strings.TrimPrefix(config.FolderPath, "/"))
		endpoint = fmt.Sprintf("%s/drives/%s/root:/%s:/children", graphAPIBaseURL, driveID, encodedPath)
	} else {
		endpoint = fmt.Sprintf("%s/drives/%s/root/children", graphAPIBaseURL, driveID)
	}

	var allItems []DriveItem

	// Fetch items with pagination
	for endpoint != "" {
		var response DriveItemsResponse
		if err := c.doRequest(ctx, "GET", endpoint, nil, &response); err != nil {
			return nil, fmt.Errorf("failed to list files: %w", err)
		}

		for _, item := range response.Value {
			// If it's a folder and recursive is enabled, fetch its contents
			if item.Folder != nil && config.Recursive {
				subItems, err := c.listFilesInFolder(ctx, driveID, item.ID, config)
				if err != nil {
					log.Warn().
						Err(err).
						Str("folder", item.Name).
						Msg("Failed to list files in subfolder")
					continue
				}
				allItems = append(allItems, subItems...)
			} else if item.File != nil {
				// It's a file, check extension filter
				if c.matchesExtensionFilter(item.Name, config.FilterExtensions) {
					allItems = append(allItems, item)
				}
			}
		}

		endpoint = response.NextLink
	}

	return allItems, nil
}

// listFilesInFolder recursively lists files in a folder
func (c *Client) listFilesInFolder(ctx context.Context, driveID, folderID string, config *types.KnowledgeSourceSharePoint) ([]DriveItem, error) {
	endpoint := fmt.Sprintf("%s/drives/%s/items/%s/children", graphAPIBaseURL, driveID, folderID)

	var allItems []DriveItem

	for endpoint != "" {
		var response DriveItemsResponse
		if err := c.doRequest(ctx, "GET", endpoint, nil, &response); err != nil {
			return nil, fmt.Errorf("failed to list files in folder: %w", err)
		}

		for _, item := range response.Value {
			if item.Folder != nil && config.Recursive {
				subItems, err := c.listFilesInFolder(ctx, driveID, item.ID, config)
				if err != nil {
					log.Warn().
						Err(err).
						Str("folder", item.Name).
						Msg("Failed to list files in subfolder")
					continue
				}
				allItems = append(allItems, subItems...)
			} else if item.File != nil {
				if c.matchesExtensionFilter(item.Name, config.FilterExtensions) {
					allItems = append(allItems, item)
				}
			}
		}

		endpoint = response.NextLink
	}

	return allItems, nil
}

// DownloadFile downloads a file's content
func (c *Client) DownloadFile(ctx context.Context, driveID, itemID string) (*DownloadedFile, error) {
	// First get the item metadata to get the download URL
	endpoint := fmt.Sprintf("%s/drives/%s/items/%s", graphAPIBaseURL, driveID, itemID)

	var item DriveItem
	if err := c.doRequest(ctx, "GET", endpoint, nil, &item); err != nil {
		return nil, fmt.Errorf("failed to get item metadata: %w", err)
	}

	// Download the content
	downloadURL := item.DownloadURL
	if downloadURL == "" {
		// Use the content endpoint if no direct download URL
		downloadURL = fmt.Sprintf("%s/drives/%s/items/%s/content", graphAPIBaseURL, driveID, itemID)
	}

	content, err := c.downloadContent(ctx, downloadURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download content: %w", err)
	}

	// Build the path from parent reference
	path := item.Name
	if item.ParentReference != nil && item.ParentReference.Path != "" {
		// Extract path after /root:
		parentPath := item.ParentReference.Path
		if idx := strings.Index(parentPath, "/root:"); idx >= 0 {
			parentPath = strings.TrimPrefix(parentPath[idx:], "/root:")
		}
		path = filepath.Join(parentPath, item.Name)
	}

	var mimeType string
	if item.File != nil {
		mimeType = item.File.MimeType
	}

	return &DownloadedFile{
		Name:         item.Name,
		Path:         path,
		Content:      content,
		MimeType:     mimeType,
		Size:         item.Size,
		WebURL:       item.WebURL,
		LastModified: item.LastModifiedTime,
	}, nil
}

// downloadContent downloads content from a URL
func (c *Client) downloadContent(ctx context.Context, downloadURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return nil, err
	}

	// Only add auth header if it's a Graph API URL (not a pre-signed download URL)
	if strings.HasPrefix(downloadURL, graphAPIBaseURL) {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download failed with status %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// matchesExtensionFilter checks if a filename matches the extension filter
func (c *Client) matchesExtensionFilter(filename string, extensions []string) bool {
	if len(extensions) == 0 {
		return true // No filter, include all files
	}

	ext := strings.ToLower(filepath.Ext(filename))
	for _, filterExt := range extensions {
		filterExt = strings.ToLower(filterExt)
		if !strings.HasPrefix(filterExt, ".") {
			filterExt = "." + filterExt
		}
		if ext == filterExt {
			return true
		}
	}

	return false
}

// doRequest performs an HTTP request to the Graph API
func (c *Client) doRequest(ctx context.Context, method, url string, body io.Reader, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}
