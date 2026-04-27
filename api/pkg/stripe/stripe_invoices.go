package stripe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/product"
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
		if errors.Is(err, store.ErrNotFound) {
			log.Info().
				Str("customer_id", invoice.Customer.ID).
				Msg("no wallet found for stripe customer id, skipping")
			return nil
		}
		return fmt.Errorf("error getting wallet from stripe: %s", err.Error())
	}

	productID := getSubscriptionInvoiceProductID(&invoice)
	if productID == "" {
		log.Info().
			Str("invoice_id", invoice.ID).
			Msg("no subscription product found on invoice lines, skipping subscription topup")
		return nil
	}

	prod, err := product.Get(productID, nil)
	if err != nil {
		return fmt.Errorf("failed to get product from stripe: %w", err)
	}

	creditsValue, ok := prod.Metadata["credits"]
	if !ok || creditsValue == "" {
		log.Info().
			Str("invoice_id", invoice.ID).
			Str("product_id", productID).
			Msg("product credits metadata missing, skipping subscription topup")
		return nil
	}

	amount, err := strconv.ParseFloat(creditsValue, 64)
	if err != nil {
		log.Warn().
			Str("invoice_id", invoice.ID).
			Str("product_id", productID).
			Str("credits", creditsValue).
			Msg("invalid product credits metadata, skipping subscription topup")
		return nil
	}

	if amount <= 0 {
		log.Info().
			Str("invoice_id", invoice.ID).
			Str("product_id", productID).
			Float64("credits", amount).
			Msg("non-positive product credits metadata, skipping subscription topup")
		return nil
	}

	_, err = s.store.UpdateWalletBalance(context.Background(), wallet.ID, amount, types.TransactionMetadata{
		TransactionType: types.TransactionTypeSubscription,
	})

	if err != nil {
		return fmt.Errorf("error updating wallet balance: %s", err.Error())
	}

	return nil
}

func getSubscriptionInvoiceProductID(invoice *stripe.Invoice) string {
	if invoice == nil || invoice.Lines == nil {
		return ""
	}

	for _, line := range invoice.Lines.Data {
		if line == nil || line.Type != stripe.InvoiceLineItemTypeSubscription {
			continue
		}
		if line.Price != nil && line.Price.Product != nil && line.Price.Product.ID != "" {
			return line.Price.Product.ID
		}
		if line.Plan != nil && line.Plan.Product != nil && line.Plan.Product.ID != "" {
			return line.Plan.Product.ID
		}
	}

	return ""
}
