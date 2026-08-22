package triggers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
)

var ErrSourceInUse = errors.New("trigger has worker attachments")

type Service struct {
	triggers    store.Triggers
	attachments store.WorkerAttachments
	events      store.Events
	now         func() time.Time
	newID       func() string
	// provisioners is the per-transport-Kind inbound-hook registry the
	// InstallInbound / InboundStatus seam dispatches on. An unregistered
	// Kind reports the transport as unsupported.
	provisioners map[transport.Kind]trigger.Inbound
}

type Deps struct {
	Triggers    store.Triggers
	Attachments store.WorkerAttachments
	Events      store.Events
	Now         func() time.Time
	NewID       func() string
	// Provisioners maps a transport Kind to its inbound-hook
	// provisioner. Each impl lives in that transport's infrastructure
	// package; the composition root registers them here. Optional.
	Provisioners map[transport.Kind]trigger.Inbound
}

type CreateParams struct {
	// ID is optional. Empty mints a fresh `tr-<id>`; a supplied value is
	// used verbatim so callers can give a Trigger a short readable handle
	// (`s-general`) the way every other org-graph id works. A collision is
	// a conflict, not a silent rename.
	ID          string
	Name        string
	Description string
	Kind        transport.Kind
	Config      json.RawMessage
	CreatedBy   string
}

type UpdateParams struct {
	Name        string
	Description string
	Kind        transport.Kind
	Config      json.RawMessage
}

func New(d Deps) *Service {
	now := d.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{triggers: d.Triggers, attachments: d.Attachments, events: d.Events, now: now, newID: d.NewID, provisioners: d.Provisioners}
}

func (s *Service) List(ctx context.Context, orgID string) ([]trigger.Trigger, error) {
	if s.triggers == nil {
		return nil, errors.New("trigger repository is not configured")
	}
	rows, err := s.triggers.Find(ctx, store.WithOrg(orgID), store.WithOrderAsc("created_at"), store.WithOrderAsc("id"))
	if err != nil {
		return nil, fmt.Errorf("list triggers: %w", err)
	}
	return rows, nil
}

func (s *Service) Get(ctx context.Context, orgID, id string) (trigger.Trigger, error) {
	if s.triggers == nil {
		return trigger.Trigger{}, errors.New("trigger repository is not configured")
	}
	rows, err := s.triggers.Find(ctx, store.WithOrg(orgID), store.WithID(id), store.WithLimit(1))
	if err != nil {
		return trigger.Trigger{}, fmt.Errorf("get trigger: %w", err)
	}
	if len(rows) == 0 {
		return trigger.Trigger{}, store.ErrNotFound
	}
	return rows[0], nil
}

func (s *Service) Create(ctx context.Context, orgID string, p CreateParams) (trigger.Trigger, error) {
	if s.triggers == nil || s.newID == nil {
		return trigger.Trigger{}, errors.New("trigger service is not configured")
	}
	id := strings.TrimSpace(p.ID)
	if id == "" {
		id = "tr-" + s.newID()
	}
	t, err := trigger.New(id, orgID, strings.TrimSpace(p.Name), strings.TrimSpace(p.Description), p.Kind, p.Config, p.CreatedBy, s.now())
	if err != nil {
		return trigger.Trigger{}, err
	}
	if err := s.triggers.Create(ctx, t); err != nil {
		return trigger.Trigger{}, fmt.Errorf("create trigger: %w", err)
	}
	return t, nil
}

func (s *Service) Update(ctx context.Context, orgID, id, revision string, p UpdateParams) (trigger.Trigger, error) {
	existing, err := s.Get(ctx, orgID, id)
	if err != nil {
		return trigger.Trigger{}, err
	}
	if revision == "" || revision != Revision(existing) {
		return trigger.Trigger{}, store.ErrConflict
	}
	updated, err := trigger.New(existing.ID, existing.OrganizationID, strings.TrimSpace(p.Name), strings.TrimSpace(p.Description), p.Kind, p.Config, existing.CreatedBy, existing.CreatedAt)
	if err != nil {
		return trigger.Trigger{}, err
	}
	if err := s.triggers.Update(ctx, updated); err != nil {
		return trigger.Trigger{}, fmt.Errorf("update trigger: %w", err)
	}
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, orgID, id string) error {
	if _, err := s.Get(ctx, orgID, id); err != nil {
		return err
	}
	if s.attachments == nil {
		return errors.New("attachment repository is not configured")
	}
	rows, err := s.attachments.Find(ctx, store.WithOrg(orgID), store.WithTriggerID(id), store.WithLimit(1))
	if err != nil {
		return fmt.Errorf("find trigger attachments: %w", err)
	}
	if len(rows) > 0 {
		return ErrSourceInUse
	}
	return s.triggers.Delete(ctx, orgID, id)
}

