package types

import (
	"encoding/json"
	"time"
)

// WolfInstance represents a Wolf streaming instance that can connect to the control plane
type WolfInstance struct {
	ID                    string    `gorm:"type:varchar(255);primaryKey" json:"id"`
	Name                  string    `gorm:"type:varchar(255);not null" json:"name"`
	Address               string    `gorm:"type:varchar(255);not null" json:"address"` // e.g., "wolf-1.example.com:8080"
	Status                string    `gorm:"type:varchar(50);not null;default:'offline'" json:"status"` // online, offline, degraded
	LastHeartbeat         time.Time `gorm:"index" json:"last_heartbeat"`
	ConnectedSandboxes    int       `gorm:"default:0" json:"connected_sandboxes"`
	MaxSandboxes          int       `gorm:"default:10" json:"max_sandboxes"`
	GPUType               string    `gorm:"type:varchar(100)" json:"gpu_type"`          // nvidia, amd, none
	SwayVersion           string    `gorm:"type:varchar(100)" json:"sway_version"`      // helix-sway image version (commit hash)
	DiskUsageJSON         string    `gorm:"type:text" json:"-"`                         // JSON-encoded disk usage metrics
	DiskAlertLevel        string    `gorm:"type:varchar(20)" json:"disk_alert_level"`   // highest alert level: ok, warning, critical
	PrivilegedModeEnabled bool      `gorm:"default:false" json:"privileged_mode_enabled"` // true if HYDRA_PRIVILEGED_MODE_ENABLED=true
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// WolfInstance status constants
const (
	WolfInstanceStatusOnline   = "online"
	WolfInstanceStatusOffline  = "offline"
	WolfInstanceStatusDegraded = "degraded"
)

// WolfInstanceRequest is the request body for registering/updating a Wolf instance
type WolfInstanceRequest struct {
	Name         string `json:"name"`
	Address      string `json:"address"`
	MaxSandboxes int    `json:"max_sandboxes,omitempty"`
	GPUType      string `json:"gpu_type,omitempty"`
}

// WolfHeartbeatRequest is the request body for Wolf instance heartbeat
type WolfHeartbeatRequest struct {
	SwayVersion           string                  `json:"sway_version,omitempty"`             // helix-sway image version (commit hash)
	DiskUsage             []DiskUsageMetric       `json:"disk_usage,omitempty"`               // disk usage metrics for monitored partitions
	ContainerUsage        []ContainerDiskUsage    `json:"container_usage,omitempty"`          // per-container disk usage breakdown
	PrivilegedModeEnabled bool                    `json:"privileged_mode_enabled,omitempty"`  // true if HYDRA_PRIVILEGED_MODE_ENABLED=true
}

// DiskUsageMetric represents disk usage for a single mount point
type DiskUsageMetric struct {
	MountPoint   string  `json:"mount_point"`   // e.g., "/var"
	TotalBytes   uint64  `json:"total_bytes"`   // total disk space
	UsedBytes    uint64  `json:"used_bytes"`    // used disk space
	AvailBytes   uint64  `json:"avail_bytes"`   // available disk space
	UsedPercent  float64 `json:"used_percent"`  // percentage used (0-100)
	AlertLevel   string  `json:"alert_level"`   // "ok", "warning", "critical"
}

// ContainerDiskUsage represents disk usage for a single container
type ContainerDiskUsage struct {
	ContainerID   string `json:"container_id"`   // Docker container ID
	ContainerName string `json:"container_name"` // Container name (e.g., "wolf-app-abc123")
	SizeBytes     uint64 `json:"size_bytes"`     // Total size of container's writable layer
	RwSizeBytes   uint64 `json:"rw_size_bytes"`  // Size of read-write layer only
}

// DiskUsageHistory stores historical disk usage data for time-series visualization
type DiskUsageHistory struct {
	ID              string    `gorm:"type:varchar(255);primaryKey" json:"id"`
	WolfInstanceID  string    `gorm:"type:varchar(255);index:idx_wolf_time,priority:1" json:"wolf_instance_id"`
	Timestamp       time.Time `gorm:"index:idx_wolf_time,priority:2" json:"timestamp"`
	MountPoint      string    `gorm:"type:varchar(255)" json:"mount_point"`
	TotalBytes      uint64    `json:"total_bytes"`
	UsedBytes       uint64    `json:"used_bytes"`
	AvailBytes      uint64    `json:"avail_bytes"`
	UsedPercent     float64   `json:"used_percent"`
	AlertLevel      string    `gorm:"type:varchar(20)" json:"alert_level"`
	ContainerUsage  string    `gorm:"type:text" json:"-"` // JSON-encoded []ContainerDiskUsage
}

// DiskUsageHistoryResponse is the API response for disk usage history
type DiskUsageHistoryResponse struct {
	WolfInstanceID string                   `json:"wolf_instance_id"`
	WolfName       string                   `json:"wolf_name"`
	History        []DiskUsageDataPoint     `json:"history"`
	Containers     []ContainerDiskUsageSummary `json:"containers,omitempty"`
}

// DiskUsageDataPoint represents a single point in time for disk usage tracking
type DiskUsageDataPoint struct {
	Timestamp   time.Time `json:"timestamp"`
	MountPoint  string    `json:"mount_point"`
	TotalMB     uint64    `json:"total_mb"`
	UsedMB      uint64    `json:"used_mb"`
	AvailMB     uint64    `json:"avail_mb"`
	UsedPercent float64   `json:"used_percent"`
	AlertLevel  string    `json:"alert_level"`
}

// ContainerDiskUsageSummary represents aggregated container disk usage
type ContainerDiskUsageSummary struct {
	ContainerName string `json:"container_name"`
	LatestSizeMB  uint64 `json:"latest_size_mb"`
}

// WolfInstanceResponse is the API response for a Wolf instance
type WolfInstanceResponse struct {
	ID                    string            `json:"id"`
	Name                  string            `json:"name"`
	Address               string            `json:"address"`
	Status                string            `json:"status"`
	LastHeartbeat         time.Time         `json:"last_heartbeat"`
	ConnectedSandboxes    int               `json:"connected_sandboxes"`
	MaxSandboxes          int               `json:"max_sandboxes"`
	GPUType               string            `json:"gpu_type"`
	SwayVersion           string            `json:"sway_version"`
	DiskUsage             []DiskUsageMetric `json:"disk_usage,omitempty"`
	DiskAlertLevel        string            `json:"disk_alert_level,omitempty"`
	PrivilegedModeEnabled bool              `json:"privileged_mode_enabled"`
	CreatedAt             time.Time         `json:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at"`
}

// ToResponse converts a WolfInstance to WolfInstanceResponse
func (w *WolfInstance) ToResponse() *WolfInstanceResponse {
	resp := &WolfInstanceResponse{
		ID:                    w.ID,
		Name:                  w.Name,
		Address:               w.Address,
		Status:                w.Status,
		LastHeartbeat:         w.LastHeartbeat,
		ConnectedSandboxes:    w.ConnectedSandboxes,
		MaxSandboxes:          w.MaxSandboxes,
		GPUType:               w.GPUType,
		SwayVersion:           w.SwayVersion,
		DiskAlertLevel:        w.DiskAlertLevel,
		PrivilegedModeEnabled: w.PrivilegedModeEnabled,
		CreatedAt:             w.CreatedAt,
		UpdatedAt:             w.UpdatedAt,
	}

	// Parse disk usage JSON if present
	if w.DiskUsageJSON != "" {
		var diskUsage []DiskUsageMetric
		if err := json.Unmarshal([]byte(w.DiskUsageJSON), &diskUsage); err == nil {
			resp.DiskUsage = diskUsage
		}
	}

	return resp
}
