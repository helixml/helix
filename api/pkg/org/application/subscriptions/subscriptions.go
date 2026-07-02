// Package subscriptions is the application service that owns the
// (Worker, Topic) link use cases — Subscribe, Unsubscribe, and the batch
// SubscribeTopics/UnsubscribeTopics — that the MCP subscribe/unsubscribe
// tools, create_bot, and the REST subscribe/unsubscribe handlers drive.
//
// Subscribe is the single primitive: "link worker W to topic S,
// validating both exist, idempotent." The batch methods loop it over
// several topics. One implementation, many callers.
//
// Depends only on the narrow store repositories it touches
// (Subscriptions/Topics/Bots) plus a clock (CLAUDE.md §5.0).
package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// Subscriptions owns the subscription use cases.
type Subscriptions struct {
	subs   store.Subscriptions
	topics store.Topics
	bots   store.Bots
	now    func() time.Time
}

// Deps are the constructor-injected collaborators for New.
type Deps struct {
	Subscriptions store.Subscriptions
	Topics        store.Topics
	Bots          store.Bots
	Now           func() time.Time
}

// New constructs the Subscriptions service.
func New(deps Deps) *Subscriptions {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Subscriptions{subs: deps.Subscriptions, topics: deps.Topics, bots: deps.Bots, now: now}
}

// Subscribe links the Worker to the Topic, validating both exist.
// Idempotent: if the link already exists it returns the existing row
// with created=false and no error. Returns store.ErrNotFound (wrapped)
// when the topic or worker is absent.
func (s *Subscriptions) Subscribe(ctx context.Context, orgID string, workerID orgchart.BotID, topicID streaming.TopicID) (sub streaming.Subscription, created bool, err error) {
	if _, err := s.topics.Get(ctx, orgID, topicID); err != nil {
		return streaming.Subscription{}, false, fmt.Errorf("topic %q: %w", topicID, err)
	}
	if _, err := s.bots.Get(ctx, orgID, workerID); err != nil {
		return streaming.Subscription{}, false, fmt.Errorf("bot %q: %w", workerID, err)
	}
	if existing, err := s.subs.Find(ctx, orgID, workerID, topicID); err == nil {
		return existing, false, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return streaming.Subscription{}, false, err
	}
	newSub, err := streaming.NewSubscription(string(workerID), topicID, s.now(), orgID)
	if err != nil {
		return streaming.Subscription{}, false, err
	}
	if err := s.subs.Create(ctx, newSub); err != nil {
		return streaming.Subscription{}, false, err
	}
	return newSub, true, nil
}

// Unsubscribe drops the (worker, topic) link. Returns store.ErrNotFound
// (wrapped) when no such link exists.
func (s *Subscriptions) Unsubscribe(ctx context.Context, orgID string, workerID orgchart.BotID, topicID streaming.TopicID) error {
	return s.subs.Delete(ctx, orgID, workerID, topicID)
}

// SubscribeTopics subscribes one Bot to several Topics in one call. It
// validates the Bot and every Topic up front (so a bad id fails the
// whole call before any write) then subscribes each via the single
// Subscribe primitive (idempotent per topic). Used by the subscribe tool
// and by lifecycle.Create to subscribe a new Bot at creation.
func (s *Subscriptions) SubscribeTopics(ctx context.Context, orgID string, botID orgchart.BotID, topicIDs []streaming.TopicID) error {
	if len(topicIDs) == 0 {
		return nil
	}
	if _, err := s.bots.Get(ctx, orgID, botID); err != nil {
		return fmt.Errorf("bot %q: %w", botID, err)
	}
	for _, tid := range topicIDs {
		if tid == "" {
			return fmt.Errorf("topicIds contains an empty entry")
		}
		if _, err := s.topics.Get(ctx, orgID, tid); err != nil {
			return fmt.Errorf("topic %q: %w", tid, err)
		}
	}
	for _, tid := range topicIDs {
		if _, _, err := s.Subscribe(ctx, orgID, botID, tid); err != nil {
			return err
		}
	}
	return nil
}

// UnsubscribeTopics drops one Bot's subscription to several Topics. It
// validates the Bot and every Topic up front, then removes each link via
// the single Unsubscribe primitive. Idempotent per topic: a topic the
// Bot isn't subscribed to is a no-op (store.ErrNotFound is swallowed).
func (s *Subscriptions) UnsubscribeTopics(ctx context.Context, orgID string, botID orgchart.BotID, topicIDs []streaming.TopicID) error {
	if len(topicIDs) == 0 {
		return nil
	}
	if _, err := s.bots.Get(ctx, orgID, botID); err != nil {
		return fmt.Errorf("bot %q: %w", botID, err)
	}
	for _, tid := range topicIDs {
		if tid == "" {
			return fmt.Errorf("topicIds contains an empty entry")
		}
		if _, err := s.topics.Get(ctx, orgID, tid); err != nil {
			return fmt.Errorf("topic %q: %w", tid, err)
		}
	}
	for _, tid := range topicIDs {
		if err := s.Unsubscribe(ctx, orgID, botID, tid); err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
	}
	return nil
}