func (s *Service) Events(ctx context.Context, orgID, id string, limit, offset int) ([]streaming.Event, int, error) {
	if _, err := s.Get(ctx, orgID, id); err != nil {
		return nil, 0, err
	}
	if s.events == nil {
		return nil, 0, errors.New("event repository is not configured")
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.events.PageForStream(ctx, orgID, id, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list trigger events: %w", err)
	}
	total, err := s.events.CountForStream(ctx, orgID, id)
	if err != nil {
		return nil, 0, fmt.Errorf("count trigger events: %w", err)
	}
	return rows, total, nil
}

func Revision(t trigger.Trigger) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%s", t.ID, t.Name, t.Description, t.Kind, t.Config)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func Sort(rows []trigger.Trigger) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].CreatedAt.Before(rows[j].CreatedAt)
	})
}

// ---- Inbound transport provisioning -------------------------------------
//
// The inbound port and its value types live in domain/trigger. What lives
// here is the application orchestration over that port: InstallInbound /
// InboundStatus read the Trigger, dispatch to the provisioner registered
// for its Kind, and persist the result. There is no outbound seam — a
// Trigger is inbound-only.

// InstallInbound provisions the provider-side inbound hook for a Trigger
// and persists the resulting transport config. It dispatches on the
// Trigger's Kind — every transport that needs external registration plugs
// in a trigger.Inbound provisioner without touching this seam.
func (s *Service) InstallInbound(ctx context.Context, orgID, id string) (trigger.InstallResult, error) {
	t, err := s.Get(ctx, orgID, id)
	if err != nil {
		return trigger.InstallResult{}, err
	}
	p, ok := s.provisioners[t.Kind]
	if !ok {
		return trigger.InstallResult{}, &trigger.Failure{Kind: trigger.FailBadRequest, Err: fmt.Errorf("%w (kind=%q)", trigger.ErrInboundUnsupported, t.Kind)}
	}
	inbound, err := p.Install(ctx, orgID, t)
	if err != nil {
		return trigger.InstallResult{}, err
	}
	if inbound.Config == nil {
		return inbound, nil
	}
	updated, err := trigger.New(t.ID, t.OrganizationID, t.Name, t.Description, t.Kind, inbound.Config, t.CreatedBy, t.CreatedAt)
	if err != nil {
		return trigger.InstallResult{}, &trigger.Failure{Kind: trigger.FailInternal, Err: fmt.Errorf("persist hook onto trigger %q: %w", id, err)}
	}
	if err := s.triggers.Update(ctx, updated); err != nil {
		return trigger.InstallResult{}, &trigger.Failure{Kind: trigger.FailInternal, Err: fmt.Errorf("persist hook onto trigger %q: %w", id, err)}
	}
	return inbound, nil
}

// InboundStatus reports the live inbound-hook state for a Trigger.
func (s *Service) InboundStatus(ctx context.Context, orgID, id string) (trigger.InboundState, error) {
	t, err := s.Get(ctx, orgID, id)
	if err != nil {
		return trigger.InboundState{}, err
	}
	p, ok := s.provisioners[t.Kind]
	if !ok {
		return trigger.InboundState{}, &trigger.Failure{Kind: trigger.FailBadRequest, Err: fmt.Errorf("%w (kind=%q)", trigger.ErrInboundUnsupported, t.Kind)}
	}
	return p.Status(ctx, orgID, t)
}

// Ensure get-or-creates a Trigger with a caller-supplied deterministic id.
// It is how automation (the Slack workspace connector, the helix-events
// reconciler, the topology reconciler) provisions the Trigger it owns
// without racing itself: an existing row is returned untouched, and a lost
// create race re-reads rather than failing.
func (s *Service) Ensure(ctx context.Context, orgID string, t trigger.Trigger) (trigger.Trigger, error) {
	existing, err := s.Get(ctx, orgID, t.ID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return trigger.Trigger{}, err
	}
	if createErr := s.triggers.Create(ctx, t); createErr != nil {
		existing, getErr := s.Get(ctx, orgID, t.ID)
		if getErr != nil {
			return trigger.Trigger{}, fmt.Errorf("ensure trigger %q: %w", t.ID, createErr)
		}
		return existing, nil
	}
	return t, nil
}

// EventStream is the durable stream a Trigger's events land on. A
// Trigger's stream is its own id — that identity is what preserved every
// converted Topic's history across the cutover.
func EventStream(t trigger.Trigger) streaming.StreamID { return t.ID }
