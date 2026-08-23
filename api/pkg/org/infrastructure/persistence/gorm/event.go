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

// eventRow is one entry on an event stream. The column is still
// `topic_id`: the cutover changed which aggregate owns a stream, not
// where its rows live, so every pre-cutover event stayed addressable
// under the same key.
type eventRow struct {
	ID        string    `gorm:"primaryKey;type:text"`
	OrgID     string    `gorm:"primaryKey;type:text;index"`
	StreamID  string    `gorm:"column:topic_id;not null;index"`
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
		orgchart.NodeID(row.Source),
		row.Body,
		row.CreatedAt,
		row.OrgID,
	)
}

type eventsRepo struct {
	*Repository[streaming.Event, eventRow]
}

func newEventsRepo(db *gorm.DB) *eventsRepo {
	return &eventsRepo{Repository: NewRepository[streaming.Event, eventRow](db, eventMapper{}, "event")}
}

func (r *eventsRepo) Append(ctx context.Context, e streaming.Event) error {
	err := r.Repository.Create(ctx, e)
	if isUniqueViolation(err) {
		return fmt.Errorf("event %q: %w", e.ID, store.ErrConflict)
	}
	return err
}

func (r *eventsRepo) DeleteForStream(ctx context.Context, orgID string, streamID streaming.StreamID) error {
	result := r.Repository.db.WithContext(ctx).
		Where("org_id = ? AND topic_id = ?", orgID, string(streamID)).
		Delete(&eventRow{})
	if result.Error != nil {
		return fmt.Errorf("delete events for stream: %w", result.Error)
	}
	return nil
}

func (r *eventsRepo) ListForStream(ctx context.Context, orgID string, streamID streaming.StreamID, limit int) ([]streaming.Event, error) {
	return r.Repository.Find(ctx,
		store.WithOrg(orgID),
		store.WithCondition("topic_id", string(streamID)),
		store.WithOrderDesc("created_at"),
		store.WithOrderDesc("id"),
		store.WithLimit(limit),
	)
}

func (r *eventsRepo) PageForStream(ctx context.Context, orgID string, streamID streaming.StreamID, limit, offset int) ([]streaming.Event, error) {
	return r.Repository.Find(ctx,
		store.WithOrg(orgID),
		store.WithCondition("topic_id", string(streamID)),
		store.WithOrderDesc("created_at"),
		store.WithOrderDesc("id"),
		store.WithLimit(limit),
		store.WithOffset(offset),
	)
}

func (r *eventsRepo) CountForStream(ctx context.Context, orgID string, streamID streaming.StreamID) (int, error) {
	return r.Repository.Count(ctx,
		store.WithOrg(orgID),
		store.WithCondition("topic_id", string(streamID)),
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

// ListForStreams returns the newest events across a set of streams. It
// replaces the old subscription join: a Worker's inbox is now the union
// of the streams behind its attachments, and a Processor branch's stream
// lives in the branch (a JSON blob), not in a joinable column — so the
// caller resolves the set and this stays one indexed IN query.
func (r *eventsRepo) ListForStreams(ctx context.Context, orgID string, streamIDs []streaming.StreamID, limit int) ([]streaming.Event, error) {
	if len(streamIDs) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(streamIDs))
	for _, id := range streamIDs {
		ids = append(ids, string(id))
	}
	return r.Repository.Find(ctx,
		store.WithOrg(orgID),
		store.WithConditionIn("topic_id", ids),
		store.WithOrderDesc("created_at"),
		store.WithOrderDesc("id"),
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
		store.WithConditionIn("topic_id", ids),
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
