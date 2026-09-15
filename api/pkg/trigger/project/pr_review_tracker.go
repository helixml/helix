package project

import (
	"context"
	"sync"
)

// prReviewKey identifies the PR a review works on. Work for different
// repositories or PRs is tracked under different keys and never interacts.
type prReviewKey struct {
	repoID string
	prID   int
}

// prReview is one in-flight review work item for a PR head: queued from the
// push that created it until its session starts, then active until its session
// finishes. superseded is set when a newer push for the same PR cancels it.
type prReview struct {
	head       string
	cancel     context.CancelFunc
	superseded bool
}

// prReviews tracks in-flight review work per repository+PR so a newer push can
// supersede (interrupt or drop) older-head work before it publishes stale
// feedback.
type prReviews struct {
	mu     sync.Mutex
	latest map[prReviewKey]string // newest head with in-flight work
	active map[prReviewKey]*prReview
}

func newPRReviews() *prReviews {
	return &prReviews{
		latest: make(map[prReviewKey]string),
		active: make(map[prReviewKey]*prReview),
	}
}

// begin queues a review for the given head. It returns nil when work for this
// head is already in flight. An active review of an older head for the same PR
// is interrupted immediately, so it cannot publish stale feedback.
func (p *prReviews) begin(key prReviewKey, head string) *prReview {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.latest[key] == head {
		return nil
	}
	p.latest[key] = head
	if cur := p.active[key]; cur != nil && cur.head != head {
		cur.superseded = true
		cur.cancel()
	}
	return &prReview{head: head}
}

// start promotes a queued review to active and returns its session context. It
// fails when a newer push for the same PR arrived while this review was being
// prepared — the queued work for the older head is thereby removed.
func (p *prReviews) start(key prReviewKey, work *prReview, parent context.Context) (context.Context, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.latest[key] != work.head {
		return nil, false
	}
	ctx, cancel := context.WithCancel(parent)
	work.cancel = cancel
	p.active[key] = work
	return ctx, true
}

// release records the terminal state of a review work item and returns whether
// it was superseded by a newer push. Idempotent.
func (p *prReviews) release(key prReviewKey, work *prReview) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.active[key] == work {
		delete(p.active, key)
	}
	if p.latest[key] == work.head {
		delete(p.latest, key)
	}
	if work.cancel != nil {
		work.cancel() // releases the context; no-op when already cancelled
	}
	return work.superseded
}

// superseded reports whether the work item was interrupted by a newer push.
func (p *prReviews) superseded(work *prReview) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return work.superseded
}
