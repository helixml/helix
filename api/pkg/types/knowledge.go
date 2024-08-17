package types

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

type AssistantKnowledge struct {
	// Name of the knowledge, will be unique within the Helix app
	Name string `json:"name" yaml:"name"`
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
	RefreshEnabled bool `json:"refresh_enabled"`
	// RefreshSchedule defines the schedule for refreshing the knowledge.
	// It can be specified in cron format or as a duration for example '@every 2h'
	// or 'every 5m' or '0 0 * * *' for daily at midnight.
	RefreshSchedule string `json:"refresh_schedule"`
}

type Knowledge struct {
	ID      string    `json:"id" gorm:"primaryKey"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`

	Name      string    `json:"name" gorm:"index"`
	Owner     string    `json:"owner" gorm:"index"` // User ID
	OwnerType OwnerType `json:"owner_type"`         // e.g. user, system, org

	State KnowledgeState `json:"state"`

	// AppID through which the knowledge was created
	AppID string `json:"app_id" gorm:"index"`

	RAGSettings RAGSettings `json:"rag_settings" gorm:"jsonb"`

	// Source defines where the raw data is fetched from. It can be
	// directly uploaded files, S3, GCS, Google Drive, Gmail, etc.
	Source KnowledgeSource `json:"source" gorm:"jsonb"`

	// RefreshEnabled defines if the knowledge should be refreshed periodically
	// or on events. For example a Google Drive knowledge can be refreshed
	// every 24 hours.
	RefreshEnabled bool `json:"refresh_enabled"`
	// RefreshSchedule defines the schedule for refreshing the knowledge.
	// It can be specified in cron format or as a duration for example '@every 2h'
	// or 'every 5m' or '0 0 * * *' for daily at midnight.
	RefreshSchedule string `json:"refresh_schedule"`
}

type KnowledgeState string

const (
	KnowledgeStatePending  KnowledgeState = "pending"
	KnowledgeStateReady    KnowledgeState = "ready"
	KnowledgeStateUpdating KnowledgeState = "updating"
	KnowledgeStateError    KnowledgeState = "error"
)

type LookupKnowledge struct {
	AppID string `json:"app_id"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Owner string `json:"owner"`
}

type KnowledgeSource struct {
	HelixDrive *KnowledgeSourceHelixDrive `json:"helix_drive"`
	S3         *KnowledgeSourceS3         `json:"s3"`
	GCS        *KnowledgeSourceGCS        `json:"gcs"`
	Web        *KnowledgeSourceWeb        `json:"web"`
	Content    *string                    `json:"text"`
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
	URLs []string `json:"urls"`
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
