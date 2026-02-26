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

// DockerDataVolumePrefix is the naming convention for per-session inner dockerd
// data volumes. Used by hydra_executor (creation) and devcontainer (mount
// conversion to bind mount, GC, cleanup).
const DockerDataVolumePrefix = "docker-data-"

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
