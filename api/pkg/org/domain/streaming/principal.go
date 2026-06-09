package streaming

import (
	"encoding/json"
	"errors"
	"fmt"
)

// PrincipalKind enumerates the sender categories the org graph
// distinguishes. The set is intentionally closed: every transport at
// the boundary resolves its raw sender into one of these kinds.
type PrincipalKind string

const (
	// PrincipalKindWorker — sender is a known internal Worker.
	PrincipalKindWorker PrincipalKind = "worker"

	// PrincipalKindTransport — sender is an external transport-native
	// identifier (email address, Slack ID, phone number, …).
	PrincipalKindTransport PrincipalKind = "transport"

	// PrincipalKindHuman — sender is an authenticated human reaching
	// in without being a Worker (operator-typed chat surface, future
	// per-user sign-in).
	PrincipalKindHuman PrincipalKind = "human"
)

// Principal is one typed sender. See package doc for the kind
// semantics. Zero-Principal represents "no principal" — system
// events, transport inbound without a resolved sender, the
// dispatcher's own activation triggers.
type Principal struct {
	Kind PrincipalKind `json:"kind"`
	ID   string        `json:"id"`
}

// NewPrincipalWorker stamps a Principal of Kind=Worker. Empty
// workerID produces an invalid Principal (caller should validate
// before use).
func NewPrincipalWorker(workerID string) Principal {
	return Principal{Kind: PrincipalKindWorker, ID: workerID}
}

// NewPrincipalTransport stamps a Principal of Kind=Transport.
func NewPrincipalTransport(id string) Principal {
	return Principal{Kind: PrincipalKindTransport, ID: id}
}

// NewPrincipalHuman stamps a Principal of Kind=Human.
func NewPrincipalHuman(id string) Principal {
	return Principal{Kind: PrincipalKindHuman, ID: id}
}

// IsZero reports the "no principal" zero value.
func (p Principal) IsZero() bool {
	return p == Principal{}
}

// Validate enforces the structural invariant: either the zero value
// or a known Kind with a non-empty ID.
func (p Principal) Validate() error {
	if p.IsZero() {
		return nil
	}
	switch p.Kind {
	case PrincipalKindWorker, PrincipalKindTransport, PrincipalKindHuman:
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

// Unused import guard — json is referenced only in the test today.
var _ = json.Marshal
