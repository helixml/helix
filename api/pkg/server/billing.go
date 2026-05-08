package server

import (
	"context"
	"fmt"
)

func (s *HelixAPIServer) ensureActiveSubscription(ctx context.Context, orgID string) error {
	wallet, err := s.Store.GetWalletByOrg(ctx, orgID)
	if err != nil {
		return fmt.Errorf("failed to get wallet: %w", err)
	}
	if !wallet.IsSubscriptionActive() {
		return fmt.Errorf("organization '%s' does not have an active subscription", orgID)
	}
	return nil
}
