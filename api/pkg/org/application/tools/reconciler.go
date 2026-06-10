package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// RoleReconciler ensures every Role in an org exposes the universal
// read baseline (BaseReadTools). It is the upgrade story for the bug
// described in helixml/helix#2546 — Roles created before the baseline
// existed (or via create_role calls that omitted reads like `managers`
// and `reports`) would otherwise leave their Workers permanently unable
// to introspect the reporting graph or read subscribed streams.
//
// The reconciler is idempotent. A second run on the same org with no
// drift performs no Roles.Update calls — tests assert this by checking
// UpdatedAt is unchanged.
//
// Shape mirrors topology.Reconciler so the two live next to each other
// in helix_org_middleware.ensureBootstrap, both running per-org on the
// first request after process start.
type RoleReconciler struct {
	Store *store.Store
	// Now seams the clock for tests. Falls back to time.Now().UTC().
	Now func() time.Time
}

func (r *RoleReconciler) now() time.Time {
	if r != nil && r.Now != nil {
		return r.Now()
	}
	return time.Now().UTC()
}

// Reconcile loads every Role in the org and, for any Role whose Tools
// list is missing one or more BaseReadTools entries, appends the
// missing names and persists the update. Order is stable (caller-
// supplied tools come first, baseline appended in BaseReadTools order),
// duplicates are dropped, and Roles already at the baseline are
// untouched (no write, no UpdatedAt bump).
//
// A nil or store-less Reconciler is a no-op so runtimes/tests that
// don't wire it degrade gracefully — same convention as
// topology.Reconciler.
func (r *RoleReconciler) Reconcile(ctx context.Context, orgID string) error {
	if r == nil || r.Store == nil {
		return nil
	}
	roles, err := r.Store.Roles.List(ctx, orgID)
	if err != nil {
		return fmt.Errorf("list roles: %w", err)
	}
	now := r.now()
	for _, role := range roles {
		merged := mergeBaseReadTools(role.Tools)
		if sameToolList(role.Tools, merged) {
			continue
		}
		role.Tools = merged
		role.UpdatedAt = now
		if err := r.Store.Roles.Update(ctx, role); err != nil {
			return fmt.Errorf("update role %q: %w", role.ID, err)
		}
	}
	return nil
}

// sameToolList reports element-wise equality. mergeBaseReadTools is
// order-stable when the input already contains the baseline, so an
// in-order comparison is sufficient to detect "no drift". This avoids
// writing (and bumping UpdatedAt on) Roles that are already correct.
func sameToolList(a, b []tool.Name) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
