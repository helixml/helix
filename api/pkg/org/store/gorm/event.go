package gorm

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain"
	"github.com/helixml/helix/api/pkg/org/event"
	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/worker"
)

type eventRow struct {
	ID        string    `gorm:"primaryKey;type:text"`
	OrgID     string    `gorm:"primaryKey;type:text;index"`
	StreamID  string    `gorm:"not null;index"`
	Source    string    `gorm:"index"` // empty for system-emitted
	Body      string    `gorm:"not null"`
	CreatedAt time.Time `gorm:"index"`
}

func (eventRow) TableName() string { return "org_events" }

type eventsRepo struct {
	db *gorm.DB
}

func (r *eventsRepo) Append(ctx context.Context, e domain.Event) error {
	row := eventToRow(e)
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	return nil
}

func (r *eventsRepo) ListForStream(ctx context.Context, orgID string, streamID stream.ID, limit int) ([]domain.Event, error) {
	query := r.db.WithContext(ctx).Where("org_id = ? AND stream_id = ?", orgID, string(streamID)).Order("created_at DESC, id DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rows []eventRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list events for stream %q in org %q: %w", streamID, orgID, err)
	}
	return rowsToEvents(rows)
}

func (r *eventsRepo) ListSince(ctx context.Context, orgID string, streamIDs []stream.ID, since event.ID, limit int) ([]domain.Event, error) {
	if len(streamIDs) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(streamIDs))
	for _, s := range streamIDs {
		ids = append(ids, string(s))
	}

	var (
		sinceTS time.Time
		sinceID string
		hasLB   bool
	)
	if since != "" {
		var pivot eventRow
		err := r.db.WithContext(ctx).Where("org_id = ? AND id = ?", orgID, string(since)).Take(&pivot).Error
		if err == nil {
			sinceTS = pivot.CreatedAt
			sinceID = pivot.ID
			hasLB = true
		}
	}

	query := r.db.WithContext(ctx).Where("org_id = ? AND stream_id IN ?", orgID, ids)
	if hasLB {
		query = query.Where("(created_at > ?) OR (created_at = ? AND id > ?)", sinceTS, sinceTS, sinceID)
	}
	query = query.Order("created_at ASC, id ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rows []eventRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list events since %q in org %q: %w", since, orgID, err)
	}
	return rowsToEvents(rows)
}

func (r *eventsRepo) ListAll(ctx context.Context, orgID string, limit int) ([]domain.Event, error) {
	query := r.db.WithContext(ctx).Where("org_id = ?", orgID).Order("created_at DESC, id DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rows []eventRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list all events in org %q: %w", orgID, err)
	}
	return rowsToEvents(rows)
}

func (r *eventsRepo) ListForWorker(ctx context.Context, orgID string, workerID worker.ID, limit int) ([]domain.Event, error) {
	query := r.db.WithContext(ctx).
		Table("org_events AS e").
		Joins("JOIN org_subscriptions AS s ON s.stream_id = e.stream_id AND s.org_id = e.org_id").
		Where("e.org_id = ? AND s.worker_id = ?", orgID, string(workerID)).
		Order("e.created_at DESC, e.id DESC").
		Select("e.*")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rows []eventRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list events for worker %q in org %q: %w", workerID, orgID, err)
	}
	return rowsToEvents(rows)
}

func eventToRow(e domain.Event) eventRow {
	return eventRow{
		ID:        string(e.ID),
		OrgID:     e.OrganizationID,
		StreamID:  string(e.StreamID),
		Source:    string(e.Source),
		Body:      e.Body,
		CreatedAt: e.CreatedAt,
	}
}

func rowToEvent(row eventRow) (domain.Event, error) {
	return domain.NewEvent(
		event.ID(row.ID),
		stream.ID(row.StreamID),
		worker.ID(row.Source),
		row.Body,
		row.CreatedAt,
		row.OrgID,
	)
}

func rowsToEvents(rows []eventRow) ([]domain.Event, error) {
	out := make([]domain.Event, 0, len(rows))
	for _, row := range rows {
		e, err := rowToEvent(row)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}
