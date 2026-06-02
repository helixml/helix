package gorm

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain"
	"github.com/helixml/helix/api/pkg/org/store"
	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/transport"
	"github.com/helixml/helix/api/pkg/org/worker"
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

func (streamMapper) ToRow(s domain.Stream) (streamRow, error) {
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

func (streamMapper) ToDomain(row streamRow) (domain.Stream, error) {
	tp := transport.Transport{Kind: transport.Kind(row.TransportKind)}
	if row.TransportConfig != "" {
		tp.Config = json.RawMessage(row.TransportConfig)
	}
	return domain.NewStream(
		stream.ID(row.ID),
		row.Name,
		row.Description,
		worker.ID(row.CreatedBy),
		row.CreatedAt,
		tp,
		row.OrgID,
	)
}

type streamsRepo struct {
	*Repository[domain.Stream, streamRow]
}

func newStreamsRepo(db *gorm.DB) *streamsRepo {
	return &streamsRepo{Repository: NewRepository[domain.Stream, streamRow](db, streamMapper{}, "stream")}
}

func (r *streamsRepo) Get(ctx context.Context, orgID string, id stream.ID) (domain.Stream, error) {
	return r.FindOne(ctx, store.WithOrg(orgID), store.WithID(string(id)))
}

func (r *streamsRepo) List(ctx context.Context, orgID string) ([]domain.Stream, error) {
	return r.Find(ctx, store.WithOrg(orgID), store.WithOrderAsc("id"))
}
