package types

import "time"

type SecretIntakeField struct {
	Name         string `json:"name"`
	Label        string `json:"label"`
	Type         string `json:"type"`
	Required     bool   `json:"required"`
	Autocomplete string `json:"autocomplete,omitempty"`
}

type SecretIntakeCreateRequest struct {
	CustomerID     string              `json:"customer_id"`
	ConversationID string              `json:"conversation_id"`
	Title          string              `json:"title"`
	Description    string              `json:"description"`
	BrandName      string              `json:"brand_name"`
	AccentColor    string              `json:"accent_color"`
	Fields         []SecretIntakeField `json:"fields"`
	ArtifactID     string              `json:"artifact_id,omitempty"`
}

type SecretIntakeCreateResult struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	InviteURL string `json:"invite_url"`
}

type SecretIntakeStatusResult struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}

// SecretIntake holds encrypted values and is never returned directly to an API or MCP caller.
type SecretIntake struct {
	ID                  string `gorm:"primaryKey"`
	ProjectID           string `gorm:"index;not null"`
	CustomerID          string `gorm:"not null"`
	ConversationID      string `gorm:"not null"`
	Title               string `gorm:"not null"`
	Description         string
	BrandName           string              `gorm:"not null"`
	AccentColor         string              `gorm:"not null"`
	Fields              []SecretIntakeField `gorm:"type:jsonb;serializer:json;not null"`
	ArtifactID          string
	ArtifactBefore      string
	ArtifactAfter       string
	Status              string `gorm:"not null"`
	InvitationHash      string `gorm:"index"`
	FlowHash            string `gorm:"index"`
	CSRFHash            string
	ValuesEncrypted     string
	InvitationExpiresAt time.Time
	FlowExpiresAt       time.Time
	ValuesExpiresAt     time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
