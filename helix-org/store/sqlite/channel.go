package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/store"
)

type channelRow struct {
	ID          string `gorm:"primaryKey;type:text"`
	Name        string `gorm:"not null;uniqueIndex"`
	Description string
	CreatedBy   string `gorm:"not null;index"`
	CreatedAt   time.Time
}

func (channelRow) TableName() string { return "channels" }

type channelsRepo struct {
	db *gorm.DB
}

func (r *channelsRepo) Create(ctx context.Context, ch domain.Channel) error {
	row := channelToRow(ch)
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("create channel: %w", err)
	}
	return nil
}

func (r *channelsRepo) Get(ctx context.Context, id domain.ChannelID) (domain.Channel, error) {
	var row channelRow
	err := r.db.WithContext(ctx).First(&row, "id = ?", string(id)).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Channel{}, fmt.Errorf("channel %q: %w", id, store.ErrNotFound)
		}
		return domain.Channel{}, fmt.Errorf("get channel %q: %w", id, err)
	}
	return rowToChannel(row)
}

func (r *channelsRepo) List(ctx context.Context) ([]domain.Channel, error) {
	var rows []channelRow
	if err := r.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	out := make([]domain.Channel, 0, len(rows))
	for _, row := range rows {
		ch, err := rowToChannel(row)
		if err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, nil
}

func channelToRow(ch domain.Channel) channelRow {
	return channelRow{
		ID:          string(ch.ID),
		Name:        ch.Name,
		Description: ch.Description,
		CreatedBy:   string(ch.CreatedBy),
		CreatedAt:   ch.CreatedAt,
	}
}

func rowToChannel(row channelRow) (domain.Channel, error) {
	return domain.NewChannel(
		domain.ChannelID(row.ID),
		row.Name,
		row.Description,
		domain.WorkerID(row.CreatedBy),
		row.CreatedAt,
	)
}
