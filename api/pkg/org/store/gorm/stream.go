package gorm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	OrgID           string `gorm:"primaryKey;type:text;index"`
	Name            string `gorm:"not null;uniqueIndex:idx_stream_org_name,priority:2"`
	Description     string
	CreatedBy       string `gorm:"not null;index"`
	CreatedAt       time.Time
	TransportKind   string `gorm:"not null;default:local"`
	TransportConfig string `gorm:"not null;default:''"`
}

func (streamRow) TableName() string { return "org_streams" }

type streamsRepo struct {
	db *gorm.DB
}

func (r *streamsRepo) Create(ctx context.Context, s domain.Stream) error {
	row, err := streamToRow(s)
	if err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("create stream: %w", err)
	}
	return nil
}

func (r *streamsRepo) Get(ctx context.Context, orgID string, id stream.ID) (domain.Stream, error) {
	var row streamRow
	err := r.db.WithContext(ctx).First(&row, "org_id = ? AND id = ?", orgID, string(id)).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Stream{}, fmt.Errorf("stream %q in org %q: %w", id, orgID, store.ErrNotFound)
		}
		return domain.Stream{}, fmt.Errorf("get stream %q in org %q: %w", id, orgID, err)
	}
	return rowToStream(row)
}

func (r *streamsRepo) List(ctx context.Context, orgID string) ([]domain.Stream, error) {
	var rows []streamRow
	if err := r.db.WithContext(ctx).Where("org_id = ?", orgID).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list streams in org %q: %w", orgID, err)
	}
	out := make([]domain.Stream, 0, len(rows))
	for _, row := range rows {
		s, err := rowToStream(row)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func streamToRow(s domain.Stream) (streamRow, error) {
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

func rowToStream(row streamRow) (domain.Stream, error) {
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
