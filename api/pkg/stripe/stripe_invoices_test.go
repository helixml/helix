package stripe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/stretchr/testify/require"
	stripe "github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/form"
	"go.uber.org/mock/gomock"
)

// invoiceWebhookBackend routes the two Stripe API GETs that
// handleInvoicePaymentPaidEvent makes: subscription.Get (used to read
// trial_source metadata) and product.Get (used to read credits metadata).
// Either can be set to a canned response; the matching path's callInvoked
// flag flips when the SDK calls in.
type invoiceWebhookBackend struct {
	t *testing.T

	subscriptionPath     string
	subscription         *stripe.Subscription
	subscriptionErr      error
	subscriptionInvoked  bool

	productPath     string
	product         *stripe.Product
	productErr      error
	productInvoked  bool
}

func (m *invoiceWebhookBackend) Call(method, path, _ string, _ stripe.ParamsContainer, v stripe.LastResponseSetter) error {
	require.Equal(m.t, http.MethodGet, method)
	switch path {
	case m.subscriptionPath:
		m.subscriptionInvoked = true
		if m.subscriptionErr != nil {
			return m.subscriptionErr
		}
		sub, ok := v.(*stripe.Subscription)
		require.True(m.t, ok, "expected *stripe.Subscription for path %s", path)
		*sub = *m.subscription
		return nil
	case m.productPath:
		m.productInvoked = true
		if m.productErr != nil {
			return m.productErr
		}
		prod, ok := v.(*stripe.Product)
		require.True(m.t, ok, "expected *stripe.Product for path %s", path)
		*prod = *m.product
		return nil
	default:
		m.t.Fatalf("unexpected Stripe API call to %s", path)
		return nil
	}
}

func (m *invoiceWebhookBackend) CallStreaming(string, string, string, stripe.ParamsContainer, stripe.StreamingLastResponseSetter) error {
	return fmt.Errorf("unexpected CallStreaming invocation")
}
func (m *invoiceWebhookBackend) CallRaw(string, string, string, *form.Values, *stripe.Params, stripe.LastResponseSetter) error {
	return fmt.Errorf("unexpected CallRaw invocation")
}
func (m *invoiceWebhookBackend) CallMultipart(string, string, string, string, *bytes.Buffer, *stripe.Params, stripe.LastResponseSetter) error {
	return fmt.Errorf("unexpected CallMultipart invocation")
}
func (m *invoiceWebhookBackend) SetMaxNetworkRetries(int64) {}

// installInvoiceWebhookBackend swaps the global Stripe backend for the test
// duration, restoring it in cleanup.
func installInvoiceWebhookBackend(t *testing.T, b *invoiceWebhookBackend) {
	t.Helper()
	original := stripe.GetBackend(stripe.APIBackend)
	stripe.SetBackend(stripe.APIBackend, b)
	t.Cleanup(func() {
		stripe.SetBackend(stripe.APIBackend, original)
	})
}

// nonAdminSubscription is the canned "real paid subscription" used by tests
// that want the auto-credit path to run as today. It has no admin_granted
// metadata and is active, so the skip branch never fires.
func nonAdminSubscription(id string) *stripe.Subscription {
	return &stripe.Subscription{
		ID:       id,
		Status:   stripe.SubscriptionStatusActive,
		Metadata: map[string]string{},
	}
}

