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
	TopicID  string    `gorm:"not null;index"`
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
		TopicID:  string(e.TopicID),
		Source:    string(e.Source),
		Body:      e.Body,
		CreatedAt: e.CreatedAt,
	}, nil
}

func (eventMapper) ToDomain(row eventRow) (streaming.Event, error) {
	return streaming.NewEvent(
		streaming.EventID(row.ID),
		streaming.TopicID(row.TopicID),
		orgchart.BotID(row.Source),
		row.Body,
		row.CreatedAt,
		row.OrgID,
	)
}

type eventsRepo struct {
	*Repository[streaming.Event, eventRow]
	bots *botsRepo
}

func newEventsRepo(db *gorm.DB, bots *botsRepo) *eventsRepo {
	return &eventsRepo{
		Repository: NewRepository[streaming.Event, eventRow](db, eventMapper{}, "event"),
		bots:       bots,
	}
}

func (r *eventsRepo) Append(ctx context.Context, e streaming.Event) error {
	return r.Repository.Create(ctx, e)
}

func (r *eventsRepo) ListForTopic(ctx context.Context, orgID string, topicID streaming.TopicID, limit int) ([]streaming.Event, error) {
	return r.Repository.Find(ctx,
		store.WithOrg(orgID),
		store.WithCondition("topic_id", string(topicID)),
		store.WithOrderDesc("created_at"),
		store.WithOrderDesc("id"),
		store.WithLimit(limit),
	)
}

func (r *eventsRepo) PageForTopic(ctx context.Context, orgID string, topicID streaming.TopicID, limit, offset int) ([]streaming.Event, error) {
	return r.Repository.Find(ctx,
		store.WithOrg(orgID),
		store.WithCondition("topic_id", string(topicID)),
		store.WithOrderDesc("created_at"),
		store.WithOrderDesc("id"),
		store.WithLimit(limit),
		store.WithOffset(offset),
	)
}

func (r *eventsRepo) CountForTopic(ctx context.Context, orgID string, topicID streaming.TopicID) (int, error) {
	return r.Repository.Count(ctx,
		store.WithOrg(orgID),
		store.WithCondition("topic_id", string(topicID)),
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

func (r *eventsRepo) ListForBot(ctx context.Context, orgID string, botID orgchart.BotID, limit int) ([]streaming.Event, error) {
	// Subscriptions are bot-anchored. Join events against
	// subscriptions for this bot directly.
	if r.bots == nil {
		return nil, fmt.Errorf("eventsRepo: bots repo not wired")
	}
	if _, err := r.bots.Get(ctx, orgID, botID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("resolve bot for event listing: %w", err)
	}
	return r.Repository.Find(ctx,
		store.WithTable("org_events AS e"),
		store.WithJoin("JOIN org_subscriptions AS s ON s.topic_id = e.topic_id AND s.org_id = e.org_id"),
		store.WithSelect("e.*"),
		store.WithCondition("e.org_id", orgID),
		store.WithCondition("s.bot_id", string(botID)),
		store.WithOrderDesc("e.created_at"),
		store.WithOrderDesc("e.id"),
		store.WithLimit(limit),
	)
}

func (r *eventsRepo) ListSince(ctx context.Context, orgID string, topicIDs []streaming.TopicID, since streaming.EventID, limit int) ([]streaming.Event, error) {
	if len(topicIDs) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(topicIDs))
	for _, s := range topicIDs {
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
