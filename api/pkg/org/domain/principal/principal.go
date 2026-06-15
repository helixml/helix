// Package principal owns the typed sender value object that every
// Event and Message carries to identify "who originated this." The
// org graph distinguishes three sender kinds today:
//
//   - KindWorker: a known internal Worker (the ID is a orgchart.WorkerID such
//     as "w-alice"). Used for events Workers themselves publish via
//     the publish/dm tools, and for activation transcript segments.
//   - KindTransport: an external sender that arrived through an
//     inbound transport (email, GitHub webhook, …) without resolving
//     to a Worker. The ID is the transport-native identifier
//     ("alice@example.com", "U0123ABCD", "+15551234567"); the Stream
//     context plus the value shape disambiguates which transport.
//   - KindHuman: an authenticated human reaching in without being a
//     Worker. Reserved for the operator-typed chat surface and any
//     future per-user sign-in path. The ID is whatever identifier
//     the host hands the bridge (Helix user_id, audit handle, …).
//
// Principal is the typed replacement for the loose pair
// (Event.Source = orgchart.WorkerID, Message.From = string) the alpha
// currently carries. Each transport translates provider-native sender
// → Principal at the boundary; everything downstream sees one type.
//
// The zero value represents "no principal" — system-emitted events,
// transport inbound where the sender can't be resolved, the
// dispatcher's own activation triggers. IsZero() reports it; Validate
// accepts it. Constructors (NewWorker, NewTransport, NewHuman) refuse
// to mint a kind-less Principal, so the zero value is the only
// kind-less Principal that can exist in the system.
package principal

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Kind enumerates the sender categories the org graph distinguishes.
type Kind string

const (
	// KindWorker — sender is a known internal Worker.
	KindWorker Kind = "worker"

	// KindTransport — sender is an external transport-native
	// identifier (email address, Slack ID, phone number, …).
	KindTransport Kind = "transport"

	// KindHuman — sender is an authenticated human reaching in
	// without being a Worker (operator-typed chat surface, future
	// per-user sign-in).
	KindHuman Kind = "human"
)

// Principal is one typed sender. See package doc for the kind
// semantics.
type Principal struct {
	Kind Kind   `json:"kind"`
	ID   string `json:"id"`
}

// NewWorker stamps a Principal of Kind=Worker. Empty workerID
// produces an invalid Principal (caller should validate before use).
func NewWorker(workerID string) Principal {
	return Principal{Kind: KindWorker, ID: workerID}
}

// NewTransport stamps a Principal of Kind=Transport.
func NewTransport(id string) Principal {
	return Principal{Kind: KindTransport, ID: id}
}

// NewHuman stamps a Principal of Kind=Human.
func NewHuman(id string) Principal {
	return Principal{Kind: KindHuman, ID: id}
}

// IsZero reports the "no principal" zero value — system-emitted
// events, transport inbound without a resolved sender.
func (p Principal) IsZero() bool {
	return p == Principal{}
}

// Validate enforces the structural invariant: either the zero value
// (no principal, both fields empty) or a known Kind with a non-empty
// ID. A populated Kind with empty ID — or vice versa — is rejected;
// such a value would silently fail downstream joins / filters that
// key off the pair.
func (p Principal) Validate() error {
	if p.IsZero() {
		return nil
	}
	switch p.Kind {
	case KindWorker, KindTransport, KindHuman:
	case "":
		return errors.New("principal: ID set but Kind missing")
	default:
		return fmt.Errorf("principal: unknown Kind %q", p.Kind)
	}
	if p.ID == "" {
		return fmt.Errorf("principal: Kind %q set but ID missing", p.Kind)
	}
	return nil
}

// JSON encoding uses Go's default struct marshaller via the `kind`
// and `id` field tags. The on-wire shape
// ({"kind":"worker","id":"w-alice"}) is pinned by principal_test.go
// so the transitional period (Event.Source / Message.From still raw
// strings) can later layer the typed Principal onto the same JSON
// column without a schema break.

// Unused import guard — json is referenced only in the test today.
var _ = json.Marshal
