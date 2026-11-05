package stripe

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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
	t                    *testing.T
	params               *stripe.CheckoutSessionParams
	endpoint             *stripe.WebhookEndpoint
	updatedEndpoint      *stripe.WebhookEndpoint
	endpointGetErr       error
	endpointUpdateErr    error
	endpointUpdateParams *stripe.WebhookEndpointParams
	calls                []string
}

func (b *topUpSessionBackend) Call(method, path, _ string, params stripe.ParamsContainer, v stripe.LastResponseSetter) error {
	b.calls = append(b.calls, method+" "+path)
	switch {
	case method == http.MethodGet && path == "/v1/webhook_endpoints/we_test":
		if b.endpointGetErr != nil {
			return b.endpointGetErr
		}
		endpoint, ok := v.(*stripe.WebhookEndpoint)
		require.True(b.t, ok)
		if b.endpoint == nil {
			b.endpoint = &stripe.WebhookEndpoint{
				ID:            "we_test",
				Status:        "enabled",
				EnabledEvents: []string{string(stripe.EventTypeCheckoutSessionCompleted), string(stripe.EventTypePaymentIntentSucceeded)},
			}
		}
		*endpoint = *b.endpoint
		return nil
	case method == http.MethodPost && path == "/v1/webhook_endpoints/we_test":
		if b.endpointUpdateErr != nil {
			return b.endpointUpdateErr
		}
		updateParams, ok := params.(*stripe.WebhookEndpointParams)
		require.True(b.t, ok)
		b.endpointUpdateParams = updateParams
		endpoint, ok := v.(*stripe.WebhookEndpoint)
		require.True(b.t, ok)
		if b.updatedEndpoint != nil {
			*endpoint = *b.updatedEndpoint
			return nil
		}
		endpoint.ID = "we_test"
		endpoint.Status = "enabled"
		for _, event := range updateParams.EnabledEvents {
			endpoint.EnabledEvents = append(endpoint.EnabledEvents, *event)
		}
		return nil
	case method == http.MethodPost && path == "/v1/checkout/sessions":
		checkoutParams, ok := params.(*stripe.CheckoutSessionParams)
		require.True(b.t, ok)
		b.params = checkoutParams
		session, ok := v.(*stripe.CheckoutSession)
		require.True(b.t, ok)
		session.URL = "https://checkout.stripe.test/session"
		return nil
	default:
		return fmt.Errorf("unexpected Stripe call: %s %s", method, path)
	}
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
		WebhookEndpointID:    "we_test",
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
		WebhookEndpointID:    "we_test",
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

func TestGetTopUpSessionURLReconcilesWebhookEventsBeforeCheckout(t *testing.T) {
	backend := &topUpSessionBackend{
		t: t,
		endpoint: &stripe.WebhookEndpoint{
			ID:            "we_test",
			Status:        "enabled",
			EnabledEvents: []string{"invoice.paid", string(stripe.EventTypePaymentIntentSucceeded)},
		},
	}
	setTopUpBackend(t, backend)

	s := NewStripe(topUpStripeConfig(), nil)
	_, err := s.GetTopUpSessionURL(TopUpSessionParams{Amount: 10})
	require.NoError(t, err)
	require.NotNil(t, backend.params)
	require.Equal(t, stripe.StringSlice([]string{
		"invoice.paid",
		string(stripe.EventTypePaymentIntentSucceeded),
		string(stripe.EventTypeCheckoutSessionCompleted),
	}), backend.endpointUpdateParams.EnabledEvents)
	require.Equal(t, []string{
		"GET /v1/webhook_endpoints/we_test",
		"POST /v1/webhook_endpoints/we_test",
		"POST /v1/checkout/sessions",
	}, backend.calls)
}

func TestGetTopUpSessionURLAcceptsWildcardWebhookEvents(t *testing.T) {
	backend := &topUpSessionBackend{
		t: t,
		endpoint: &stripe.WebhookEndpoint{
			ID:            "we_test",
			Status:        "enabled",
			EnabledEvents: []string{"*"},
		},
	}
	setTopUpBackend(t, backend)

	s := NewStripe(topUpStripeConfig(), nil)
	_, err := s.GetTopUpSessionURL(TopUpSessionParams{Amount: 10})
	require.NoError(t, err)
	require.Nil(t, backend.endpointUpdateParams)
	require.NotNil(t, backend.params)
}

func TestGetTopUpSessionURLAddsBothWebhookEvents(t *testing.T) {
	backend := &topUpSessionBackend{
		t: t,
		endpoint: &stripe.WebhookEndpoint{
			ID:            "we_test",
			Status:        "enabled",
			EnabledEvents: []string{"invoice.paid"},
		},
	}
	setTopUpBackend(t, backend)

	s := NewStripe(topUpStripeConfig(), nil)
	_, err := s.GetTopUpSessionURL(TopUpSessionParams{Amount: 10})
	require.NoError(t, err)
	require.Equal(t, stripe.StringSlice([]string{
		"invoice.paid",
		string(stripe.EventTypeCheckoutSessionCompleted),
		string(stripe.EventTypePaymentIntentSucceeded),
	}), backend.endpointUpdateParams.EnabledEvents)
}

func TestGetTopUpSessionURLRefusesCheckoutWithoutValidWebhook(t *testing.T) {
	tests := []struct {
		name     string
		config   config.Stripe
		backend  *topUpSessionBackend
		contains string
	}{
		{
			name:     "missing endpoint id",
			config:   config.Stripe{SecretKey: "sk_test", WebhookSigningSecret: "whsec_test"},
			backend:  &topUpSessionBackend{},
			contains: "webhook endpoint ID is required",
		},
		{
			name:     "endpoint read fails",
			config:   topUpStripeConfig(),
			backend:  &topUpSessionBackend{endpointGetErr: errors.New("Stripe unavailable")},
			contains: "get Stripe webhook endpoint",
		},
		{
			name:   "endpoint disabled",
			config: topUpStripeConfig(),
			backend: &topUpSessionBackend{endpoint: &stripe.WebhookEndpoint{
				ID: "we_test", Status: "disabled",
			}},
			contains: "is not enabled",
		},
		{
			name:   "endpoint update fails",
			config: topUpStripeConfig(),
			backend: &topUpSessionBackend{
				endpoint:          &stripe.WebhookEndpoint{ID: "we_test", Status: "enabled"},
				endpointUpdateErr: errors.New("Stripe unavailable"),
			},
			contains: "update Stripe webhook endpoint",
		},
		{
			name:   "endpoint update omits event",
			config: topUpStripeConfig(),
			backend: &topUpSessionBackend{
				endpoint: &stripe.WebhookEndpoint{ID: "we_test", Status: "enabled"},
				updatedEndpoint: &stripe.WebhookEndpoint{
					ID: "we_test", Status: "enabled", EnabledEvents: []string{string(stripe.EventTypePaymentIntentSucceeded)},
				},
			},
			contains: "missing required credit events",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.backend.t = t
			setTopUpBackend(t, tt.backend)
			s := NewStripe(tt.config, nil)
			_, err := s.GetTopUpSessionURL(TopUpSessionParams{Amount: 10})
			require.ErrorContains(t, err, tt.contains)
			require.Nil(t, tt.backend.params)
		})
	}
}

