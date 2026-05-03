package types

import (
	"time"

	"gorm.io/datatypes"
)

// SandboxRuntime identifies the type of sandbox to create.
// Today only ubuntu-desktop is implemented; later runtimes will spin up
// lightweight Docker containers.
type SandboxRuntime string

const (
	// SandboxRuntimeUbuntuDesktop is the full Ubuntu desktop image used for
	// dev containers, but with no Zed/agent autoboot — a clean desktop the
	// user can stream into and exec commands inside.
	SandboxRuntimeUbuntuDesktop SandboxRuntime = "ubuntu-desktop"

	// SandboxRuntimeHeadlessUbuntu spins up a plain `ubuntu:22.04` container
	// running `sleep infinity` — no GUI, no agent, just a long-lived shell
	// to exec into. Useful for CI-style scripted workloads.
	SandboxRuntimeHeadlessUbuntu SandboxRuntime = "headless-ubuntu"
)

// SandboxStatus reflects lifecycle state of a sandbox.
type SandboxStatus string

const (
	SandboxStatusPending  SandboxStatus = "pending"
	SandboxStatusRunning  SandboxStatus = "running"
	SandboxStatusStopping SandboxStatus = "stopping"
	SandboxStatusStopped  SandboxStatus = "stopped"
	SandboxStatusFailed   SandboxStatus = "failed"
)

// Sandbox is a user-created ephemeral container managed via the Sandboxes API.
// Sandboxes are scoped to an organization and never persisted across deletion.
type Sandbox struct {
	ID             string `json:"id" gorm:"primaryKey"`
	Name           string `json:"name" gorm:"size:128;index"`
	OrganizationID string `json:"organization_id" gorm:"size:64;index;not null"`
	// ProjectID is optional. When set, the sandbox is associated with a
	// specific project for organisational/UI grouping purposes; nothing in the
	// lifecycle path branches on it. Empty means org-scoped only.
	ProjectID     string         `json:"project_id,omitempty" gorm:"size:64;index"`
	Owner         string         `json:"owner" gorm:"size:64;index;not null"`
	Runtime       SandboxRuntime `json:"runtime" gorm:"size:64;not null"`
	Image         string         `json:"image" gorm:"size:255"`
	Status        SandboxStatus  `json:"status" gorm:"size:32;index;not null"`
	StatusMessage string         `json:"status_message,omitempty" gorm:"size:1024"`

	VCPUs    int `json:"vcpus" gorm:"not null;default:1"`
	MemoryMB int `json:"memory_mb" gorm:"not null;default:2048"`

	// Persistent indicates that the sandbox should mount a persistent
	// workspace volume (so files survive across reboots/restarts of the
	// underlying container). Non-persistent sandboxes use the container's
	// ephemeral filesystem only.
	Persistent bool `json:"persistent" gorm:"not null;default:false"`

	// HostDeviceID is the RevDial device ID of the hydra host that runs the
	// underlying container. Empty until the controller schedules it.
	HostDeviceID string `json:"host_device_id,omitempty" gorm:"size:255;index"`
	ContainerID  string `json:"container_id,omitempty" gorm:"size:255"`

	// Display fields apply to desktop runtimes.
	DisplayWidth  int `json:"display_width,omitempty"`
	DisplayHeight int `json:"display_height,omitempty"`
	DisplayFPS    int `json:"display_fps,omitempty"`

	Env  datatypes.JSON `json:"env,omitempty" gorm:"type:jsonb"`
	Tags datatypes.JSON `json:"tags,omitempty" gorm:"type:jsonb"`

	// TimeoutSeconds is the lifetime in seconds; ExpiresAt = CreatedAt + TimeoutSeconds.
	TimeoutSeconds int `json:"timeout_seconds" gorm:"not null;default:3600"`

	CreatedAt            time.Time  `json:"created_at" gorm:"index"`
	UpdatedAt            time.Time  `json:"updated_at"`
	StartedAt            *time.Time `json:"started_at,omitempty"`
	StoppedAt            *time.Time `json:"stopped_at,omitempty"`
	BillingLastChargedAt *time.Time `json:"billing_last_charged_at,omitempty" gorm:"index"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty" gorm:"index"`
	DeletedAt            *time.Time `json:"deleted_at,omitempty" gorm:"index"`
}

// TableName tells GORM which table to use.
func (Sandbox) TableName() string {
	return "sandboxes"
}

