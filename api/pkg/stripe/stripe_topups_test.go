package stripe

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	stripe "github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/form"
	"github.com/stripe/stripe-go/v76/webhook"
	"go.uber.org/mock/gomock"
)

type topUpSessionBackend struct {
	t      *testing.T
	params *stripe.CheckoutSessionParams
}

func (b *topUpSessionBackend) Call(method, path, _ string, params stripe.ParamsContainer, v stripe.LastResponseSetter) error {
	require.Equal(b.t, http.MethodPost, method)
	require.Equal(b.t, "/v1/checkout/sessions", path)

	checkoutParams, ok := params.(*stripe.CheckoutSessionParams)
	require.True(b.t, ok)
	b.params = checkoutParams

	session, ok := v.(*stripe.CheckoutSession)
	require.True(b.t, ok)
	session.URL = "https://checkout.stripe.test/session"
	return nil
}

func (b *topUpSessionBackend) CallStreaming(string, string, string, stripe.ParamsContainer, stripe.StreamingLastResponseSetter) error {
	return errors.New("unexpected CallStreaming invocation")
}

func (b *topUpSessionBackend) CallRaw(string, string, string, *form.Values, *stripe.Params, stripe.LastResponseSetter) error {
	return errors.New("unexpected CallRaw invocation")
}

func (b *topUpSessionBackend) CallMultipart(string, string, string, string, *bytes.Buffer, *stripe.Params, stripe.LastResponseSetter) error {
	return errors.New("unexpected CallMultipart invocation")
}

func (b *topUpSessionBackend) SetMaxNetworkRetries(int64) {}

func TestGetTopUpSessionURLRequiresPromoCreditPriceID(t *testing.T) {
	backend := &topUpSessionBackend{t: t}
	originalBackend := stripe.GetBackend(stripe.APIBackend)
	stripe.SetBackend(stripe.APIBackend, backend)
	t.Cleanup(func() { stripe.SetBackend(stripe.APIBackend, originalBackend) })

	s := NewStripe(config.Stripe{
		SecretKey:            "sk_test",
		WebhookSigningSecret: "whsec_test",
	}, nil)

	_, err := s.GetTopUpSessionURL(TopUpSessionParams{Amount: 5})
	require.EqualError(t, err, "stripe promo credit price ID is required")
	require.Nil(t, backend.params)
}

func TestGetTopUpSessionURLUsesPromoCreditPriceForFiveDollars(t *testing.T) {
	backend := &topUpSessionBackend{t: t}
	originalBackend := stripe.GetBackend(stripe.APIBackend)
	stripe.SetBackend(stripe.APIBackend, backend)
	t.Cleanup(func() { stripe.SetBackend(stripe.APIBackend, originalBackend) })

	s := NewStripe(config.Stripe{
		AppURL:               "https://app.helix.test",
		SecretKey:            "sk_test",
		WebhookSigningSecret: "whsec_test",
		PromoCreditPriceID:   "price_promo_credits",
	}, nil)

	url, err := s.GetTopUpSessionURL(TopUpSessionParams{
		StripeCustomerID: "cus_123",
		UserID:           "user_123",
		Amount:           5,
	})
	require.NoError(t, err)
	require.Equal(t, "https://checkout.stripe.test/session", url)
	require.True(t, *backend.params.AllowPromotionCodes)
	require.Equal(t, stripe.CheckoutSessionModePayment, stripe.CheckoutSessionMode(*backend.params.Mode))
	require.Len(t, backend.params.LineItems, 1)
	require.Equal(t, "price_promo_credits", *backend.params.LineItems[0].Price)
	require.Nil(t, backend.params.LineItems[0].PriceData)
}

