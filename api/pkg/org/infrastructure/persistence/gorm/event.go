package gorm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
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

type eventMapper struct{}

func (eventMapper) ToRow(e streaming.Event) (eventRow, error) {
	return eventRow{
		ID:        string(e.ID),
		OrgID:     e.OrganizationID,
		StreamID:  string(e.StreamID),
		Source:    string(e.Source),
		Body:      e.Body,
		CreatedAt: e.CreatedAt,
	}, nil
}

func (eventMapper) ToDomain(row eventRow) (streaming.Event, error) {
	return streaming.NewEvent(
		streaming.EventID(row.ID),
		streaming.StreamID(row.StreamID),
		orgchart.WorkerID(row.Source),
		row.Body,
		row.CreatedAt,
		row.OrgID,
	)
}

type eventsRepo struct {
	*Repository[streaming.Event, eventRow]
	workers *workersRepo
}

func newEventsRepo(db *gorm.DB, workers *workersRepo) *eventsRepo {
	return &eventsRepo{
		Repository: NewRepository[streaming.Event, eventRow](db, eventMapper{}, "event"),
		workers:    workers,
	}
}

func (r *eventsRepo) Append(ctx context.Context, e streaming.Event) error {
	return r.Repository.Create(ctx, e)
}

func (r *eventsRepo) ListForStream(ctx context.Context, orgID string, streamID streaming.StreamID, limit int) ([]streaming.Event, error) {
	return r.Repository.Find(ctx,
		store.WithOrg(orgID),
		store.WithCondition("stream_id", string(streamID)),
		store.WithOrderDesc("created_at"),
		store.WithOrderDesc("id"),
		store.WithLimit(limit),
	)
}

func (r *eventsRepo) ListAll(ctx context.Context, orgID string, limit int) ([]streaming.Event, error) {
	return r.Repository.Find(ctx,
		store.WithOrg(orgID),
		store.WithOrderDesc("created_at"),
		store.WithOrderDesc("id"),
		store.WithLimit(limit),
	)
}

func (r *eventsRepo) ListForWorker(ctx context.Context, orgID string, workerID orgchart.WorkerID, limit int) ([]streaming.Event, error) {
	// Subscriptions are worker-anchored. Join events against
	// subscriptions for this worker directly.
	if r.workers == nil {
		return nil, fmt.Errorf("eventsRepo: workers repo not wired")
	}
	if _, err := r.workers.Get(ctx, orgID, workerID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("resolve worker for event listing: %w", err)
	}
	return r.Repository.Find(ctx,
		store.WithTable("org_events AS e"),
		store.WithJoin("JOIN org_subscriptions AS s ON s.stream_id = e.stream_id AND s.org_id = e.org_id"),
		store.WithSelect("e.*"),
		store.WithCondition("e.org_id", orgID),
		store.WithCondition("s.worker_id", string(workerID)),
		store.WithOrderDesc("e.created_at"),
		store.WithOrderDesc("e.id"),
		store.WithLimit(limit),
	)
}

func (r *eventsRepo) ListSince(ctx context.Context, orgID string, streamIDs []streaming.StreamID, since streaming.EventID, limit int) ([]streaming.Event, error) {
	if len(streamIDs) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(streamIDs))
	for _, s := range streamIDs {
		ids = append(ids, string(s))
	}

	// Look up the cursor pivot's (created_at, id) so we can resolve
	// "events strictly after the pivot" without depending on the
	// caller passing a timestamp. A missing pivot silently degrades
	// to "from the beginning" — same as the prior implementation.
	var (
		sinceTS time.Time
		sinceID string
		hasLB   bool
	)
	if since != "" {
		pivot, err := r.Repository.FindOne(ctx,
			store.WithOrg(orgID),
			store.WithID(string(since)),
		)
		if err == nil {
			sinceTS = pivot.CreatedAt
			sinceID = string(pivot.ID)
			hasLB = true
		} else if !errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("lookup since-pivot %q: %w", since, err)
		}
	}

	opts := []store.Option{
		store.WithCondition("org_id", orgID),
		store.WithConditionIn("stream_id", ids),
	}
	if hasLB {
		opts = append(opts, store.WithWhere("(created_at > ?) OR (created_at = ? AND id > ?)", sinceTS, sinceTS, sinceID))
	}
	opts = append(opts,
		store.WithOrderAsc("created_at"),
		store.WithOrderAsc("id"),
		store.WithLimit(limit),
	)
	return r.Repository.Find(ctx, opts...)
}
