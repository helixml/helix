package stripe

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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
	priceParams := &stripe.PriceListParams{
		LookupKeys: stripe.StringSlice([]string{
			s.cfg.PriceLookupKey,
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
		Mode: stripe.String(string(stripe.CheckoutSessionModeSubscription)),
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

	isSubscriptionActive := true
	if eventType == types.SubscriptionEventTypeDeleted {
		isSubscriptionActive = false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wallet, err := s.store.GetWalletByStripeCustomerID(ctx, stripeCustomerID)
	if err != nil {
		return err
	}

	wallet.StripeSubscriptionID = subscription.ID
	wallet.SubscriptionActive = isSubscriptionActive

	_, err = s.store.UpdateWallet(ctx, wallet)
	if err != nil {
		return err
	}

	return nil
}
