// Package event owns the Event concept: a single entry on a Stream's
// log. Today this package only carries the ID type; the Event struct
// follows in a later migration.
package event

// ID identifies an Event. Convention: `e-<uuid>`.
type ID string
