package types

import (
	"encoding/json"
	"time"
)

type OrgAuditEventType string

const (
	OrgAuditEventMCPCall       OrgAuditEventType = "mcp_call"
	OrgAuditEventSSHConnection OrgAuditEventType = "ssh_connection"
	OrgAuditEventSSHCommand    OrgAuditEventType = "ssh_command"
)

type OrgAuditStatus string

const (
	OrgAuditStatusAttempted OrgAuditStatus = "attempted"
	OrgAuditStatusSucceeded OrgAuditStatus = "succeeded"
	OrgAuditStatusFailed    OrgAuditStatus = "failed"
)

type OrgAuditActorType string

const (
	OrgAuditActorBot  OrgAuditActorType = "bot"
	OrgAuditActorUser OrgAuditActorType = "user"
)

type OrgAuditMetadata struct {
	Arguments     json.RawMessage `json:"arguments,omitempty"`
	AssetRef      string          `json:"asset_ref,omitempty"`
	Command       string          `json:"command,omitempty"`
	CommandID     string          `json:"command_id,omitempty"`
	Error         string          `json:"error,omitempty"`
	RemoteAddress string          `json:"remote_address,omitempty"`
	LocalAddress  string          `json:"local_address,omitempty"`
	SSHUser       string          `json:"ssh_user,omitempty"`
	ClientVersion string          `json:"client_version,omitempty"`
	DurationMS    int64           `json:"duration_ms,omitempty"`
}

// OrgAuditLog is an append-only audit trail entry for actions performed
// within a Helix organization.
type OrgAuditLog struct {
	ID             string            `json:"id" gorm:"primaryKey;size:255"`
	OrganizationID string            `json:"organization_id" gorm:"size:255;index;not null"`
	ProjectID      string            `json:"project_id,omitempty" gorm:"size:255;index"`
	UserID         string            `json:"user_id,omitempty" gorm:"size:255;index"`
	ActorID        string            `json:"actor_id" gorm:"size:255;index;not null"`
	ActorType      OrgAuditActorType `json:"actor_type" gorm:"size:50;index;not null"`
	AssetID        string            `json:"asset_id,omitempty" gorm:"size:255;index"`
	EventType      OrgAuditEventType `json:"event_type" gorm:"size:50;index;not null"`
	Action         string            `json:"action" gorm:"size:255;index;not null"`
	Status         OrgAuditStatus    `json:"status" gorm:"size:50;index;not null"`
	Metadata       OrgAuditMetadata  `json:"metadata,omitempty" gorm:"type:jsonb;serializer:json"`
	CreatedAt      time.Time         `json:"created_at" gorm:"index"`
}

type OrgAuditLogFilters struct {
	OrganizationID string            `json:"organization_id"`
	ProjectID      string            `json:"project_id,omitempty"`
	UserID         string            `json:"user_id,omitempty"`
	ActorID        string            `json:"actor_id,omitempty"`
	AssetID        string            `json:"asset_id,omitempty"`
	EventType      OrgAuditEventType `json:"event_type,omitempty"`
	Action         string            `json:"action,omitempty"`
	Status         OrgAuditStatus    `json:"status,omitempty"`
	StartDate      *time.Time        `json:"start_date,omitempty"`
	EndDate        *time.Time        `json:"end_date,omitempty"`
	Search         string            `json:"search,omitempty"`
	Limit          int               `json:"limit,omitempty"`
	Offset         int               `json:"offset,omitempty"`
}

type OrgAuditLogResponse struct {
	Logs   []*OrgAuditLog `json:"logs"`
	Total  int64          `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}
