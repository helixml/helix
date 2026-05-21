// Package worker owns the Worker concept: a person or AI agent that
// occupies one or more Positions and holds Grants. Today this package
// only carries the ID type; the Worker struct and its constructors
// follow in a later migration that lifts helix-org/domain/worker.go
// into this canonical home.
package worker

// ID identifies a Worker. Convention: `w-<lowercase-firstname>`
// (e.g. `w-mark`, `w-priya`) — see the hire_worker tool description
// for the rule. Stored as a string so external systems (logs, URLs,
// JSON) can carry it unchanged.
type ID string
