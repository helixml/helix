package gorm

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

type subscriptionRow struct {
	OrgID      string `gorm:"primaryKey;type:text;index"`
	PositionID string `gorm:"primaryKey;type:text"`
	StreamID   string `gorm:"primaryKey;type:text"`
	CreatedAt  time.Time
}

func (subscriptionRow) TableName() string { return "org_subscriptions" }

type subscriptionMapper struct{}

func (subscriptionMapper) ToRow(sub streaming.Subscription) (subscriptionRow, error) {
	return subscriptionRow{
		OrgID:      sub.OrganizationID,
		PositionID: string(sub.PositionID),
		StreamID:   string(sub.StreamID),
		CreatedAt:  sub.CreatedAt,
	}, nil
}

func (subscriptionMapper) ToDomain(row subscriptionRow) (streaming.Subscription, error) {
	return streaming.NewSubscription(
		row.PositionID,
		streaming.StreamID(row.StreamID),
		row.CreatedAt,
		row.OrgID,
	)
}

type subscriptionsRepo struct {
	*Repository[streaming.Subscription, subscriptionRow]
}

func newSubscriptionsRepo(db *gorm.DB) *subscriptionsRepo {
	return &subscriptionsRepo{Repository: NewRepository[streaming.Subscription, subscriptionRow](db, subscriptionMapper{}, "subscription")}
}

func (r *subscriptionsRepo) Delete(ctx context.Context, orgID string, positionID orgchart.PositionID, streamID streaming.StreamID) error {
	return r.Repository.Delete(ctx,
		store.WithOrg(orgID),
		store.WithCondition("position_id", string(positionID)),
		store.WithCondition("stream_id", string(streamID)),
	)
}

func (r *subscriptionsRepo) Find(ctx context.Context, orgID string, positionID orgchart.PositionID, streamID streaming.StreamID) (streaming.Subscription, error) {
	return r.FindOne(ctx,
		store.WithOrg(orgID),
		store.WithCondition("position_id", string(positionID)),
		store.WithCondition("stream_id", string(streamID)),
	)
}

func (r *subscriptionsRepo) ListForPosition(ctx context.Context, orgID string, positionID orgchart.PositionID) ([]streaming.Subscription, error) {
	return r.Repository.Find(ctx,
		store.WithOrg(orgID),
		store.WithCondition("position_id", string(positionID)),
		store.WithOrderAsc("stream_id"),
	)
}

func (r *subscriptionsRepo) ListForStream(ctx context.Context, orgID string, streamID streaming.StreamID) ([]streaming.Subscription, error) {
	return r.Repository.Find(ctx,
		store.WithOrg(orgID),
		store.WithCondition("stream_id", string(streamID)),
		store.WithOrderAsc("position_id"),
	)
}
