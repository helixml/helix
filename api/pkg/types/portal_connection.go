package types

import "time"

type PortalConnectionStatus string

const (
	PortalConnectionPasswordPending PortalConnectionStatus = "password_pending"
	PortalConnectionOTPPending      PortalConnectionStatus = "otp_pending"
	PortalConnectionConnected       PortalConnectionStatus = "connected"
	PortalConnectionFailed          PortalConnectionStatus = "failed"
	PortalConnectionExpired         PortalConnectionStatus = "expired"
	PortalConnectionRevoked         PortalConnectionStatus = "revoked"
)

// PortalConnectionAttempt is never serialized directly to an API response.
type PortalConnectionAttempt struct {
	ID                  string                 `gorm:"primaryKey"`
	ProjectID           string                 `gorm:"index;not null"`
	CustomerID          string                 `gorm:"not null"`
	ConversationID      string                 `gorm:"not null"`
	Portal              string                 `gorm:"not null"`
	BrandName           string                 `gorm:"not null"`
	AccentColor         string                 `gorm:"not null"`
	Status              PortalConnectionStatus `gorm:"not null"`
	InvitationHash      string                 `gorm:"index"`
	FlowHash            string                 `gorm:"index"`
	CSRFHash            string
	SessionEncrypted    string
	PasswordAttempts    int
	OTPAttempts         int
	InvitationExpiresAt time.Time
	FlowExpiresAt       time.Time
	SessionExpiresAt    time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
