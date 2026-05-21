// Package stream owns the Stream concept: a named source of events
// that Workers publish to and subscribe to. Today this package only
// carries the ID type; the Stream struct, Subscription, and the
// per-Stream pub/sub machinery follow in later migrations.
package stream

// ID identifies a Stream. Convention: `s-<slug>` (e.g. `s-general`,
// `s-inbox`). Activation streams use the deterministic pattern
// `s-activations-<workerID>`.
type ID string
