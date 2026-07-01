package hydra

import "time"

// ScopeType represents the type of external agent session
type ScopeType string

const (
	ScopeTypeSpecTask    ScopeType = "spectask"
	ScopeTypeSession     ScopeType = "session"
	ScopeTypeExploratory ScopeType = "exploratory"
)

// HealthResponse is the response from the health endpoint
type HealthResponse struct {
	Status          string `json:"status"`
	ActiveInstances int    `json:"active_instances"`
	Version         string `json:"version"`
}

// containerSessionIDLabel is the Docker label hydra stamps on every dev
// container with its session id, so hydra can re-adopt running containers
// (with the exact session id) after a restart — see
// DevContainerManager.RecoverDevContainersFromDocker.
const containerSessionIDLabel = "helix.session_id"

// containerPersistentLabel marks long-lived (web-service) containers so the
// boot-time stopped-container reaper skips them and they survive a reboot.
const containerPersistentLabel = "helix.persistent"

// DevContainerType represents the type of dev container
type DevContainerType string

const (
	DevContainerTypeSway     DevContainerType = "sway"     // Sway compositor with Zed
	DevContainerTypeUbuntu   DevContainerType = "ubuntu"   // GNOME with Zed
	DevContainerTypeHeadless DevContainerType = "headless" // No GUI, just agent (future)
)

// DevContainerStatus represents the current status of a dev container
type DevContainerStatus string

const (
	DevContainerStatusStarting DevContainerStatus = "starting"
	DevContainerStatusRunning  DevContainerStatus = "running"
	DevContainerStatusStopped  DevContainerStatus = "stopped"
	DevContainerStatusError    DevContainerStatus = "error"
)

// MountConfig represents a volume mount configuration
type MountConfig struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	ReadOnly    bool   `json:"readonly,omitempty"`
	Type        string `json:"type,omitempty"` // "" = bind mount (default), "volume" = Docker named volume
}

// CreateDevContainerRequest creates a dev container (Zed+agent environment) for a session
type CreateDevContainerRequest struct {
	SessionID string `json:"session_id"`

	// Container configuration
	Image         string        `json:"image"`          // e.g., "helix-ubuntu:latest"
	ContainerName string        `json:"container_name"` // e.g., "ubuntu-external-ses_xxx"
	Hostname      string        `json:"hostname"`
	Env           []string      `json:"env"` // KEY=value format
	Mounts        []MountConfig `json:"mounts"`

	// Display settings (optional - headless containers omit these)
	DisplayWidth  int `json:"display_width,omitempty"`
	DisplayHeight int `json:"display_height,omitempty"`
	DisplayFPS    int `json:"display_fps,omitempty"`

	// Dev container type
	ContainerType DevContainerType `json:"container_type"`

	// GPU settings
	GPUVendor string `json:"gpu_vendor"` // "nvidia", "amd", "intel", ""

	// GPUIndex pins this dev container to a specific GPU on a multi-GPU
	// host. Counted starting at 0. Pointer so omission means "no pin"
	// (current behaviour: all GPUs visible) — preserves backwards
	// compatibility for callers that don't yet pass a value. When set:
	//   nvidia: NVIDIA_VISIBLE_DEVICES=<n> (instead of =all)
	//   amd:    only /dev/dri/renderD<128+n> mounted (instead of all)
	//   intel:  only /dev/dri/renderD<128+n> mounted
	// HELIX_GPU_INDEX=<n> is also set in the container env so
	// detect-render-node.sh picks the matching card device for Mutter
	// + GStreamer encoder. See Decision 15 in the sandbox-absorbs-runner
	// design doc.
	GPUIndex *int `json:"gpu_index,omitempty"`

	// Docker socket to use (from Hydra isolation or default sandbox dockerd)
	// If empty, uses the sandbox's default Docker socket
	DockerSocket string `json:"docker_socket,omitempty"`

	// User ID for SSH key mounting and ownership
	UserID string `json:"user_id,omitempty"`

	// Network to attach to (defaults to bridge)
	Network string `json:"network,omitempty"`

	// Privileged mode (required for docker-in-desktop: inner dockerd needs it)
	Privileged bool `json:"privileged,omitempty"`

	// ProjectID for golden Docker cache lookup (per-project overlayfs)
	ProjectID string `json:"project_id,omitempty"`

	// GoldenBuild marks this as a golden cache build session.
	// Golden build sessions use a plain directory (not overlay) for Docker data,
	// and the data is promoted to golden when the container exits with code 0.
	GoldenBuild bool `json:"golden_build,omitempty"`

	// VCPUs caps the number of CPUs the container can use. 0 = no cap.
	VCPUs int `json:"vcpus,omitempty"`

	// MemoryMB caps the memory the container can use, in MB. 0 = no cap.
	MemoryMB int `json:"memory_mb,omitempty"`

	// Entrypoint and Cmd override the image defaults. Used by the Sandboxes
	// API "headless" runtime to keep a plain ubuntu container alive with
	// `sleep infinity` so users can exec into it.
	Entrypoint []string `json:"entrypoint,omitempty"`
	Cmd        []string `json:"cmd,omitempty"`

	// SkipImageValidation lets the caller use a non-helix-prefixed image (e.g.
	// `ubuntu:22.04`). The Sandboxes API headless runtime sets this so plain
	// docker images can be used.
	SkipImageValidation bool `json:"skip_image_validation,omitempty"`

	// Persistent marks a long-lived dev container (e.g. a hosted web service)
	// that must survive host reboots and dockerd restarts. It gets a Docker
	// restart policy (unless-stopped) and is exempt from stopped-container
	// reaping, so a reboot self-heals in seconds with /data and the image
	// cache intact — instead of the slow provision-fresh-sandbox + full
	// rebuild recovery path.
	Persistent bool `json:"persistent,omitempty"`
}

