package stripe

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v76/webhook"
)

// End-to-end regression tests for the credit checkout flow: real signed
// Stripe wire-format payloads delivered through ProcessWebhook into a real
// Postgres store. Skipped unless POSTGRES_HOST is set (store tests need
// Postgres; run against the dev stack's helix_test database).

const testWebhookSecret = "whsec_regression_test"

func newWebhookTestStore(t *testing.T) store.Store {
	t.Helper()
	if os.Getenv("POSTGRES_HOST") == "" {
		t.Skip("POSTGRES_HOST not set; skipping webhook integration test")
	}
	var storeCfg config.Store
	require.NoError(t, envconfig.Process("", &storeCfg))
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)
	db, err := store.NewPostgresStore(storeCfg, ps)
	require.NoError(t, err)
	return db
}

func deliverEvent(t *testing.T, s *Stripe, payload []byte) int {
	t.Helper()
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload: payload,
		Secret:  testWebhookSecret,
	})
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(signed.Payload))
	req.Header.Set("Stripe-Signature", signed.Header)
	rec := httptest.NewRecorder()
	s.ProcessWebhook(rec, req)
	return rec.Code
}

// checkoutSessionEvent builds a checkout.session.completed wire payload. piID
// is the PaymentIntent reference Stripe puts on the session ("" renders
// payment_intent: null, which Stripe sends when the session has no
// PaymentIntent — the fully discounted no-PI case).
func checkoutSessionEvent(csID, cusID, orgID, userID, paymentStatus string, subtotalCents, totalCents int64, piID string) []byte {
	piField := "null"
	if piID != "" {
		piField = fmt.Sprintf("%q", piID)
	}
	return []byte(fmt.Sprintf(`{
  "id": "evt_%s",
  "object": "event",
  "api_version": "2024-06-20",
  "created": 1757900000,
  "data": {
    "object": {
      "id": "%s",
      "object": "checkout.session",
      "mode": "payment",
      "payment_status": "%s",
      "status": "complete",
      "amount_subtotal": %d,
      "amount_total": %d,
      "currency": "usd",
      "customer": "%s",
      "metadata": {
        "type": "topup",
        "user_id": "%s",
        "org_id": "%s",
        "topup_amount_cents": "%d"
      },
      "payment_intent": %s
    }
  },
  "livemode": false,
  "type": "checkout.session.completed"
}`, csID, csID, paymentStatus, subtotalCents, totalCents, cusID, userID, orgID, subtotalCents, piField))
}

// paymentIntentSucceededEvent builds a payment_intent.succeeded wire payload
// for a topup payment intent whose metadata carries the requested amount
// (amountCents may exceed the collected amount when a coupon discounts the
// payment).
func paymentIntentSucceededEvent(piID, cusID, orgID, userID string, amountCents int64) []byte {
	return []byte(fmt.Sprintf(`{
  "id": "evt_%s",
  "object": "event",
  "api_version": "2024-06-20",
  "created": 1757900001,
  "data": {
    "object": {
      "id": "%s",
      "object": "payment_intent",
      "amount": %d,
      "currency": "usd",
      "status": "succeeded",
      "customer": "%s",
      "metadata": {
        "type": "topup",
        "user_id": "%s",
        "org_id": "%s",
        "topup_amount_cents": "%d"
      }
    }
  },
  "livemode": false,
  "type": "payment_intent.succeeded"
}`, piID, piID, amountCents, cusID, userID, orgID, amountCents))
}

// paymentIntentMetadatalessEvent builds a payment_intent.succeeded payload for
// a topup payment intent without the requested-amount metadata.
func paymentIntentMetadatalessEvent(piID, cusID, orgID, userID string, amountCents int64) []byte {
	return []byte(fmt.Sprintf(`{
  "id": "evt_%s",
  "object": "event",
  "api_version": "2024-06-20",
  "created": 1757900002,
  "data": {
    "object": {
      "id": "%s",
      "object": "payment_intent",
      "amount": %d,
      "currency": "usd",
      "status": "succeeded",
      "customer": "%s",
      "metadata": {
        "type": "topup",
        "user_id": "%s",
        "org_id": "%s"
      }
    }
  },
  "livemode": false,
  "type": "payment_intent.succeeded"
}`, piID, piID, amountCents, cusID, userID, orgID))
}

