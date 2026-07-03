// Package helixevents keeps each org's single, generic "Helix events"
// topic in existence. It is a small reconciler in the same spirit as
// application/slackrouting: run it on org bootstrap (and it is safe to
// run repeatedly) and the org is guaranteed to have exactly one
// inbound-only Topic of transport kind helix_events, onto which every
// Helix event flows (spec-task attention events today; project
// lifecycle, PR, CI, membership, … later).
//
// The topic id is deterministic (TopicID, below) so the reconciler, the
// attention-event publisher, and any consumer all agree on its identity
// without a lookup — the same pattern the Slack workspace topic uses.
// Consumers route events to bots with the ordinary filter-processor +
// subscribe primitives, keyed on the message's extra payload
// (domain / event_type / project_id); no per-project topics.
package helixevents

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// TopicID is the deterministic id of an org's single Helix events topic.
// It is unique per org via the (id, orgID) composite key, so the same
// literal is correct in every org (mirrors the Slack workspace topic).
const TopicID streaming.TopicID = "s-helix-events"

const (
	topicName        = "Helix events"
	topicDescription = "Org-wide Helix event bus (spec-task attention events, and future event types)."
)

// Reconciler ensures the single Helix events topic exists for an org.
// Construct with New. It depends only on the Topics repository (CLAUDE.md
// helix-org philosophy: small interfaces).
type Reconciler struct {
	topics store.Topics
	now    func() time.Time
	logger *slog.Logger
}

// Deps are the constructor-injected collaborators.
type Deps struct {
	Topics store.Topics
	Now    func() time.Time
	Logger *slog.Logger
}

// New builds a Reconciler. A nil Topics repo yields a Reconciler whose
// Reconcile no-ops, so runtimes/tests that don't wire the store degrade
// gracefully.
func New(deps Deps) *Reconciler {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Reconciler{topics: deps.Topics, now: now, logger: logger}
}

// Reconcile ensures the org's single helix_events topic exists. Safe to
// call at startup and on every org bootstrap; idempotent. A nil/unwired
// Reconciler is a no-op. It does NOT touch legacy per-project spectask
// topics — those are cleaned up manually by the operator.
func (r *Reconciler) Reconcile(ctx context.Context, orgID string) error {
	if r == nil || r.topics == nil {
		return nil
	}
	if orgID == "" {
		return nil
	}

	if _, err := r.topics.Get(ctx, orgID, TopicID); err == nil {
		return nil // already present
	} else if !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("helixevents: lookup topic: %w", err)
	}

	topic, err := streaming.NewTopic(
		TopicID,
		topicName,
		topicDescription,
		"", // system-managed: no creator worker
		r.now(),
		transport.Transport{Kind: transport.KindHelixEvents},
		orgID,
	)
	if err != nil {
		return fmt.Errorf("helixevents: build topic: %w", err)
	}
	if err := r.topics.Create(ctx, topic); err != nil {
		// Lost the create race with a concurrent reconcile/publish? A
		// now-present row means the outcome we wanted holds.
		if _, getErr := r.topics.Get(ctx, orgID, TopicID); getErr != nil {
			return fmt.Errorf("helixevents: create topic: %w", err)
		}
	}
	return nil
}
