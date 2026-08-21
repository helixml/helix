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
}

type Deps struct {
	Triggers    store.Triggers
	Attachments store.WorkerAttachments
	Events      store.Events
	Now         func() time.Time
	NewID       func() string
}

type CreateParams struct {
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
	return &Service{triggers: d.Triggers, attachments: d.Attachments, events: d.Events, now: now, newID: d.NewID}
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
	t, err := trigger.New("tr-"+s.newID(), orgID, strings.TrimSpace(p.Name), strings.TrimSpace(p.Description), p.Kind, p.Config, p.CreatedBy, s.now())
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
	rows, err := s.events.PageForTopic(ctx, orgID, streaming.TopicID(id), limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list trigger events: %w", err)
	}
	total, err := s.events.CountForTopic(ctx, orgID, streaming.TopicID(id))
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
