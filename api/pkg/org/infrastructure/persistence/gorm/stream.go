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

type streamRow struct {
	ID              string `gorm:"primaryKey;type:text"`
	OrgID           string `gorm:"primaryKey;type:text;index;uniqueIndex:idx_stream_org_name,priority:1"`
	Name            string `gorm:"not null;uniqueIndex:idx_stream_org_name,priority:2"`
	Description     string
	CreatedBy       string `gorm:"not null;index"`
	CreatedAt       time.Time
	TransportKind   string `gorm:"not null;default:local"`
	TransportConfig string `gorm:"not null;default:''"`
}

func (streamRow) TableName() string { return "org_streams" }

type streamMapper struct{}

func (streamMapper) ToRow(s streaming.Stream) (streamRow, error) {
	cfg := ""
	if len(s.Transport.Config) > 0 {
		cfg = string(s.Transport.Config)
	}
	return streamRow{
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

func (streamMapper) ToDomain(row streamRow) (streaming.Stream, error) {
	tp := transport.Transport{Kind: transport.Kind(row.TransportKind)}
	if row.TransportConfig != "" {
		tp.Config = json.RawMessage(row.TransportConfig)
	}
	return streaming.NewStream(
		streaming.StreamID(row.ID),
		row.Name,
		row.Description,
		orgchart.WorkerID(row.CreatedBy),
		row.CreatedAt,
		tp,
		row.OrgID,
	)
}

type streamsRepo struct {
	*Repository[streaming.Stream, streamRow]
}

func newStreamsRepo(db *gorm.DB) *streamsRepo {
	return &streamsRepo{Repository: NewRepository[streaming.Stream, streamRow](db, streamMapper{}, "stream")}
}

func (r *streamsRepo) Get(ctx context.Context, orgID string, id streaming.StreamID) (streaming.Stream, error) {
	return r.FindOne(ctx, store.WithOrg(orgID), store.WithID(string(id)))
}

func (r *streamsRepo) List(ctx context.Context, orgID string) ([]streaming.Stream, error) {
	return r.Find(ctx, store.WithOrg(orgID), store.WithOrderAsc("id"))
}

// ListByTransportKind returns every stream whose transport_kind matches,
// across every org. The cron stream scheduler uses this to enumerate
// schedules globally before placing them on its in-process gocron
// scheduler. Per-tenant request paths must stay on Get / List.
func (r *streamsRepo) ListByTransportKind(ctx context.Context, kind transport.Kind) ([]streaming.Stream, error) {
	var rows []streamRow
	if err := r.db.WithContext(ctx).
		Where("transport_kind = ?", string(kind)).
		Order("org_id, id").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list streams by transport kind %q: %w", kind, err)
	}
	out := make([]streaming.Stream, 0, len(rows))
	for _, row := range rows {
		s, err := (streamMapper{}).ToDomain(row)
		if err != nil {
			return nil, fmt.Errorf("hydrate stream %s/%s: %w", row.OrgID, row.ID, err)
		}
		out = append(out, s)
	}
	return out, nil
}

// Update rewrites the mutable subset (name, description, transport
// kind + config) of the row identified by (id, orgID). Immutable
// fields on the passed Stream are ignored. Returns store.ErrNotFound
// when no row matches.
func (r *streamsRepo) Update(ctx context.Context, s streaming.Stream) error {
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

// Delete removes the stream row and structurally cascades the
// subscriptions that reference it: every worker-anchored row for this
// stream is dropped in the same transaction, so firing a worker (which
// deletes its s-activations-<id> stream) can't leave other workers'
// subscriptions pointing at a stream that no longer exists.
func (r *streamsRepo) Delete(ctx context.Context, orgID string, id streaming.StreamID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("org_id = ? AND stream_id = ?", orgID, string(id)).
			Delete(&subscriptionRow{}).Error; err != nil {
			return fmt.Errorf("delete stream: drop subscriptions: %w", err)
		}
		res := tx.Where("org_id = ? AND id = ?", orgID, string(id)).Delete(&streamRow{})
		if res.Error != nil {
			return fmt.Errorf("delete stream: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("stream: %w", store.ErrNotFound)
		}
		return nil
	})
}
