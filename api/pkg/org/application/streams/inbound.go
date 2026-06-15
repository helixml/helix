package streams

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// The inbound/outbound transport ports (streaming.Inbound /
// streaming.Outbound) and their value types live in the streaming domain.
// What lives here is the application orchestration over the inbound port:
// InstallInbound / InboundStatus read the Stream, dispatch to the
// provisioner registered for its transport Kind, and persist the result.
// (The outbound side is driven by the dispatcher, which owns its own
// streaming.Outbound registry — there is no streams-service seam for it.)

// InstallInbound provisions the provider-side inbound hook for a Stream
// and persists the resulting transport config. It dispatches on the
// Stream's transport Kind — every transport that needs external
// registration plugs in a streaming.Inbound provisioner without touching
// this seam.
func (s *Streams) InstallInbound(ctx context.Context, orgID string, id streaming.StreamID) (streaming.InstallResult, error) {
	stream, err := s.streams.Get(ctx, orgID, id)
	if err != nil {
		return streaming.InstallResult{}, err
	}
	p, ok := s.provisioners[stream.Transport.Kind]
	if !ok {
		return streaming.InstallResult{}, &streaming.Failure{Kind: streaming.FailBadRequest, Err: fmt.Errorf("%w (kind=%q)", streaming.ErrInboundUnsupported, stream.Transport.Kind)}
	}
	inbound, err := p.Install(ctx, orgID, stream)
	if err != nil {
		return streaming.InstallResult{}, err
	}
	if inbound.Config != nil {
		if _, err := s.Update(ctx, orgID, id, UpdateParams{
			Name:        stream.Name,
			Description: stream.Description,
			Transport:   &TransportPatch{Config: inbound.Config},
		}); err != nil {
			return streaming.InstallResult{}, &streaming.Failure{Kind: streaming.FailInternal, Err: fmt.Errorf("persist hook onto stream %q: %w", id, err)}
		}
	}
	return inbound, nil
}

// InboundStatus reports the live inbound-hook state for a Stream.
func (s *Streams) InboundStatus(ctx context.Context, orgID string, id streaming.StreamID) (streaming.InboundState, error) {
	stream, err := s.streams.Get(ctx, orgID, id)
	if err != nil {
		return streaming.InboundState{}, err
	}
	p, ok := s.provisioners[stream.Transport.Kind]
	if !ok {
		return streaming.InboundState{}, &streaming.Failure{Kind: streaming.FailBadRequest, Err: fmt.Errorf("%w (kind=%q)", streaming.ErrInboundUnsupported, stream.Transport.Kind)}
	}
	return p.Status(ctx, orgID, stream)
}
