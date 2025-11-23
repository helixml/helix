package types

import "time"

// WolfInstance represents a Wolf streaming instance that can connect to the control plane
type WolfInstance struct {
	ID                 string    `gorm:"type:varchar(255);primaryKey" json:"id"`
	Name               string    `gorm:"type:varchar(255);not null" json:"name"`
	Address            string    `gorm:"type:varchar(255);not null" json:"address"` // e.g., "wolf-1.example.com:8080"
	Status             string    `gorm:"type:varchar(50);not null;default:'offline'" json:"status"` // online, offline, degraded
	LastHeartbeat      time.Time `gorm:"index" json:"last_heartbeat"`
	ConnectedSandboxes int       `gorm:"default:0" json:"connected_sandboxes"`
	MaxSandboxes       int       `gorm:"default:10" json:"max_sandboxes"`
	GPUType            string    `gorm:"type:varchar(100)" json:"gpu_type"` // nvidia, amd, none
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
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

// WolfInstanceResponse is the API response for a Wolf instance
type WolfInstanceResponse struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	Address            string    `json:"address"`
	Status             string    `json:"status"`
	LastHeartbeat      time.Time `json:"last_heartbeat"`
	ConnectedSandboxes int       `json:"connected_sandboxes"`
	MaxSandboxes       int       `json:"max_sandboxes"`
	GPUType            string    `json:"gpu_type"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// ToResponse converts a WolfInstance to WolfInstanceResponse
func (w *WolfInstance) ToResponse() *WolfInstanceResponse {
	return &WolfInstanceResponse{
		ID:                 w.ID,
		Name:               w.Name,
		Address:            w.Address,
		Status:             w.Status,
		LastHeartbeat:      w.LastHeartbeat,
		ConnectedSandboxes: w.ConnectedSandboxes,
		MaxSandboxes:       w.MaxSandboxes,
		GPUType:            w.GPUType,
		CreatedAt:          w.CreatedAt,
		UpdatedAt:          w.UpdatedAt,
	}
}
