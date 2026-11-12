package types

import "time"

// RequestAuditEvent is one captured HTTP request: who made it, from where,
// what they asked for, and how it ended.
//
// It is an append-only forensic record, not product data. Nothing here is
// allowed to contain a raw credential: CredentialFingerprint is a truncated
// HMAC and the request body and query string are redacted before they are
// stored.
type RequestAuditEvent struct {
	// ID is the primary key and half of the keyset pagination cursor.
	ID string `json:"id" gorm:"primaryKey;size:64;index:idx_request_audit_created_id,priority:2"`

	Created    time.Time `json:"created" gorm:"index:idx_request_audit_created_id,priority:1"`
	DurationMS int64     `json:"duration_ms"`

	Method        string `json:"method" gorm:"size:16;index"`
	RouteTemplate string `json:"route_template" gorm:"size:512"`
	Path          string `json:"path" gorm:"size:2048"`
	Query         string `json:"query" gorm:"size:4096"`
	Status        int    `json:"status" gorm:"index"`
	ResponseBytes int64  `json:"response_bytes"`

	UserID         string `json:"user_id" gorm:"size:255;index"`
	UserEmail      string `json:"user_email" gorm:"size:320"`
	UserAdmin      bool   `json:"user_admin"`
	OrganizationID string `json:"organization_id" gorm:"size:255"`
	TokenType      string `json:"token_type" gorm:"size:64"`
	APIKeyType     string `json:"api_key_type" gorm:"size:64"`

	// CredentialSource names where the credential came from (session, bearer,
	// api-key, cookie, query, socket). CredentialFingerprint is the first 12
	// hex characters of an HMAC-SHA256 of the credential, keyed from the
	// deployment's encryption key - enough to correlate a stolen key with
	// requests without storing the key or allowing an offline brute force.
	CredentialSource      string `json:"credential_source" gorm:"size:64"`
	CredentialFingerprint string `json:"credential_fingerprint" gorm:"size:64"`

	// RemoteAddr is the raw TCP peer as reported by net/http. RemoteIP and
	// RemotePort split it so the address is a usable filter (the raw value
	// includes the ephemeral port). ClientIP is filled only when the peer is
	// inside HELIX_AUDIT_LOG_TRUSTED_PROXIES; the proxy headers below are
	// otherwise recorded as the client sent them and are NOT trusted.
	RemoteAddr     string `json:"remote_addr" gorm:"size:255"`
	RemoteIP       string `json:"remote_ip" gorm:"size:128;index"`
	RemotePort     int    `json:"remote_port"`
	ClientIP       string `json:"client_ip,omitempty" gorm:"size:128;index"`
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

// RequestAuditEventFilters selects events for the admin read API. Pagination is
// keyset: pass back NextCursor from the previous response. Total is only
// computed when IncludeTotal is set, because a COUNT over a 180-day audit table
// is far more expensive than the page itself.
type RequestAuditEventFilters struct {
	UserID       string     `json:"user_id,omitempty"`
	RemoteIP     string     `json:"remote_ip,omitempty"`
	ClientIP     string     `json:"client_ip,omitempty"`
	PathPrefix   string     `json:"path,omitempty"`
	Method       string     `json:"method,omitempty"`
	Status       *int       `json:"status,omitempty"`
	Since        *time.Time `json:"since,omitempty"`
	Until        *time.Time `json:"until,omitempty"`
	Limit        int        `json:"limit,omitempty"`
	Cursor       string     `json:"cursor,omitempty"`
	IncludeTotal bool       `json:"include_total,omitempty"`
}

// RequestAuditEventResponse is newest-first. NextCursor is empty on the last
// page.
type RequestAuditEventResponse struct {
	Events     []*RequestAuditEvent `json:"events"`
	NextCursor string               `json:"next_cursor,omitempty"`
	Total      *int64               `json:"total,omitempty"`
	Limit      int                  `json:"limit"`
}