func TestGetTopUpSessionURLUsesInlineProductForOtherAmounts(t *testing.T) {
	backend := &topUpSessionBackend{t: t}
	originalBackend := stripe.GetBackend(stripe.APIBackend)
	stripe.SetBackend(stripe.APIBackend, backend)
	t.Cleanup(func() { stripe.SetBackend(stripe.APIBackend, originalBackend) })

	s := NewStripe(config.Stripe{
		AppURL:               "https://app.helix.test",
		SecretKey:            "sk_test",
		WebhookSigningSecret: "whsec_test",
	}, nil)

	_, err := s.GetTopUpSessionURL(TopUpSessionParams{Amount: 10})
	require.NoError(t, err)
	require.Len(t, backend.params.LineItems, 1)
	lineItem := backend.params.LineItems[0]
	require.Nil(t, lineItem.Price)
	require.Equal(t, "usd", *lineItem.PriceData.Currency)
	require.Equal(t, "Helix Credits", *lineItem.PriceData.ProductData.Name)
	require.Equal(t, "Top up of $10.00", *lineItem.PriceData.ProductData.Description)
	require.Equal(t, int64(1000), *lineItem.PriceData.UnitAmount)
}

func TestHandleTopUpEvent_UsesRequestedAmountMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := store.NewMockStore(ctrl)
	s := NewStripe(config.Stripe{}, db)
	wallet := &types.Wallet{ID: "wallet_123"}

	db.EXPECT().GetWalletByUser(gomock.Any(), "user_123").Return(wallet, nil)
	db.EXPECT().UpdateWalletBalance(gomock.Any(), "wallet_123", 25.0, types.TransactionMetadata{
		TransactionType:       types.TransactionTypeTopUp,
		StripePaymentIntentID: "pi_123",
	}).Return(wallet, nil)

	event := stripeEvent(t, stripe.EventTypePaymentIntentSucceeded, &stripe.PaymentIntent{
		ID:     "pi_123",
		Amount: 500,
		Metadata: map[string]string{
			topUpMetadataType:        topUpMetadataTypeValue,
			topUpMetadataUserID:      "user_123",
			topUpMetadataAmountCents: "2500",
		},
	})

	err := s.handleTopUpEvent(event)
	require.NoError(t, err)
}

func TestHandleTopUpCheckoutSessionCompleted_NoPaymentRequired(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := store.NewMockStore(ctrl)
	s := NewStripe(config.Stripe{}, db)
	wallet := &types.Wallet{ID: "wallet_123"}

	db.EXPECT().GetWalletByUser(gomock.Any(), "user_123").Return(wallet, nil)
	db.EXPECT().UpdateWalletBalance(gomock.Any(), "wallet_123", 50.0, types.TransactionMetadata{
		TransactionType:         types.TransactionTypeTopUp,
		StripeCheckoutSessionID: "cs_free_123",
	}).Return(wallet, nil)

	event := stripeEvent(t, stripe.EventTypeCheckoutSessionCompleted, &stripe.CheckoutSession{
		ID:             "cs_free_123",
		Mode:           stripe.CheckoutSessionModePayment,
		PaymentStatus:  stripe.CheckoutSessionPaymentStatusNoPaymentRequired,
		AmountSubtotal: 5000,
		Metadata: map[string]string{
			topUpMetadataType:        topUpMetadataTypeValue,
			topUpMetadataUserID:      "user_123",
			topUpMetadataAmountCents: "5000",
		},
	})

	err := s.handleTopUpCheckoutSessionCompletedEvent(event)
	require.NoError(t, err)
}

func TestHandleTopUpCheckoutSessionCompleted_FallsBackToCustomerForOldSessions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := store.NewMockStore(ctrl)
	s := NewStripe(config.Stripe{}, db)
	wallet := &types.Wallet{ID: "wallet_123"}

	db.EXPECT().GetWalletByStripeCustomerID(gomock.Any(), "cus_123").Return(wallet, nil)
	db.EXPECT().UpdateWalletBalance(gomock.Any(), "wallet_123", 50.0, types.TransactionMetadata{
		TransactionType:         types.TransactionTypeTopUp,
		StripeCheckoutSessionID: "cs_free_123",
	}).Return(wallet, nil)

	event := stripeEvent(t, stripe.EventTypeCheckoutSessionCompleted, &stripe.CheckoutSession{
		ID:             "cs_free_123",
		Mode:           stripe.CheckoutSessionModePayment,
		PaymentStatus:  stripe.CheckoutSessionPaymentStatusNoPaymentRequired,
		AmountSubtotal: 5000,
		Customer:       &stripe.Customer{ID: "cus_123"},
	})

	err := s.handleTopUpCheckoutSessionCompletedEvent(event)
	require.NoError(t, err)
}

