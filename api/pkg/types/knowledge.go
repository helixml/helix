package types

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/pgvector/pgvector-go"
)

type AssistantKnowledge struct {
	// Name of the knowledge, will be unique within the Helix app
	Name string `json:"name" yaml:"name"`
	// Description of the knowledge, will be used in the prompt
	// to explain the knowledge to the assistant
	Description string `json:"description" yaml:"description"`
	// RAGSettings defines the settings for the RAG system, how
	// chunking is configured and where the index/query service is
	// hosted.
	RAGSettings RAGSettings `json:"rag_settings" yaml:"rag_settings"`

	// Source defines where the raw data is fetched from. It can be
	// directly uploaded files, S3, GCS, Google Drive, Gmail, etc.
	Source KnowledgeSource `json:"source" gorm:"jsonb"`

	// RefreshEnabled defines if the knowledge should be refreshed periodically
	// or on events. For example a Google Drive knowledge can be refreshed
	// every 24 hours.
	RefreshEnabled bool `json:"refresh_enabled" yaml:"refresh_enabled"`
	// RefreshSchedule defines the schedule for refreshing the knowledge.
	// It can be specified in cron format or as a duration for example '@every 2h'
	// or 'every 5m' or '0 0 * * *' for daily at midnight.
	RefreshSchedule string `json:"refresh_schedule" yaml:"refresh_schedule"`
}

type Knowledge struct {
	ID      string    `json:"id" gorm:"primaryKey"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`

	Name      string    `json:"name" gorm:"index"`
	Owner     string    `json:"owner" gorm:"index"` // User ID
	OwnerType OwnerType `json:"owner_type"`         // e.g. user, system, org

	State   KnowledgeState `json:"state"`
	Message string         `json:"message"` // Set if something wrong happens

	Progress KnowledgeProgress `json:"progress" gorm:"-"` // Ephemeral state from knowledge controller

	// AppID through which the knowledge was created
	AppID string `json:"app_id" gorm:"index"`

	// Description of the knowledge, will be used in the prompt
	// to explain the knowledge to the assistant
	Description string `json:"description"`

	RAGSettings RAGSettings `json:"rag_settings" yaml:"rag_settings" gorm:"jsonb"`

	// Source defines where the raw data is fetched from. It can be
	// directly uploaded files, S3, GCS, Google Drive, Gmail, etc.
	Source KnowledgeSource `json:"source" gorm:"jsonb"`

	// Version of the knowledge, will be used to separate different versions
	// of the same knowledge when updating it. Format is
	// YYYY-MM-DD-HH-MM-SS.
	Version string `json:"version" yaml:"version"`

	// RefreshEnabled defines if the knowledge should be refreshed periodically
	// or on events. For example a Google Drive knowledge can be refreshed
	// every 24 hours.
	RefreshEnabled bool `json:"refresh_enabled" yaml:"refresh_enabled"`
	// RefreshSchedule defines the schedule for refreshing the knowledge.
	// It can be specified in cron format or as a duration for example '@every 2h'
	// or 'every 5m' or '0 0 * * *' for daily at midnight.
	RefreshSchedule string `json:"refresh_schedule" yaml:"refresh_schedule"`

	// Size of the knowledge in bytes
	Size int64 `json:"size"`

	Versions []*KnowledgeVersion `json:"versions" `

	NextRun time.Time `json:"next_run" gorm:"-"` // Populated by the cron job controller

	// URLs crawled in the last run (should match last knowledge version)
	CrawledSources *CrawledSources `json:"crawled_sources" gorm:"jsonb"`
}

func (k *Knowledge) GetDataEntityID() string {
	return GetDataEntityID(k.ID, k.Version)
}

type KnowledgeVersion struct {
	ID              string          `json:"id" gorm:"primaryKey"`
	Created         time.Time       `json:"created"`
	Updated         time.Time       `json:"updated"`
	KnowledgeID     string          `json:"knowledge_id"`
	Version         string          `json:"version"`
	Size            int64           `json:"size"`
	State           KnowledgeState  `json:"state"`
	Message         string          `json:"message"` // Set if something wrong happens
	CrawledSources  *CrawledSources `json:"crawled_sources" gorm:"jsonb"`
	EmbeddingsModel string          `json:"embeddings_model" yaml:"embeddings_model"` // Model used to embed the knowledge
	Provider        string          `json:"provider" yaml:"provider"`
}

func (k *KnowledgeVersion) GetDataEntityID() string {
	return GetDataEntityID(k.KnowledgeID, k.Version)
}

