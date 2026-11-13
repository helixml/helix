package stripe

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"sync"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v76"
)

// invoicePaidPayload returns testdata/paid.json rewritten to a unique
// customer and invoice id so reruns against a shared database don't collide.
func invoicePaidPayload(t *testing.T, cusID, invoiceID string) []byte {
	t.Helper()
	payload, err := os.ReadFile("testdata/paid.json")
	require.NoError(t, err)
	payload = bytes.ReplaceAll(payload, []byte("cus_SqicesZoU7LrDR"), []byte(cusID))
	return bytes.ReplaceAll(payload, []byte("in_1Rv1oVFNNvjhkCqzrdVp2JkZ"), []byte(invoiceID))
}

func TestInvoicePaidRedeliveryCreditsOnceIntegration(t *testing.T) {
	db := newWebhookTestStore(t)
	s := NewStripe(config.Stripe{WebhookSigningSecret: testWebhookSecret}, db)
	installInvoiceWebhookBackend(t, &invoiceWebhookBackend{
		t:                t,
		subscriptionPath: invoiceSubscriptionPath,
		subscription:     nonAdminSubscription("sub_1Rv1oVFNNvjhkCqzI3vgc41P"),
		productPath:      invoiceProductPath,
		product: &stripe.Product{
			ID:       "prod_Sqia5TdP4XGk1x",
			Metadata: map[string]string{"credits": "250"},
		},
	})

	suffix := system.GenerateID()
	cusID := "cus_inv_" + suffix
	wallet, err := db.CreateWallet(context.Background(), &types.Wallet{
		OrgID:            "org_inv_" + suffix,
		StripeCustomerID: cusID,
	})
	require.NoError(t, err)

	first := invoicePaidPayload(t, cusID, "in_inv_a_"+suffix)
	require.Equal(t, http.StatusOK, deliverEvent(t, s, first))
	// Stripe redelivers the same invoice.paid, sequentially and concurrently.
	require.Equal(t, http.StatusOK, deliverEvent(t, s, first))
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			deliverEvent(t, s, first)
		}()
	}
	wg.Wait()

	got, err := db.GetWallet(context.Background(), wallet.ID)
	require.NoError(t, err)
	require.Equal(t, 250.0, got.Balance)

	// The next billing period's invoice credits again.
	require.Equal(t, http.StatusOK, deliverEvent(t, s, invoicePaidPayload(t, cusID, "in_inv_b_"+suffix)))
	got, err = db.GetWallet(context.Background(), wallet.ID)
	require.NoError(t, err)
	require.Equal(t, 500.0, got.Balance)

	txns, err := db.ListTransactions(context.Background(), &store.ListTransactionsQuery{
		WalletID:        wallet.ID,
		TransactionType: types.TransactionTypeSubscription,
	})
	require.NoError(t, err)
	require.Len(t, txns, 2)
}
