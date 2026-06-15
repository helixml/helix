package streams

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// InboundProvisioner registers / inspects the provider-side inbound hook
// for a Stream whose Transport needs external wiring (GitHub today, Slack
// next). One implementation per transport Kind lives in that transport's
// infrastructure package; the composition root registers them by Kind.
// This keeps the per-provider API specifics out of the application layer
// — the streams service just dispatches on the Stream's transport Kind.
type InboundProvisioner interface {
	// Install registers (or adopts) the inbound hook and returns its
	// coordinates plus the transport-config JSON to persist on the Stream
	// (provider hook id/url merged in). A nil Config means "nothing to
	// persist". Failures should be a *Failure so the adapter can map the
	// HTTP status.
	Install(ctx context.Context, orgID string, stream streaming.Stream) (Inbound, error)
	// Status reports the live hook state. Read-only; "can't tell"
	// conditions degrade to InboundState{State:"unknown"} rather than an
	// error.
	Status(ctx context.Context, orgID string, stream streaming.Stream) (InboundState, error)
}

// Inbound is the result of a successful Install.
type Inbound struct {
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

// FailKind categorises an inbound-provisioning failure so the REST
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
// Provisioners (and the seam) return it; the adapter switches on Kind.
type Failure struct {
	Kind FailKind
	Err  error
}

func (f *Failure) Error() string { return f.Err.Error() }
func (f *Failure) Unwrap() error { return f.Err }

// ErrInboundUnsupported is returned (wrapped in a FailBadRequest Failure)
// when the Stream's transport Kind has no registered provisioner.
var ErrInboundUnsupported = errors.New("transport does not support inbound webhook provisioning")

// InstallInbound provisions the provider-side inbound hook for a Stream
// and persists the resulting transport config. It dispatches on the
// Stream's transport Kind — every transport that needs external
// registration plugs in a provisioner without touching this seam.
func (s *Streams) InstallInbound(ctx context.Context, orgID string, id streaming.StreamID) (Inbound, error) {
	stream, err := s.streams.Get(ctx, orgID, id)
	if err != nil {
		return Inbound{}, err
	}
	p, ok := s.provisioners[stream.Transport.Kind]
	if !ok {
		return Inbound{}, &Failure{Kind: FailBadRequest, Err: fmt.Errorf("%w (kind=%q)", ErrInboundUnsupported, stream.Transport.Kind)}
	}
	inbound, err := p.Install(ctx, orgID, stream)
	if err != nil {
		return Inbound{}, err
	}
	if inbound.Config != nil {
		if _, err := s.Update(ctx, orgID, id, UpdateParams{
			Name:        stream.Name,
			Description: stream.Description,
			Transport:   &TransportPatch{Config: inbound.Config},
		}); err != nil {
			return Inbound{}, &Failure{Kind: FailInternal, Err: fmt.Errorf("persist hook onto stream %q: %w", id, err)}
		}
	}
	return inbound, nil
}

// InboundStatus reports the live inbound-hook state for a Stream.
func (s *Streams) InboundStatus(ctx context.Context, orgID string, id streaming.StreamID) (InboundState, error) {
	stream, err := s.streams.Get(ctx, orgID, id)
	if err != nil {
		return InboundState{}, err
	}
	p, ok := s.provisioners[stream.Transport.Kind]
	if !ok {
		return InboundState{}, &Failure{Kind: FailBadRequest, Err: fmt.Errorf("%w (kind=%q)", ErrInboundUnsupported, stream.Transport.Kind)}
	}
	return p.Status(ctx, orgID, stream)
}
