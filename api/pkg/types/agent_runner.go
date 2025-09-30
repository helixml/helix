package types

import (
	"time"
)

// AgentRunner represents an external agent runner
type AgentRunner struct {
	ID              string    `json:"id" gorm:"primaryKey"`              // Runner ID
	Status          string    `json:"status"`                            // "online", "offline", "starting", "stopping"
	LastSeen        time.Time `json:"last_seen"`                         // Last heartbeat from runner
	CreatedAt       time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt       time.Time `json:"updated_at" gorm:"autoUpdateTime"`
	Version         string    `json:"version,omitempty"`                      // Runner version
	Labels          string    `json:"labels,omitempty" gorm:"type:text"`      // JSON string of labels
	ActiveSessions  int       `json:"active_sessions" gorm:"default:0"`       // Number of active sessions
	MaxSessions     int       `json:"max_sessions" gorm:"default:10"`         // Maximum concurrent sessions
	HealthStatus    string    `json:"health_status" gorm:"default:'unknown'"` // "healthy", "unhealthy", "unknown"
	LastHealthCheck time.Time `json:"last_health_check"`
	Metadata        string    `json:"metadata,omitempty" gorm:"type:text"` // JSON string for additional metadata
}

// AgentRunnerStatus represents the current status of an agent runner
type AgentRunnerStatus struct {
	ID              string    `json:"id"`
	Status          string    `json:"status"`
	LastSeen        time.Time `json:"last_seen"`
	ActiveSessions  int       `json:"active_sessions"`
	MaxSessions     int       `json:"max_sessions"`
	HealthStatus    string    `json:"health_status"`
	LastHealthCheck time.Time `json:"last_health_check"`
	Version         string    `json:"version,omitempty"`
}

// ListAgentRunnersQuery represents query parameters for listing agent runners
type ListAgentRunnersQuery struct {
	Page         int    `json:"page"`
	PageSize     int    `json:"page_size"`
	Status       string `json:"status,omitempty"`        // Filter by status
	HealthStatus string `json:"health_status,omitempty"` // Filter by health status
	OnlineOnly   bool   `json:"online_only"`             // Only return online runners
	OrderBy      string `json:"order_by,omitempty"`      // "created_at", "last_seen", "id"
}

// AgentRunnersResponse represents the response for listing agent runners
type AgentRunnersResponse struct {
	Runners  []*AgentRunner `json:"runners"`
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}
