package types

import (
	"encoding/json"
	"time"
)

type WebhookEndpointStatus string

const (
	WebhookEndpointStatusActive   WebhookEndpointStatus = "active"
	WebhookEndpointStatusDisabled WebhookEndpointStatus = "disabled"
)

type WebhookDeliveryStatus string

const (
	WebhookDeliveryStatusPending    WebhookDeliveryStatus = "pending"
	WebhookDeliveryStatusProcessing WebhookDeliveryStatus = "processing"
	WebhookDeliveryStatusRetrying   WebhookDeliveryStatus = "retrying"
	WebhookDeliveryStatusDelivered  WebhookDeliveryStatus = "delivered"
	WebhookDeliveryStatusFailed     WebhookDeliveryStatus = "failed"
	WebhookDeliveryStatusDisabled   WebhookDeliveryStatus = "disabled"
)

const (
	WebhookEventSpecTaskCreated       = "spec_task.created"
	WebhookEventSpecTaskStatusChanged = "spec_task.status_changed"
	WebhookEventArtifactPublished     = "artifact.published"
)

var SupportedWebhookEvents = []string{
	WebhookEventSpecTaskCreated,
	WebhookEventSpecTaskStatusChanged,
	WebhookEventArtifactPublished,
}

// WebhookEndpoint is an organization-owned Standard Webhooks destination.
// Signing secrets are encrypted at rest and never serialized by the API.
type WebhookEndpoint struct {
	ID                      string                `json:"id" gorm:"primaryKey"`
	OrganizationID          string                `json:"organization_id" gorm:"index;index:idx_webhook_endpoint_match,priority:1;not null"`
	ProjectID               string                `json:"project_id,omitempty" gorm:"index;index:idx_webhook_endpoint_match,priority:2"`
	URL                     string                `json:"url" gorm:"type:text;not null"`
	Description             string                `json:"description,omitempty" gorm:"type:text"`
	Events                  []string              `json:"events" gorm:"type:jsonb;serializer:json;not null"`
	Status                  WebhookEndpointStatus `json:"status" gorm:"size:32;index;index:idx_webhook_endpoint_match,priority:3;not null"`
	SecretEncrypted         string                `json:"-" gorm:"type:text;not null"`
	SecretPreview           string                `json:"secret_preview" gorm:"size:16;not null"`
	PreviousSecretEncrypted string                `json:"-" gorm:"type:text"`
	PreviousSecretExpiresAt *time.Time            `json:"-"`
	DisabledReason          string                `json:"disabled_reason,omitempty" gorm:"type:text"`
	CreatedBy               string                `json:"created_by" gorm:"index;not null"`
	UpdatedBy               string                `json:"updated_by" gorm:"index;not null"`
	CreatedAt               time.Time             `json:"created_at"`
	UpdatedAt               time.Time             `json:"updated_at"`
}

// WebhookEvent is the immutable, thin outbox payload. Consumers use its IDs to
// fetch authoritative state from Helix rather than treating the payload as a snapshot.
type WebhookEvent struct {
	ID             string          `json:"id" gorm:"primaryKey"`
	APIVersion     string          `json:"api_version" gorm:"size:32;not null"`
	Type           string          `json:"type" gorm:"size:128;index;not null"`
	OrganizationID string          `json:"organization_id" gorm:"index;not null"`
	ProjectID      string          `json:"project_id,omitempty" gorm:"index"`
	Data           json.RawMessage `json:"data" gorm:"type:jsonb;not null"`
	CreatedAt      time.Time       `json:"timestamp" gorm:"index"`
}

type WebhookDelivery struct {
	ID             string                `json:"id" gorm:"primaryKey"`
	EventID        string                `json:"event_id" gorm:"uniqueIndex:idx_webhook_event_endpoint;index;not null"`
	EndpointID     string                `json:"endpoint_id" gorm:"uniqueIndex:idx_webhook_event_endpoint;index;not null"`
	Status         WebhookDeliveryStatus `json:"status" gorm:"size:32;index;index:idx_webhook_delivery_due,priority:1;not null"`
	AttemptCount   int                   `json:"attempt_count" gorm:"not null"`
	NextAttemptAt  time.Time             `json:"next_attempt_at" gorm:"index;index:idx_webhook_delivery_due,priority:2;not null"`
	LockedUntil    *time.Time            `json:"-" gorm:"index"`
	LastAttemptAt  *time.Time            `json:"last_attempt_at,omitempty"`
	DeliveredAt    *time.Time            `json:"delivered_at,omitempty"`
	LastStatusCode int                   `json:"last_status_code,omitempty"`
	LastError      string                `json:"last_error,omitempty" gorm:"type:text"`
	CreatedAt      time.Time             `json:"created_at"`
	UpdatedAt      time.Time             `json:"updated_at"`
}

type SpecTaskWebhookData struct {
	SpecTaskID      string         `json:"spec_task_id"`
	ProjectID       string         `json:"project_id"`
	OrganizationID  string         `json:"organization_id"`
	Status          SpecTaskStatus `json:"status"`
	PreviousStatus  SpecTaskStatus `json:"previous_status,omitempty"`
	StatusUpdatedAt *time.Time     `json:"status_updated_at,omitempty"`
}

type ArtifactWebhookData struct {
	ArtifactID       string       `json:"artifact_id"`
	ProjectID        string       `json:"project_id"`
	OrganizationID   string       `json:"organization_id"`
	ActiveVersionID  string       `json:"active_version_id"`
	Kind             ArtifactKind `json:"kind"`
	SourceSpecTaskID string       `json:"source_spec_task_id,omitempty"`
}
