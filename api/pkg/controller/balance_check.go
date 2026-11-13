package controller

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/wallet"
)

func (c *Controller) HasEnoughBalance(ctx context.Context, user *types.User, orgID string, clientBillingEnabled bool) (bool, error) {
	// Skip balance check for runner tokens (system-level access)
	// Runner tokens are used by internal services like Kodit for enrichments
	if user.TokenType == types.TokenTypeRunner {
		return true, nil
	}

	if user.Waitlisted {
		return false, fmt.Errorf("user is waitlisted")
	}

	return wallet.HasEnoughBalance(ctx, c.Options.Store, c.Options.Config.Stripe, orgID, user.ID, clientBillingEnabled)
}
