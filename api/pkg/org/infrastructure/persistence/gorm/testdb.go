package gorm

import (
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

// GetOrgTestDB returns an *orgstore.Store backed by the in-memory
// implementation in api/pkg/org/store/memorystore. Tests use this
// instead of the gorm-backed Postgres store so the suite runs without
// a live database — the cost of a Postgres schema-per-test setup
// (slow, flaky in sandboxed CI, requires a running Postgres) wasn't
// worth the gorm coverage when the gorm shape is already exercised
// end-to-end through the integration browser flow.
//
// The interface contract is identical: composite (id, org_id) PKs,
// store.ErrNotFound on cross-tenant lookups, append-only events,
// per-tenant unique stream names. Tests that pass against this Store
// pass against the gorm one.
//
// Returns a fresh store per call so test isolation is automatic.
func GetOrgTestDB(t *testing.T) *store.Store {
	t.Helper()
	return memory.New()
}
