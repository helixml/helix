package stripe

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/price"
	"github.com/stripe/stripe-go/v76/subscription"
)

// CreateTrialSubscription creates a Stripe subscription with a trial period and
// no payment method attached. At trial end Stripe auto-cancels because there's
// no payment method and MissingPaymentMethod is set to "cancel".
//
// The webhook (customer.subscription.created) will sync status=trialing onto
// the wallet asynchronously. Callers that need synchronous state can read it
// from the returned Subscription.
func (s *Stripe) CreateTrialSubscription(_ context.Context, wallet *types.Wallet, days int) (*stripe.Subscription, error) {
	if err := s.EnabledError(); err != nil {
		return nil, err
	}
	if wallet.StripeCustomerID == "" {
		return nil, fmt.Errorf("wallet has no stripe customer id")
	}
	if days <= 0 {
		return nil, fmt.Errorf("trial days must be positive")
	}

	priceList := price.List(&stripe.PriceListParams{
		LookupKeys: stripe.StringSlice([]string{s.cfg.OrgPriceLookupKey}),
	})
	var p *stripe.Price
	for priceList.Next() {
		p = priceList.Price()
	}
	if p == nil {
		return nil, fmt.Errorf("price not found for lookup key %s", s.cfg.OrgPriceLookupKey)
	}

	params := &stripe.SubscriptionParams{
		Customer: stripe.String(wallet.StripeCustomerID),
		Items: []*stripe.SubscriptionItemsParams{
			{Price: stripe.String(p.ID)},
		},
		TrialPeriodDays: stripe.Int64(int64(days)),
		TrialSettings: &stripe.SubscriptionTrialSettingsParams{
			EndBehavior: &stripe.SubscriptionTrialSettingsEndBehaviorParams{
				MissingPaymentMethod: stripe.String("cancel"),
			},
		},
	}
	params.AddMetadata("user_id", wallet.UserID)
	params.AddMetadata("org_id", wallet.OrgID)
	params.AddMetadata("trial_source", "admin_granted")

	sub, err := subscription.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create trial subscription: %w", err)
	}
	return sub, nil
}

// CancelTrialSubscription cancels a Stripe subscription immediately. The
// webhook (customer.subscription.deleted) marks the wallet as canceled.
func (s *Stripe) CancelTrialSubscription(_ context.Context, subscriptionID string) error {
	if err := s.EnabledError(); err != nil {
		return err
	}
	if subscriptionID == "" {
		return fmt.Errorf("subscription id is required")
	}
	if _, err := subscription.Cancel(subscriptionID, nil); err != nil {
		return fmt.Errorf("failed to cancel subscription: %w", err)
	}
	return nil
}
