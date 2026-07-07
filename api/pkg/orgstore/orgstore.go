// Package orgstore is Helix's organization/tenant subsystem (Organizations,
// Teams, Memberships, Roles, Invitations, AccessGrants + RBAC), extracted so it
// can operate on a plain *gorm.DB and be imported by other services (HelixOS)
// without pulling in the full *PostgresStore.
//
// It is the single source of truth: Helix's *PostgresStore embeds *Store and
// delegates its org methods here, and Helix's server authorizer delegates to
// the Authorizer here. Downstream consumers vendor this package via normal Go
// module vendoring, so upstream fixes flow through `go get`/`go mod vendor`.
//
// This is distinct from api/pkg/org, which is the agent org-graph runtime.
package orgstore

import (
	"errors"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

// ErrNotFound is returned when a queried org-subsystem row does not exist.
// Helix's store package aliases its own ErrNotFound to this value so existing
// error comparisons keep working across the two packages.
var ErrNotFound = errors.New("not found")

// Store is the org subsystem's data-access layer over a *gorm.DB.
type Store struct {
	gdb *gorm.DB
}

// New returns a Store backed by the given gorm DB. The connection lifecycle is
// owned by the caller.
func New(gdb *gorm.DB) *Store { return &Store{gdb: gdb} }

// GormDB exposes the underlying DB for callers that need it.
func (s *Store) GormDB() *gorm.DB { return s.gdb }

// Models returns every org-subsystem table for AutoMigrate.
func Models() []interface{} {
	return []interface{}{
		&types.Organization{},
		&types.OrganizationMembership{},
		&types.Team{},
		&types.TeamMembership{},
		&types.Role{},
		&types.OrganizationInvitation{},
		&types.AccessGrant{},
		&types.AccessGrantRoleBinding{},
	}
}

// Migrate creates the org-subsystem tables. Helix migrates these via its own
// aggregate AutoMigrate; standalone consumers (HelixOS) call this.
func (s *Store) Migrate() error {
	return s.gdb.AutoMigrate(Models()...)
}
