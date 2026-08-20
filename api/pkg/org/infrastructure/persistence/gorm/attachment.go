package gorm

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/attachment"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"gorm.io/gorm"
)

type attachmentRow struct {
	ID          string  `gorm:"primaryKey;type:text"`
	OrgID       string  `gorm:"primaryKey;type:text;index"`
	WorkerID    string  `gorm:"not null;index"`
	TriggerID   *string `gorm:"index;check:chk_attachment_source,(trigger_id IS NOT NULL AND processor_id IS NULL AND output_id IS NULL) OR (trigger_id IS NULL AND processor_id IS NOT NULL AND output_id IS NOT NULL)"`
	ProcessorID *string `gorm:"index"`
	OutputID    *string `gorm:"index"`
	CreatedBy   string
	CreatedAt   time.Time
}

func (attachmentRow) TableName() string { return "org_worker_attachments" }

type attachmentMapper struct{}

func (attachmentMapper) ToRow(a attachment.Attachment) (attachmentRow, error) {
	r := attachmentRow{ID: a.ID, OrgID: a.OrganizationID, WorkerID: string(a.WorkerID), CreatedBy: a.CreatedBy, CreatedAt: a.CreatedAt}
	if a.Source.Kind == eventsource.KindTrigger {
		r.TriggerID = &a.Source.TriggerID
	} else {
		p, o := string(a.Source.ProcessorID), a.Source.OutputID
		r.ProcessorID = &p
		r.OutputID = &o
	}
	return r, nil
}
func (attachmentMapper) ToDomain(r attachmentRow) (attachment.Attachment, error) {
	var src eventsource.SourceRef
	if r.TriggerID != nil {
		src = eventsource.Trigger(*r.TriggerID)
	} else if r.ProcessorID != nil && r.OutputID != nil {
		src = eventsource.ProcessorOutput(*r.ProcessorID, *r.OutputID)
	} else {
		return attachment.Attachment{}, fmt.Errorf("attachment %q has invalid source columns", r.ID)
	}
	return attachment.New(r.ID, r.OrgID, orgchart.NodeID(r.WorkerID), src, r.CreatedBy, r.CreatedAt)
}

type attachmentsRepo struct {
	*Repository[attachment.Attachment, attachmentRow]
}

func newAttachmentsRepo(db *gorm.DB) *attachmentsRepo {
	return &attachmentsRepo{NewRepository[attachment.Attachment, attachmentRow](db, attachmentMapper{}, "worker attachment")}
}
func (r *attachmentsRepo) Create(ctx context.Context, a attachment.Attachment) error {
	if err := r.Repository.Create(ctx, a); err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("worker source attachment %w", store.ErrConflict)
		}
		return err
	}
	return nil
}
func (r *attachmentsRepo) Delete(ctx context.Context, orgID, id string) error {
	return r.Repository.Delete(ctx, store.WithOrg(orgID), store.WithID(id))
}
func (r *attachmentsRepo) Find(ctx context.Context, opts ...store.Option) ([]attachment.Attachment, error) {
	return r.Repository.Find(ctx, opts...)
}
