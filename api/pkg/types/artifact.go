package types

import (
	"time"

	"gorm.io/gorm"
)

type ArtifactKind string

const (
	ArtifactKindSingleFile ArtifactKind = "single_file"
	ArtifactKindSPA        ArtifactKind = "spa"
)

type ArtifactVisibility string

const (
	ArtifactVisibilityProject ArtifactVisibility = "project"
	ArtifactVisibilityPublic  ArtifactVisibility = "public"
)

type ArtifactFile struct {
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	SHA256      string `json:"sha256"`
}

// Artifact is the stable project-scoped identity of a static page or app.
// Content is immutable and versioned separately in ArtifactVersion.
type Artifact struct {
	ID              string             `json:"id" gorm:"primaryKey"`
	ProjectID       string             `json:"project_id" gorm:"index;not null"`
	OrganizationID  string             `json:"organization_id" gorm:"index"`
	Name            string             `json:"name" gorm:"not null"`
	Description     string             `json:"description,omitempty"`
	Kind            ArtifactKind       `json:"kind" gorm:"not null"`
	Entrypoint      string             `json:"entrypoint" gorm:"not null"`
	Visibility      ArtifactVisibility `json:"visibility" gorm:"not null;default:project"`
	ActiveVersionID string             `json:"active_version_id" gorm:"index;not null"`
	CreatedBy       string             `json:"created_by" gorm:"index;not null"`
	UpdatedBy       string             `json:"updated_by" gorm:"index;not null"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
	DeletedAt       gorm.DeletedAt     `json:"deleted_at,omitempty" gorm:"index"`

	ActiveVersion *ArtifactVersion `json:"active_version,omitempty" gorm:"-"`
	URL           string           `json:"url,omitempty" gorm:"-"`
	SubdomainURL  string           `json:"subdomain_url,omitempty" gorm:"-"`
}

// ArtifactVersion is an immutable deployment of an Artifact.
type ArtifactVersion struct {
	ID               string         `json:"id" gorm:"primaryKey"`
	ArtifactID       string         `json:"artifact_id" gorm:"uniqueIndex:idx_artifact_version,priority:1;index;not null"`
	Version          int            `json:"version" gorm:"uniqueIndex:idx_artifact_version,priority:2;not null"`
	StoragePrefix    string         `json:"-" gorm:"not null"`
	Files            []ArtifactFile `json:"files" gorm:"type:jsonb;serializer:json;not null"`
	FileCount        int            `json:"file_count" gorm:"not null"`
	TotalBytes       int64          `json:"total_bytes" gorm:"not null"`
	ContentSHA256    string         `json:"content_sha256" gorm:"not null"`
	SourceSessionID  string         `json:"source_session_id,omitempty" gorm:"index"`
	SourceSpecTaskID string         `json:"source_spec_task_id,omitempty" gorm:"index"`
	CreatedBy        string         `json:"created_by" gorm:"index;not null"`
	CreatedAt        time.Time      `json:"created_at"`
}

type ArtifactsListResponse struct {
	Artifacts []*Artifact `json:"artifacts"`
}

type ArtifactVersionsListResponse struct {
	Versions []*ArtifactVersion `json:"versions"`
}
