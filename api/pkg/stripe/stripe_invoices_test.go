package stripe

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/stretchr/testify/require"
	stripe "github.com/stripe/stripe-go/v76"
	"go.uber.org/mock/gomock"
)

func Test_handleInvoicePaymentPaidEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	store := store.NewMockStore(ctrl)
	s := NewStripe(config.Stripe{
		SecretKey: "sk_test_invoice_paid",
	}, store)

	bts, err := os.ReadFile("testdata/paid.json")
	require.NoError(t, err)

	var event stripe.Event
	err = json.Unmarshal(bts, &event)
	require.NoError(t, err)

	// should get wallet
	wallet := &types.Wallet{
		ID: "123",
	}
	store.EXPECT().GetWalletByStripeCustomerID(gomock.Any(), "cus_SqicesZoU7LrDR").Return(wallet, nil)

	mockBackend := &mockProductBackend{
		t:            t,
		expectedPath: "/v1/products/prod_Sqia5TdP4XGk1x",
		product: &stripe.Product{
			ID:       "prod_Sqia5TdP4XGk1x",
			Metadata: map[string]string{"credits": "250"},
		},
	}

	originalAPIBackend := stripe.GetBackend(stripe.APIBackend)
	stripe.SetBackend(stripe.APIBackend, mockBackend)
	t.Cleanup(func() {
		stripe.SetBackend(stripe.APIBackend, originalAPIBackend)
	})

	store.EXPECT().UpdateWalletBalance(gomock.Any(), "123", float64(250), types.TransactionMetadata{
		TransactionType: types.TransactionTypeSubscription,
	}).Return(wallet, nil)

	err = s.handleInvoicePaymentPaidEvent(event)
	require.NoError(t, err)
	require.True(t, mockBackend.callInvoked)
}

func Test_handleInvoicePaymentPaidEvent_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := store.NewMockStore(ctrl)
	s := NewStripe(config.Stripe{}, db)

	bts, err := os.ReadFile("testdata/paid.json")
	require.NoError(t, err)

	var event stripe.Event
	err = json.Unmarshal(bts, &event)
	require.NoError(t, err)

	db.EXPECT().GetWalletByStripeCustomerID(gomock.Any(), "cus_SqicesZoU7LrDR").Return(nil, store.ErrNotFound)

	err = s.handleInvoicePaymentPaidEvent(event)
	require.NoError(t, err)
}

func Test_handleInvoicePaymentPaidEvent_ProductCreditsMissing(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	store := store.NewMockStore(ctrl)
	s := NewStripe(config.Stripe{
		SecretKey: "sk_test_invoice_paid",
	}, store)

	bts, err := os.ReadFile("testdata/paid.json")
	require.NoError(t, err)

	var event stripe.Event
	err = json.Unmarshal(bts, &event)
	require.NoError(t, err)

	wallet := &types.Wallet{
		ID: "123",
	}
	store.EXPECT().GetWalletByStripeCustomerID(gomock.Any(), "cus_SqicesZoU7LrDR").Return(wallet, nil)

	mockBackend := &mockProductBackend{
		t:            t,
		expectedPath: "/v1/products/prod_Sqia5TdP4XGk1x",
		product: &stripe.Product{
			ID:       "prod_Sqia5TdP4XGk1x",
			Metadata: map[string]string{},
		},
	}

	originalAPIBackend := stripe.GetBackend(stripe.APIBackend)
	stripe.SetBackend(stripe.APIBackend, mockBackend)
	t.Cleanup(func() {
		stripe.SetBackend(stripe.APIBackend, originalAPIBackend)
	})

	err = s.handleInvoicePaymentPaidEvent(event)
	require.NoError(t, err)
	require.True(t, mockBackend.callInvoked)
}

func Test_handleInvoicePaymentPaidEvent_ProductCreditsInvalid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	store := store.NewMockStore(ctrl)
	s := NewStripe(config.Stripe{
		SecretKey: "sk_test_invoice_paid",
	}, store)

	bts, err := os.ReadFile("testdata/paid.json")
	require.NoError(t, err)

	var event stripe.Event
	err = json.Unmarshal(bts, &event)
	require.NoError(t, err)

	wallet := &types.Wallet{
		ID: "123",
	}
	store.EXPECT().GetWalletByStripeCustomerID(gomock.Any(), "cus_SqicesZoU7LrDR").Return(wallet, nil)

	mockBackend := &mockProductBackend{
		t:            t,
		expectedPath: "/v1/products/prod_Sqia5TdP4XGk1x",
		product: &stripe.Product{
			ID:       "prod_Sqia5TdP4XGk1x",
			Metadata: map[string]string{"credits": "not-a-number"},
		},
	}

	originalAPIBackend := stripe.GetBackend(stripe.APIBackend)
	stripe.SetBackend(stripe.APIBackend, mockBackend)
	t.Cleanup(func() {
		stripe.SetBackend(stripe.APIBackend, originalAPIBackend)
	})

	err = s.handleInvoicePaymentPaidEvent(event)
	require.NoError(t, err)
	require.True(t, mockBackend.callInvoked)
}

func Test_handleInvoicePaymentPaidEvent_ProductLookupFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	store := store.NewMockStore(ctrl)
	s := NewStripe(config.Stripe{
		SecretKey: "sk_test_invoice_paid",
	}, store)

	bts, err := os.ReadFile("testdata/paid.json")
	require.NoError(t, err)

	var event stripe.Event
	err = json.Unmarshal(bts, &event)
	require.NoError(t, err)

	wallet := &types.Wallet{
		ID: "123",
	}
	store.EXPECT().GetWalletByStripeCustomerID(gomock.Any(), "cus_SqicesZoU7LrDR").Return(wallet, nil)

	mockBackend := &mockProductBackend{
		t:            t,
		expectedPath: "/v1/products/prod_Sqia5TdP4XGk1x",
		err:          os.ErrNotExist,
	}

	originalAPIBackend := stripe.GetBackend(stripe.APIBackend)
	stripe.SetBackend(stripe.APIBackend, mockBackend)
	t.Cleanup(func() {
		stripe.SetBackend(stripe.APIBackend, originalAPIBackend)
	})

	err = s.handleInvoicePaymentPaidEvent(event)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "failed to get product from stripe"))
	require.True(t, mockBackend.callInvoked)
}
