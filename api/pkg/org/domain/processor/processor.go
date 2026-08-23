// Package processor owns the Processor aggregate — the node that sits
// on the edge between Topics and reshapes or routes the Messages that
// flow across it. A Processor reads one input Topic and writes one or
// more output Topics; the logic in between is a Kind.
//
// Shape mirrors api/pkg/org/domain/transport (per CLAUDE.md "No
// discriminator switches with branching logic" + "One file per
// variant"):
//
//   - This file owns the umbrella: the Kind enum, the Processor and
//     Output value types, the Strategy + Config interfaces, the
//     strategies map, KindValues, and the Kind-agnostic Validate /
//     Process dispatch.
//   - Each Kind lives in its own sibling file (template.go,
//     truncate.go, filter.go, js.go) holding its Config type, that
//     Config's Validate rules, and its Process implementation.
//   - Adding a new Kind = a new file + a new constant + one map entry
//     (in strategies AND kindOrder). No edits to Processor.Validate.
//
// Most transforms are pure domain functions (text/template, byte caps)
// with no I/O. The js Kind is intentionally side-effecting: scripts may
// call an embedded HTTP client, so Process carries a context for
// cancellation and timeouts. Contrast transports (sources/sinks), which
// do real network I/O and live in infrastructure/.
package processor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// ProcessorID identifies a Processor. Convention: `p-<slug>` (e.g.
// `p-format-inbox`). Like the other org IDs it is a string alias, not
// a distinct named type (see orgchart/ids.go for the rationale).
type ProcessorID = string

// LegacyOutputID deterministically identifies a pre-ID branch by the
// immutable event stream it writes to. It is used only by the upgrade
// migration that gave existing branches durable identities.
func LegacyOutputID(streamID streaming.StreamID) string { return "po-topic-" + string(streamID) }

// Kind names the implementation that owns a Processor's behaviour.
// Constants are defined alongside their Config in each Kind's own file
// (KindTemplate in template.go, KindTruncate in truncate.go, …).
type Kind string

// Output is one durable branch of a Processor: a stable identity Workers
// attach to, plus — for the filter Kind — the predicate that selects it.
//
//   - ID is the branch's durable identity. It is what a Worker attachment
//     and a downstream Processor input name, and it never changes once
//     allocated.
//   - StreamID is the append-only event stream this branch writes its
//     Results to. It is a persistence key only: it is what makes the
//     branch's history survive the Topic cutover, since a converted branch
//     keeps the stream its old output Topic used. Never routed on.
//   - Match is the routing predicate. Empty means unconditional — a
//     transform always passes; a filter Output with an empty Match is
//     a default/else branch. Non-empty Match is only meaningful to the
//     filter Kind (see filter.go); transform Kinds require it empty.
//   - Label is a human-facing name for the branch, shown on the chart.
//
// StreamID keeps the `TopicID` JSON key so the outputs blob persisted
// before the cutover still decodes.
type Output struct {
	ID       string             `json:"id,omitempty"`
	StreamID streaming.StreamID `json:"TopicID"`
	Match    string
	Label    string
	// ManagedFor names the orgchart.NodeID this route was
	// auto-generated for by a reconciler (e.g. the Slack auto-router).
	// Empty means a human-authored ("manual") route: reconcilers must
	// never rewrite or garbage-collect it. Non-empty makes the route
	// reconciler-owned, keyed by that Worker — present-while-the-Worker-
	// exists, removed when the Worker is gone. Stored in the Outputs JSON
	// blob, so it needs no schema migration.
	ManagedFor string `json:",omitempty"`
}

// Source returns the terminal reference that names this branch.
func (p Processor) Source(o Output) eventsource.SourceRef {
	return eventsource.ProcessorOutput(p.ID, o.ID)
}

// Output returns the branch with the given durable id.
func (p Processor) Output(id string) (Output, bool) {
	for _, o := range p.Outputs {
		if o.ID == id {
			return o, true
		}
	}
	return Output{}, false
}

// Result is one (branch, Message) pair produced by Process. A transform
// returns exactly one; a filter returns one per Output whose predicate
// matched (zero = a drop, N = a content-based router). Output carries the
// branch's durable id and stream, so the runner can both route and persist
// without a second lookup.
type Result struct {
	Output  Output
	Message streaming.Message
}

// Strategy takes the raw config blob from Processor.Config and produces
// a typed Config. The Strategy is stateless — one zero-value instance
// per Kind lives in the strategies map.
type Strategy interface {
	ParseConfig(json.RawMessage) (Config, error)
}

// Config is the parsed, Kind-specific configuration. Validate enforces
// the Kind's rules against the Processor's Outputs; Process turns one
// input Message into zero or more Results. This single interface serves
// transform (always 1 result), filter (0/1), router (N), and js (0..N
// with optional HTTP side effects) — the runner just publishes whatever
// comes back. ctx is for cancellation/timeouts (used by KindJS HTTP);
// pure kinds ignore it.
type Config interface {
	Validate(outputs []Output) error
	Process(ctx context.Context, in streaming.Message, outputs []Output) ([]Result, error)
}

// kindOrder pins the canonical display order of Kinds, mirroring
// transport.kindOrder. It is part of the public surface — JSON Schema
// enum lists and "(valid: …)" error messages read from it. Tests pin
// it explicitly.
var kindOrder = []Kind{KindTemplate, KindTruncate, KindFilter, KindJS}

// strategies registers every known Kind's Strategy. Adding a new Kind
// means a new file defining its Kind constant + Config, plus one entry
// here AND in kindOrder. Processor.Validate does not change.
var strategies = map[Kind]Strategy{
	KindTemplate: template{},
	KindTruncate: truncate{},
	KindFilter:   filter{},
	KindJS:       js{},
}

