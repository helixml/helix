package stripe

import (
	"encoding/json"
	"os"
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
	s := NewStripe(config.Stripe{}, store)

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

	// should update wallet balance
	store.EXPECT().UpdateWalletBalance(gomock.Any(), "123", float64(20), types.TransactionMetadata{
		TransactionType: types.TransactionTypeSubscription,
	}).Return(wallet, nil)

	err = s.handleInvoicePaymentPaidEvent(event)
	require.NoError(t, err)
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
