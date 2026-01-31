package stripe

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v76"
	"go.uber.org/mock/gomock"
)

func Test_handleSubscriptionEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	store := store.NewMockStore(ctrl)
	s := NewStripe(config.Stripe{}, store)

	bts, err := os.ReadFile("testdata/sub_active.json")
	require.NoError(t, err)

	var event stripe.Event
	err = json.Unmarshal(bts, &event)
	require.NoError(t, err)

	// Get wallet
	wallet := &types.Wallet{
		ID: "123",
	}
	store.EXPECT().GetWalletByStripeCustomerID(gomock.Any(), "cus_SqicesZoU7LrDR").Return(wallet, nil)

	// Should update wallet with sub info
	store.EXPECT().UpdateWallet(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, wallet *types.Wallet) (*types.Wallet, error) {
		require.Equal(t, wallet.StripeSubscriptionID, "sub_1Rv1oVFNNvjhkCqzI3vgc41P")
		require.Equal(t, wallet.SubscriptionCurrentPeriodStart, int64(1754942123))
		require.Equal(t, wallet.SubscriptionCurrentPeriodEnd, int64(1757620523))
		require.Equal(t, wallet.SubscriptionCreated, int64(1754942123))
		require.Equal(t, wallet.SubscriptionStatus, stripe.SubscriptionStatusActive)
		return wallet, nil
	})

	err = s.handleSubscriptionEvent(event)
	require.NoError(t, err)

}

func Test_handleSubscriptionEvent_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := store.NewMockStore(ctrl)
	s := NewStripe(config.Stripe{}, db)

	bts, err := os.ReadFile("testdata/sub_active.json")
	require.NoError(t, err)

	var event stripe.Event
	err = json.Unmarshal(bts, &event)
	require.NoError(t, err)

	db.EXPECT().GetWalletByStripeCustomerID(gomock.Any(), "cus_SqicesZoU7LrDR").Return(nil, store.ErrNotFound)

	err = s.handleSubscriptionEvent(event)
	require.NoError(t, err)

}
