package stripe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/config"
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
	ReturnURL        string // Optional custom return URL (overrides default success/cancel URLs)
	TrialPeriodDays  int64  // Optional card-backed trial; zero creates the subscription without a trial
}

const onboardingTrialCredits = 1.0
const trialSourceOnboarding = "onboarding"

func subscriptionIsLive(sub *stripe.Subscription) bool {
	return sub != nil && sub.Status != stripe.SubscriptionStatusCanceled &&
		sub.Status != stripe.SubscriptionStatusIncompleteExpired
}

func validateOrgSubscriptionPrice(p *stripe.Price, cfg config.Stripe) error {
	if p.UnitAmount != cfg.OrgPriceCents || string(p.Currency) != cfg.OrgPriceCurrency ||
		p.Recurring == nil || string(p.Recurring.Interval) != cfg.OrgPriceInterval {
		return fmt.Errorf(
			"organization subscription price %s does not match configured %d %s/%s",
			p.ID, cfg.OrgPriceCents, cfg.OrgPriceCurrency, cfg.OrgPriceInterval,
		)
	}
	return nil
}

func (s *Stripe) GetCheckoutSessionURL(
	params SubscriptionSessionParams,
) (string, error) {
	err := s.EnabledError()
	if err != nil {
		return "", err
	}
	if params.TrialPeriodDays > 0 {
		subscriptions, err := s.ListSubscriptions(params.StripeCustomerID)
		if err != nil {
			return "", fmt.Errorf("failed to check existing subscriptions: %w", err)
		}
		for _, sub := range subscriptions {
			if subscriptionIsLive(sub) {
				return "", fmt.Errorf("customer already has a live subscription")
			}
		}
	}

	defaultSuccessURL := s.cfg.AppURL + "/account?success=true&session_id={CHECKOUT_SESSION_ID}"
	defaultCancelURL := s.cfg.AppURL + "/account?canceled=true"
	if params.OrgID != "" {
		defaultSuccessURL = s.cfg.AppURL + "/orgs/" + params.OrgName + "/billing?success=true&session_id={CHECKOUT_SESSION_ID}"
		defaultCancelURL = s.cfg.AppURL + "/orgs/" + params.OrgName + "/billing?canceled=true"
	}
	successURL, cancelURL, err := checkoutReturnURLs(
		s.cfg.AppURL,
		params.ReturnURL,
		defaultSuccessURL,
		defaultCancelURL,
	)
	if err != nil {
		return "", err
	}

	priceLookupKey := s.cfg.PriceLookupKey
	if params.OrgID != "" {
		priceLookupKey = s.cfg.OrgPriceLookupKey
	}

	priceParams := &stripe.PriceListParams{
		Active: stripe.Bool(true),
		LookupKeys: stripe.StringSlice([]string{
			priceLookupKey,
		}),
	}
	priceResult := price.List(priceParams)
	var price *stripe.Price
	// Lookup keys should be unique; if Stripe returns more than one active
	// match, consistently use the first result.
	if priceResult.Next() {
		price = priceResult.Price()
	}
	if err := priceResult.Err(); err != nil {
		return "", fmt.Errorf("failed to find price for lookup key %s: %w", priceLookupKey, err)
	}
	if price == nil {
		return "", fmt.Errorf("price not found for lookup key %s", priceLookupKey)
	}
	if params.OrgID != "" {
		if err := validateOrgSubscriptionPrice(price, s.cfg); err != nil {
			return "", err
		}
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
	if params.TrialPeriodDays > 0 {
		checkoutParams.PaymentMethodCollection = stripe.String("always")
		checkoutParams.SubscriptionData.TrialPeriodDays = stripe.Int64(params.TrialPeriodDays)
		checkoutParams.SubscriptionData.Metadata["trial_source"] = trialSourceOnboarding
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

	wasTrialing := wallet.SubscriptionStatus == stripe.SubscriptionStatusTrialing
	wallet.StripeSubscriptionID = subscription.ID
	wallet.SubscriptionCurrentPeriodStart = subscription.CurrentPeriodStart
	wallet.SubscriptionCurrentPeriodEnd = subscription.CurrentPeriodEnd
	wallet.SubscriptionCreated = subscription.Created
	wallet.SubscriptionCancelAtPeriodEnd = subscription.CancelAtPeriodEnd

	if eventType == types.SubscriptionEventTypeDeleted {
		wallet.SubscriptionStatus = stripe.SubscriptionStatusCanceled
	} else {
		wallet.SubscriptionStatus = subscription.Status
	}

	_, err = s.store.UpdateWallet(ctx, wallet)
	if err != nil {
		return fmt.Errorf("failed to update wallet: %w", err)
	}

	if subscription.Metadata["trial_source"] == trialSourceOnboarding {
		meta := types.TransactionMetadata{
			StripeSubscriptionID: subscription.ID,
			UserID:               subscription.Metadata["user_id"],
		}
		trialCanceled := (eventType == types.SubscriptionEventTypeDeleted && wasTrialing) ||
			(subscription.Status == stripe.SubscriptionStatusTrialing && subscription.CancelAtPeriodEnd)
		if trialCanceled {
			meta.TransactionType = types.TransactionTypeTrialRevoke
			meta.IdempotencyKey = "trial-credit-revoke:" + subscription.ID
			if _, err := s.store.UpdateWalletBalance(ctx, wallet.ID, -onboardingTrialCredits, meta); err != nil {
				return fmt.Errorf("failed to revoke trial credits: %w", err)
			}
		} else if subscription.Status == stripe.SubscriptionStatusTrialing {
			meta.TransactionType = types.TransactionTypeTrialCredit
			meta.IdempotencyKey = "trial-credit-grant:" + subscription.ID
			if _, err := s.store.UpdateWalletBalance(ctx, wallet.ID, onboardingTrialCredits, meta); err != nil {
				return fmt.Errorf("failed to grant trial credits: %w", err)
			}
		}
	}

	return nil
}