// invoiceSubscriptionPath returns the GET path the Stripe SDK uses for
// subscription.Get. Centralised so the test fixture's hard-coded
// sub_1Rv1oVFNNvjhkCqzI3vgc41P doesn't need to be repeated.
const invoiceSubscriptionPath = "/v1/subscriptions/sub_1Rv1oVFNNvjhkCqzI3vgc41P"
const invoiceProductPath = "/v1/products/prod_Sqia5TdP4XGk1x"

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

	backend := &invoiceWebhookBackend{
		t:                t,
		subscriptionPath: invoiceSubscriptionPath,
		subscription:     nonAdminSubscription("sub_1Rv1oVFNNvjhkCqzI3vgc41P"),
		productPath:      invoiceProductPath,
		product: &stripe.Product{
			ID:       "prod_Sqia5TdP4XGk1x",
			Metadata: map[string]string{"credits": "250"},
		},
	}
	installInvoiceWebhookBackend(t, backend)

	store.EXPECT().UpdateWalletBalance(gomock.Any(), "123", float64(250), types.TransactionMetadata{
		TransactionType: types.TransactionTypeSubscription,
	}).Return(wallet, nil)

	err = s.handleInvoicePaymentPaidEvent(event)
	require.NoError(t, err)
	require.True(t, backend.subscriptionInvoked)
	require.True(t, backend.productInvoked)
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

	backend := &invoiceWebhookBackend{
		t:                t,
		subscriptionPath: invoiceSubscriptionPath,
		subscription:     nonAdminSubscription("sub_1Rv1oVFNNvjhkCqzI3vgc41P"),
		productPath:      invoiceProductPath,
		product: &stripe.Product{
			ID:       "prod_Sqia5TdP4XGk1x",
			Metadata: map[string]string{},
		},
	}
	installInvoiceWebhookBackend(t, backend)

	err = s.handleInvoicePaymentPaidEvent(event)
	require.NoError(t, err)
	require.True(t, backend.productInvoked)
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

	backend := &invoiceWebhookBackend{
		t:                t,
		subscriptionPath: invoiceSubscriptionPath,
		subscription:     nonAdminSubscription("sub_1Rv1oVFNNvjhkCqzI3vgc41P"),
		productPath:      invoiceProductPath,
		product: &stripe.Product{
			ID:       "prod_Sqia5TdP4XGk1x",
			Metadata: map[string]string{"credits": "not-a-number"},
		},
	}
	installInvoiceWebhookBackend(t, backend)

	err = s.handleInvoicePaymentPaidEvent(event)
	require.NoError(t, err)
	require.True(t, backend.productInvoked)
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

	backend := &invoiceWebhookBackend{
		t:                t,
		subscriptionPath: invoiceSubscriptionPath,
		subscription:     nonAdminSubscription("sub_1Rv1oVFNNvjhkCqzI3vgc41P"),
		productPath:      invoiceProductPath,
		productErr:       os.ErrNotExist,
	}
	installInvoiceWebhookBackend(t, backend)

	err = s.handleInvoicePaymentPaidEvent(event)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "failed to get product from stripe"))
	require.True(t, backend.productInvoked)
}

// Test_handleInvoicePaymentPaidEvent_AdminGrantedTrial_SkipsAutoCredit pins
// down the fix for the SaaS bug reported by Karolis: an admin-granted trial
// activation produced a wallet balance N+100 instead of N, because the auto
// product.metadata.credits topup stacked on top of the form value. Now any
// subscription whose metadata carries trial_source=admin_granted AND is
// still in trialing status causes the handler to return without crediting.
func Test_handleInvoicePaymentPaidEvent_AdminGrantedTrial_SkipsAutoCredit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	store := store.NewMockStore(ctrl)
	s := NewStripe(config.Stripe{SecretKey: "sk_test_invoice_paid"}, store)

	bts, err := os.ReadFile("testdata/paid.json")
	require.NoError(t, err)
	var event stripe.Event
	require.NoError(t, json.Unmarshal(bts, &event))

	wallet := &types.Wallet{ID: "wal_admin_trial"}
	store.EXPECT().GetWalletByStripeCustomerID(gomock.Any(), "cus_SqicesZoU7LrDR").Return(wallet, nil)

	backend := &invoiceWebhookBackend{
		t:                t,
		subscriptionPath: invoiceSubscriptionPath,
		subscription: &stripe.Subscription{
			ID:     "sub_1Rv1oVFNNvjhkCqzI3vgc41P",
			Status: stripe.SubscriptionStatusTrialing,
			Metadata: map[string]string{
				"trial_source": "admin_granted",
			},
		},
		// Product path intentionally unset: the handler must skip BEFORE
		// product.Get is called. If the product call fires, the routing
		// backend t.Fatals on unexpected path.
	}
	installInvoiceWebhookBackend(t, backend)

	// Critical: no UpdateWalletBalance expectation, gomock will fail the
	// test if the handler tries to credit.
	err = s.handleInvoicePaymentPaidEvent(event)
	require.NoError(t, err)
	require.True(t, backend.subscriptionInvoked, "subscription should have been fetched")
	require.False(t, backend.productInvoked, "product should NOT be fetched when admin-granted trial is detected")
}

