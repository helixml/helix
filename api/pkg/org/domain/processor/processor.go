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
//     truncate.go, filter.go) holding its Config type, that Config's
//     Validate rules, and its Process implementation.
//   - Adding a new Kind = a new file + a new constant + one map entry
//     (in strategies AND kindOrder). No edits to Processor.Validate.
//
// Transforms are pure domain functions (text/template, byte caps) with
// no I/O, so they live here in the domain layer and unit-test with no
// store or HTTP. Contrast transports, which do real network I/O and
// live in infrastructure/.
package processor

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// ProcessorID identifies a Processor. Convention: `p-<slug>` (e.g.
// `p-format-inbox`). Like the other org IDs it is a string alias, not
// a distinct named type (see orgchart/ids.go for the rationale).
type ProcessorID = string

// Kind names the implementation that owns a Processor's behaviour.
// Constants are defined alongside their Config in each Kind's own file
// (KindTemplate in template.go, KindTruncate in truncate.go, …).
type Kind string

// Output is one destination of a Processor: an auto-provisioned local
// Topic plus, for the filter Kind, the predicate that selects it.
//
//   - TopicID is the output Topic the Result is published to.
//   - Match is the routing predicate. Empty means unconditional — a
//     transform always passes; a filter Output with an empty Match is
//     a default/else branch. Non-empty Match is only meaningful to the
//     filter Kind (see filter.go); transform Kinds require it empty.
//   - Label is a human-facing name for the branch, shown on the chart.
type Output struct {
	TopicID streaming.TopicID
	Match   string
	Label   string
	// Owned is true when this Processor auto-provisioned the output
	// Topic (the default) and is therefore responsible for tearing it
	// down on delete. False when the branch points at a pre-existing,
	// shared Topic (explicit output) that outlives the Processor.
	Owned bool
}

// Result is one (output Topic, Message) pair produced by Process. A
// transform returns exactly one; a filter returns one per Output whose
// predicate matched (zero = a drop, N = a content-based router).
type Result struct {
	TopicID streaming.TopicID
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
// transform (always 1 result), filter (0/1), and router (N) — the
// runner just publishes whatever comes back.
type Config interface {
	Validate(outputs []Output) error
	Process(in streaming.Message, outputs []Output) ([]Result, error)
}

// kindOrder pins the canonical display order of Kinds, mirroring
// transport.kindOrder. It is part of the public surface — JSON Schema
// enum lists and "(valid: …)" error messages read from it. Tests pin
// it explicitly.
var kindOrder = []Kind{KindTemplate, KindTruncate, KindFilter}

// strategies registers every known Kind's Strategy. Adding a new Kind
// means a new file defining its Kind constant + Config, plus one entry
// here AND in kindOrder. Processor.Validate does not change.
var strategies = map[Kind]Strategy{
	KindTemplate: template{},
	KindTruncate: truncate{},
	KindFilter:   filter{},
}

// KindValues lists every registered Kind in canonical display order.
// Returns a copy so callers cannot mutate the canonical order.
func KindValues() []Kind {
	out := make([]Kind, len(kindOrder))
	copy(out, kindOrder)
	return out
}

// Processor is a node that reads its InputTopicID and writes its
// Outputs, applying its Kind's logic to each Message in between.
//
// CreatedBy is an orgchart.WorkerID stored as a plain string — a
// cosmetic anchor for the chart, exactly like Topic.CreatedBy; the
// processor aggregate does not import orgchart.
type Processor struct {
	ID             ProcessorID
	OrganizationID string
	Name           string
	InputTopicID   streaming.TopicID
	Outputs        []Output
	Kind           Kind
	Config         json.RawMessage
	CreatedBy      string // orgchart.WorkerID
	CreatedAt      time.Time
}

// NewProcessor validates and constructs a Processor. orgID, id, name,
// inputTopicID and at least one Output are required; the Config is
// validated against the Kind's rules. createdBy is optional (cosmetic
// anchor, like NewTopic).
func NewProcessor(id ProcessorID, name string, inputTopicID streaming.TopicID, kind Kind, config json.RawMessage, outputs []Output, createdBy string, createdAt time.Time, orgID string) (Processor, error) {
	p := Processor{
		ID:             id,
		OrganizationID: orgID,
		Name:           name,
		InputTopicID:   inputTopicID,
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
	if p.InputTopicID == "" {
		return errors.New("processor input topic is empty")
	}
	if len(p.Outputs) == 0 {
		return errors.New("processor has no outputs")
	}
	for i, o := range p.Outputs {
		if o.TopicID == "" {
			return fmt.Errorf("processor output %d has empty topic id", i)
		}
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
// zero or more Results. Pure — no I/O, no store. The caller (the
// execution runner) publishes each Result.
func (p Processor) Process(in streaming.Message) ([]Result, error) {
	s, ok := strategies[p.Kind]
	if !ok {
		return nil, fmt.Errorf("unknown processor kind %q", p.Kind)
	}
	c, err := s.ParseConfig(p.Config)
	if err != nil {
		return nil, err
	}
	return c.Process(in, p.Outputs)
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
