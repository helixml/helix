// Package transport owns the per-Stream I/O contract — the Kind enum,
// the canonical Transport struct, the per-Kind Config types, and
// validation. Every Stream has one Transport; the default (KindLocal)
// keeps events inside the system, other Kinds compose external I/O
// over the same in-process store.
//
// Canonical home, lifted from helix-org/domain/transport.go in B1.
// Names lose the redundant "Transport" prefix that the old package
// needed: `transport.Kind`, `transport.KindEmail`, `transport.WebhookConfig`.
// The `Transport` struct itself keeps its name because callers read
// `transport.Transport` as one phrase. See ADR-0001 §1 (Stream
// canonical) and the B1 entry in
// helix-org/design/2026-05-21-redesign/08-migration-plan.md.
//
// Shape (per CLAUDE.md "No discriminator switches with branching
// logic" + "One file per variant"):
//
//   - This file owns the umbrella: Kind enum, Transport struct,
//     Strategy + Config interfaces, the strategies map, and the
//     per-Kind typed accessors on Transport.
//   - Each Kind lives in its own sibling file (local.go, webhook.go,
//     email.go, github.go) holding its Config type, that Config's
//     Validate() rules, its Strategy implementation, and its parser.
//   - Adding a new Kind = a new file + a new constant + one map entry.
//     No edits to Transport.Validate.
package transport

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Kind names the implementation that owns a Stream's I/O. Constants
// are defined alongside their Strategy in each Kind's own file.
type Kind string

// Strategy is the contract every Kind satisfies: take the raw config
// blob from Transport.Config and produce a typed Config value. The
// Strategy itself is stateless — one zero-value instance per Kind
// lives in the strategies map.
type Strategy interface {
	ParseConfig(json.RawMessage) (Config, error)
}

// Config is the parsed shape of a Transport's per-Kind configuration.
// Each Kind's Strategy returns a concrete Config type whose Validate
// method enforces the rules specific to that Kind.
//
// Callers needing strongly-typed access use the per-Kind accessors on
// Transport (e.g. Transport.WebhookConfig()) rather than asserting on
// the Config interface — those accessors return concrete types and
// give the caller better ergonomics than `c.(WebhookConfig)`.
type Config interface {
	Validate() error
}

// kindOrder pins the canonical display order of Kinds: Local first
// (it's the default), then the others in the order they were added to
// the system. The order is part of the public surface — it shows up
// in JSON Schema enum lists, in "(valid: …)" error messages, and in
// the MCP create_stream tool description. Tests pin it explicitly.
var kindOrder = []Kind{KindLocal, KindWebhook, KindEmail, KindGitHub}

// strategies registers every known Kind's Strategy. Adding a new Kind
// means adding a new file in this package that defines its Kind
// constant, its Config type with Validate(), and its Strategy
// implementation — plus one entry here AND in kindOrder. Validate()
// itself does not change.
var strategies = map[Kind]Strategy{
	KindLocal:   local{},
	KindWebhook: webhook{},
	KindEmail:   email{},
	KindGitHub:  github{},
}

// KindValues lists every registered Kind in canonical display order
// (see kindOrder). Source of truth for the JSON Schema `enum`
// constraint surfaced through MCP and for listing valid options in
// validation errors. Returns a copy so callers cannot mutate the
// canonical order.
func KindValues() []Kind {
	out := make([]Kind, len(kindOrder))
	copy(out, kindOrder)
	return out
}

// Transport describes how events on a Stream move to and from the
// outside world. Internal Streams use KindLocal — that is still a
// transport, just one whose endpoints are both inside the system.
//
// Config is opaque per-Kind JSON; each Kind's Strategy decides how to
// parse it. Callers that need the typed config call the matching
// accessor — Transport.WebhookConfig(), Transport.EmailConfig(), etc.
// — which live in each Kind's own file in this package, not here. The
// umbrella stays Kind-agnostic.
type Transport struct {
	Kind   Kind
	Config json.RawMessage
}

// Validate dispatches to the Kind's Strategy. There is intentionally
// no switch on t.Kind here — adding a new Kind must not require
// editing this function (Open/Closed).
func (t Transport) Validate() error {
	if t.Kind == "" {
		return errors.New("transport kind is empty")
	}
	s, ok := strategies[t.Kind]
	if !ok {
		return fmt.Errorf("unknown transport kind %q (valid: %s)", t.Kind, quotedKinds(KindValues()))
	}
	c, err := s.ParseConfig(t.Config)
	if err != nil {
		return err
	}
	return c.Validate()
}

// quotedKinds renders a slice of Kind values as a comma-separated list
// of quoted strings, e.g. `"local", "webhook"`. Used in the
// unknown-kind error message; pinned in transport_test.go.
//
// Inlined from helix-org/orgchart.QuotedList to keep this package
// self-contained. When more types in api/pkg/org/ start duplicating
// this helper, factor it out into a shared internal location (not
// before).
func quotedKinds(vals []Kind) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = strconv.Quote(string(v))
	}
	return strings.Join(parts, ", ")
}
