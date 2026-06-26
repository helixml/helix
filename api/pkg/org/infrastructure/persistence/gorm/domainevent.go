package gorm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain/domainevent"
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

func toDomainEventRow(e domainevent.DomainEvent) domainEventRow {
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
	}
}

func fromDomainEventRow(row domainEventRow) domainevent.DomainEvent {
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
	}
}

type domainEventsRepo struct {
	db *gorm.DB
}

func newDomainEventsRepo(db *gorm.DB) *domainEventsRepo {
	return &domainEventsRepo{db: db}
}

func (r *domainEventsRepo) Append(ctx context.Context, e domainevent.DomainEvent) error {
	if err := r.db.WithContext(ctx).Create(toDomainEventRow(e)).Error; err != nil {
		return fmt.Errorf("append domain event %q: %w", e.ID, err)
	}
	return nil
}

func (r *domainEventsRepo) ListBySubject(ctx context.Context, orgID string, typ domainevent.Type, subject string, since time.Time) ([]domainevent.DomainEvent, error) {
	q := r.db.WithContext(ctx).
		Where("org_id = ? AND type = ? AND subject = ?", orgID, typ, subject)
	if !since.IsZero() {
		q = q.Where("created_at >= ?", since.UTC())
	}
	var rows []domainEventRow
	if err := q.Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list domain events for subject %q: %w", subject, err)
	}
	out := make([]domainevent.DomainEvent, len(rows))
	for i, row := range rows {
		out[i] = fromDomainEventRow(row)
	}
	return out, nil
}
