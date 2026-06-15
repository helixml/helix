// Package streams is the application service that owns the structural
// Stream use cases — Create, Update, Delete — plus the inbound-transport
// provisioning seam (InstallInbound / InboundStatus), which orchestrates
// the streaming.Inbound domain port. It is the single home for the
// stream-mutation logic that the MCP `create_stream` tool and the REST
// stream handlers used to each implement independently. Both adapters now
// parse their protocol and delegate here, so the invariants (auto-id,
// transport defaulting, the update read-modify-write merge, org-scoping)
// cannot drift between callers.
//
// The service depends only on the narrow store.Streams repository
// interface plus a clock and id-generator — never the whole
// *store.Store (CLAUDE.md helix-org philosophy: small interfaces,
// ≤4 collaborators).
package streams

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// Streams owns the stream-mutation use cases. It is a collection-shaped
// noun (CLAUDE.md §5.0: name by what it is, not what it does).
type Streams struct {
	streams store.Streams
	now     func() time.Time
	newID   func() string
	// provisioners is the per-transport-Kind inbound-webhook registry the
	// InstallInbound / InboundStatus seam dispatches on. nil/empty → those
	// endpoints report the transport as unsupported.
	provisioners map[transport.Kind]streaming.Inbound
}

// Deps are the constructor-injected collaborators for New. Grouping
// them in a struct keeps the call site reading as named fields.
type Deps struct {
	Streams store.Streams
	Now     func() time.Time
	NewID   func() string
	// Provisioners maps a transport Kind to its inbound-webhook
	// provisioner (GitHub today, Slack next). Each impl lives in that
	// transport's infrastructure package; the composition root registers
	// them here. Optional.
	Provisioners map[transport.Kind]streaming.Inbound
}

// New constructs the Streams service. Now/NewID fall back to wall-clock
// time and a counter-free stub so a partially-wired Deps still produces
// valid streams in tests; production always supplies both.
func New(deps Deps) *Streams {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Streams{streams: deps.Streams, now: now, newID: deps.NewID, provisioners: deps.Provisioners}
}

// CreateParams describes a new Stream. ID is optional — when empty a
// fresh `s-<id>` is minted. Transport's zero value means "local".
type CreateParams struct {
	ID          string
	Name        string
	Description string
	CreatedBy   string
	Transport   transport.Transport
}

// Create builds and persists a new Stream, returning the created
// aggregate. Name uniqueness and org-scoping are enforced by the repo;
// transport defaulting and validation by streaming.NewStream.
func (s *Streams) Create(ctx context.Context, orgID string, p CreateParams) (streaming.Stream, error) {
	id := streaming.StreamID(strings.TrimSpace(p.ID))
	if id == "" {
		id = streaming.StreamID("s-" + s.newID())
	}
	stream, err := streaming.NewStream(id, p.Name, p.Description, p.CreatedBy, s.now(), p.Transport, orgID)
	if err != nil {
		return streaming.Stream{}, err
	}
	if err := s.streams.Create(ctx, stream); err != nil {
		return streaming.Stream{}, err
	}
	return stream, nil
}

// TransportPatch is the partial transport update applied by Update.
// An empty Kind leaves the existing kind; a nil Config leaves the
// existing config. This mirrors the "tweak the github repo / events
// whitelist without re-stating the kind" flow the chart UI drives.
type TransportPatch struct {
	Kind   string
	Config json.RawMessage
}

// UpdateParams patches the mutable subset of a Stream: name,
// description, and (optionally) transport kind + config.
type UpdateParams struct {
	Name        string
	Description string
	Transport   *TransportPatch
}

// Update reads the existing Stream, merges the patch, and persists the
// result — the read-modify-write the REST handler used to do inline.
// Returns store.ErrNotFound (wrapped) when the (orgID, id) row is
// absent, including cross-tenant id guesses.
func (s *Streams) Update(ctx context.Context, orgID string, id streaming.StreamID, p UpdateParams) (streaming.Stream, error) {
	existing, err := s.streams.Get(ctx, orgID, id)
	if err != nil {
		return streaming.Stream{}, err
	}
	tr := existing.Transport
	if p.Transport != nil {
		if k := strings.TrimSpace(p.Transport.Kind); k != "" {
			tr.Kind = transport.Kind(k)
		}
		if p.Transport.Config != nil {
			tr.Config = p.Transport.Config
		}
	}
	updated, err := streaming.NewStream(
		existing.ID, p.Name, p.Description,
		existing.CreatedBy, existing.CreatedAt, tr, existing.OrganizationID,
	)
	if err != nil {
		return streaming.Stream{}, err
	}
	if err := s.streams.Update(ctx, updated); err != nil {
		return streaming.Stream{}, err
	}
	return updated, nil
}

// Delete removes the Stream row. Subscriptions cascade in the repo
// (worker-anchored rows drop with the stream). Returns store.ErrNotFound
// (wrapped) when the row is absent.
func (s *Streams) Delete(ctx context.Context, orgID string, id streaming.StreamID) error {
	return s.streams.Delete(ctx, orgID, id)
}

// ---- Inbound transport wiring -------------------------------------------
//
// The inbound/outbound transport ports (streaming.Inbound /
// streaming.Outbound) and their value types are domain ports in the
// streaming package. What lives here is the application orchestration over
// the inbound port: InstallInbound / InboundStatus read the Stream,
// dispatch to the provisioner registered for its transport Kind, and
// persist the result. (The outbound side is driven by the dispatcher,
// which owns its own streaming.Outbound registry — there is no
// streams-service seam for it.)

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
