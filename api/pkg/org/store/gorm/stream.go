package gorm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/transport"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/api/pkg/org/domain"
	"github.com/helixml/helix/api/pkg/org/store"
)

type streamRow struct {
	ID              string `gorm:"primaryKey;type:text"`
	Name            string `gorm:"not null;uniqueIndex"`
	Description     string
	CreatedBy       string `gorm:"not null;index"`
	CreatedAt       time.Time
	TransportKind   string `gorm:"not null;default:local"`
	TransportConfig string `gorm:"not null;default:''"`
}

func (streamRow) TableName() string { return "streams" }

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

func (r *streamsRepo) Get(ctx context.Context, id stream.ID) (domain.Stream, error) {
	var row streamRow
	err := r.db.WithContext(ctx).First(&row, "id = ?", string(id)).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Stream{}, fmt.Errorf("stream %q: %w", id, store.ErrNotFound)
		}
		return domain.Stream{}, fmt.Errorf("get stream %q: %w", id, err)
	}
	return rowToStream(row)
}

func (r *streamsRepo) List(ctx context.Context) ([]domain.Stream, error) {
	var rows []streamRow
	if err := r.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list streams: %w", err)
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
		Name:            s.Name,
		Description:     s.Description,
		CreatedBy:       string(s.CreatedBy),
		CreatedAt:       s.CreatedAt,
		TransportKind:   string(s.Transport.Kind),
		TransportConfig: cfg,
	}, nil
}

func rowToStream(row streamRow) (domain.Stream, error) {
	transport := transport.Transport{Kind: transport.Kind(row.TransportKind)}
	if row.TransportConfig != "" {
		transport.Config = json.RawMessage(row.TransportConfig)
	}
	return domain.NewStream(
		stream.ID(row.ID),
		row.Name,
		row.Description,
		worker.ID(row.CreatedBy),
		row.CreatedAt,
		transport,
	)
}
