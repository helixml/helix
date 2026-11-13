// Package wallet holds the rules for charging usage to Helix credits, shared
// by inference and web search.
package wallet

import (
	"context"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// ErrNoActiveSubscription is returned when billing requires a paid plan and
// the organization has none.
var ErrNoActiveSubscription = errors.New("does not have an active subscription")

// For returns the wallet usage is charged to: the organization's when orgID
// is set, otherwise the user's.
func For(ctx context.Context, s store.Store, orgID, userID string) (*types.Wallet, error) {
	if orgID != "" {
		return s.GetWalletByOrg(ctx, orgID)
	}
	return s.GetWalletByUser(ctx, userID)
}

// HasEnoughBalance reports whether billable usage may start. It always may
// when billing is disabled; otherwise the organization must have a paid plan
// when one is required, and, when the client bills usage, the wallet must
// hold at least the minimum inference balance.
func HasEnoughBalance(ctx context.Context, s store.Store, cfg config.Stripe, orgID, userID string, clientBillingEnabled bool) (bool, error) {
	if !cfg.BillingEnabled {
		return true, nil
	}

	if cfg.RequireActiveSubscription {
		subscription, err := s.GetWalletByOrg(ctx, orgID)
		if err != nil {
			return false, fmt.Errorf("failed to get subscription: %w", err)
		}
		if !subscription.HasPaidPlan() {
			return false, fmt.Errorf("organization '%s' %w", orgID, ErrNoActiveSubscription)
		}
	}

	if !clientBillingEnabled {
		return true, nil
	}

	w, err := For(ctx, s, orgID, userID)
	if err != nil {
		return false, fmt.Errorf("failed to get wallet: %w", err)
	}
	return w.Balance >= cfg.MinimumInferenceBalance, nil
}