func setTopUpBackend(t *testing.T, backend stripe.Backend) {
	t.Helper()
	originalBackend := stripe.GetBackend(stripe.APIBackend)
	stripe.SetBackend(stripe.APIBackend, backend)
	t.Cleanup(func() { stripe.SetBackend(stripe.APIBackend, originalBackend) })
}

func topUpStripeConfig() config.Stripe {
	return config.Stripe{
		AppURL:               "https://app.helix.test",
		SecretKey:            "sk_test",
		WebhookSigningSecret: "whsec_test",
		WebhookEndpointID:    "we_test",
	}
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

func TestHandleTopUpEvent_SendsCreditsAddedNotification(t *testing.T) {
	ctrl := gomock.NewController(t)
	db := store.NewMockStore(ctrl)
	slack := &recordingSlackSender{}
	s := NewStripe(config.Stripe{}, db)
	s.SetSlackSender(slack)
	wallet := &types.Wallet{ID: "wallet_123", UserID: "user_123", Balance: 10}
	updatedWallet := &types.Wallet{ID: "wallet_123", UserID: "user_123", Balance: 35}

	db.EXPECT().GetWalletByUser(gomock.Any(), "user_123").Return(wallet, nil)
	db.EXPECT().UpdateWalletBalance(gomock.Any(), "wallet_123", 25.0, gomock.Any()).Return(updatedWallet, nil)
	db.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(&types.User{Email: "test@example.com"}, nil)

	event := stripeEvent(t, stripe.EventTypePaymentIntentSucceeded, &stripe.PaymentIntent{
		ID:     "pi_123",
		Amount: 2500,
		Metadata: map[string]string{
			topUpMetadataType:   topUpMetadataTypeValue,
			topUpMetadataUserID: "user_123",
		},
	})
	require.NoError(t, s.handleTopUpEvent(event))
	require.Equal(t, []string{"💳 Credits added: test@example.com — $25.00"}, slack.messages)
}

func TestHandleTopUpEvent_DoesNotNotifyAlreadyProcessedTopUp(t *testing.T) {
	ctrl := gomock.NewController(t)
	db := store.NewMockStore(ctrl)
	slack := &recordingSlackSender{}
	s := NewStripe(config.Stripe{}, db)
	s.SetSlackSender(slack)
	wallet := &types.Wallet{ID: "wallet_123", UserID: "user_123", Balance: 10}

	db.EXPECT().GetWalletByUser(gomock.Any(), "user_123").Return(wallet, nil)
	db.EXPECT().UpdateWalletBalance(gomock.Any(), "wallet_123", 25.0, gomock.Any()).Return(nil, nil)

	event := stripeEvent(t, stripe.EventTypePaymentIntentSucceeded, &stripe.PaymentIntent{
		ID:     "pi_123",
		Amount: 2500,
		Metadata: map[string]string{
			topUpMetadataType:   topUpMetadataTypeValue,
			topUpMetadataUserID: "user_123",
		},
	})
	require.NoError(t, s.handleTopUpEvent(event))
	require.Empty(t, slack.messages)
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
