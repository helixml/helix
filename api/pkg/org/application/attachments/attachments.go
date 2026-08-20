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
