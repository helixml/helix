package transport

import "encoding/json"

// KindLocal is the default: SQLite + broadcaster + dispatcher. No
// external I/O.
const KindLocal Kind = "local"

// LocalTransport is the zero-config default Transport. Treat the
// returned value as immutable.
func LocalTransport() Transport {
	return Transport{Kind: KindLocal}
}

// LocalConfig is the empty config for KindLocal. The local transport
// has nothing to configure; any config blob is ignored (validates the
// historical "junk config tolerated" behaviour pinned in
// transport_test.go).
type LocalConfig struct{}

// Validate always succeeds — KindLocal has no rules to enforce.
func (LocalConfig) Validate() error { return nil }

// local is the Strategy for KindLocal.
type local struct{}

// ParseConfig ignores the raw blob and returns an empty LocalConfig.
// The local transport accepts any input (or none) as valid.
func (local) ParseConfig(_ json.RawMessage) (Config, error) {
	return LocalConfig{}, nil
}
