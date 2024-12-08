package types

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"
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

	State           KnowledgeState `json:"state"`
	Message         string         `json:"message"` // Set if something wrong happens
	ProgressPercent int            `json:"progress_percent"`

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

	// URLs crawled in the last run
	CrawledURLs *CrawledURLs `json:"crawled_urls" gorm:"jsonb"`
}

func (k *Knowledge) GetDataEntityID() string {
	return GetDataEntityID(k.ID, k.Version)
}

type KnowledgeVersion struct {
	ID          string         `json:"id" gorm:"primaryKey"`
	Created     time.Time      `json:"created"`
	Updated     time.Time      `json:"updated"`
	KnowledgeID string         `json:"knowledge_id"`
	Version     string         `json:"version"`
	Size        int64          `json:"size"`
	State       KnowledgeState `json:"state"`
	Message     string         `json:"message"` // Set if something wrong happens
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
	KnowledgeStatePending  KnowledgeState = "pending"
	KnowledgeStateIndexing KnowledgeState = "indexing"
	KnowledgeStateReady    KnowledgeState = "ready"
	KnowledgeStateError    KnowledgeState = "error"
)

type KnowledgeSource struct {
	Filestore *KnowledgeSourceHelixFilestore `json:"filestore" yaml:"filestore"`
	S3        *KnowledgeSourceS3             `json:"s3"`
	GCS       *KnowledgeSourceGCS            `json:"gcs"`
	Web       *KnowledgeSourceWeb            `json:"web"`
	Content   *string                        `json:"text"`
}

func (m KnowledgeSource) Value() (driver.Value, error) {
	j, err := json.Marshal(m)
	return j, err
}

func (t *KnowledgeSource) Scan(src interface{}) error {
	source, ok := src.([]byte)
	if !ok {
		return errors.New("type assertion .([]byte) failed.")
	}
	var result KnowledgeSource
	if err := json.Unmarshal(source, &result); err != nil {
		return err
	}
	*t = result
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
	MaxPages    int    `json:"max_pages" yaml:"max_pages"` // Limit number of pages to crawl to avoid infinite crawling (max 500 by default)
	UserAgent   string `json:"user_agent" yaml:"user_agent"`
	Readability bool   `json:"readability" yaml:"readability"` // Apply readability middleware to the HTML content
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
	Path string `json:"path" yaml:"path"`
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

// CrawledDocument used internally to work with the crawled data
type CrawledDocument struct {
	ID          string
	Title       string
	Description string
	SourceURL   string
	Content     string
}

type KnowledgeSearchResult struct {
	Knowledge  *Knowledge          `json:"knowledge"`
	Results    []*SessionRAGResult `json:"results"`
	DurationMs int64               `json:"duration_ms"`
}

type CrawledURLs struct {
	URLs []*CrawledURL `json:"urls"`
}

func (m CrawledURLs) Value() (driver.Value, error) {
	j, err := json.Marshal(m)
	return j, err
}

func (t *CrawledURLs) Scan(src interface{}) error {
	source, ok := src.([]byte)
	if !ok {
		return errors.New("type assertion .([]byte) failed.")
	}
	var result CrawledURLs
	if err := json.Unmarshal(source, &result); err != nil {
		return err
	}
	*t = result
	return nil
}

func (CrawledURLs) GormDataType() string {
	return "json"
}

type CrawledURL struct {
	URL        string `json:"url"`
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
}
