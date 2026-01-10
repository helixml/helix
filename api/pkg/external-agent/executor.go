package external_agent

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// Executor defines the interface for external agent executors
type Executor interface {
	// Desktop session methods
	StartDesktop(ctx context.Context, agent *types.DesktopAgent) (*types.DesktopAgentResponse, error)
	StopDesktop(ctx context.Context, sessionID string) error
	GetSession(sessionID string) (*ZedSession, error)
	CleanupExpiredSessions(ctx context.Context, timeout time.Duration)
	ListSessions() []*ZedSession

	// Multi-session SpecTask methods
	StartZedInstance(ctx context.Context, agent *types.DesktopAgent) (*types.DesktopAgentResponse, error)
	CreateZedThread(ctx context.Context, instanceID, threadID string, config map[string]interface{}) error
	StopZedInstance(ctx context.Context, instanceID string) error
	GetInstanceStatus(instanceID string) (*ZedInstanceStatus, error)
	ListInstanceThreads(instanceID string) ([]*ZedThreadInfo, error)

	// Screenshot support
	FindContainerBySessionID(ctx context.Context, helixSessionID string) (string, error)

	// Reconciliation support
	HasRunningContainer(ctx context.Context, sessionID string) bool
}

// Shared types used by all executor implementations

// ZedInstanceInfo tracks information about a Zed instance
type ZedInstanceInfo struct {
	InstanceID   string    `json:"instanceID"`
	SpecTaskID   string    `json:"specTaskID"`   // Optional - null for personal dev environments
	UserID       string    `json:"userID"`       // Always required
	AppID        string    `json:"appID"`        // Helix App ID for configuration (MCP servers, tools, etc.)
	InstanceType string    `json:"instanceType"` // "spec_task", "personal_dev", "shared_workspace"
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"createdAt"`
	LastActivity time.Time `json:"lastActivity"`
	ProjectPath  string    `json:"projectPath"`
	ThreadCount  int       `json:"threadCount"`

	// Personal dev environment specific
	IsPersonalEnv   bool     `json:"is_personal_env"`
	EnvironmentName string   `json:"environment_name,omitempty"` // User-friendly name
	ConfiguredTools []string `json:"configured_tools,omitempty"` // MCP servers enabled
	DataSources     []string `json:"data_sources,omitempty"`     // Connected data sources
	StreamURL       string   `json:"stream_url,omitempty"`       // Video streaming URL

	// Display configuration for streaming
	DisplayWidth  int `json:"display_width,omitempty"`  // Streaming resolution width
	DisplayHeight int `json:"display_height,omitempty"` // Streaming resolution height
	DisplayFPS    int `json:"display_fps,omitempty"`    // Streaming framerate

	// Container information for direct network access
	ContainerName string `json:"container_name,omitempty"` // Docker container name
}

// ZedInstanceStatus represents the current status of a Zed instance
type ZedInstanceStatus struct {
	InstanceID    string     `json:"instance_id"`
	SpecTaskID    string     `json:"spec_task_id,omitempty"`
	Status        string     `json:"status"`
	ThreadCount   int        `json:"thread_count"`
	ActiveThreads int        `json:"active_threads"`
	LastActivity  *time.Time `json:"last_activity,omitempty"`
	ProjectPath   string     `json:"project_path,omitempty"`
}

// ZedThreadInfo represents information about a thread within an instance
type ZedThreadInfo struct {
	ThreadID      string                 `json:"thread_id"`
	WorkSessionID string                 `json:"work_session_id"`
	Status        string                 `json:"status"`
	CreatedAt     time.Time              `json:"created_at"`
	LastActivity  *time.Time             `json:"last_activity,omitempty"`
	Config        map[string]interface{} `json:"config,omitempty"`
}

// ZedSession represents a single Zed session
type ZedSession struct {
	SessionID      string    `json:"session_id"`       // Agent session ID (key for external agents)
	HelixSessionID string    `json:"helix_session_id"` // Helix session ID (for screenshot lookup)
	UserID         string    `json:"user_id"`
	Status         string    `json:"status"`
	StartTime      time.Time `json:"start_time"`
	LastAccess     time.Time `json:"last_access"`
	ProjectPath    string    `json:"project_path,omitempty"`
	ContainerAppID string `json:"container_app_id,omitempty"` // Deprecated: legacy field
	DevContainerID string `json:"dev_container_id,omitempty"` // Container ID for the dev container
	ContainerName  string `json:"container_name,omitempty"`   // Container hostname for DNS lookup

	// Container fields
	ContainerID string `json:"container_id,omitempty"` // Docker container ID
	ContainerIP string `json:"container_ip,omitempty"` // Container IP address on helix_default network
	SandboxID   string `json:"sandbox_id,omitempty"`   // Sandbox running this container (for RevDial routing)
}
