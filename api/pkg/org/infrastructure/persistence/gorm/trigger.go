package gorm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
	"gorm.io/gorm"
)

type triggerRow struct {
	ID        string `gorm:"primaryKey;type:text"`
	OrgID     string `gorm:"primaryKey;type:text;index;uniqueIndex:idx_trigger_org_name,priority:1"`
	Name      string `gorm:"not null;uniqueIndex:idx_trigger_org_name,priority:2"`
	Kind      string `gorm:"not null;index"`
	Config    string `gorm:"not null;default:''"`
	CreatedBy string
	CreatedAt time.Time
}

func (triggerRow) TableName() string { return "org_triggers" }

type triggerMapper struct{}

func (triggerMapper) ToRow(t trigger.Trigger) (triggerRow, error) {
	return triggerRow{ID: t.ID, OrgID: t.OrganizationID, Name: t.Name, Kind: string(t.Kind), Config: string(t.Config), CreatedBy: t.CreatedBy, CreatedAt: t.CreatedAt}, nil
}
func (triggerMapper) ToDomain(r triggerRow) (trigger.Trigger, error) {
	var config json.RawMessage
	if r.Config != "" {
		config = json.RawMessage(r.Config)
	}
	return trigger.New(r.ID, r.OrgID, r.Name, transport.Kind(r.Kind), config, r.CreatedBy, r.CreatedAt)
}

type triggersRepo struct {
	*Repository[trigger.Trigger, triggerRow]
}

func newTriggersRepo(db *gorm.DB) *triggersRepo {
	return &triggersRepo{NewRepository[trigger.Trigger, triggerRow](db, triggerMapper{}, "trigger")}
}
func (r *triggersRepo) Create(ctx context.Context, t trigger.Trigger) error {
	if err := r.Repository.Create(ctx, t); err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("trigger %q %w", t.Name, store.ErrConflict)
		}
		return err
	}
	return nil
}
func (r *triggersRepo) Update(ctx context.Context, t trigger.Trigger) error {
	err := r.Repository.Update(ctx, store.WithOrg(t.OrganizationID), store.WithID(t.ID), store.WithUpdates(map[string]any{"name": t.Name, "kind": string(t.Kind), "config": string(t.Config)}))
	if isUniqueViolation(err) {
		return fmt.Errorf("trigger %q %w", t.Name, store.ErrConflict)
	}
	return err
}
func (r *triggersRepo) Delete(ctx context.Context, orgID, id string) error {
	return r.Repository.Delete(ctx, store.WithOrg(orgID), store.WithID(id))
}
func (r *triggersRepo) Find(ctx context.Context, opts ...store.Option) ([]trigger.Trigger, error) {
	return r.Repository.Find(ctx, opts...)
}
