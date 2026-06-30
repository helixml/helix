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
	OrgID     string `gorm:"primaryKey;type:text;index"`
	BotID  string `gorm:"primaryKey;type:text"`
	TopicID  string `gorm:"primaryKey;type:text"`
	CreatedAt time.Time
}

func (subscriptionRow) TableName() string { return "org_subscriptions" }

type subscriptionMapper struct{}

func (subscriptionMapper) ToRow(sub streaming.Subscription) (subscriptionRow, error) {
	return subscriptionRow{
		OrgID:     sub.OrganizationID,
		BotID:  string(sub.BotID),
		TopicID:  string(sub.TopicID),
		CreatedAt: sub.CreatedAt,
	}, nil
}

func (subscriptionMapper) ToDomain(row subscriptionRow) (streaming.Subscription, error) {
	return streaming.NewSubscription(
		row.BotID,
		streaming.TopicID(row.TopicID),
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

func (r *subscriptionsRepo) Delete(ctx context.Context, orgID string, botID orgchart.BotID, topicID streaming.TopicID) error {
	return r.Repository.Delete(ctx,
		store.WithOrg(orgID),
		store.WithCondition("bot_id", string(botID)),
		store.WithCondition("topic_id", string(topicID)),
	)
}

func (r *subscriptionsRepo) Find(ctx context.Context, orgID string, botID orgchart.BotID, topicID streaming.TopicID) (streaming.Subscription, error) {
	return r.FindOne(ctx,
		store.WithOrg(orgID),
		store.WithCondition("bot_id", string(botID)),
		store.WithCondition("topic_id", string(topicID)),
	)
}

func (r *subscriptionsRepo) ListForBot(ctx context.Context, orgID string, botID orgchart.BotID) ([]streaming.Subscription, error) {
	return r.Repository.Find(ctx,
		store.WithOrg(orgID),
		store.WithCondition("bot_id", string(botID)),
		store.WithOrderAsc("topic_id"),
	)
}

func (r *subscriptionsRepo) ListForTopic(ctx context.Context, orgID string, topicID streaming.TopicID) ([]streaming.Subscription, error) {
	return r.Repository.Find(ctx,
		store.WithOrg(orgID),
		store.WithCondition("topic_id", string(topicID)),
		store.WithOrderAsc("bot_id"),
	)
}
