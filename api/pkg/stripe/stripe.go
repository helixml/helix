package stripe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"github.com/stripe/stripe-go/v76"
	portalsession "github.com/stripe/stripe-go/v76/billingportal/session"
	"github.com/stripe/stripe-go/v76/checkout/session"
	"github.com/stripe/stripe-go/v76/customer"
	"github.com/stripe/stripe-go/v76/price"
	"github.com/stripe/stripe-go/v76/subscription"
	"github.com/stripe/stripe-go/v76/webhook"
)

type EventHandler func(eventType types.SubscriptionEventType, user types.StripeUser) error

type TopUpEventHandler func(paymentIntentID, orgID, userID string, amount float64) error

type Stripe struct {
	cfg   config.Stripe
	store store.Store
	// subscriptionEventHandler EventHandler
	// topUpEventHandler        TopUpEventHandler
}

func NewStripe(
	cfg config.Stripe,
	store store.Store,
	// subscriptionEventHandler EventHandler,
	// topUpEventHandler TopUpEventHandler,
) *Stripe {
	if cfg.SecretKey != "" {
		stripe.Key = cfg.SecretKey
	}
	return &Stripe{
		cfg:   cfg,
		store: store,
	}
}

func (s *Stripe) Enabled() bool {
	return s.cfg.SecretKey != "" && s.cfg.WebhookSigningSecret != ""
}

func (s *Stripe) EnabledError() error {
	if s.cfg.SecretKey == "" {
		return fmt.Errorf("stripe secret key is required")
	}
	if s.cfg.WebhookSigningSecret == "" {
		return fmt.Errorf("stripe webhook signing secret is required")
	}
	return nil
}

// CreateStripeCustomer - creates a stripe customer for a user or organization, Stripe customer
// ID is then stored in the wallet.
func (s *Stripe) CreateStripeCustomer(user *types.User, orgID string) (string, error) {
	customerParams := &stripe.CustomerParams{
		Email: stripe.String(user.Email),
	}

	if orgID != "" {
		customerParams.Description = stripe.String(fmt.Sprintf("organization %s", orgID))
		customerParams.AddMetadata("account_type", "organization")
		customerParams.AddMetadata("org_id", orgID)
	} else {
		customerParams.Description = stripe.String(fmt.Sprintf("user %s", user.ID))
		customerParams.AddMetadata("account_type", "user")
		customerParams.AddMetadata("user_id", user.ID)
	}

	customer, err := customer.New(customerParams)
	if err != nil {
		return "", err
	}
	return customer.ID, nil
}

func (s *Stripe) ListSubscriptions(stripeCustomerID string) ([]*stripe.Subscription, error) {
	subscriptions := subscription.List(
		&stripe.SubscriptionListParams{
			Customer: stripe.String(stripeCustomerID),
		},
	)

	var subs []*stripe.Subscription
	for subscriptions.Next() {
		sub := subscriptions.Subscription()
		subs = append(subs, sub)
	}
	return subs, nil
}

func (s *Stripe) getSubscriptionURL(id string) string {
	testMode := ""
	if strings.HasPrefix(s.cfg.SecretKey, "sk_test_") {
		testMode = "/test"
	}
	return fmt.Sprintf("https://dashboard.stripe.com%s/subscriptions/%s", testMode, id)
}

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
		Mode: stripe.String(string(stripe.CheckoutSessionModePayment)),
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

var eventMap = map[stripe.EventType]types.SubscriptionEventType{
	"customer.subscription.deleted": types.SubscriptionEventTypeDeleted,
	"customer.subscription.updated": types.SubscriptionEventTypeUpdated,
	"customer.subscription.created": types.SubscriptionEventTypeCreated,
}

// Activates/deactivates subscription for a wallet
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

func (s *Stripe) handleTopUpEvent(event stripe.Event) error {
	var paymentIntent stripe.PaymentIntent
	err := json.Unmarshal(event.Data.Raw, &paymentIntent)
	if err != nil {
		return fmt.Errorf("error parsing payment intent JSON: %s", err.Error())
	}

	userID := paymentIntent.Metadata["user_id"]
	if userID == "" {
		return fmt.Errorf("no user_id found in metadata")
	}

	orgID := paymentIntent.Metadata["org_id"]
	if orgID == "" {
		return fmt.Errorf("no org_id found in metadata")
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

	}

	// Create topup record
	topUp := &types.TopUp{
		StripePaymentIntentID: paymentIntent.ID,
		WalletID:              wallet.ID,
		Amount:                amount,
	}

	_, err = s.store.CreateTopUp(ctx, topUp)
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

// handleInvoicePaymentPaidEvent is received when a user pays an invoice for a subscription
func (s *Stripe) handleInvoicePaymentPaidEvent(event stripe.Event) error {

	return nil
}

func (s *Stripe) ProcessWebhook(w http.ResponseWriter, req *http.Request) {
	const MaxBodyBytes = int64(65536)
	bodyReader := http.MaxBytesReader(w, req.Body, MaxBodyBytes)
	payload, err := io.ReadAll(bodyReader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading request body: %v\n", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	endpointSecret := s.cfg.WebhookSigningSecret
	signatureHeader := req.Header.Get("Stripe-Signature")
	event, err := webhook.ConstructEventWithOptions(payload, signatureHeader, endpointSecret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})
	if err != nil {
		log.Error().Msgf("Error verifying webhook signature: %s", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Handle different event types
	switch event.Type {
	case "customer.subscription.deleted", "customer.subscription.updated", "customer.subscription.created":
		err := s.handleSubscriptionEvent(event)
		if err != nil {
			log.Error().Msgf("Error handling subscription event: %s", err.Error())
		}
		return
	case "invoice_payment.paid":
		err := s.handleInvoicePaymentPaidEvent(event)
		if err != nil {
			log.Error().Msgf("Error handling invoice payment paid event: %s", err.Error())
		}
		return
	case "payment_intent.succeeded":
		err := s.handleTopUpEvent(event)
		if err != nil {
			log.Error().Msgf("Error handling top up event: %s", err.Error())
		}
		return
	default:
		// Log unhandled events but don't fail
		// fmt.Fprintf(os.Stderr, "Unhandled event type: %s\n", event.Type)
		// err = nil
		log.Info().Msgf("Unhandled event type: %s", event.Type)
		return
	}
}