// CreateSandboxRequest is the API payload for POST /organizations/{org}/sandboxes.
type CreateSandboxRequest struct {
	Name string `json:"name,omitempty"`
	// Runtime selects one of the operator-configured runtimes
	// (e.g. "headless-ubuntu", "node22", "ubuntu-desktop"). Mutually
	// exclusive with Image.
	Runtime SandboxRuntime `json:"runtime,omitempty"`
	// Image is an optional explicit Docker image override. Only honoured
	// when the operator has set HELIX_SANDBOX_ALLOW_CUSTOM_IMAGE=true.
	// Mutually exclusive with Runtime.
	Image          string            `json:"image,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	Tags           map[string]string `json:"tags,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
	VCPUs          int               `json:"vcpus,omitempty"`
	MemoryMB       int               `json:"memory_mb,omitempty"`
	DisplayWidth   int               `json:"display_width,omitempty"`
	DisplayHeight  int               `json:"display_height,omitempty"`
	DisplayFPS     int               `json:"display_fps,omitempty"`
	// ProjectID optionally associates the sandbox with a project the caller
	// belongs to. Empty means org-scoped only.
	ProjectID string `json:"project_id,omitempty"`
	// Persistent makes the sandbox keep a workspace mount across container
	// restarts. Files written under /home/retro/work survive teardown until
	// the sandbox is explicitly deleted.
	Persistent bool `json:"persistent,omitempty"`
}

// UpdateSandboxRequest is the API payload for PATCH /sandboxes/{id}.
type UpdateSandboxRequest struct {
	Name           *string            `json:"name,omitempty"`
	TimeoutSeconds *int               `json:"timeout_seconds,omitempty"`
	Tags           *map[string]string `json:"tags,omitempty"`
}

// SandboxListResponse is the API response for GET /organizations/{org}/sandboxes.
type SandboxListResponse struct {
	Sandboxes []*Sandbox `json:"sandboxes"`
	Total     int        `json:"total"`
}

// SandboxCommandStatus reflects state of an exec'd command inside a sandbox.
type SandboxCommandStatus string

const (
	SandboxCommandStatusPending  SandboxCommandStatus = "pending"
	SandboxCommandStatusRunning  SandboxCommandStatus = "running"
	SandboxCommandStatusFinished SandboxCommandStatus = "finished"
	SandboxCommandStatusFailed   SandboxCommandStatus = "failed"
	SandboxCommandStatusKilled   SandboxCommandStatus = "killed"
)

// SandboxCommand is the in-memory representation of a command run via
// POST /sandboxes/{id}/commands. Not persisted in the database — the source
// of truth lives in hydra for the lifetime of the sandbox.
type SandboxCommand struct {
	ID         string               `json:"id"`
	SandboxID  string               `json:"sandbox_id"`
	Cmd        string               `json:"cmd"`
	Args       []string             `json:"args,omitempty"`
	Cwd        string               `json:"cwd,omitempty"`
	Env        map[string]string    `json:"env,omitempty"`
	Sudo       bool                 `json:"sudo,omitempty"`
	Detached   bool                 `json:"detached,omitempty"`
	Status     SandboxCommandStatus `json:"status"`
	ExitCode   *int                 `json:"exit_code,omitempty"`
	StartedAt  time.Time            `json:"started_at"`
	FinishedAt *time.Time           `json:"finished_at,omitempty"`
	// Stdout/Stderr are only populated for non-detached commands that
	// completed before the response was returned.
	Stdout string `json:"stdout,omitempty"`
	Stderr string `json:"stderr,omitempty"`
}

// RunSandboxCommandRequest is the body for POST /sandboxes/{id}/commands.
type RunSandboxCommandRequest struct {
	Cmd      string            `json:"cmd"`
	Args     []string          `json:"args,omitempty"`
	Cwd      string            `json:"cwd,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
	Sudo     bool              `json:"sudo,omitempty"`
	Detached bool              `json:"detached,omitempty"`
	// TimeoutSeconds is per-command timeout. Defaults to 60s if 0 and !detached.
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
}

// SandboxFileEntry describes a single entry returned by the directory list endpoint.
type SandboxFileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"mod_time"`
}

// SandboxFileListResponse is the response for GET /sandboxes/{id}/files/list.
type SandboxFileListResponse struct {
	Path    string             `json:"path"`
	Entries []SandboxFileEntry `json:"entries"`
}
