package domain

import (
	"errors"
	"strings"
	"time"
)

// Config is one operational-config row: a key, an opaque JSON value,
// and audit metadata. Keys are flat dot-namespaced strings owned by
// subsystems (e.g. "claude.bin", "transport.postmark"). Values are
// stored as JSON strings — schema validation is the registry's
// concern, not the storage layer's.
//
// Operational config is set through the helix-org config CLI, never
// through MCP. See design/config.md for the access-pattern split.
type Config struct {
	Key       string
	Value     string // JSON-encoded
	UpdatedAt time.Time
	UpdatedBy WorkerID // empty until auth lands
}

// NewConfig validates and constructs a Config. Key must be non-empty,
// dot-namespaced (no spaces, no leading/trailing dots), and the value
// must be non-empty. Value JSON shape is the registry's responsibility.
func NewConfig(key, value string, updatedAt time.Time, updatedBy WorkerID) (Config, error) {
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
	return Config{
		Key:       key,
		Value:     value,
		UpdatedAt: updatedAt.UTC(),
		UpdatedBy: updatedBy,
	}, nil
}
