package sqlite

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix-org/domain"
)

type eventRow struct {
	ID        string    `gorm:"primaryKey;type:text"`
	ChannelID string    `gorm:"not null;index"`
	Source    string    `gorm:"index"` // empty for system-emitted
	Body      string    `gorm:"not null"`
	CreatedAt time.Time `gorm:"index"`
}

func (eventRow) TableName() string { return "events" }

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

func (r *eventsRepo) ListForChannel(ctx context.Context, channelID domain.ChannelID, limit int) ([]domain.Event, error) {
	query := r.db.WithContext(ctx).Where("channel_id = ?", string(channelID)).Order("created_at DESC, id DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rows []eventRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list events for channel %q: %w", channelID, err)
	}
	return rowsToEvents(rows)
}

func (r *eventsRepo) ListSince(ctx context.Context, channelIDs []domain.ChannelID, since domain.EventID, limit int) ([]domain.Event, error) {
	if len(channelIDs) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(channelIDs))
	for _, c := range channelIDs {
		ids = append(ids, string(c))
	}

	// Resolve `since` to its (created_at, id) pair. If the event is unknown
	// (empty since, or stale), we fall back to "no lower bound" — same as if
	// the caller passed nothing.
	var (
		sinceTS time.Time
		sinceID string
		hasLB   bool
	)
	if since != "" {
		var pivot eventRow
		err := r.db.WithContext(ctx).Where("id = ?", string(since)).Take(&pivot).Error
		if err == nil {
			sinceTS = pivot.CreatedAt
			sinceID = pivot.ID
			hasLB = true
		}
		// gorm.ErrRecordNotFound and other errors fall through to "no lower
		// bound" — tail callers tolerate this and just see recent history.
	}

	query := r.db.WithContext(ctx).Where("channel_id IN ?", ids)
	if hasLB {
		// (created_at, id) > (sinceTS, sinceID)
		query = query.Where("(created_at > ?) OR (created_at = ? AND id > ?)", sinceTS, sinceTS, sinceID)
	}
	query = query.Order("created_at ASC, id ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rows []eventRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list events since %q: %w", since, err)
	}
	return rowsToEvents(rows)
}

func (r *eventsRepo) ListForWorker(ctx context.Context, workerID domain.WorkerID, limit int) ([]domain.Event, error) {
	// Join events with streams to return only events on channels the worker
	// subscribes to, newest first.
	query := r.db.WithContext(ctx).
		Table("events AS e").
		Joins("JOIN streams AS s ON s.channel_id = e.channel_id").
		Where("s.worker_id = ?", string(workerID)).
		Order("e.created_at DESC, e.id DESC").
		Select("e.*")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rows []eventRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list events for worker %q: %w", workerID, err)
	}
	return rowsToEvents(rows)
}

func eventToRow(e domain.Event) eventRow {
	return eventRow{
		ID:        string(e.ID),
		ChannelID: string(e.ChannelID),
		Source:    string(e.Source),
		Body:      e.Body,
		CreatedAt: e.CreatedAt,
	}
}

func rowToEvent(row eventRow) (domain.Event, error) {
	return domain.NewEvent(
		domain.EventID(row.ID),
		domain.ChannelID(row.ChannelID),
		domain.WorkerID(row.Source),
		row.Body,
		row.CreatedAt,
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