// KindValues lists every registered Kind in canonical display order.
// Returns a copy so callers cannot mutate the canonical order.
func KindValues() []Kind {
	out := make([]Kind, len(kindOrder))
	copy(out, kindOrder)
	return out
}

// Processor is a node that reads its InputSource and writes its
// Outputs, applying its Kind's logic to each Message in between.
//
// InputSource is a terminal reference: exactly one Trigger, or exactly one
// durable output branch of another Processor. A zero InputSource means the
// Processor is unwired — valid but inert.
//
// CreatedBy is an orgchart.NodeID stored as a plain string — a
// cosmetic anchor for the chart, exactly like Topic.CreatedBy; the
// processor aggregate does not import orgchart.
type Processor struct {
	ID             ProcessorID
	OrganizationID string
	Name           string
	InputSource    eventsource.SourceRef
	Outputs        []Output
	Kind           Kind
	Config         json.RawMessage
	CreatedBy      string // orgchart.NodeID, or SystemActor for automation
	CreatedAt      time.Time
}

// SystemActor is the CreatedBy value automation stamps on the records it
// owns (the Slack auto-router) in place of a human Worker id. It is a
// sentinel orgchart.NodeID — not a real Worker — so the chart leaves it
// unanchored.
const SystemActor = "helix"

// Automated reports whether this Processor was created by Helix automation
// (the Slack auto-router) rather than by a human — i.e. CreatedBy is the
// SystemActor sentinel. The route reconciler and the post-routing hook key
// on it; the API surfaces it so the UI can show the thread-follow toggle.
func (p Processor) Automated() bool { return p.CreatedBy == SystemActor }

// NewProcessor validates and constructs a Processor. orgID, id, name and
// at least one Output are required; the Config is validated against the
// Kind's rules. input may be zero (unwired). createdBy is optional — a
// cosmetic chart anchor.
func NewProcessor(id ProcessorID, name string, input eventsource.SourceRef, kind Kind, config json.RawMessage, outputs []Output, createdBy string, createdAt time.Time, orgID string) (Processor, error) {
	p := Processor{
		ID:             id,
		OrganizationID: orgID,
		Name:           name,
		InputSource:    input,
		Outputs:        outputs,
		Kind:           kind,
		Config:         config,
		CreatedBy:      createdBy,
		CreatedAt:      createdAt.UTC(),
	}
	if createdAt.IsZero() {
		return Processor{}, errors.New("processor createdAt is zero")
	}
	if err := p.Validate(); err != nil {
		return Processor{}, err
	}
	return p, nil
}

// Validate dispatches to the Kind's Strategy. There is intentionally no
// switch on p.Kind here — adding a new Kind must not require editing
// this function (Open/Closed), exactly as transport.Transport.Validate.
func (p Processor) Validate() error {
	if p.ID == "" {
		return errors.New("processor id is empty")
	}

	if p.OrganizationID == "" {
		return errors.New("processor orgID is empty")
	}

	if p.Name == "" {
		return errors.New("processor name is empty")
	}

	// A zero InputSource is allowed: a processor with no input is valid
	// but inert — it sits on the chart unwired until a Trigger (or another
	// processor's output branch) is connected to its IN port. Deleting
	// the input edge clears it back to this state.
	if !p.InputSource.Zero() {
		if err := p.InputSource.Validate(); err != nil {
			return fmt.Errorf("processor input: %w", err)
		}
		if p.InputSource.Kind == eventsource.KindProcessorOutput && p.InputSource.ProcessorID == p.ID {
			return errors.New("processor input is its own output")
		}
	}
	if len(p.Outputs) == 0 {
		return errors.New("processor has no outputs")
	}
	seenOutputIDs := make(map[string]struct{}, len(p.Outputs))
	for i, o := range p.Outputs {
		if o.ID == "" {
			return fmt.Errorf("processor output %d has empty id", i)
		}
		if o.StreamID == "" {
			return fmt.Errorf("processor output %d has empty stream id", i)
		}
		if _, exists := seenOutputIDs[o.ID]; exists {
			return fmt.Errorf("processor output id %q is duplicated", o.ID)
		}
		seenOutputIDs[o.ID] = struct{}{}
	}
	if p.Kind == "" {
		return errors.New("processor kind is empty")
	}
	s, ok := strategies[p.Kind]
	if !ok {
		return fmt.Errorf("unknown processor kind %q (valid: %s)", p.Kind, quotedKinds(KindValues()))
	}
	c, err := s.ParseConfig(p.Config)
	if err != nil {
		return err
	}
	return c.Validate(p.Outputs)
}

// Process applies the Processor's Kind to one input Message, returning
// zero or more Results. Most kinds are pure (no I/O); KindJS may issue
// HTTP from the script. The caller (the execution runner) publishes
// each Result. ctx is threaded for cancellation and HTTP timeouts.
func (p Processor) Process(ctx context.Context, in streaming.Message) ([]Result, error) {
	s, ok := strategies[p.Kind]
	if !ok {
		return nil, fmt.Errorf("unknown processor kind %q", p.Kind)
	}

	c, err := s.ParseConfig(p.Config)
	if err != nil {
		return nil, err
	}

	return c.Process(ctx, in, p.Outputs)
}

// quotedKinds renders a slice of Kind values as a comma-separated list
// of quoted strings, e.g. `"template", "truncate"`. Mirrors
// transport.quotedKinds; pinned in processor_test.go.
func quotedKinds(vals []Kind) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = strconv.Quote(string(v))
	}
	return strings.Join(parts, ", ")
}
