package stripe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"github.com/stripe/stripe-go/v76"
	portalsession "github.com/stripe/stripe-go/v76/billingportal/session"
	"github.com/stripe/stripe-go/v76/checkout/session"
	"github.com/stripe/stripe-go/v76/price"
)

type SubscriptionSessionParams struct {
	StripeCustomerID string
	OrgID            string
	OrgName          string // Used for redirect URL (for example 'acme-org' for 'orgs/acme-org/billing')
	UserID           string
	Amount           float64
}

func (s *Stripe) GetCheckoutSessionURL(
	params SubscriptionSessionParams,
) (string, error) {
	err := s.EnabledError()
	if err != nil {
		return "", err
	}

	priceLookupKey := s.cfg.PriceLookupKey
	if params.OrgID != "" {
		priceLookupKey = s.cfg.OrgPriceLookupKey
	}

	priceParams := &stripe.PriceListParams{
		LookupKeys: stripe.StringSlice([]string{
			priceLookupKey,
		}),
	}
	priceResult := price.List(priceParams)
	var price *stripe.Price
	for priceResult.Next() {
		price = priceResult.Price()
	}
	if price == nil {
		return "", fmt.Errorf("price not found")
	}

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
		Mode:                stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		// this is how we link the subscription to our user
		SubscriptionData: &stripe.CheckoutSessionSubscriptionDataParams{
			Metadata: map[string]string{
				"user_id": params.UserID,
				"org_id":  params.OrgID,
			},
		},
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(price.ID),
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

func (s *Stripe) GetPortalSessionURL(
	stripeCustomerID string, orgName string,
) (string, error) {
	returnURL := s.cfg.AppURL + "/account"
	if orgName != "" {
		returnURL = s.cfg.AppURL + "/orgs/" + orgName + "/billing"
	}

	params := &stripe.BillingPortalSessionParams{
		Customer:  stripe.String(stripeCustomerID),
		ReturnURL: stripe.String(returnURL),
	}

	ps, err := portalsession.New(params)

	if err != nil {
		return "", err
	}

	return ps.URL, nil
}

func (s *Stripe) handleSubscriptionEvent(event stripe.Event) error {
	eventType, ok := eventMap[event.Type]
	if !ok {
		return fmt.Errorf("unhandled event type: %s", event.Type)
	}
	var subscription stripe.Subscription
	err := json.Unmarshal(event.Data.Raw, &subscription)
	if err != nil {
		return fmt.Errorf("error parsing webhook JSON: %s", err.Error())
	}

	stripeCustomerID := subscription.Customer.ID
	if stripeCustomerID == "" {
		log.Error().Any("subscription", subscription).Msgf("no stripe customer id found in subscription")
		return fmt.Errorf("no stripe customer id found in subscription")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wallet, err := s.store.GetWalletByStripeCustomerID(ctx, stripeCustomerID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			log.Info().
				Str("customer_id", stripeCustomerID).
				Str("subscription_id", subscription.ID).
				Msg("no wallet found for stripe customer id, skipping")
			return nil
		}
		return err
	}

	wallet.StripeSubscriptionID = subscription.ID
	wallet.SubscriptionCurrentPeriodStart = subscription.CurrentPeriodStart
	wallet.SubscriptionCurrentPeriodEnd = subscription.CurrentPeriodEnd
	wallet.SubscriptionCreated = subscription.Created

	if eventType == types.SubscriptionEventTypeDeleted {
		wallet.SubscriptionStatus = stripe.SubscriptionStatusCanceled
	} else {
		wallet.SubscriptionStatus = subscription.Status
	}

	_, err = s.store.UpdateWallet(ctx, wallet)
	if err != nil {
		return fmt.Errorf("failed to update wallet: %w", err)
	}

	// Create topup for subscription created events
	if eventType == types.SubscriptionEventTypeCreated && subscription.Status == stripe.SubscriptionStatusActive {
		err = s.createSubscriptionTopup(ctx, wallet, &subscription, "subscription_created")
		if err != nil {
			log.Error().
				Str("wallet_id", wallet.ID).
				Str("subscription_id", subscription.ID).
				Err(err).
				Msg("failed to create topup for subscription creation")
			// Don't fail the entire webhook processing, just log the error
		}
	}

	return nil
}

// createSubscriptionTopup creates a topup record for subscription events
func (s *Stripe) createSubscriptionTopup(ctx context.Context, wallet *types.Wallet, subscription *stripe.Subscription, reason string) error {
	// Get the subscription price amount
	if len(subscription.Items.Data) == 0 {
		return fmt.Errorf("no subscription items found")
	}

	if subscription.Items.Data[0].Price == nil {
		log.Error().
			Str("wallet_id", wallet.ID).
			Str("subscription_id", subscription.ID).
			Str("subscription_item_id", subscription.Items.Data[0].ID).
			Any("subscription", subscription).
			Msg("no price found for subscription item")
		return fmt.Errorf("no price found for subscription item")
	}

	// Get the first subscription item (assuming single product subscription)
	subscriptionItem := subscription.Items.Data[0]

	// Calculate the amount in dollars (Stripe amounts are in cents)
	amount := float64(subscriptionItem.Price.UnitAmount) / 100.0

	// Adjust balance
	meta := types.TransactionMetadata{
		TransactionType: types.TransactionTypeSubscription,
	}
	_, err := s.store.UpdateWalletBalance(ctx, wallet.ID, amount, meta)
	if err != nil {
		return fmt.Errorf("failed to update wallet balance: %w", err)
	}

	log.Info().
		Str("wallet_id", wallet.ID).
		Str("user_id", wallet.UserID).
		Str("org_id", wallet.OrgID).
		Float64("amount", amount).
		Str("subscription_id", subscription.ID).
		Str("reason", reason).
		Msg("subscription topup created successfully")

	return nil
}
