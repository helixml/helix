package streaming

import (
	"context"
	"encoding/json"
	"errors"
)

// Inbound is the domain port for a Topic's inbound transport: registering
// and inspecting the provider-side hook that delivers events INTO the
// Topic (a GitHub repo webhook, an inbound email address, …). One
// implementation per transport Kind lives in that transport's
// infrastructure package; the application layer dispatches on the Topic's
// Kind. Outbound is the send-side mirror.
type Inbound interface {
	// Install registers (or adopts) the inbound hook and returns its
	// coordinates plus the transport-config JSON to persist on the Topic
	// (provider hook id/url merged in). A nil Config means "nothing to
	// persist". Failures should be a *Failure so the adapter can map the
	// HTTP status.
	Install(ctx context.Context, orgID string, topic Topic) (InstallResult, error)
	// Status reports the live hook state. Read-only; "can't tell"
	// conditions degrade to InboundState{State:"unknown"} rather than an
	// error.
	Status(ctx context.Context, orgID string, topic Topic) (InboundState, error)
}

// InstallResult is the result of a successful Inbound.Install.
type InstallResult struct {
	WebhookID      int64
	WebhookHTMLURL string
	PayloadURL     string
	// Config is the transport config to persist on the Topic, with the
	// hook coordinates merged in. nil = nothing to persist.
	Config json.RawMessage
	// Notice is a non-fatal, human-facing message the caller surfaces to
	// the user when Install succeeded as far as it could but couldn't
	// fully self-provision — e.g. Slack can self-join a PUBLIC channel,
	// but a PRIVATE channel must be joined by a human (`/invite @bot`),
	// which no API can do remotely. Empty = nothing to say. Distinct from
	// an error: the Topic is valid and usable once the user acts on it.
	Notice string
}

// AutoInstaller is an optional extension a streaming.Inbound provisioner
// may implement to request that its Install run automatically when a
// Topic of its Kind is created — e.g. Slack joining the bound channel so
// the operator needs no separate "install" click. Provisioners that
// drive installation through their own explicit step (GitHub's per-repo
// webhook install) simply don't implement it, and topic creation leaves
// them untouched. The decision lives here in the domain, not in any UI.
type AutoInstaller interface {
	AutoInstallOnCreate() bool
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
	// FailNotFound — the topic doesn't exist (404).
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
// when the Topic's transport Kind has no registered Inbound provisioner.
var ErrInboundUnsupported = errors.New("transport does not support inbound webhook provisioning")