// DevContainerResponse is the response after creating/querying a dev container
type DevContainerResponse struct {
	SessionID     string             `json:"session_id"`
	ContainerID   string             `json:"container_id"`
	ContainerName string             `json:"container_name"`
	Status        DevContainerStatus `json:"status"`
	Error         string             `json:"error,omitempty"`

	// Network info for RevDial/screenshot-server connections
	IPAddress string `json:"ip_address,omitempty"`

	// Container type
	ContainerType DevContainerType `json:"container_type"`

	// Desktop environment info (for debug panel)
	DesktopVersion string `json:"desktop_version,omitempty"` // helix-ubuntu image version (commit hash)
	GPUVendor      string `json:"gpu_vendor,omitempty"`      // nvidia, amd, intel, or ""
	RenderNode     string `json:"render_node,omitempty"`     // /dev/dri/renderD128 or SOFTWARE
}

// DevContainer represents a running dev container managed by Hydra
type DevContainer struct {
	SessionID     string             `json:"session_id"`
	ContainerID   string             `json:"container_id"`
	ContainerName string             `json:"container_name"`
	Status        DevContainerStatus `json:"status"`
	IPAddress     string             `json:"ip_address"`
	ContainerType DevContainerType   `json:"container_type"`
	UserID        string             `json:"user_id"`
	CreatedAt     time.Time          `json:"created_at"`
	DockerSocket  string             `json:"docker_socket"` // Which dockerd manages this container

	// Golden build fields
	IsGoldenBuild bool   `json:"is_golden_build,omitempty"` // This is a golden cache build session
	ProjectID     string `json:"project_id,omitempty"`      // Project ID for golden promotion
}

// ListDevContainersResponse is the response listing all dev containers
type ListDevContainersResponse struct {
	Containers []DevContainerResponse `json:"containers"`
}

// GPUInfo represents information about a single GPU
type GPUInfo struct {
	Index       int    `json:"index"`
	Name        string `json:"name"`
	Vendor      string `json:"vendor"` // "nvidia", "amd", "intel"
	MemoryTotal int64  `json:"memory_total_bytes"`
	MemoryUsed  int64  `json:"memory_used_bytes"`
	MemoryFree  int64  `json:"memory_free_bytes"`
	Utilization int    `json:"utilization_percent"` // GPU core utilization
	Temperature int    `json:"temperature_celsius"`
}

// SystemStatsResponse is the response for system stats endpoint
type SystemStatsResponse struct {
	GPUs             []GPUInfo `json:"gpus"`
	ActiveContainers int       `json:"active_containers"`
	ActiveSessions   int       `json:"active_sessions"`
}

// VersionResponse is the response for the version endpoint
type VersionResponse struct {
	Version string   `json:"version"`
	Routes  []string `json:"routes"`
}

// GCReconcileRequest is the durable, DB-driven garbage-collection request the
// API sends to hydra. The API computes the live-set from Postgres (the source
// of truth that survives reboots) and hydra reconciles on-disk resources
// (session zvols, per-task/session workspace dirs) against it.
type GCReconcileRequest struct {
	// LiveSessionIDs is the set of session IDs (full ses_… strings) that the
	// API considers alive. zvols and session workspace dirs for IDs NOT in this
	// set are candidates for reaping (subject to the grace period).
	LiveSessionIDs []string `json:"live_session_ids"`

	// LiveSpecTaskIDs is the set of spec-task IDs (full spt_… strings) the API
	// considers alive. Spec-task workspace dirs for IDs NOT in this set are
	// candidates for reaping (subject to the grace period).
	LiveSpecTaskIDs []string `json:"live_spec_task_ids"`

	// GracePeriodSeconds is the minimum age (since creation / mtime) a resource
	// must reach before it can be reaped. Guards against racing newly-created
	// resources the DB live-set hasn't caught up with yet.
	GracePeriodSeconds int `json:"grace_period_seconds"`

	// DryRun reports what WOULD be reaped without destroying anything.
	DryRun bool `json:"dry_run"`
}

// GCSkip records a resource the reconciler chose NOT to reap, with the reason.
type GCSkip struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// GCReconcileResponse is hydra's report of what it reaped (or would reap, in
// dry-run mode) and what it deliberately skipped.
type GCReconcileResponse struct {
	ZvolsReaped         []string `json:"zvols_reaped"`
	ZvolsSkipped        []GCSkip `json:"zvols_skipped"`
	WorkspacesReaped    []string `json:"workspaces_reaped"`
	WorkspacesSkipped   []GCSkip `json:"workspaces_skipped"`
	FileCopyDirsReaped  []string `json:"file_copy_dirs_reaped"`
	FileCopyDirsSkipped []GCSkip `json:"file_copy_dirs_skipped"`
	GoldensFlattened    []string `json:"goldens_flattened"`
	BytesFreed          int64    `json:"bytes_freed"`
}