// foreignPaymentIntentEvent builds a payment_intent.succeeded payload without
// topup metadata (e.g. another product's payment on the same account).
func foreignPaymentIntentEvent(piID, cusID string, amountCents int64) []byte {
	return []byte(fmt.Sprintf(`{
  "id": "evt_%s",
  "object": "event",
  "api_version": "2024-06-20",
  "created": 1757900003,
  "data": {
    "object": {
      "id": "%s",
      "object": "payment_intent",
      "amount": %d,
      "currency": "usd",
      "status": "succeeded",
      "customer": "%s"
    }
  },
  "livemode": false,
  "type": "payment_intent.succeeded"
}`, piID, piID, amountCents, cusID))
}

type purchaseFixture struct {
	suffix   string
	orgID    string
	userID   string
	cusID    string
	walletID string

	// piID is the PaymentIntent created for the purchase ("" when the fully
	// discounted session completes without one).
	piID string

	sessionPayload []byte
	piPayload      []byte // nil when Stripe sends no payment_intent.succeeded
}

// newPurchaseFixture seeds an org wallet and builds the two webhook payloads
// for a $5 topup purchase. discount=true models the fully discounted coupon
// purchase: $5 subtotal, $0 collected, checkout.session.completed with
// payment_status no_payment_required, and (when withPI) a $0
// payment_intent.succeeded carrying the requested amount in metadata.
func newPurchaseFixture(t *testing.T, db store.Store, discount, withPI bool) purchaseFixture {
	t.Helper()
	suffix := system.GenerateID()
	f := purchaseFixture{
		suffix: suffix,
		orgID:  "org_chk_" + suffix,
		userID: "user_chk_" + suffix,
		cusID:  "cus_chk_" + suffix,
		piID:   map[bool]string{true: "pi_chk_" + suffix, false: ""}[withPI],
	}
	wallet, err := db.CreateWallet(context.Background(), &types.Wallet{
		OrgID:            f.orgID,
		StripeCustomerID: f.cusID,
		Balance:          0,
	})
	require.NoError(t, err)
	f.walletID = wallet.ID

	paymentStatus, totalCents := "paid", int64(500)
	if discount {
		paymentStatus, totalCents = "no_payment_required", 0
	}
	f.sessionPayload = checkoutSessionEvent(
		"cs_chk_"+suffix, f.cusID, f.orgID, f.userID,
		paymentStatus, 500, totalCents, f.piID,
	)
	if withPI {
		f.piPayload = paymentIntentSucceededEvent(f.piID, f.cusID, f.orgID, f.userID, 500)
	}
	return f
}

func requireCreditedExactlyOnce(t *testing.T, db store.Store, f purchaseFixture) {
	t.Helper()
	wallet, err := db.GetWallet(context.Background(), f.walletID)
	require.NoError(t, err)
	require.Equal(t, 5.0, wallet.Balance)

	topUps, err := db.ListTopUps(context.Background(), &store.ListTopUpsQuery{WalletID: f.walletID})
	require.NoError(t, err)
	require.Len(t, topUps, 1)
	require.Equal(t, 5.0, topUps[0].Amount)
}

