// Package topics is the application service that owns the structural
// Topic use cases — Create, Update, Delete — plus the inbound-transport
// provisioning seam (InstallInbound / InboundStatus), which orchestrates
// the streaming.Inbound domain port. It is the single home for the
// topic-mutation logic that the MCP `create_topic` tool and the REST
// topic handlers used to each implement independently. Both adapters now
// parse their protocol and delegate here, so the invariants (auto-id,
// transport defaulting, the update read-modify-write merge, org-scoping)
// cannot drift between callers.
//
// The service depends only on the narrow store.Topics repository
// interface plus a clock and id-generator — never the whole
// *store.Store (CLAUDE.md helix-org philosophy: small interfaces,
// ≤4 collaborators).
package topics

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

// Topics owns the topic-mutation use cases. It is a collection-shaped
// noun (CLAUDE.md §5.0: name by what it is, not what it does).
type Topics struct {
	topics store.Topics
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
	Topics store.Topics
	Now     func() time.Time
	NewID   func() string
	// Provisioners maps a transport Kind to its inbound-webhook
	// provisioner (GitHub today, Slack next). Each impl lives in that
	// transport's infrastructure package; the composition root registers
	// them here. Optional.
	Provisioners map[transport.Kind]streaming.Inbound
}

// New constructs the Topics service. Now/NewID fall back to wall-clock
// time and a counter-free stub so a partially-wired Deps still produces
// valid topics in tests; production always supplies both.
func New(deps Deps) *Topics {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Topics{topics: deps.Topics, now: now, newID: deps.NewID, provisioners: deps.Provisioners}
}

// CreateParams describes a new Topic. ID is optional — when empty a
// fresh `s-<id>` is minted. Transport's zero value means "local".
type CreateParams struct {
	ID          string
	Name        string
	Description string
	CreatedBy   string
	Transport   transport.Transport
}

// Create builds and persists a new Topic, returning the created
// aggregate. Name uniqueness and org-scoping are enforced by the repo;
// transport defaulting and validation by streaming.NewTopic.
func (s *Topics) Create(ctx context.Context, orgID string, p CreateParams) (streaming.Topic, error) {
	id := streaming.TopicID(strings.TrimSpace(p.ID))
	if id == "" {
		id = streaming.TopicID("s-" + s.newID())
	}
	topic, err := streaming.NewTopic(id, p.Name, p.Description, p.CreatedBy, s.now(), p.Transport, orgID)
	if err != nil {
		return streaming.Topic{}, err
	}
	if err := s.topics.Create(ctx, topic); err != nil {
		return streaming.Topic{}, err
	}
	return topic, nil
}

// TransportPatch is the partial transport update applied by Update.
// An empty Kind leaves the existing kind; a nil Config leaves the
// existing config. This mirrors the "tweak the github repo / events
// whitelist without re-stating the kind" flow the chart UI drives.
type TransportPatch struct {
	Kind   string
	Config json.RawMessage
}

// UpdateParams patches the mutable subset of a Topic: name,
// description, and (optionally) transport kind + config.
type UpdateParams struct {
	Name        string
	Description string
	Transport   *TransportPatch
}

// Update reads the existing Topic, merges the patch, and persists the
// result — the read-modify-write the REST handler used to do inline.
// Returns store.ErrNotFound (wrapped) when the (orgID, id) row is
// absent, including cross-tenant id guesses.
func (s *Topics) Update(ctx context.Context, orgID string, id streaming.TopicID, p UpdateParams) (streaming.Topic, error) {
	existing, err := s.topics.Get(ctx, orgID, id)
	if err != nil {
		return streaming.Topic{}, err
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
	updated, err := streaming.NewTopic(
		existing.ID, p.Name, p.Description,
		existing.CreatedBy, existing.CreatedAt, tr, existing.OrganizationID,
	)
	if err != nil {
		return streaming.Topic{}, err
	}
	if err := s.topics.Update(ctx, updated); err != nil {
		return streaming.Topic{}, err
	}
	return updated, nil
}

// Delete removes the Topic row. Subscriptions cascade in the repo
// (worker-anchored rows drop with the topic). Returns store.ErrNotFound
// (wrapped) when the row is absent.
func (s *Topics) Delete(ctx context.Context, orgID string, id streaming.TopicID) error {
	return s.topics.Delete(ctx, orgID, id)
}

// ---- Inbound transport wiring -------------------------------------------
//
// The inbound/outbound transport ports (streaming.Inbound /
// streaming.Outbound) and their value types are domain ports in the
// streaming package. What lives here is the application orchestration over
// the inbound port: InstallInbound / InboundStatus read the Topic,
// dispatch to the provisioner registered for its transport Kind, and
// persist the result. (The outbound side is driven by the dispatcher,
// which owns its own streaming.Outbound registry — there is no
// topics-service seam for it.)

// InstallInbound provisions the provider-side inbound hook for a Topic
// and persists the resulting transport config. It dispatches on the
// Topic's transport Kind — every transport that needs external
// registration plugs in a streaming.Inbound provisioner without touching
// this seam.
func (s *Topics) InstallInbound(ctx context.Context, orgID string, id streaming.TopicID) (streaming.InstallResult, error) {
	topic, err := s.topics.Get(ctx, orgID, id)
	if err != nil {
		return streaming.InstallResult{}, err
	}
	p, ok := s.provisioners[topic.Transport.Kind]
	if !ok {
		return streaming.InstallResult{}, &streaming.Failure{Kind: streaming.FailBadRequest, Err: fmt.Errorf("%w (kind=%q)", streaming.ErrInboundUnsupported, topic.Transport.Kind)}
	}
	inbound, err := p.Install(ctx, orgID, topic)
	if err != nil {
		return streaming.InstallResult{}, err
	}
	if inbound.Config != nil {
		if _, err := s.Update(ctx, orgID, id, UpdateParams{
			Name:        topic.Name,
			Description: topic.Description,
			Transport:   &TransportPatch{Config: inbound.Config},
		}); err != nil {
			return streaming.InstallResult{}, &streaming.Failure{Kind: streaming.FailInternal, Err: fmt.Errorf("persist hook onto topic %q: %w", id, err)}
		}
	}
	return inbound, nil
}

// InboundStatus reports the live inbound-hook state for a Topic.
func (s *Topics) InboundStatus(ctx context.Context, orgID string, id streaming.TopicID) (streaming.InboundState, error) {
	topic, err := s.topics.Get(ctx, orgID, id)
	if err != nil {
		return streaming.InboundState{}, err
	}
	p, ok := s.provisioners[topic.Transport.Kind]
	if !ok {
		return streaming.InboundState{}, &streaming.Failure{Kind: streaming.FailBadRequest, Err: fmt.Errorf("%w (kind=%q)", streaming.ErrInboundUnsupported, topic.Transport.Kind)}
	}
	return p.Status(ctx, orgID, topic)
}
