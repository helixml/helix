// Package grant owns the Grant concept: an authorisation record that
// says a Worker holds a particular Tool. Today this package only
// carries the ID type; the Grant struct (currently
// helix-org/domain.ToolGrant) follows in a later migration.
package grant

// ID identifies a Grant. Convention: `g-<uuid>`.
type ID string
