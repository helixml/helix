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
	ScopeType     ScopeType `json:"scope_type"`     // spectask, session, or exploratory
	ScopeID       string    `json:"scope_id"`       // The ID of the scope (task ID, session ID)
	UserID        string    `json:"user_id"`        // User who owns this instance
	MaxContainers int       `json:"max_containers"` // Optional limit on containers (0 = unlimited)
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
