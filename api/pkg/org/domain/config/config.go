// Package config owns the Config entity — one operational-config row
// (key + opaque JSON value + audit metadata) persisted by the configs
// store. Distinct from application/configregistry, which is the
// runtime helper that registers schema, validates writes, and reads
// typed values through a Configs store using this entity as its row
// shape.
//
// Lifted from api/pkg/org/domain/config.go in the DDD restructure.
package config

import (
	"errors"
	"strings"
	"time"
)

// Config is one operational-config row: a key, an opaque JSON value,
// and an updated-at timestamp. Keys are flat dot-namespaced strings
// owned by subsystems (e.g. "claude.bin", "transport.postmark"). Values
// are stored as JSON strings — schema validation is the registry's
// concern, not the storage layer's.
type Config struct {
	OrganizationID string
	Key            string
	Value          string // JSON-encoded
	UpdatedAt      time.Time
}

// New validates and constructs a Config. orgID is required — every
// config row is tenant-scoped via the composite (key, org_id) PK.
func New(key, value string, updatedAt time.Time, orgID string) (Config, error) {
	if key == "" {
		return Config{}, errors.New("config key is empty")
	}
	if strings.ContainsAny(key, " \t\n") {
		return Config{}, errors.New("config key contains whitespace")
	}
	if strings.HasPrefix(key, ".") || strings.HasSuffix(key, ".") {
		return Config{}, errors.New("config key has leading or trailing dot")
	}
	if value == "" {
		return Config{}, errors.New("config value is empty")
	}
	if updatedAt.IsZero() {
		return Config{}, errors.New("config updatedAt is zero")
	}
	if orgID == "" {
		return Config{}, errors.New("config orgID is empty")
	}
	return Config{
		OrganizationID: orgID,
		Key:            key,
		Value:          value,
		UpdatedAt:      updatedAt.UTC(),
	}, nil
}
