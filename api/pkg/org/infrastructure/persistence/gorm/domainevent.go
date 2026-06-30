package gorm

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain/domainevent"
	"github.com/helixml/helix/api/pkg/org/domain/store"
)

// domainEventRow is the GORM row for an append-only DomainEvent.
//
// One composite index serves the only read path — ListBySubject —
// exactly: (org_id, type, subject, created_at DESC). The three equality
// columns are the prefix (org_id first: every query is org-scoped and it is
// the most selective leading filter), and created_at is the trailing column
// so Postgres satisfies the `created_at >= ?` window AND the
// `ORDER BY created_at DESC` from the index alone — no separate filter or
// sort step. The leading prefix also covers cheaper future reads
// (e.g. by (org_id, type)).
//
// Deliberately NO standalone created_at index: nothing queries created_at
// without the full subject prefix, so it would only cost write throughput on
// this append-heavy table. A retention/pruning job (a non-goal today) would
// add its own index if/when it lands.
type domainEventRow struct {
	ID        string    `gorm:"primaryKey;type:text"`
	OrgID     string    `gorm:"not null;type:text;index:idx_org_domain_events_lookup,priority:1"`
	Type      string    `gorm:"not null;type:text;index:idx_org_domain_events_lookup,priority:2"`
	Subject   string    `gorm:"not null;type:text;index:idx_org_domain_events_lookup,priority:3"`
	Worker    string    `gorm:"type:text"`
	Source    string    `gorm:"type:text"`
	Metadata  string    `gorm:"type:text"`
	CreatedAt time.Time `gorm:"not null;index:idx_org_domain_events_lookup,priority:4,sort:desc"`
}

func (domainEventRow) TableName() string { return "org_domain_events" }

// domainEventMapper converts between the aggregate and its row, the only
// per-entity glue the generic Repository needs.
type domainEventMapper struct{}

func (domainEventMapper) ToRow(e domainevent.DomainEvent) (domainEventRow, error) {
	meta := ""
	if len(e.Metadata) > 0 {
		meta = string(e.Metadata)
	}
	return domainEventRow{
		ID:        e.ID,
		OrgID:     e.OrganizationID,
		Type:      e.Type,
		Subject:   e.Subject,
		Worker:    e.Worker,
		Source:    e.Source,
		Metadata:  meta,
		CreatedAt: e.CreatedAt,
	}, nil
}

func (domainEventMapper) ToDomain(row domainEventRow) (domainevent.DomainEvent, error) {
	var meta json.RawMessage
	if row.Metadata != "" {
		meta = json.RawMessage(row.Metadata)
	}
	return domainevent.DomainEvent{
		ID:             row.ID,
		OrganizationID: row.OrgID,
		Type:           row.Type,
		Subject:        row.Subject,
		Worker:         row.Worker,
		Source:         row.Source,
		Metadata:       meta,
		CreatedAt:      row.CreatedAt,
	}, nil
}

// domainEventsRepo wraps the generic Repository, exactly like every other
// per-entity store (topicsRepo, activationsRepo). Append/ListBySubject just
// name the two operations the domainevent.Repository port exposes; all the
// gorm boilerplate lives in the generic primitive.
type domainEventsRepo struct {
	*Repository[domainevent.DomainEvent, domainEventRow]
}

func newDomainEventsRepo(db *gorm.DB) *domainEventsRepo {
	return &domainEventsRepo{Repository: NewRepository[domainevent.DomainEvent, domainEventRow](db, domainEventMapper{}, "domain event")}
}

// Append records one event (a Create under the hood).
func (r *domainEventsRepo) Append(ctx context.Context, e domainevent.DomainEvent) error {
	return r.Repository.Create(ctx, e)
}

// ListBySubject is the membership-projection read: the (org, type, subject)
// equalities plus the optional created_at window, newest first — served by
// idx_org_domain_events_lookup. Expressed in store.Options so it rides the
// same query layer as every other repo.
func (r *domainEventsRepo) ListBySubject(ctx context.Context, orgID string, typ domainevent.Type, subject string, since time.Time) ([]domainevent.DomainEvent, error) {
	opts := []store.Option{
		store.WithOrg(orgID),
		store.WithCondition("type", string(typ)),
		store.WithCondition("subject", subject),
		store.WithOrderDesc("created_at"),
	}
	if !since.IsZero() {
		opts = append(opts, store.WithWhere("created_at >= ?", since.UTC()))
	}
	return r.Repository.Find(ctx, opts...)
}
