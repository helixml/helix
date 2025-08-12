package stripe

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"github.com/stripe/stripe-go/v76"
)

func (s *Stripe) handleInvoicePaymentPaidEvent(event stripe.Event) error {
	var invoice stripe.Invoice
	err := json.Unmarshal(event.Data.Raw, &invoice)
	if err != nil {
		return fmt.Errorf("error parsing invoice JSON: %s", err.Error())
	}

	// Only process invoices with successful payment
	if invoice.Status != stripe.InvoiceStatusPaid {
		log.Debug().
			Str("invoice_id", invoice.ID).
			Str("status", string(invoice.Status)).
			Msg("invoice payment not successful")
		return nil
	}

	// Handle only subscription invoices, normal top-ups
	// are handled by the payment intent webhook
	if invoice.Subscription == nil {
		log.Debug().
			Str("invoice_id", invoice.ID).
			Msg("invoice has no subscription")
		return nil
	}

	wallet, err := s.store.GetWalletByStripeCustomerID(context.Background(), invoice.Customer.ID)
	if err != nil {
		return fmt.Errorf("error getting wallet from stripe: %s", err.Error())
	}

	// Top-up the wallet by the amount of the invoice
	amount := float64(invoice.AmountPaid) / 100.0

	_, err = s.store.UpdateWalletBalance(context.Background(), wallet.ID, amount, types.TransactionMetadata{
		TransactionType: types.TransactionTypeSubscription,
	})

	if err != nil {
		return fmt.Errorf("error updating wallet balance: %s", err.Error())
	}

	return nil
}
