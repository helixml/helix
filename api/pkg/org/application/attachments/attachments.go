package attachments

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/attachment"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
)

type Service struct {
	store *store.Store
	now   func() time.Time
	newID func() string
}
type Deps struct {
	Store *store.Store
	Now   func() time.Time
	NewID func() string
}

func New(d Deps) *Service {
	now := d.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	newID := d.NewID
	return &Service{store: d.Store, now: now, newID: newID}
}

func (s *Service) Create(ctx context.Context, orgID string, workerID orgchart.NodeID, source eventsource.SourceRef, createdBy string) (attachment.Attachment, error) {
	if s.store == nil || s.store.WorkerAttachments == nil {
		return attachment.Attachment{}, errors.New("create worker attachment: attachment repository is not configured")
	}
	if s.newID == nil {
		return attachment.Attachment{}, errors.New("create worker attachment: id generator is not configured")
	}
	worker, err := s.store.Nodes.Get(ctx, orgID, workerID)
	if err != nil {
		return attachment.Attachment{}, fmt.Errorf("create worker attachment: get worker %q: %w", workerID, err)
	}
	if worker.IsHuman() {
		return attachment.Attachment{}, fmt.Errorf("create worker attachment: worker %q is human", workerID)
	}
	if err := s.validateSource(ctx, orgID, source); err != nil {
		return attachment.Attachment{}, fmt.Errorf("create worker attachment: %w", err)
	}
	a, err := attachment.New("wa-"+s.newID(), orgID, workerID, source, createdBy, s.now())
	if err != nil {
		return attachment.Attachment{}, fmt.Errorf("create worker attachment: %w", err)
	}
	if err := s.store.WorkerAttachments.Create(ctx, a); err != nil {
		return attachment.Attachment{}, fmt.Errorf("create worker attachment: persist: %w", err)
	}
	return a, nil
}
func (s *Service) Delete(ctx context.Context, orgID, id string) error {
	if s.store == nil || s.store.WorkerAttachments == nil {
		return errors.New("delete worker attachment: attachment repository is not configured")
	}
	if err := s.store.WorkerAttachments.Delete(ctx, orgID, id); err != nil {
		return fmt.Errorf("delete worker attachment %q: %w", id, err)
	}
	return nil
}
func (s *Service) validateSource(ctx context.Context, orgID string, source eventsource.SourceRef) error {
	if err := source.Validate(); err != nil {
		return err
	}
	switch source.Kind {
	case eventsource.KindTrigger:
		rows, err := s.store.Triggers.Find(ctx, store.WithOrg(orgID), store.WithID(source.TriggerID), store.WithLimit(1))
		if err != nil {
			return fmt.Errorf("find trigger %q: %w", source.TriggerID, err)
		}
		if len(rows) == 0 {
			return fmt.Errorf("trigger %q: %w", source.TriggerID, store.ErrNotFound)
		}
	case eventsource.KindProcessorOutput:
		p, err := s.store.Processors.Get(ctx, orgID, source.ProcessorID)
		if err != nil {
			return fmt.Errorf("get processor %q: %w", source.ProcessorID, err)
		}
		for _, out := range p.Outputs {
			if out.ID == source.OutputID {
				return nil
			}
		}
		return fmt.Errorf("processor %q output %q: %w", source.ProcessorID, source.OutputID, store.ErrNotFound)
	}
	return nil
}

// ListForWorker returns every source a Worker is attached to, in stable
// creation order.
func (s *Service) ListForWorker(ctx context.Context, orgID string, workerID orgchart.NodeID) ([]attachment.Attachment, error) {
	if s.store == nil || s.store.WorkerAttachments == nil {
		return nil, errors.New("list worker attachments: attachment repository is not configured")
	}
	if _, err := s.store.Nodes.Get(ctx, orgID, workerID); err != nil {
		return nil, fmt.Errorf("list worker attachments: get worker %q: %w", workerID, err)
	}
	rows, err := s.store.WorkerAttachments.Find(ctx, store.WithOrg(orgID), store.WithWorkerID(workerID), store.WithOrderAsc("created_at"), store.WithOrderAsc("id"))
	if err != nil {
		return nil, fmt.Errorf("list worker attachments: %w", err)
	}
	return rows, nil
}

// AttachAll attaches one Worker to several sources in a single call, so a
// caller wiring up a Worker completes that intent in one step rather than
// a chain of round-trips. Idempotent per source: a source the Worker is
// already attached to is skipped, not an error.
func (s *Service) AttachAll(ctx context.Context, orgID string, workerID orgchart.NodeID, sources []eventsource.SourceRef, createdBy string) error {
	if s.store == nil || s.store.WorkerAttachments == nil {
		return errors.New("attach worker: attachment repository is not configured")
	}
	existing, err := s.ListForWorker(ctx, orgID, workerID)
	if err != nil {
		return err
	}
	attached := make(map[string]struct{}, len(existing))
	for _, a := range existing {
		attached[a.Source.Key()] = struct{}{}
	}
	for _, src := range sources {
		if _, ok := attached[src.Key()]; ok {
			continue
		}
		if _, err := s.Create(ctx, orgID, workerID, src, createdBy); err != nil {
			return err
		}
		attached[src.Key()] = struct{}{}
	}
	return nil
}

// DetachAll removes one Worker's attachment to several sources in a
// single call. Idempotent per source: a source the Worker is not attached
// to is skipped, not an error.
func (s *Service) DetachAll(ctx context.Context, orgID string, workerID orgchart.NodeID, sources []eventsource.SourceRef) error {
	if s.store == nil || s.store.WorkerAttachments == nil {
		return errors.New("detach worker: attachment repository is not configured")
	}
	existing, err := s.ListForWorker(ctx, orgID, workerID)
	if err != nil {
		return err
	}
	wanted := make(map[string]struct{}, len(sources))
	for _, src := range sources {
		if err := src.Validate(); err != nil {
			return fmt.Errorf("detach worker: %w", err)
		}
		wanted[src.Key()] = struct{}{}
	}
	for _, a := range existing {
		if _, ok := wanted[a.Source.Key()]; !ok {
			continue
		}
		if err := s.Delete(ctx, orgID, a.ID); err != nil {
			return err
		}
	}
	return nil
}
