package gorm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/api/pkg/org/domain"
	"github.com/helixml/helix/api/pkg/org/store"
)

type subscriptionRow struct {
	WorkerID  string `gorm:"primaryKey;type:text"`
	StreamID  string `gorm:"primaryKey;type:text"`
	CreatedAt time.Time
}

func (subscriptionRow) TableName() string { return "org_subscriptions" }

type subscriptionsRepo struct {
	db *gorm.DB
}

func (r *subscriptionsRepo) Create(ctx context.Context, sub domain.Subscription) error {
	row := subscriptionToRow(sub)
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("create subscription: %w", err)
	}
	return nil
}

func (r *subscriptionsRepo) Delete(ctx context.Context, workerID worker.ID, streamID stream.ID) error {
	res := r.db.WithContext(ctx).Delete(&subscriptionRow{}, "worker_id = ? AND stream_id = ?", string(workerID), string(streamID))
	if res.Error != nil {
		return fmt.Errorf("delete subscription (%q,%q): %w", workerID, streamID, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("subscription (%q,%q): %w", workerID, streamID, store.ErrNotFound)
	}
	return nil
}

func (r *subscriptionsRepo) Find(ctx context.Context, workerID worker.ID, streamID stream.ID) (domain.Subscription, error) {
	var row subscriptionRow
	err := r.db.WithContext(ctx).Where("worker_id = ? AND stream_id = ?", string(workerID), string(streamID)).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Subscription{}, fmt.Errorf("subscription (%q,%q): %w", workerID, streamID, store.ErrNotFound)
		}
		return domain.Subscription{}, fmt.Errorf("find subscription (%q,%q): %w", workerID, streamID, err)
	}
	return rowToSubscription(row)
}

func (r *subscriptionsRepo) ListForWorker(ctx context.Context, workerID worker.ID) ([]domain.Subscription, error) {
	var rows []subscriptionRow
	if err := r.db.WithContext(ctx).Where("worker_id = ?", string(workerID)).Order("stream_id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list subscriptions for worker %q: %w", workerID, err)
	}
	return rowsToSubscriptions(rows)
}

func (r *subscriptionsRepo) ListForStream(ctx context.Context, streamID stream.ID) ([]domain.Subscription, error) {
	var rows []subscriptionRow
	if err := r.db.WithContext(ctx).Where("stream_id = ?", string(streamID)).Order("worker_id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list subscriptions for stream %q: %w", streamID, err)
	}
	return rowsToSubscriptions(rows)
}

func subscriptionToRow(sub domain.Subscription) subscriptionRow {
	return subscriptionRow{
		WorkerID:  string(sub.WorkerID),
		StreamID:  string(sub.StreamID),
		CreatedAt: sub.CreatedAt,
	}
}

func rowToSubscription(row subscriptionRow) (domain.Subscription, error) {
	return domain.NewSubscription(
		worker.ID(row.WorkerID),
		stream.ID(row.StreamID),
		row.CreatedAt,
	)
}

func rowsToSubscriptions(rows []subscriptionRow) ([]domain.Subscription, error) {
	out := make([]domain.Subscription, 0, len(rows))
	for _, row := range rows {
		s, err := rowToSubscription(row)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}
