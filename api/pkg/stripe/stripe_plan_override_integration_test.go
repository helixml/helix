package stripe

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v76"
)

// overrideAfterReadStore sets an admin plan override right after a Stripe
// path has read the wallet, so the Stripe write lands on a stale read.
type overrideAfterReadStore struct {
	store.Store
	t    *testing.T
	plan string
}

func (s *overrideAfterReadStore) GetWalletByStripeCustomerID(ctx context.Context, cusID string) (*types.Wallet, error) {
	wallet, err := s.Store.GetWalletByStripeCustomerID(ctx, cusID)
	require.NoError(s.t, err)
	_, err = s.Store.SetWalletPlanOverride(ctx, wallet.ID, s.plan)
	require.NoError(s.t, err)
	return wallet, nil
}

func TestPlanOverrideSurvivesStripeSyncIntegration(t *testing.T) {
	db := newWebhookTestStore(t)

	newWallet := func(t *testing.T) *types.Wallet {
		suffix := system.GenerateID()
		wallet, err := db.CreateWallet(context.Background(), &types.Wallet{
			OrgID:                "org_plan_" + suffix,
			StripeCustomerID:     "cus_plan_" + suffix,
			StripeSubscriptionID: "sub_plan_" + suffix,
			SubscriptionStatus:   stripe.SubscriptionStatusActive,
		})
		require.NoError(t, err)
		return wallet
	}

	t.Run("subscription webhook after admin override", func(t *testing.T) {
		wallet := newWallet(t)
		s := NewStripe(config.Stripe{}, &overrideAfterReadStore{Store: db, t: t, plan: types.PlanOverridePro})

		require.NoError(t, s.handleSubscriptionEvent(stripeEvent(t, stripe.EventTypeCustomerSubscriptionUpdated, &stripe.Subscription{
			ID:               wallet.StripeSubscriptionID,
			Customer:         &stripe.Customer{ID: wallet.StripeCustomerID},
			Status:           stripe.SubscriptionStatusPastDue,
			CurrentPeriodEnd: 4242,
		})))

		got, err := db.GetWallet(context.Background(), wallet.ID)
		require.NoError(t, err)
		require.Equal(t, types.PlanOverridePro, got.PlanOverride)
		require.Equal(t, stripe.SubscriptionStatusPastDue, got.SubscriptionStatus)
		require.Equal(t, int64(4242), got.SubscriptionCurrentPeriodEnd)
	})

	t.Run("subscription sync from a stale wallet read", func(t *testing.T) {
		wallet := newWallet(t)
		stale, err := db.GetWallet(context.Background(), wallet.ID)
		require.NoError(t, err)

		_, err = db.SetWalletPlanOverride(context.Background(), wallet.ID, types.PlanOverrideFree)
		require.NoError(t, err)

		backend := &mockSubscriptionBackend{
			t:            t,
			expectedPath: "/v1/subscriptions/" + wallet.StripeSubscriptionID,
			subscription: &stripe.Subscription{
				ID:               wallet.StripeSubscriptionID,
				Status:           stripe.SubscriptionStatusCanceled,
				CurrentPeriodEnd: 4343,
			},
		}
		original := stripe.GetBackend(stripe.APIBackend)
		stripe.SetBackend(stripe.APIBackend, backend)
		t.Cleanup(func() { stripe.SetBackend(stripe.APIBackend, original) })

		NewStripe(config.Stripe{SecretKey: "sk_test_sync"}, db).SyncSubscription(context.Background(), stale, false)
		require.True(t, backend.callInvoked)

		got, err := db.GetWallet(context.Background(), wallet.ID)
		require.NoError(t, err)
		require.Equal(t, types.PlanOverrideFree, got.PlanOverride)
		require.Equal(t, stripe.SubscriptionStatusCanceled, got.SubscriptionStatus)
		require.Equal(t, int64(4343), got.SubscriptionCurrentPeriodEnd)
		// The caller's wallet reflects the stored row, override included.
		require.Equal(t, types.PlanOverrideFree, stale.PlanOverride)
	})
}
