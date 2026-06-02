package gorm

import (
	"context"
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

type subscriptionMapper struct{}

func (subscriptionMapper) ToRow(sub domain.Subscription) (subscriptionRow, error) {
	return subscriptionRow{
		OrgID:     sub.OrganizationID,
		WorkerID:  string(sub.WorkerID),
		StreamID:  string(sub.StreamID),
		CreatedAt: sub.CreatedAt,
	}, nil
}

func (subscriptionMapper) ToDomain(row subscriptionRow) (domain.Subscription, error) {
	return domain.NewSubscription(
		worker.ID(row.WorkerID),
		stream.ID(row.StreamID),
		row.CreatedAt,
		row.OrgID,
	)
}

type subscriptionsRepo struct {
	*Repository[domain.Subscription, subscriptionRow]
}

func newSubscriptionsRepo(db *gorm.DB) *subscriptionsRepo {
	return &subscriptionsRepo{Repository: NewRepository[domain.Subscription, subscriptionRow](db, subscriptionMapper{}, "subscription")}
}

func (r *subscriptionsRepo) Delete(ctx context.Context, orgID string, workerID worker.ID, streamID stream.ID) error {
	return r.Repository.Delete(ctx,
		store.WithOrg(orgID),
		store.WithCondition("worker_id", string(workerID)),
		store.WithCondition("stream_id", string(streamID)),
	)
}

func (r *subscriptionsRepo) Find(ctx context.Context, orgID string, workerID worker.ID, streamID stream.ID) (domain.Subscription, error) {
	return r.FindOne(ctx,
		store.WithOrg(orgID),
		store.WithCondition("worker_id", string(workerID)),
		store.WithCondition("stream_id", string(streamID)),
	)
}

func (r *subscriptionsRepo) ListForWorker(ctx context.Context, orgID string, workerID worker.ID) ([]domain.Subscription, error) {
	return r.Repository.Find(ctx,
		store.WithOrg(orgID),
		store.WithCondition("worker_id", string(workerID)),
		store.WithOrderAsc("stream_id"),
	)
}

func (r *subscriptionsRepo) ListForStream(ctx context.Context, orgID string, streamID stream.ID) ([]domain.Subscription, error) {
	return r.Repository.Find(ctx,
		store.WithOrg(orgID),
		store.WithCondition("stream_id", string(streamID)),
		store.WithOrderAsc("worker_id"),
	)
}
