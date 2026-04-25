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

type streamRow struct {
	ID        string `gorm:"primaryKey;type:text"`
	WorkerID  string `gorm:"not null;uniqueIndex:idx_stream_worker_channel"`
	ChannelID string `gorm:"not null;uniqueIndex:idx_stream_worker_channel"`
	CreatedAt time.Time
}

func (streamRow) TableName() string { return "streams" }

type streamsRepo struct {
	db *gorm.DB
}

func (r *streamsRepo) Create(ctx context.Context, s domain.Stream) error {
	row := streamToRow(s)
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("create stream: %w", err)
	}
	return nil
}

func (r *streamsRepo) Delete(ctx context.Context, id domain.StreamID) error {
	res := r.db.WithContext(ctx).Delete(&streamRow{}, "id = ?", string(id))
	if res.Error != nil {
		return fmt.Errorf("delete stream %q: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("stream %q: %w", id, store.ErrNotFound)
	}
	return nil
}

func (r *streamsRepo) FindForWorkerAndChannel(ctx context.Context, workerID domain.WorkerID, channelID domain.ChannelID) (domain.Stream, error) {
	var row streamRow
	err := r.db.WithContext(ctx).Where("worker_id = ? AND channel_id = ?", string(workerID), string(channelID)).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Stream{}, fmt.Errorf("stream for worker %q channel %q: %w", workerID, channelID, store.ErrNotFound)
		}
		return domain.Stream{}, fmt.Errorf("find stream for worker %q channel %q: %w", workerID, channelID, err)
	}
	return rowToStream(row)
}

func (r *streamsRepo) ListForWorker(ctx context.Context, workerID domain.WorkerID) ([]domain.Stream, error) {
	var rows []streamRow
	if err := r.db.WithContext(ctx).Where("worker_id = ?", string(workerID)).Order("channel_id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list streams for worker %q: %w", workerID, err)
	}
	return rowsToStreams(rows)
}

func (r *streamsRepo) ListForChannel(ctx context.Context, channelID domain.ChannelID) ([]domain.Stream, error) {
	var rows []streamRow
	if err := r.db.WithContext(ctx).Where("channel_id = ?", string(channelID)).Order("worker_id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list streams for channel %q: %w", channelID, err)
	}
	return rowsToStreams(rows)
}

func streamToRow(s domain.Stream) streamRow {
	return streamRow{
		ID:        string(s.ID),
		WorkerID:  string(s.WorkerID),
		ChannelID: string(s.ChannelID),
		CreatedAt: s.CreatedAt,
	}
}

func rowToStream(row streamRow) (domain.Stream, error) {
	return domain.NewStream(
		domain.StreamID(row.ID),
		domain.WorkerID(row.WorkerID),
		domain.ChannelID(row.ChannelID),
		row.CreatedAt,
	)
}

func rowsToStreams(rows []streamRow) ([]domain.Stream, error) {
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