func GetDataEntityID(knowledgeID, version string) string {
	if version == "" {
		return knowledgeID
	}
	return fmt.Sprintf("%s-%s", knowledgeID, version)
}

type KnowledgeState string

const (
	KnowledgeStatePreparing KnowledgeState = "preparing"
	KnowledgeStatePending   KnowledgeState = "pending"
	KnowledgeStateIndexing  KnowledgeState = "indexing"
	KnowledgeStateReady     KnowledgeState = "ready"
	KnowledgeStateError     KnowledgeState = "error"
)

type KnowledgeSource struct {
	Filestore  *KnowledgeSourceHelixFilestore `json:"filestore" yaml:"filestore"`
	S3         *KnowledgeSourceS3             `json:"s3"`
	GCS        *KnowledgeSourceGCS            `json:"gcs"`
	Web        *KnowledgeSourceWeb            `json:"web"`
	Text       *string                        `json:"text"`
	SharePoint *KnowledgeSourceSharePoint     `json:"sharepoint" yaml:"sharepoint"`
}

func (k KnowledgeSource) Value() (driver.Value, error) {
	j, err := json.Marshal(k)
	return j, err
}

func (k *KnowledgeSource) Scan(src interface{}) error {
	source, ok := src.([]byte)
	if !ok {
		return errors.New("type assertion .([]byte) failed")
	}
	var result KnowledgeSource
	if err := json.Unmarshal(source, &result); err != nil {
		return err
	}
	*k = result
	return nil
}

func (KnowledgeSource) GormDataType() string {
	return "json"
}

type KnowledgeSourceWeb struct {
	Excludes []string               `json:"excludes" yaml:"excludes"`
	URLs     []string               `json:"urls" yaml:"urls"`
	Auth     KnowledgeSourceWebAuth `json:"auth" yaml:"auth"`
	// Additional options for the crawler
	Crawler *WebsiteCrawler `json:"crawler" yaml:"crawler"`
}

type WebsiteCrawler struct {
	Firecrawl *Firecrawl `json:"firecrawl" yaml:"firecrawl"`

	Enabled     bool   `json:"enabled" yaml:"enabled"`
	MaxDepth    int    `json:"max_depth" yaml:"max_depth"` // Limit crawl depth to avoid infinite crawling
	UserAgent   string `json:"user_agent" yaml:"user_agent"`
	Readability bool   `json:"readability" yaml:"readability"` // Apply readability middleware to the HTML content

	IgnoreRobotsTxt bool `json:"ignore_robots_txt" yaml:"ignore_robots_txt"`
}

type Firecrawl struct {
	APIKey string `json:"api_key" yaml:"api_key"`
	APIURL string `json:"api_url" yaml:"api_url"`
}

type KnowledgeSourceWebAuth struct {
	Username string
	Password string
}

type KnowledgeSourceHelixFilestore struct {
	Path       string `json:"path" yaml:"path"`
	SeedZipURL string `json:"seed_zip_url,omitempty" yaml:"seed_zip_url,omitempty"`
}

// KnowledgeSourceS3 authentication through AWS IAM role
type KnowledgeSourceS3 struct {
	Bucket string `json:"bucket"`
	Path   string `json:"path"`
}

// KnowledgeSourceGCS authentication through GCP service account
type KnowledgeSourceGCS struct {
	Bucket string `json:"bucket"`
	Path   string `json:"path"`
}

type KnowledgeSourceGithub struct {
	Owner            string   `json:"owner"`
	Repository       string   `json:"repository"`
	Branch           string   `json:"branch"`
	FilterPaths      []string `json:"filter_paths"`
	FilterExtensions []string `json:"filter_extensions"`
}

