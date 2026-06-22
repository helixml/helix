// Package subscriptions is the application service that owns the
// (Worker, Topic) link use cases — Subscribe, Unsubscribe, Invite —
// that the MCP subscribe/unsubscribe/invite_workers tools and the REST
// subscribe/unsubscribe handlers used to each implement independently.
//
// Subscribe is the single primitive: "link worker W to topic S,
// validating both exist, idempotent." The MCP subscribe tool passes the
// caller's own id; the REST handler passes a path worker; Invite loops
// over many. One implementation, three callers.
//
// Depends only on the narrow store repositories it touches
// (Subscriptions/Topics/Workers) plus a clock (CLAUDE.md §5.0).
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
	subs    store.Subscriptions
	topics store.Topics
	workers store.Workers
	now     func() time.Time
}

// Deps are the constructor-injected collaborators for New.
type Deps struct {
	Subscriptions store.Subscriptions
	Topics       store.Topics
	Workers       store.Workers
	Now           func() time.Time
}

// New constructs the Subscriptions service.
func New(deps Deps) *Subscriptions {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Subscriptions{subs: deps.Subscriptions, topics: deps.Topics, workers: deps.Workers, now: now}
}

// Subscribe links the Worker to the Topic, validating both exist.
// Idempotent: if the link already exists it returns the existing row
// with created=false and no error. Returns store.ErrNotFound (wrapped)
// when the topic or worker is absent.
func (s *Subscriptions) Subscribe(ctx context.Context, orgID string, workerID orgchart.WorkerID, topicID streaming.TopicID) (sub streaming.Subscription, created bool, err error) {
	if _, err := s.topics.Get(ctx, orgID, topicID); err != nil {
		return streaming.Subscription{}, false, fmt.Errorf("topic %q: %w", topicID, err)
	}
	if _, err := s.workers.Get(ctx, orgID, workerID); err != nil {
		return streaming.Subscription{}, false, fmt.Errorf("worker %q: %w", workerID, err)
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
func (s *Subscriptions) Unsubscribe(ctx context.Context, orgID string, workerID orgchart.WorkerID, topicID streaming.TopicID) error {
	return s.subs.Delete(ctx, orgID, workerID, topicID)
}

// Invite subscribes several Workers to one Topic, validating the
// topic and every worker up front (so a bad id fails the whole call
// before any write). Idempotent per worker. Used to open DMs / pull
// colleagues into a thread.
func (s *Subscriptions) Invite(ctx context.Context, orgID string, topicID streaming.TopicID, workerIDs []orgchart.WorkerID) error {
	if len(workerIDs) == 0 {
		return fmt.Errorf("workerIds must contain at least one worker")
	}
	if _, err := s.topics.Get(ctx, orgID, topicID); err != nil {
		return fmt.Errorf("topic %q: %w", topicID, err)
	}
	for _, wid := range workerIDs {
		if wid == "" {
			return fmt.Errorf("workerIds contains an empty entry")
		}
		if _, err := s.workers.Get(ctx, orgID, wid); err != nil {
			return fmt.Errorf("worker %q: %w", wid, err)
		}
	}
	for _, wid := range workerIDs {
		if _, err := s.subs.Find(ctx, orgID, wid, topicID); err == nil {
			continue
		} else if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		sub, err := streaming.NewSubscription(string(wid), topicID, s.now(), orgID)
		if err != nil {
			return err
		}
		if err := s.subs.Create(ctx, sub); err != nil {
			return err
		}
	}
	return nil
}
