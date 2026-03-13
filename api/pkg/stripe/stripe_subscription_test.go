package stripe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/form"
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

type mockSubscriptionBackend struct {
	t             *testing.T
	expectedPath  string
	subscription  *stripe.Subscription
	callInvoked   bool
}

func (m *mockSubscriptionBackend) Call(method, path, _ string, _ stripe.ParamsContainer, v stripe.LastResponseSetter) error {
	require.Equal(m.t, http.MethodGet, method)
	require.Equal(m.t, m.expectedPath, path)
	require.NotNil(m.t, v)

	sub, ok := v.(*stripe.Subscription)
	require.True(m.t, ok)
	*sub = *m.subscription
	m.callInvoked = true
	return nil
}

func (m *mockSubscriptionBackend) CallStreaming(string, string, string, stripe.ParamsContainer, stripe.StreamingLastResponseSetter) error {
	return fmt.Errorf("unexpected CallStreaming invocation")
}

func (m *mockSubscriptionBackend) CallRaw(string, string, string, *form.Values, *stripe.Params, stripe.LastResponseSetter) error {
	return fmt.Errorf("unexpected CallRaw invocation")
}

func (m *mockSubscriptionBackend) CallMultipart(string, string, string, string, *bytes.Buffer, *stripe.Params, stripe.LastResponseSetter) error {
	return fmt.Errorf("unexpected CallMultipart invocation")
}

func (m *mockSubscriptionBackend) SetMaxNetworkRetries(int64) {}

type mockProductBackend struct {
	t               *testing.T
	expectedPath    string
	product         *stripe.Product
	err             error
	callInvoked     bool
}

func (m *mockProductBackend) Call(method, path, _ string, _ stripe.ParamsContainer, v stripe.LastResponseSetter) error {
	require.Equal(m.t, http.MethodGet, method)
	require.Equal(m.t, m.expectedPath, path)
	require.NotNil(m.t, v)
	m.callInvoked = true
	if m.err != nil {
		return m.err
	}

	prod, ok := v.(*stripe.Product)
	require.True(m.t, ok)
	*prod = *m.product
	return nil
}

func (m *mockProductBackend) CallStreaming(string, string, string, stripe.ParamsContainer, stripe.StreamingLastResponseSetter) error {
	return fmt.Errorf("unexpected CallStreaming invocation")
}

func (m *mockProductBackend) CallRaw(string, string, string, *form.Values, *stripe.Params, stripe.LastResponseSetter) error {
	return fmt.Errorf("unexpected CallRaw invocation")
}

func (m *mockProductBackend) CallMultipart(string, string, string, string, *bytes.Buffer, *stripe.Params, stripe.LastResponseSetter) error {
	return fmt.Errorf("unexpected CallMultipart invocation")
}

func (m *mockProductBackend) SetMaxNetworkRetries(int64) {}

func TestSyncSubscription_UpdatesAndPersistsWallet(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	store := store.NewMockStore(ctrl)
	s := NewStripe(config.Stripe{
		SecretKey: "sk_test_sync",
	}, store)

	wallet := &types.Wallet{
		ID:                   "wallet_123",
		StripeSubscriptionID: "sub_sync_123",
	}

	mockBackend := &mockSubscriptionBackend{
		t:            t,
		expectedPath: "/v1/subscriptions/sub_sync_123",
		subscription: &stripe.Subscription{
			ID:                 "sub_sync_123",
			Status:             stripe.SubscriptionStatusPastDue,
			CurrentPeriodStart: 1111,
			CurrentPeriodEnd:   2222,
			CancelAtPeriodEnd:  true,
		},
	}

	originalAPIBackend := stripe.GetBackend(stripe.APIBackend)
	stripe.SetBackend(stripe.APIBackend, mockBackend)
	t.Cleanup(func() {
		stripe.SetBackend(stripe.APIBackend, originalAPIBackend)
	})

	store.EXPECT().UpdateWallet(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, got *types.Wallet) (*types.Wallet, error) {
		require.Equal(t, wallet.ID, got.ID)
		require.Equal(t, wallet.StripeSubscriptionID, got.StripeSubscriptionID)
		require.Equal(t, stripe.SubscriptionStatusPastDue, got.SubscriptionStatus)
		require.Equal(t, int64(1111), got.SubscriptionCurrentPeriodStart)
		require.Equal(t, int64(2222), got.SubscriptionCurrentPeriodEnd)
		require.True(t, got.SubscriptionCancelAtPeriodEnd)
		return got, nil
	})

	s.SyncSubscription(context.Background(), wallet)

	require.True(t, mockBackend.callInvoked)
	require.Equal(t, stripe.SubscriptionStatusPastDue, wallet.SubscriptionStatus)
	require.Equal(t, int64(1111), wallet.SubscriptionCurrentPeriodStart)
	require.Equal(t, int64(2222), wallet.SubscriptionCurrentPeriodEnd)
	require.True(t, wallet.SubscriptionCancelAtPeriodEnd)
}
