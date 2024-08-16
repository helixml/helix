package types

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

type Knowledge struct {
	ID        string         `json:"id" gorm:"primaryKey"`
	Created   time.Time      `json:"created"`
	Updated   time.Time      `json:"updated"`
	Name      string         `json:"name"`
	Type      DataEntityType `json:"type"`
	Owner     string         `json:"owner" gorm:"index"` // User ID
	OwnerType OwnerType      `json:"owner_type"`         // e.g. user, system, org

	RAGSettings RAGSettings `json:"rag_settings" gorm:"jsonb"`

	// Source defines where the raw data is fetched from. It can be
	// directly uploaded files, S3, GCS, Google Drive, Gmail, etc.
	Source KnowledgeSource `json:"source" gorm:"jsonb"`
	// IntegrationID defines which integration is used to access the
	// data source. By default Helix looks up based on the source type
	// if only one integration for type is set.
	IntegrationID string `json:"integration_id"`

	// RefreshEnabled defines if the knowledge should be refreshed periodically
	// or on events. For example a Google Drive knowledge can be refreshed
	// every 24 hours.
	RefreshEnabled bool `json:"refresh_enabled"`
	// RefreshSchedule defines the schedule for refreshing the knowledge.
	// It can be specified in cron format or as a duration for example '@every 2h'
	// or 'every 5m' or '0 0 * * *' for daily at midnight.
	RefreshSchedule string `json:"refresh_schedule"`
}

type KnowledgeSourceType string

const (
	KnowledgeSourceTypeHelixDrive  KnowledgeSourceType = "helix_drive" // Files directly uploaded
	KnowledgeSourceTypeS3          KnowledgeSourceType = "s3"
	KnowledgeSourceTypeGCS         KnowledgeSourceType = "gcs"
	KnowledgeSourceTypeGoogleDrive KnowledgeSourceType = "google_drive"
	KnowledgeSourceTypeGmail       KnowledgeSourceType = "gmail"
)

type KnowledgeSource struct {
	HelixDrive *KnowledgeSourceHelixDrive `json:"helix_drive"`
	S3         *KnowledgeSourceS3         `json:"s3"`
	GCS        *KnowledgeSourceGCS        `json:"gcs"`
	URL        *KnowledgeSourceURL        `json:"url"`
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

type KnowledgeSourceURL struct {
	URL string `json:"url"`
}

type KnowledgeSourceHelixDrive struct {
	Path string `json:"path"`
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
