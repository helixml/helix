package types

import (
	"time"
)

// VHostTargetKind discriminates what kind of target a vhost_routes row points at.
type VHostTargetKind string

const (
	// VHostTargetProjectWebService routes to a project's currently active
	// web-service sandbox. target_id is a project ID (prj_*).
	VHostTargetProjectWebService VHostTargetKind = "project_web_service"

	// VHostTargetSandboxPreview routes to a specific sandbox or session
	// container for a "Share preview" dev URL. target_id is a session ID
	// (ses_*) or sandbox ID (sbx_*).
	VHostTargetSandboxPreview VHostTargetKind = "sandbox_preview"
)

// VHostRoute maps a hostname to a routable target inside Helix. The same
// table holds project web service routes (user-named hostnames) and sandbox
// preview tokens (random share-* hostnames).
type VHostRoute struct {
	ID         string          `gorm:"primaryKey" json:"id"`
	Hostname   string          `gorm:"uniqueIndex" json:"hostname"` // always lowercased
	TargetKind VHostTargetKind `gorm:"index" json:"target_kind"`
	TargetID   string          `gorm:"index" json:"target_id"`
	Port       int             `json:"port"` // destination port inside the container

	// IsDefault is true for project default subdomains (<slug>.<base>).
	// User-added custom domains and preview tokens are false.
	IsDefault bool `json:"is_default"`

	// VerifiedAt is non-null once the route is usable. Auto-set for default
	// subdomains and preview tokens; set after DNS verification for custom
	// domains.
	VerifiedAt *time.Time `json:"verified_at,omitempty"`

	// VerificationToken is only meaningful for custom domains awaiting
	// DNS-based verification. Null for default and preview rows.
	VerificationToken string `json:"verification_token,omitempty"`

	CreatedAt time.Time  `json:"created_at"`
	RotatedAt *time.Time `json:"rotated_at,omitempty"`
}

// TableName pins the GORM table to "vhost_routes" — without this, GORM
// would derive "v_host_routes" from the camel-cased struct name.
func (VHostRoute) TableName() string { return "vhost_routes" }

// ProjectWebServiceState is the per-project enablement and runtime state for
// the web-service hosting feature.
type ProjectWebServiceState struct {
	ProjectID         string    `gorm:"primaryKey" json:"project_id"`
	Enabled           bool      `json:"enabled"`
	ContainerPort     int       `json:"container_port"` // port the project's web app binds to inside its container
	ActiveSandboxID   string    `json:"active_sandbox_id,omitempty"`
	UpdatedAt         time.Time `json:"updated_at"`
	CreatedAt         time.Time `json:"created_at"`
}

// WebServiceDeployStatus tracks the lifecycle of a single web-service deploy.
type WebServiceDeployStatus string

const (
	WebServiceDeployStatusPending    WebServiceDeployStatus = "pending"
	WebServiceDeployStatusBuilding   WebServiceDeployStatus = "building"
	WebServiceDeployStatusLive       WebServiceDeployStatus = "live"
	WebServiceDeployStatusFailed     WebServiceDeployStatus = "failed"
	WebServiceDeployStatusSuperseded WebServiceDeployStatus = "superseded"
)

// WebServiceDeploy records one deploy attempt for a project's web service.
type WebServiceDeploy struct {
	ID         string                 `gorm:"primaryKey" json:"id"`
	ProjectID  string                 `gorm:"index" json:"project_id"`
	SandboxID  string                 `json:"sandbox_id,omitempty"`
	CommitSHA  string                 `json:"commit_sha"`
	Status     WebServiceDeployStatus `gorm:"index" json:"status"`
	StartedAt  time.Time              `json:"started_at"`
	FinishedAt *time.Time             `json:"finished_at,omitempty"`
	LogPath    string                 `json:"log_path,omitempty"`
	Error      string                 `json:"error,omitempty"`
}
