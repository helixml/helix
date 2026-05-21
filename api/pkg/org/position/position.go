// Package position owns the Position concept: a slot in the org tree
// that instantiates a Role. Today this package only carries the ID
// type; the Position struct follows in a later migration.
package position

// ID identifies a Position. Convention: `p-<slug>`; `p-root` is the
// conventional root Position created at bootstrap.
type ID string
