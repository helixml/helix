package types

import "time"

// RequestAuditEvent is one captured HTTP request: who made it, from where,
// what they asked for, and how it ended.
//
// It is an append-only forensic record, not product data. Nothing here is
// allowed to contain a raw credential: CredentialFingerprint is a truncated
// hash and the request body and query string are redacted before they are
// stored.
type RequestAuditEvent struct {
	ID string `json:"id" gorm:"primaryKey;size:64"`

	Created    time.Time `json:"created" gorm:"index"`
	DurationMS int64     `json:"duration_ms"`

	Method        string `json:"method" gorm:"size:16"`
	RouteTemplate string `json:"route_template" gorm:"size:512"`
	Path          string `json:"path" gorm:"size:2048"`
	Query         string `json:"query" gorm:"size:4096"`
	Status        int    `json:"status"`
	ResponseBytes int64  `json:"response_bytes"`

	UserID         string `json:"user_id" gorm:"size:255;index"`
	UserEmail      string `json:"user_email" gorm:"size:320"`
	UserAdmin      bool   `json:"user_admin"`
	OrganizationID string `json:"organization_id" gorm:"size:255"`
	TokenType      string `json:"token_type" gorm:"size:64"`
	APIKeyType     string `json:"api_key_type" gorm:"size:64"`

	// CredentialSource names where the credential came from (session, bearer,
	// api-key, cookie, query, socket). CredentialFingerprint is the first 12
	// hex characters of sha256(credential) - enough to correlate a stolen key
	// with requests without storing the key.
	CredentialSource      string `json:"credential_source" gorm:"size:64"`
	CredentialFingerprint string `json:"credential_fingerprint" gorm:"size:64"`

	// RemoteAddr is the TCP peer. The proxy headers below are recorded as the
	// client sent them and are NOT trusted (they are forgeable).
	RemoteAddr     string `json:"remote_addr" gorm:"size:255;index"`
	CFConnectingIP string `json:"cf_connecting_ip,omitempty" gorm:"size:64"`
	XForwardedFor  string `json:"x_forwarded_for,omitempty" gorm:"size:1024"`
	XRealIP        string `json:"x_real_ip,omitempty" gorm:"size:64"`
	UserAgent      string `json:"user_agent,omitempty" gorm:"size:1024"`
	RequestID      string `json:"request_id" gorm:"size:64"`

	ContentType   string `json:"content_type,omitempty" gorm:"size:255"`
	BodySize      int64  `json:"body_size"`
	Body          string `json:"body,omitempty" gorm:"type:text"`
	BodyTruncated bool   `json:"body_truncated"`
}

func (RequestAuditEvent) TableName() string { return "request_audit_events" }

type RequestAuditEventFilters struct {
	UserID     string     `json:"user_id,omitempty"`
	RemoteAddr string     `json:"remote_addr,omitempty"`
	PathPrefix string     `json:"path,omitempty"`
	Method     string     `json:"method,omitempty"`
	Status     *int       `json:"status,omitempty"`
	Since      *time.Time `json:"since,omitempty"`
	Until      *time.Time `json:"until,omitempty"`
	Limit      int        `json:"limit,omitempty"`
	Offset     int        `json:"offset,omitempty"`
}

// RequestAuditEventResponse is newest-first.
type RequestAuditEventResponse struct {
	Events []*RequestAuditEvent `json:"events"`
	Total  int64                `json:"total"`
	Limit  int                  `json:"limit"`
	Offset int                  `json:"offset"`
}