func TestHandleTopUpCheckoutSessionCompleted_IncludesPaymentIntentID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := store.NewMockStore(ctrl)
	s := NewStripe(config.Stripe{}, db)
	wallet := &types.Wallet{ID: "wallet_123"}

	db.EXPECT().GetWalletByUser(gomock.Any(), "user_123").Return(wallet, nil)
	db.EXPECT().UpdateWalletBalance(gomock.Any(), "wallet_123", 50.0, types.TransactionMetadata{
		TransactionType:         types.TransactionTypeTopUp,
		StripePaymentIntentID:   "pi_123",
		StripeCheckoutSessionID: "cs_paid_123",
	}).Return(wallet, nil)

	event := stripeEvent(t, stripe.EventTypeCheckoutSessionCompleted, &stripe.CheckoutSession{
		ID:             "cs_paid_123",
		Mode:           stripe.CheckoutSessionModePayment,
		PaymentStatus:  stripe.CheckoutSessionPaymentStatusPaid,
		AmountSubtotal: 5000,
		PaymentIntent:  &stripe.PaymentIntent{ID: "pi_123"},
		Metadata: map[string]string{
			topUpMetadataType:        topUpMetadataTypeValue,
			topUpMetadataUserID:      "user_123",
			topUpMetadataAmountCents: "5000",
		},
	})

	err := s.handleTopUpCheckoutSessionCompletedEvent(event)
	require.NoError(t, err)
}

func stripeEvent(t *testing.T, eventType stripe.EventType, object any) stripe.Event {
	t.Helper()

	raw, err := json.Marshal(object)
	require.NoError(t, err)

	return stripe.Event{
		Type: eventType,
		Data: &stripe.EventData{Raw: raw},
	}
}

// A failing wallet mutation must surface as a non-2xx webhook status so Stripe
// retries the delivery; the store deduplicates retries, so the purchase still
// credits the wallet exactly once.
func TestProcessWebhook_Returns500OnFailedWalletMutation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := store.NewMockStore(ctrl)
	s := NewStripe(config.Stripe{WebhookSigningSecret: testWebhookSecret}, db)
	wallet := &types.Wallet{ID: "wallet_123"}

	db.EXPECT().GetWalletByUser(gomock.Any(), "user_123").Return(wallet, nil)
	db.EXPECT().UpdateWalletBalance(gomock.Any(), "wallet_123", 25.0, types.TransactionMetadata{
		TransactionType:       types.TransactionTypeTopUp,
		StripePaymentIntentID: "pi_123",
	}).Return(nil, errors.New("database unavailable"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(
		paymentIntentSucceededEvent("pi_123", "cus_123", "", "user_123", 2500)))
	req.Header.Set("Stripe-Signature", webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload: paymentIntentSucceededEvent("pi_123", "cus_123", "", "user_123", 2500),
		Secret:  testWebhookSecret,
	}).Header)
	s.ProcessWebhook(rec, req)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

// Payment intents that are not ours must be answered with 200 — retrying them
// can never succeed.
func TestProcessWebhook_ForeignPaymentIntentReturns200(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := store.NewMockStore(ctrl)
	s := NewStripe(config.Stripe{WebhookSigningSecret: testWebhookSecret}, db)

	rec := httptest.NewRecorder()
	payload := foreignPaymentIntentEvent("pi_foreign", "cus_123", 500)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payload))
	req.Header.Set("Stripe-Signature", webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload: payload,
		Secret:  testWebhookSecret,
	}).Header)
	s.ProcessWebhook(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}
