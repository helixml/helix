package gorm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain"
	"github.com/helixml/helix/api/pkg/org/store"
	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/worker"
)

type subscriptionRow struct {
	OrgID     string `gorm:"primaryKey;type:text;index"`
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

func (r *subscriptionsRepo) Delete(ctx context.Context, orgID string, workerID worker.ID, streamID stream.ID) error {
	res := r.db.WithContext(ctx).Delete(&subscriptionRow{}, "org_id = ? AND worker_id = ? AND stream_id = ?", orgID, string(workerID), string(streamID))
	if res.Error != nil {
		return fmt.Errorf("delete subscription (%q,%q,%q): %w", orgID, workerID, streamID, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("subscription (%q,%q,%q): %w", orgID, workerID, streamID, store.ErrNotFound)
	}
	return nil
}

func (r *subscriptionsRepo) Find(ctx context.Context, orgID string, workerID worker.ID, streamID stream.ID) (domain.Subscription, error) {
	var row subscriptionRow
	err := r.db.WithContext(ctx).Where("org_id = ? AND worker_id = ? AND stream_id = ?", orgID, string(workerID), string(streamID)).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Subscription{}, fmt.Errorf("subscription (%q,%q,%q): %w", orgID, workerID, streamID, store.ErrNotFound)
		}
		return domain.Subscription{}, fmt.Errorf("find subscription (%q,%q,%q): %w", orgID, workerID, streamID, err)
	}
	return rowToSubscription(row)
}

func (r *subscriptionsRepo) ListForWorker(ctx context.Context, orgID string, workerID worker.ID) ([]domain.Subscription, error) {
	var rows []subscriptionRow
	if err := r.db.WithContext(ctx).Where("org_id = ? AND worker_id = ?", orgID, string(workerID)).Order("stream_id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list subscriptions for worker %q in org %q: %w", workerID, orgID, err)
	}
	return rowsToSubscriptions(rows)
}

func (r *subscriptionsRepo) ListForStream(ctx context.Context, orgID string, streamID stream.ID) ([]domain.Subscription, error) {
	var rows []subscriptionRow
	if err := r.db.WithContext(ctx).Where("org_id = ? AND stream_id = ?", orgID, string(streamID)).Order("worker_id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list subscriptions for stream %q in org %q: %w", streamID, orgID, err)
	}
	return rowsToSubscriptions(rows)
}

func subscriptionToRow(sub domain.Subscription) subscriptionRow {
	return subscriptionRow{
		OrgID:     sub.OrganizationID,
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
		row.OrgID,
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
