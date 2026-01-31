package stripe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/checkout/session"
)

type TopUpSessionParams struct {
	StripeCustomerID string
	OrgID            string
	OrgName          string // Used for redirect URL (for example 'acme-org' for 'orgs/acme-org/billing')
	UserID           string
	Amount           float64
}

func (s *Stripe) GetTopUpSessionURL(
	params TopUpSessionParams,
) (string, error) {
	err := s.EnabledError()
	if err != nil {
		return "", err
	}

	// Convert amount to cents for Stripe
	amountInCents := int64(params.Amount * 100)

	successURL := s.cfg.AppURL + "/account?success=true&session_id={CHECKOUT_SESSION_ID}"
	if params.OrgID != "" {
		successURL = s.cfg.AppURL + "/orgs/" + params.OrgName + "/billing?success=true&session_id={CHECKOUT_SESSION_ID}"
	}

	cancelURL := s.cfg.AppURL + "/account?canceled=true"
	if params.OrgID != "" {
		cancelURL = s.cfg.AppURL + "/orgs/" + params.OrgName + "/billing?canceled=true"
	}

	checkoutParams := &stripe.CheckoutSessionParams{
		AllowPromotionCodes: stripe.Bool(true),
		Mode:                stripe.String(string(stripe.CheckoutSessionModePayment)),
		// this is how we link the payment to our user
		PaymentIntentData: &stripe.CheckoutSessionPaymentIntentDataParams{
			Metadata: map[string]string{
				"user_id": params.UserID,
				"org_id":  params.OrgID,
				"type":    "topup",
			},
		},
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency: stripe.String("usd"),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name:        stripe.String("Helix Credits"),
						Description: stripe.String(fmt.Sprintf("Top up of $%.2f", params.Amount)),
					},
					UnitAmount: stripe.Int64(amountInCents),
				},
				Quantity: stripe.Int64(1),
			},
		},
		Customer:   stripe.String(params.StripeCustomerID),
		SuccessURL: stripe.String(successURL),
		CancelURL:  stripe.String(cancelURL),
	}

	newSession, err := session.New(checkoutParams)
	if err != nil {
		return "", err
	}

	return newSession.URL, nil
}

func (s *Stripe) handleTopUpEvent(event stripe.Event) error {
	var paymentIntent stripe.PaymentIntent
	err := json.Unmarshal(event.Data.Raw, &paymentIntent)
	if err != nil {
		return fmt.Errorf("error parsing payment intent JSON: %s", err.Error())
	}

	userID := paymentIntent.Metadata["user_id"]
	orgID := paymentIntent.Metadata["org_id"]

	// If both are empty, do not process the event
	if userID == "" && orgID == "" {
		log.Info().
			Str("payment_intent_id", paymentIntent.ID).
			Msg("no user or org id found in payment intent metadata, skipping")
		return nil
	}

	// Check if this is a topup payment
	paymentType := paymentIntent.Metadata["type"]
	if paymentType != "topup" {
		return fmt.Errorf("payment is not a topup")
	}

	// Calculate the amount in dollars (Stripe amounts are in cents)
	amount := float64(paymentIntent.Amount) / 100.0

	ctx := context.Background()

	var (
		wallet *types.Wallet
	)

	switch {
	case orgID != "":
		wallet, err = s.store.GetWalletByOrg(ctx, orgID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				// Create wallet if it doesn't exist
				wallet, err = s.store.CreateWallet(ctx, &types.Wallet{
					OrgID:   orgID,
					Balance: s.cfg.InitialBalance,
				})
				if err != nil {
					return fmt.Errorf("failed to create wallet for org %s: %w", orgID, err)
				}
			}
		}
		if wallet == nil {
			return fmt.Errorf("no wallet found for org %s", orgID)
		}

	case userID != "":
		// Get or create user's wallet
		wallet, err = s.store.GetWalletByUser(ctx, userID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				// Create wallet if it doesn't exist
				wallet, err = s.store.CreateWallet(ctx, &types.Wallet{
					UserID:  userID,
					Balance: s.cfg.InitialBalance,
				})
				if err != nil {
					return fmt.Errorf("failed to create wallet for user %s: %w", userID, err)
				}
			} else {
				return fmt.Errorf("failed to get wallet for user %s: %w", userID, err)
			}
		}

	default:
		return fmt.Errorf("no wallet found for user %s or org %s", userID, orgID)
	}

	_, err = s.store.UpdateWalletBalance(ctx, wallet.ID, amount, types.TransactionMetadata{
		TransactionType:       types.TransactionTypeTopUp,
		StripePaymentIntentID: paymentIntent.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to create topup for user %s: %w", userID, err)
	}

	log.Info().
		Str("user_id", userID).
		Float64("amount", amount).
		Str("wallet_id", wallet.ID).
		Msg("topup completed successfully")

	return nil
}
