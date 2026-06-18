package streaming

import (
	"context"
	"encoding/json"
	"errors"
)

// Inbound is the domain port for a Stream's inbound transport: registering
// and inspecting the provider-side hook that delivers events INTO the
// Stream (a GitHub repo webhook, an inbound email address, …). One
// implementation per transport Kind lives in that transport's
// infrastructure package; the application layer dispatches on the Stream's
// Kind. Outbound is the send-side mirror.
type Inbound interface {
	// Install registers (or adopts) the inbound hook and returns its
	// coordinates plus the transport-config JSON to persist on the Stream
	// (provider hook id/url merged in). A nil Config means "nothing to
	// persist". Failures should be a *Failure so the adapter can map the
	// HTTP status.
	Install(ctx context.Context, orgID string, stream Stream) (InstallResult, error)
	// Status reports the live hook state. Read-only; "can't tell"
	// conditions degrade to InboundState{State:"unknown"} rather than an
	// error.
	Status(ctx context.Context, orgID string, stream Stream) (InboundState, error)
}

// InstallResult is the result of a successful Inbound.Install.
type InstallResult struct {
	WebhookID      int64
	WebhookHTMLURL string
	PayloadURL     string
	// Config is the transport config to persist on the Stream, with the
	// hook coordinates merged in. nil = nothing to persist.
	Config json.RawMessage
}

// InboundState is the live hook state: State is "installed" | "missing"
// | "unknown"; Detail explains an "unknown".
type InboundState struct {
	State          string
	WebhookID      int64
	WebhookHTMLURL string
	Active         bool
	PayloadURL     string
	Detail         string
}

// FailKind categorises an inbound-provisioning failure so the interface
// adapter maps it to the right HTTP status without string-matching.
type FailKind int

const (
	// FailBadRequest — malformed input / unsupported transport (400).
	FailBadRequest FailKind = iota
	// FailPrecondition — a precondition isn't met: no public URL,
	// loopback URL, no token/credentials (412).
	FailPrecondition
	// FailUpstream — the provider (GitHub/…) rejected or errored (502).
	FailUpstream
	// FailInternal — our side failed (secret persist, marshal) (500).
	FailInternal
	// FailNotFound — the stream doesn't exist (404).
	FailNotFound
)

// Failure carries the failure category + operator-facing message.
// Provisioners (and the application seam) return it; the adapter switches
// on Kind.
type Failure struct {
	Kind FailKind
	Err  error
}

func (f *Failure) Error() string { return f.Err.Error() }
func (f *Failure) Unwrap() error { return f.Err }

// ErrInboundUnsupported is returned (wrapped in a FailBadRequest Failure)
// when the Stream's transport Kind has no registered Inbound provisioner.
var ErrInboundUnsupported = errors.New("transport does not support inbound webhook provisioning")
