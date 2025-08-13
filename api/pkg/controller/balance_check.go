package controller

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/types"
)

func (c *Controller) hasEnoughBalance(ctx context.Context, user *types.User, orgID string, client openai.Client) (bool, error) {
	if !c.Options.Config.Stripe.BillingEnabled {
		// Billing not enabled
		return true, nil
	}

	if !client.BillingEnabled() {
		// Billing not enabled for this client
		return true, nil
	}

	var (
		wallet *types.Wallet
		err    error
	)

	if orgID != "" {
		wallet, err = c.Options.Store.GetWalletByOrg(ctx, orgID)
		if err != nil {
			return false, fmt.Errorf("failed to get wallet: %w", err)
		}
	} else {
		wallet, err = c.Options.Store.GetWalletByUser(ctx, user.ID)
		if err != nil {
			return false, fmt.Errorf("failed to get wallet: %w", err)
		}
	}

	if wallet.Balance < c.Options.Config.Stripe.MinimumInferenceBalance {
		return false, nil
	}
	return true, nil
}
