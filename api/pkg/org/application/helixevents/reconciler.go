// Package helixevents keeps each org's single, generic "Helix events"
// Trigger in existence. It is a small reconciler in the same spirit as
// application/slackrouting: run it on org bootstrap (and it is safe to
// run repeatedly) and the org is guaranteed to have exactly one Trigger
// of transport kind helix_events, onto which every Helix event flows
// (spec-task attention events today; project lifecycle, PR, CI,
// membership, … later).
//
// The Trigger id is deterministic (TriggerID, below) so the reconciler,
// the attention-event publisher, and any consumer all agree on its
// identity without a lookup — the same pattern the Slack workspace
// Trigger uses. Consumers route events to Workers with the ordinary
// filter-processor + attachment primitives, keyed on the message's extra
// payload (domain / event_type / project_id); no per-project Triggers.
package helixevents

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
)

// TriggerID is the deterministic id of an org's single Helix events
// Trigger. It is unique per org via the (id, orgID) composite key, so the
// same literal is correct in every org (mirrors the Slack workspace
// Trigger). It doubles as the id of the stream carrying its history.
const TriggerID = "s-helix-events"

const (
	triggerName        = "Helix events"
	triggerDescription = "Org-wide Helix event bus (spec-task attention events, and future event types)."
)

// Reconciler ensures the single Helix events Trigger exists for an org.
// Construct with New. It depends only on the Triggers repository
// (CLAUDE.md helix-org philosophy: small interfaces).
type Reconciler struct {
	triggers store.Triggers
	now      func() time.Time
	logger   *slog.Logger
}

// Deps are the constructor-injected collaborators.
type Deps struct {
	Triggers store.Triggers
	Now      func() time.Time
	Logger   *slog.Logger
}

// New builds a Reconciler. A nil Triggers repo yields a Reconciler whose
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
	return &Reconciler{triggers: deps.Triggers, now: now, logger: logger}
}

// Reconcile ensures the org's single helix_events Trigger exists. Safe to
// call at startup and on every org bootstrap; idempotent. A nil/unwired
// Reconciler is a no-op.
func (r *Reconciler) Reconcile(ctx context.Context, orgID string) error {
	if r == nil || r.triggers == nil {
		return nil
	}
	if orgID == "" {
		return nil
	}

	if rows, err := r.triggers.Find(ctx, store.WithOrg(orgID), store.WithID(TriggerID), store.WithLimit(1)); err != nil {
		return fmt.Errorf("helixevents: lookup trigger: %w", err)
	} else if len(rows) > 0 {
		return nil // already present
	}

	t, err := trigger.New(
		TriggerID,
		orgID,
		triggerName,
		triggerDescription,
		transport.KindHelixEvents,
		nil,
		"", // system-managed: no creator worker
		r.now(),
	)
	if err != nil {
		return fmt.Errorf("helixevents: build trigger: %w", err)
	}
	if err := r.triggers.Create(ctx, t); err != nil {
		// Lost the create race with a concurrent reconcile/publish? A
		// now-present row means the outcome we wanted holds.
		rows, getErr := r.triggers.Find(ctx, store.WithOrg(orgID), store.WithID(TriggerID), store.WithLimit(1))
		if getErr != nil || len(rows) == 0 {
			return fmt.Errorf("helixevents: create trigger: %w", err)
		}
	}
	return nil
}
