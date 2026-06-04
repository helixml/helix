package stripe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/checkout/session"
)

const (
	topUpMetadataType        = "type"
	topUpMetadataUserID      = "user_id"
	topUpMetadataOrgID       = "org_id"
	topUpMetadataAmountCents = "topup_amount_cents"
	topUpMetadataTypeValue   = "topup"
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
	amountInCents := int64(math.Round(params.Amount * 100))
	metadata := topUpMetadata(params.UserID, params.OrgID, amountInCents)

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
		Metadata:            cloneMetadata(metadata),
		// this is how we link the payment to our user
		PaymentIntentData: &stripe.CheckoutSessionPaymentIntentDataParams{
			Metadata: cloneMetadata(metadata),
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

	userID := paymentIntent.Metadata[topUpMetadataUserID]
	orgID := paymentIntent.Metadata[topUpMetadataOrgID]

	// If both are empty, do not process the event
	if userID == "" && orgID == "" {
		log.Info().
			Str("payment_intent_id", paymentIntent.ID).
			Msg("no user or org id found in payment intent metadata, skipping")
		return nil
	}

	// Check if this is a topup payment
	paymentType := paymentIntent.Metadata[topUpMetadataType]
	if paymentType != topUpMetadataTypeValue {
		return fmt.Errorf("payment is not a topup")
	}

	amount, err := topUpAmountDollars(paymentIntent.Metadata, paymentIntent.Amount)
	if err != nil {
		return err
	}

	ctx := context.Background()
	wallet, err := s.getTopUpWallet(ctx, userID, orgID, stripeCustomerIDFromPaymentIntent(&paymentIntent))
	if err != nil {
		return err
	}
	if wallet == nil {
		return nil
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

func (s *Stripe) handleTopUpCheckoutSessionCompletedEvent(event stripe.Event) error {
	var checkoutSession stripe.CheckoutSession
	err := json.Unmarshal(event.Data.Raw, &checkoutSession)
	if err != nil {
		return fmt.Errorf("error parsing checkout session JSON: %s", err.Error())
	}

	if checkoutSession.Mode != stripe.CheckoutSessionModePayment {
		return nil
	}

	if checkoutSession.PaymentStatus != stripe.CheckoutSessionPaymentStatusPaid &&
		checkoutSession.PaymentStatus != stripe.CheckoutSessionPaymentStatusNoPaymentRequired {
		log.Info().
			Str("checkout_session_id", checkoutSession.ID).
			Str("payment_status", string(checkoutSession.PaymentStatus)).
			Msg("checkout session payment not complete, skipping topup")
		return nil
	}

	paymentType := checkoutSession.Metadata[topUpMetadataType]
	if paymentType != "" && paymentType != topUpMetadataTypeValue {
		return nil
	}

	userID := checkoutSession.Metadata[topUpMetadataUserID]
	orgID := checkoutSession.Metadata[topUpMetadataOrgID]
	stripeCustomerID := stripeCustomerIDFromCheckoutSession(&checkoutSession)

	if userID == "" && orgID == "" && stripeCustomerID == "" {
		log.Info().
			Str("checkout_session_id", checkoutSession.ID).
			Msg("no user, org, or customer id found in checkout session metadata, skipping")
		return nil
	}

	amount, err := topUpAmountDollars(checkoutSession.Metadata, checkoutSession.AmountSubtotal)
	if err != nil {
		return err
	}

	ctx := context.Background()
	wallet, err := s.getTopUpWallet(ctx, userID, orgID, stripeCustomerID)
	if err != nil {
		return err
	}
	if wallet == nil {
		return nil
	}

	_, err = s.store.UpdateWalletBalance(ctx, wallet.ID, amount, types.TransactionMetadata{
		TransactionType:         types.TransactionTypeTopUp,
		StripePaymentIntentID:   paymentIntentIDFromCheckoutSession(&checkoutSession),
		StripeCheckoutSessionID: checkoutSession.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to create topup for checkout session %s: %w", checkoutSession.ID, err)
	}

	log.Info().
		Str("user_id", userID).
		Str("org_id", orgID).
		Float64("amount", amount).
		Str("wallet_id", wallet.ID).
		Str("checkout_session_id", checkoutSession.ID).
		Msg("topup checkout session completed successfully")

	return nil
}

func (s *Stripe) getTopUpWallet(ctx context.Context, userID, orgID, stripeCustomerID string) (*types.Wallet, error) {
	switch {
	case orgID != "":
		wallet, err := s.store.GetWalletByOrg(ctx, orgID)
		if err == nil {
			return wallet, nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("failed to get wallet for org %s: %w", orgID, err)
		}

		wallet, err = s.store.CreateWallet(ctx, &types.Wallet{
			OrgID:            orgID,
			Balance:          s.cfg.InitialBalance,
			StripeCustomerID: stripeCustomerID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create wallet for org %s: %w", orgID, err)
		}
		return wallet, nil

	case userID != "":
		wallet, err := s.store.GetWalletByUser(ctx, userID)
		if err == nil {
			return wallet, nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("failed to get wallet for user %s: %w", userID, err)
		}

		wallet, err = s.store.CreateWallet(ctx, &types.Wallet{
			UserID:           userID,
			Balance:          s.cfg.InitialBalance,
			StripeCustomerID: stripeCustomerID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create wallet for user %s: %w", userID, err)
		}
		return wallet, nil

	case stripeCustomerID != "":
		wallet, err := s.store.GetWalletByStripeCustomerID(ctx, stripeCustomerID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				log.Info().
					Str("customer_id", stripeCustomerID).
					Msg("no wallet found for stripe customer id, skipping topup")
				return nil, nil
			}
			return nil, fmt.Errorf("failed to get wallet for stripe customer %s: %w", stripeCustomerID, err)
		}
		return wallet, nil

	default:
		return nil, fmt.Errorf("no wallet found for user %s or org %s", userID, orgID)
	}
}

func topUpMetadata(userID, orgID string, amountInCents int64) map[string]string {
	return map[string]string{
		topUpMetadataUserID:      userID,
		topUpMetadataOrgID:       orgID,
		topUpMetadataType:        topUpMetadataTypeValue,
		topUpMetadataAmountCents: strconv.FormatInt(amountInCents, 10),
	}
}

func cloneMetadata(metadata map[string]string) map[string]string {
	cloned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func topUpAmountDollars(metadata map[string]string, fallbackCents int64) (float64, error) {
	amountInCents := fallbackCents
	if rawAmount := metadata[topUpMetadataAmountCents]; rawAmount != "" {
		parsedAmount, err := strconv.ParseInt(rawAmount, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid topup amount metadata %q: %w", rawAmount, err)
		}
		amountInCents = parsedAmount
	}

	if amountInCents <= 0 {
		return 0, fmt.Errorf("topup amount must be greater than 0")
	}

	return float64(amountInCents) / 100.0, nil
}

func stripeCustomerIDFromPaymentIntent(paymentIntent *stripe.PaymentIntent) string {
	if paymentIntent == nil || paymentIntent.Customer == nil {
		return ""
	}
	return paymentIntent.Customer.ID
}

func stripeCustomerIDFromCheckoutSession(checkoutSession *stripe.CheckoutSession) string {
	if checkoutSession == nil || checkoutSession.Customer == nil {
		return ""
	}
	return checkoutSession.Customer.ID
}

func paymentIntentIDFromCheckoutSession(checkoutSession *stripe.CheckoutSession) string {
	if checkoutSession == nil || checkoutSession.PaymentIntent == nil {
		return ""
	}
	return checkoutSession.PaymentIntent.ID
}