func TestCreditCheckoutIntegration(t *testing.T) {
	db := newWebhookTestStore(t)
	s := NewStripe(config.Stripe{
		WebhookSigningSecret: testWebhookSecret,
		AppURL:               "http://localhost:8080",
	}, db)

	t.Run("coupon purchase with payment intent credits org wallet exactly once", func(t *testing.T) {
		// Fully discounted coupon purchase with a $0 receipt: $5 subtotal, $0
		// collected. Stripe sends checkout.session.completed (payment_status
		// no_payment_required) and payment_intent.succeeded for the $0
		// PaymentIntent — in either order, each with retries.
		for _, piFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("pi_first=%t", piFirst), func(t *testing.T) {
				f := newPurchaseFixture(t, db, true, true)

				order := [][]byte{f.sessionPayload, f.piPayload}
				if piFirst {
					order = [][]byte{order[1], order[0]}
				}
				for _, payload := range order {
					require.Equal(t, http.StatusOK, deliverEvent(t, s, payload))
				}
				// Stripe retries every event; refreshing or revisiting the
				// return URL must not duplicate the credit either.
				for _, payload := range order {
					require.Equal(t, http.StatusOK, deliverEvent(t, s, payload))
					require.Equal(t, http.StatusOK, deliverEvent(t, s, payload))
				}

				requireCreditedExactlyOnce(t, db, f)
			})
		}
	})

	t.Run("coupon purchase without payment intent credits exactly once", func(t *testing.T) {
		// If Stripe completes the fully discounted session without a
		// PaymentIntent, checkout.session.completed is the only crediting event.
		f := newPurchaseFixture(t, db, true, false)
		require.Equal(t, http.StatusOK, deliverEvent(t, s, f.sessionPayload))
		require.Equal(t, http.StatusOK, deliverEvent(t, s, f.sessionPayload))

		requireCreditedExactlyOnce(t, db, f)
	})

	t.Run("normal card payment credits org wallet exactly once", func(t *testing.T) {
		for _, piFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("pi_first=%t", piFirst), func(t *testing.T) {
				f := newPurchaseFixture(t, db, false, true)

				order := [][]byte{f.sessionPayload, f.piPayload}
				if piFirst {
					order = [][]byte{order[1], order[0]}
				}
				for _, payload := range order {
					require.Equal(t, http.StatusOK, deliverEvent(t, s, payload))
				}
				for _, payload := range order {
					require.Equal(t, http.StatusOK, deliverEvent(t, s, payload))
					require.Equal(t, http.StatusOK, deliverEvent(t, s, payload))
				}

				requireCreditedExactlyOnce(t, db, f)
			})
		}
	})

	t.Run("incomplete payment grants no credits", func(t *testing.T) {
		f := newPurchaseFixture(t, db, false, false)
		unpaid := checkoutSessionEvent(
			"cs_unpaid_"+f.suffix, f.cusID, f.orgID, f.userID,
			"unpaid", 500, 500, "")
		require.Equal(t, http.StatusOK, deliverEvent(t, s, unpaid))

		wallet, err := db.GetWallet(context.Background(), f.walletID)
		require.NoError(t, err)
		require.Equal(t, 0.0, wallet.Balance)

		topUps, err := db.ListTopUps(context.Background(), &store.ListTopUpsQuery{WalletID: f.walletID})
		require.NoError(t, err)
		require.Empty(t, topUps)
	})

	t.Run("zero amount payment intent without metadata skips", func(t *testing.T) {
		// A $0 payment intent without the requested-amount metadata cannot know
		// what to credit; it must skip cleanly (not error) so the webhook
		// returns 200 and the session event owns the purchase.
		f := newPurchaseFixture(t, db, true, false)
		piPayload := paymentIntentMetadatalessEvent("pi_nometa_"+f.suffix, f.cusID, f.orgID, f.userID, 0)
		require.Equal(t, http.StatusOK, deliverEvent(t, s, piPayload))

		wallet, err := db.GetWallet(context.Background(), f.walletID)
		require.NoError(t, err)
		require.Equal(t, 0.0, wallet.Balance)
	})

	t.Run("zero amount payment intent with metadata credits requested amount", func(t *testing.T) {
		// The $0 receipt case: the customer paid $0 but purchased $5 of credits.
		f := newPurchaseFixture(t, db, true, false)
		piPayload := paymentIntentSucceededEvent("pi_zero_"+f.suffix, f.cusID, f.orgID, f.userID, 500)
		require.Equal(t, http.StatusOK, deliverEvent(t, s, piPayload))
		require.Equal(t, http.StatusOK, deliverEvent(t, s, piPayload))

		requireCreditedExactlyOnce(t, db, f)
	})

	t.Run("foreign payment intent is skipped", func(t *testing.T) {
		f := newPurchaseFixture(t, db, false, false)
		require.Equal(t, http.StatusOK, deliverEvent(t, s, foreignPaymentIntentEvent("pi_foreign_"+f.suffix, f.cusID, 500)))

		wallet, err := db.GetWallet(context.Background(), f.walletID)
		require.NoError(t, err)
		require.Equal(t, 0.0, wallet.Balance)
	})
}