// Test_handleInvoicePaymentPaidEvent_AdminGrantedConvertedToPaid pins the
// "once they convert to paid, give them credits normally" promise. After
// the trial converts (status moves to active, possibly because the user
// added a payment method via the Customer Portal), the same trial_source
// metadata is still on the subscription, but the gate's status check
// stops applying so the credit lands.
func Test_handleInvoicePaymentPaidEvent_AdminGrantedConvertedToPaid_StillCredits(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	store := store.NewMockStore(ctrl)
	s := NewStripe(config.Stripe{SecretKey: "sk_test_invoice_paid"}, store)

	bts, err := os.ReadFile("testdata/paid.json")
	require.NoError(t, err)
	var event stripe.Event
	require.NoError(t, json.Unmarshal(bts, &event))

	wallet := &types.Wallet{ID: "wal_converted"}
	store.EXPECT().GetWalletByStripeCustomerID(gomock.Any(), "cus_SqicesZoU7LrDR").Return(wallet, nil)

	backend := &invoiceWebhookBackend{
		t:                t,
		subscriptionPath: invoiceSubscriptionPath,
		subscription: &stripe.Subscription{
			ID:     "sub_1Rv1oVFNNvjhkCqzI3vgc41P",
			Status: stripe.SubscriptionStatusActive, // converted to paid
			Metadata: map[string]string{
				"trial_source": "admin_granted",
			},
		},
		productPath: invoiceProductPath,
		product: &stripe.Product{
			ID:       "prod_Sqia5TdP4XGk1x",
			Metadata: map[string]string{"credits": "200"},
		},
	}
	installInvoiceWebhookBackend(t, backend)

	store.EXPECT().UpdateWalletBalance(gomock.Any(), "wal_converted", float64(200), types.TransactionMetadata{
		TransactionType: types.TransactionTypeSubscription,
	}).Return(wallet, nil)

	err = s.handleInvoicePaymentPaidEvent(event)
	require.NoError(t, err)
	require.True(t, backend.subscriptionInvoked)
	require.True(t, backend.productInvoked)
}

// Test_handleInvoicePaymentPaidEvent_SubscriptionFetchFails_FallsThrough
// ensures a transient Stripe API hiccup on the subscription lookup doesn't
// break the credit flow for normal paid subscriptions. We log and continue.
func Test_handleInvoicePaymentPaidEvent_SubscriptionFetchFails_FallsThrough(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	store := store.NewMockStore(ctrl)
	s := NewStripe(config.Stripe{SecretKey: "sk_test_invoice_paid"}, store)

	bts, err := os.ReadFile("testdata/paid.json")
	require.NoError(t, err)
	var event stripe.Event
	require.NoError(t, json.Unmarshal(bts, &event))

	wallet := &types.Wallet{ID: "wal_sub_err"}
	store.EXPECT().GetWalletByStripeCustomerID(gomock.Any(), "cus_SqicesZoU7LrDR").Return(wallet, nil)

	backend := &invoiceWebhookBackend{
		t:                t,
		subscriptionPath: invoiceSubscriptionPath,
		subscriptionErr:  fmt.Errorf("transient stripe error"),
		productPath:      invoiceProductPath,
		product: &stripe.Product{
			ID:       "prod_Sqia5TdP4XGk1x",
			Metadata: map[string]string{"credits": "150"},
		},
	}
	installInvoiceWebhookBackend(t, backend)

	store.EXPECT().UpdateWalletBalance(gomock.Any(), "wal_sub_err", float64(150), types.TransactionMetadata{
		TransactionType: types.TransactionTypeSubscription,
	}).Return(wallet, nil)

	err = s.handleInvoicePaymentPaidEvent(event)
	require.NoError(t, err)
	require.True(t, backend.subscriptionInvoked)
	require.True(t, backend.productInvoked, "product credit path should run even if subscription fetch failed")
}
