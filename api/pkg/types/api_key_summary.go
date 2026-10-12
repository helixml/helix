package types

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

// apiKeyIDPrefix marks a derived API key ID. Key secrets start with
// APIKeyPrefix, so the two can never be confused.
const apiKeyIDPrefix = "key_"

// apiKeyVisiblePrefixLen is how much of the secret a listing may show to tell
// keys apart: APIKeyPrefix plus four random characters (24 bits of a 256-bit
// secret), the same amount the UI used to show when masking.
const apiKeyVisiblePrefixLen = len(APIKeyPrefix) + 4

// APIKeySummary is an API key without its secret. Listings that can include
// keys the caller does not hold the secret for (org owners listing members'
// keys, scoped keys listing their owner's keys) return this, never ApiKey.
type APIKeySummary struct {
	// ID is a stable, non-secret handle for the key, derived from a hash of
	// the secret. Use it to delete a key you did not create.
	ID             string     `json:"id"`
	KeyPrefix      string     `json:"key_prefix"`
	Name           string     `json:"name"`
	Owner          string     `json:"owner"`
	OwnerType      OwnerType  `json:"owner_type"`
	Created        time.Time  `json:"created"`
	Type           APIKeyType `json:"type"`
	AppID          string     `json:"app_id,omitempty"`
	OrganizationID string     `json:"organization_id,omitempty"`
	ProjectID      string     `json:"project_id,omitempty"`
}

// APIKeyID derives the non-secret ID for an API key secret. The api_keys table
// is keyed by the secret itself, so the ID is computed rather than stored; a
// SHA-256 prefix cannot be reversed into the secret.
func APIKeyID(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return apiKeyIDPrefix + hex.EncodeToString(sum[:12])
}

// IsAPIKeyID reports whether s is shaped like an ID from APIKeyID.
func IsAPIKeyID(s string) bool {
	return strings.HasPrefix(s, apiKeyIDPrefix) && len(s) == len(apiKeyIDPrefix)+24
}

// Summary returns the key's metadata without its secret.
func (k *ApiKey) Summary() *APIKeySummary {
	// Only show a prefix of a long secret; a short one would be mostly revealed.
	prefix := ""
	if len(k.Key) > 2*apiKeyVisiblePrefixLen {
		prefix = k.Key[:apiKeyVisiblePrefixLen]
	}
	s := &APIKeySummary{
		ID:             APIKeyID(k.Key),
		KeyPrefix:      prefix,
		Name:           k.Name,
		Owner:          k.Owner,
		OwnerType:      k.OwnerType,
		Created:        k.Created,
		Type:           k.Type,
		OrganizationID: k.OrganizationID,
		ProjectID:      k.ProjectID,
	}
	if k.AppID != nil && k.AppID.Valid {
		s.AppID = k.AppID.String
	}
	return s
}
