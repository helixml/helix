package hydra

import "time"

// ScopeType represents the type of external agent session
type ScopeType string

const (
	ScopeTypeSpecTask    ScopeType = "spectask"
	ScopeTypeSession     ScopeType = "session"
	ScopeTypeExploratory ScopeType = "exploratory"
)

// DockerInstanceStatus represents the current status of a dockerd instance
type DockerInstanceStatus string

const (
	StatusRunning DockerInstanceStatus = "running"
	StatusStopped DockerInstanceStatus = "stopped"
	StatusStarting DockerInstanceStatus = "starting"
	StatusError   DockerInstanceStatus = "error"
)

// CreateDockerInstanceRequest is the request to create a new dockerd instance
type CreateDockerInstanceRequest struct {
	ScopeType     ScopeType `json:"scope_type"`      // spectask, session, or exploratory
	ScopeID       string    `json:"scope_id"`        // The ID of the scope (task ID, session ID)
	UserID        string    `json:"user_id"`         // User who owns this instance
	MaxContainers int       `json:"max_containers"`  // Optional limit on containers (0 = unlimited)
	UseHostDocker bool      `json:"use_host_docker"` // If true and privileged mode enabled, use host Docker socket
}

// DockerInstanceResponse is the response containing dockerd instance info
type DockerInstanceResponse struct {
	ScopeType    ScopeType            `json:"scope_type"`
	ScopeID      string               `json:"scope_id"`
	DockerSocket string               `json:"docker_socket"` // Path to the Docker socket
	DockerHost   string               `json:"docker_host"`   // DOCKER_HOST env var value
	DataRoot     string               `json:"data_root"`     // Path to Docker data directory
	Status       DockerInstanceStatus `json:"status"`
	Error        string               `json:"error,omitempty"`
	// Network bridging info for desktop-to-dev-container communication
	BridgeName string `json:"bridge_name,omitempty"` // Bridge interface name (e.g., "hydra3")
	Subnet     string `json:"subnet,omitempty"`      // Subnet for this network (e.g., "10.200.3.0/24")
	Gateway    string `json:"gateway,omitempty"`     // Gateway IP (e.g., "10.200.3.1")
}

// DeleteDockerInstanceResponse is the response when stopping a dockerd
type DeleteDockerInstanceResponse struct {
	ScopeType         ScopeType            `json:"scope_type"`
	ScopeID           string               `json:"scope_id"`
	Status            DockerInstanceStatus `json:"status"`
	ContainersStopped int                  `json:"containers_stopped"`
	DataPreserved     bool                 `json:"data_preserved"`
}

// DockerInstanceStatusResponse is the detailed status of a dockerd instance
type DockerInstanceStatusResponse struct {
	ScopeType      ScopeType            `json:"scope_type"`
	ScopeID        string               `json:"scope_id"`
	Status         DockerInstanceStatus `json:"status"`
	ContainerCount int                  `json:"container_count"`
	UptimeSeconds  int64                `json:"uptime_seconds"`
	DockerSocket   string               `json:"docker_socket"`
	DataRoot       string               `json:"data_root"`
	DataSizeBytes  int64                `json:"data_size_bytes"`
	UserID         string               `json:"user_id"`
	CreatedAt      time.Time            `json:"created_at"`
}

// ListDockerInstancesResponse is the response listing all dockerd instances
type ListDockerInstancesResponse struct {
	Instances []DockerInstanceStatusResponse `json:"instances"`
}

// PurgeDockerInstanceResponse is the response when purging dockerd data
type PurgeDockerInstanceResponse struct {
	ScopeType        ScopeType `json:"scope_type"`
	ScopeID          string    `json:"scope_id"`
	Status           string    `json:"status"` // "purged"
	DataDeletedBytes int64     `json:"data_deleted_bytes"`
}

// DockerInstance represents a running dockerd instance managed by Hydra
type DockerInstance struct {
	ScopeType     ScopeType            `json:"scope_type"`
	ScopeID       string               `json:"scope_id"`
	UserID        string               `json:"user_id"`
	Status        DockerInstanceStatus `json:"status"`
	PID           int                  `json:"pid"`
	SocketPath    string               `json:"socket_path"`
	DataRoot      string               `json:"data_root"`
	ExecRoot      string               `json:"exec_root"`
	PIDFile       string               `json:"pid_file"`
	ConfigFile    string               `json:"config_file"`
	MaxContainers int                  `json:"max_containers"`
	StartedAt     time.Time            `json:"started_at"`
	BridgeIndex   uint8                `json:"bridge_index"` // Unique index for bridge IP range (1-254)
	BridgeName    string               `json:"bridge_name"`  // Bridge interface name (e.g., "hydra1")

	// Desktop bridging state (for self-healing after container restarts)
	DesktopBridged     bool   `json:"desktop_bridged"`      // Whether desktop is currently bridged
	DesktopContainerID string `json:"desktop_container_id"` // Container ID/name that was bridged
	DesktopPID         int    `json:"desktop_pid"`          // PID of bridged container (for detecting restart)
	VethBridgeName     string `json:"veth_bridge_name"`     // Bridge-side veth name for cleanup
}

// InstanceKey returns a unique key for this instance
func (d *DockerInstance) InstanceKey() string {
	return string(d.ScopeType) + "-" + d.ScopeID
}

// HealthResponse is the response from the health endpoint
type HealthResponse struct {
	Status          string `json:"status"`
	ActiveInstances int    `json:"active_instances"`
	Version         string `json:"version"`
}

// BridgeDesktopRequest is the request to bridge a desktop container to a Hydra network
// This enables the desktop (on Wolf's dockerd) to access dev containers (on Hydra's dockerd)
type BridgeDesktopRequest struct {
	SessionID          string `json:"session_id"`           // Which Hydra dockerd to bridge to
	DesktopContainerID string `json:"desktop_container_id"` // Container ID on Wolf's dockerd
}

// BridgeDesktopResponse is the response after bridging a desktop to Hydra network
type BridgeDesktopResponse struct {
	DesktopIP string `json:"desktop_ip"` // IP assigned to desktop on Hydra network (e.g., "10.200.3.254")
	Gateway   string `json:"gateway"`    // Gateway/DNS server (e.g., "10.200.3.1")
	Subnet    string `json:"subnet"`     // Subnet for this Hydra network (e.g., "10.200.3.0/24")
	Interface string `json:"interface"`  // Interface name added to desktop (e.g., "eth1")
}
