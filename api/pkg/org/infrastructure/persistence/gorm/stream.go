package gorm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

type topicRow struct {
	ID              string `gorm:"primaryKey;type:text"`
	OrgID           string `gorm:"primaryKey;type:text;index;uniqueIndex:idx_topic_org_name,priority:1"`
	Name            string `gorm:"not null;uniqueIndex:idx_topic_org_name,priority:2"`
	Description     string
	CreatedBy       string `gorm:"not null;index"`
	CreatedAt       time.Time
	TransportKind   string `gorm:"not null;default:local"`
	TransportConfig string `gorm:"not null;default:''"`
}

func (topicRow) TableName() string { return "org_topics" }

type topicMapper struct{}

func (topicMapper) ToRow(s streaming.Topic) (topicRow, error) {
	cfg := ""
	if len(s.Transport.Config) > 0 {
		cfg = string(s.Transport.Config)
	}
	return topicRow{
		ID:              string(s.ID),
		OrgID:           s.OrganizationID,
		Name:            s.Name,
		Description:     s.Description,
		CreatedBy:       string(s.CreatedBy),
		CreatedAt:       s.CreatedAt,
		TransportKind:   string(s.Transport.Kind),
		TransportConfig: cfg,
	}, nil
}

func (topicMapper) ToDomain(row topicRow) (streaming.Topic, error) {
	tp := transport.Transport{Kind: transport.Kind(row.TransportKind)}
	if row.TransportConfig != "" {
		tp.Config = json.RawMessage(row.TransportConfig)
	}
	return streaming.NewTopic(
		streaming.TopicID(row.ID),
		row.Name,
		row.Description,
		orgchart.BotID(row.CreatedBy),
		row.CreatedAt,
		tp,
		row.OrgID,
	)
}

type topicsRepo struct {
	*Repository[streaming.Topic, topicRow]
}

func newTopicsRepo(db *gorm.DB) *topicsRepo {
	return &topicsRepo{Repository: NewRepository[streaming.Topic, topicRow](db, topicMapper{}, "topic")}
}

// Create maps a unique-constraint violation (the idx_topic_org_name index)
// to store.ErrConflict so adapters return 409 instead of a leaked driver
// error — mirroring processorsRepo.Create.
func (r *topicsRepo) Create(ctx context.Context, t streaming.Topic) error {
	if err := r.Repository.Create(ctx, t); err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("a topic named %q in this org %w", t.Name, store.ErrConflict)
		}
		return err
	}
	return nil
}

func (r *topicsRepo) Get(ctx context.Context, orgID string, id streaming.TopicID) (streaming.Topic, error) {
	return r.FindOne(ctx, store.WithOrg(orgID), store.WithID(string(id)))
}

func (r *topicsRepo) List(ctx context.Context, orgID string) ([]streaming.Topic, error) {
	return r.Find(ctx, store.WithOrg(orgID), store.WithOrderAsc("id"))
}

// ListByTransportKind returns every topic whose transport_kind matches,
// across every org. The cron topic scheduler uses this to enumerate
// schedules globally before placing them on its in-process gocron
// scheduler. Per-tenant request paths must stay on Get / List.
func (r *topicsRepo) ListByTransportKind(ctx context.Context, kind transport.Kind) ([]streaming.Topic, error) {
	var rows []topicRow
	if err := r.db.WithContext(ctx).
		Where("transport_kind = ?", string(kind)).
		Order("org_id, id").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list topics by transport kind %q: %w", kind, err)
	}
	out := make([]streaming.Topic, 0, len(rows))
	for _, row := range rows {
		s, err := (topicMapper{}).ToDomain(row)
		if err != nil {
			return nil, fmt.Errorf("hydrate topic %s/%s: %w", row.OrgID, row.ID, err)
		}
		out = append(out, s)
	}
	return out, nil
}

// Update rewrites the mutable subset (name, description, transport
// kind + config) of the row identified by (id, orgID). Immutable
// fields on the passed Topic are ignored. Returns store.ErrNotFound
// when no row matches.
func (r *topicsRepo) Update(ctx context.Context, s streaming.Topic) error {
	cfg := ""
	if len(s.Transport.Config) > 0 {
		cfg = string(s.Transport.Config)
	}
	updates := map[string]any{
		"name":             s.Name,
		"description":      s.Description,
		"transport_kind":   string(s.Transport.Kind),
		"transport_config": cfg,
	}
	return r.Repository.Update(ctx,
		store.WithOrg(s.OrganizationID),
		store.WithID(string(s.ID)),
		store.WithUpdates(updates),
	)
}

// Delete removes the topic row and structurally cascades the
// subscriptions that reference it: every worker-anchored row for this
// topic is dropped in the same transaction, so firing a worker (which
// deletes its s-transcript-<id> topic) can't leave other workers'
// subscriptions pointing at a topic that no longer exists.
func (r *topicsRepo) Delete(ctx context.Context, orgID string, id streaming.TopicID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("org_id = ? AND topic_id = ?", orgID, string(id)).
			Delete(&subscriptionRow{}).Error; err != nil {
			return fmt.Errorf("delete topic: drop subscriptions: %w", err)
		}
		res := tx.Where("org_id = ? AND id = ?", orgID, string(id)).Delete(&topicRow{})
		if res.Error != nil {
			return fmt.Errorf("delete topic: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("topic: %w", store.ErrNotFound)
		}
		return nil
	})
}
