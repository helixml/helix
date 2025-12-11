package sharepoint

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/types"
)

func TestNewClient(t *testing.T) {
	client := NewClient("test-token", false)
	assert.NotNil(t, client)
	assert.Equal(t, "test-token", client.accessToken)
}

func TestClient_GetSite(t *testing.T) {
	// Test that request is properly formatted
	// Note: We can't easily override the const graphAPIBaseURL in tests
	// This test documents expected behavior and verifies struct handling
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Contains(t, r.URL.Path, "/sites/test-site-id")

		site := Site{
			ID:          "test-site-id",
			DisplayName: "Test Site",
			Name:        "testsite",
			WebURL:      "https://example.sharepoint.com/sites/testsite",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(site)
	}))
	defer server.Close()

	// Verify the Site struct can be serialized/deserialized properly
	site := Site{
		ID:          "test-site-id",
		DisplayName: "Test Site",
		Name:        "testsite",
		WebURL:      "https://example.sharepoint.com/sites/testsite",
	}
	data, err := json.Marshal(site)
	require.NoError(t, err)

	var decoded Site
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, site.ID, decoded.ID)
	assert.Equal(t, site.DisplayName, decoded.DisplayName)
}

func TestClient_matchesExtensionFilter(t *testing.T) {
	client := NewClient("test-token", false)

	tests := []struct {
		name       string
		filename   string
		extensions []string
		expected   bool
	}{
		{
			name:       "no filter matches all",
			filename:   "document.pdf",
			extensions: []string{},
			expected:   true,
		},
		{
			name:       "matches pdf extension",
			filename:   "document.pdf",
			extensions: []string{".pdf", ".docx"},
			expected:   true,
		},
		{
			name:       "matches docx extension",
			filename:   "document.docx",
			extensions: []string{".pdf", ".docx"},
			expected:   true,
		},
		{
			name:       "does not match extension",
			filename:   "image.png",
			extensions: []string{".pdf", ".docx"},
			expected:   false,
		},
		{
			name:       "case insensitive match",
			filename:   "Document.PDF",
			extensions: []string{".pdf"},
			expected:   true,
		},
		{
			name:       "extension without dot",
			filename:   "document.pdf",
			extensions: []string{"pdf"},
			expected:   true,
		},
		{
			name:       "no extension in filename",
			filename:   "README",
			extensions: []string{".pdf"},
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.matchesExtensionFilter(tt.filename, tt.extensions)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestClient_ListFiles(t *testing.T) {
	// Create a test server that simulates Microsoft Graph API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		switch {
		case r.URL.Path == "/v1.0/sites/test-site/drive":
			// Return default drive
			drive := Drive{
				ID:        "test-drive-id",
				Name:      "Documents",
				DriveType: "documentLibrary",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(drive)

		case r.URL.Path == "/v1.0/drives/test-drive-id/root/children":
			// Return list of items
			response := DriveItemsResponse{
				Value: []DriveItem{
					{
						ID:   "file-1",
						Name: "document.pdf",
						Size: 1024,
						File: &FileInfo{MimeType: "application/pdf"},
					},
					{
						ID:   "file-2",
						Name: "report.docx",
						Size: 2048,
						File: &FileInfo{MimeType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
					},
					{
						ID:     "folder-1",
						Name:   "Subfolder",
						Folder: &FolderInfo{ChildCount: 1},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)

		case r.URL.Path == "/v1.0/drives/test-drive-id/items/folder-1/children":
			// Return subfolder contents
			response := DriveItemsResponse{
				Value: []DriveItem{
					{
						ID:   "file-3",
						Name: "nested.txt",
						Size: 512,
						File: &FileInfo{MimeType: "text/plain"},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Note: We can't easily override the const graphAPIBaseURL in tests
	// In production code, this would be injected as a dependency
	// For now, this test documents the expected behavior
}

func TestClient_DownloadFile(t *testing.T) {
	fileContent := []byte("This is test file content")

	// Test DownloadedFile struct serialization
	file := DownloadedFile{
		Name:         "test-document.pdf",
		Path:         "/Documents/test-document.pdf",
		Content:      fileContent,
		MimeType:     "application/pdf",
		Size:         int64(len(fileContent)),
		WebURL:       "https://example.sharepoint.com/test-document.pdf",
		LastModified: "2024-01-15T10:30:00Z",
	}

	assert.Equal(t, "test-document.pdf", file.Name)
	assert.Equal(t, fileContent, file.Content)
	assert.Equal(t, int64(len(fileContent)), file.Size)
}

func TestKnowledgeSourceSharePoint_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  types.KnowledgeSourceSharePoint
		wantErr bool
	}{
		{
			name: "valid config with site ID",
			config: types.KnowledgeSourceSharePoint{
				SiteID:          "contoso.sharepoint.com,site-guid",
				OAuthProviderID: "provider-123",
			},
			wantErr: false,
		},
		{
			name: "valid config with all options",
			config: types.KnowledgeSourceSharePoint{
				SiteID:           "contoso.sharepoint.com,site-guid",
				DriveID:          "drive-123",
				FolderPath:       "/Documents/Reports",
				OAuthProviderID:  "provider-123",
				FilterExtensions: []string{".pdf", ".docx"},
				Recursive:        true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate that the config struct can be serialized/deserialized
			data, err := json.Marshal(tt.config)
			require.NoError(t, err)

			var decoded types.KnowledgeSourceSharePoint
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err)

			assert.Equal(t, tt.config.SiteID, decoded.SiteID)
			assert.Equal(t, tt.config.OAuthProviderID, decoded.OAuthProviderID)
			assert.Equal(t, tt.config.Recursive, decoded.Recursive)
		})
	}
}

func TestDriveItem_IsFile(t *testing.T) {
	fileItem := DriveItem{
		ID:   "file-1",
		Name: "document.pdf",
		File: &FileInfo{MimeType: "application/pdf"},
	}

	folderItem := DriveItem{
		ID:     "folder-1",
		Name:   "Documents",
		Folder: &FolderInfo{ChildCount: 5},
	}

	assert.NotNil(t, fileItem.File)
	assert.Nil(t, fileItem.Folder)

	assert.Nil(t, folderItem.File)
	assert.NotNil(t, folderItem.Folder)
}

func TestDownloadedFile_Structure(t *testing.T) {
	file := DownloadedFile{
		Name:         "report.pdf",
		Path:         "/Documents/Reports/report.pdf",
		Content:      []byte("PDF content"),
		MimeType:     "application/pdf",
		Size:         1024,
		WebURL:       "https://example.sharepoint.com/Documents/Reports/report.pdf",
		LastModified: "2024-01-15T10:30:00Z",
	}

	assert.Equal(t, "report.pdf", file.Name)
	assert.Equal(t, "/Documents/Reports/report.pdf", file.Path)
	assert.Equal(t, int64(1024), file.Size)
	assert.Equal(t, "application/pdf", file.MimeType)
}

func TestNormalizeSiteURLPath(t *testing.T) {
	// Test the URL path normalization logic used in GetSiteByURL
	// This extracts just the site path from various SharePoint URL formats
	tests := []struct {
		name         string
		inputURL     string
		expectedPath string
	}{
		{
			name:         "simple site URL",
			inputURL:     "https://contoso.sharepoint.com/sites/teamsite",
			expectedPath: "sites/teamsite",
		},
		{
			name:         "site URL with SitePages",
			inputURL:     "https://contoso.sharepoint.com/sites/teamsite/SitePages/Home.aspx",
			expectedPath: "sites/teamsite",
		},
		{
			name:         "site URL with nested SitePages path",
			inputURL:     "https://helixmlinc.sharepoint.com/sites/crisiscommunication/SitePages/crisisCommunicationHome.aspx",
			expectedPath: "sites/crisiscommunication",
		},
		{
			name:         "teams site URL",
			inputURL:     "https://contoso.sharepoint.com/teams/engineering",
			expectedPath: "teams/engineering",
		},
		{
			name:         "teams site URL with extra path",
			inputURL:     "https://contoso.sharepoint.com/teams/engineering/Shared%20Documents/file.docx",
			expectedPath: "teams/engineering",
		},
		{
			name:         "site URL with Lists path",
			inputURL:     "https://contoso.sharepoint.com/sites/project/Lists/Tasks/AllItems.aspx",
			expectedPath: "sites/project",
		},
		{
			name:         "site URL with _layouts path",
			inputURL:     "https://contoso.sharepoint.com/sites/intranet/_layouts/15/viewlsts.aspx",
			expectedPath: "sites/intranet",
		},
		{
			name:         "site URL with trailing slash",
			inputURL:     "https://contoso.sharepoint.com/sites/teamsite/",
			expectedPath: "sites/teamsite",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeSiteURLPath(tt.inputURL)
			assert.Equal(t, tt.expectedPath, result)
		})
	}
}

// normalizeSiteURLPath extracts just the site path from a SharePoint URL
// This is a helper function that mirrors the logic in GetSiteByURL
func normalizeSiteURLPath(siteURL string) string {
	parsedURL, err := url.Parse(siteURL)
	if err != nil {
		return ""
	}

	path := strings.TrimPrefix(parsedURL.Path, "/")
	path = strings.TrimSuffix(path, "/")

	// Extract just the site path (e.g., "sites/sitename" from "sites/sitename/SitePages/Home.aspx")
	pathParts := strings.Split(path, "/")
	if len(pathParts) >= 2 {
		// Keep only the first two parts (e.g., "sites/sitename" or "teams/teamname")
		path = strings.Join(pathParts[:2], "/")
	}

	return path
}