// KnowledgeSourceSharePoint represents a SharePoint document library or folder as a knowledge source.
// Authentication is done via Microsoft OAuth (OAuthProviderTypeMicrosoft) which provides
// access to the Microsoft Graph API for SharePoint operations.
//
// Setup Requirements:
//  1. Create a Microsoft OAuth provider in Helix (Settings → OAuth Providers)
//     with type "microsoft" and the following scopes:
//     - Sites.Read.All (to list sites and drives)
//     - Files.Read.All (to download files)
//  2. Connect your Microsoft account via the OAuth provider
//  3. Get the SharePoint Site ID from the Graph API or the site URL:
//     - Format: "hostname,site-guid,web-guid" (e.g., "contoso.sharepoint.com,abc123,def456")
//     - Can be obtained via: GET https://graph.microsoft.com/v1.0/sites/{hostname}:/{site-path}
//
// Example Configuration:
//
//	sharepoint:
//	  site_id: "contoso.sharepoint.com,abc123-def456-789,xyz789-abc123-456"
//	  folder_path: "/Documents/Reports"
//	  oauth_provider_id: "provider-uuid"
//	  filter_extensions: [".pdf", ".docx"]
//	  recursive: true
type KnowledgeSourceSharePoint struct {
	// SiteID is the SharePoint site ID (can be obtained from Graph API or site URL)
	SiteID string `json:"site_id" yaml:"site_id"`
	// DriveID is the document library drive ID (optional, defaults to the site's default drive)
	DriveID string `json:"drive_id,omitempty" yaml:"drive_id,omitempty"`
	// FolderPath is the path to a specific folder within the drive (optional, defaults to root)
	FolderPath string `json:"folder_path,omitempty" yaml:"folder_path,omitempty"`
	// OAuthProviderID is the ID of the Microsoft OAuth provider to use for authentication
	OAuthProviderID string `json:"oauth_provider_id" yaml:"oauth_provider_id"`
	// FilterExtensions limits which file types to include (e.g., [".pdf", ".docx", ".txt"])
	FilterExtensions []string `json:"filter_extensions,omitempty" yaml:"filter_extensions,omitempty"`
	// Recursive determines whether to include files in subfolders
	Recursive bool `json:"recursive" yaml:"recursive"`
}

// CrawledDocument used internally to work with the crawled data
type CrawledDocument struct {
	ID          string
	Title       string
	Description string
	SourceURL   string
	Content     string
	StatusCode  int
	DurationMs  int64
	Message     string
}

type KnowledgeSearchResult struct {
	Knowledge  *Knowledge          `json:"knowledge"`
	Results    []*SessionRAGResult `json:"results"`
	DurationMs int64               `json:"duration_ms"`
}

type CrawledSources struct {
	URLs []*CrawledURL `json:"urls"`
	// TODO: files?
}

func (c CrawledSources) Value() (driver.Value, error) {
	j, err := json.Marshal(c)
	return j, err
}

func (c *CrawledSources) Scan(src interface{}) error {
	source, ok := src.([]byte)
	if !ok {
		return errors.New("type assertion .([]byte) failed")
	}
	var result CrawledSources
	if err := json.Unmarshal(source, &result); err != nil {
		return err
	}
	*c = result
	return nil
}

func (CrawledSources) GormDataType() string {
	return "json"
}

type CrawledURL struct {
	URL        string `json:"url"`
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	DurationMs int64  `json:"duration_ms"`
	DocumentID string `json:"document_id"`
}

type KnowledgeProgress struct {
	Step           string    `json:"step"`
	Progress       int       `json:"progress"`
	StartedAt      time.Time `json:"started_at"`
	ElapsedSeconds int       `json:"elapsed_seconds"`
	Message        string    `json:"message"`
}

type KnowledgeEmbeddingItem struct {
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DataEntityID    string `gorm:"index"` // Knowledge ID + Version
	DocumentGroupID string `gorm:"index"`
	DocumentID      string `gorm:"index"`
	Source          string
	Embedding384    *pgvector.Vector `gorm:"type:vector(384)"`  // For 384 dimensions ("gte-small")
	Embedding512    *pgvector.Vector `gorm:"type:vector(512)"`  // For 512 dimensions ("gte-medium")
	Embedding1024   *pgvector.Vector `gorm:"type:vector(1024)"` // For 1024 dimensions ("gte-large")
	Embedding1536   *pgvector.Vector `gorm:"type:vector(1536)"` // For 1536 dimensions ("gte-small")
	Embedding3584   *pgvector.Vector `gorm:"type:vector(3584)"` // For 3584 dimensions ("gte-small")
	Content         string           // Content of the knowledge
	ContentOffset   int              // Offset of the content in the knowledge
	EmbeddingsModel string           // Model used to embed the knowledge
}

type Dimensions int

const (
	Dimensions384  Dimensions = 384
	Dimensions512  Dimensions = 512
	Dimensions1024 Dimensions = 1024
	Dimensions1536 Dimensions = 1536
	Dimensions3584 Dimensions = 3584
)

type KnowledgeEmbeddingQuery struct {
	DataEntityID  string
	Embedding384  pgvector.Vector // Query by embedding
	Embedding512  pgvector.Vector // Query by embedding
	Embedding1024 pgvector.Vector // Query by embedding
	Embedding1536 pgvector.Vector // Query by embedding
	Embedding3584 pgvector.Vector // Query by embedding
	Content       string          // Optional for full text search
	Limit         int             // Limit the number of results
}
