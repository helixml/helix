package stripe

import (
	"encoding/json"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	stripe "github.com/stripe/stripe-go/v76"
	"go.uber.org/mock/gomock"
)

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
