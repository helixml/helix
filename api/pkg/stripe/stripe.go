package stripe

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"github.com/stripe/stripe-go/v76"
	portalsession "github.com/stripe/stripe-go/v76/billingportal/session"
	"github.com/stripe/stripe-go/v76/checkout/session"
	"github.com/stripe/stripe-go/v76/customer"
	"github.com/stripe/stripe-go/v76/price"
	"github.com/stripe/stripe-go/v76/webhook"
)

type EventHandler func(eventType types.SubscriptionEventType, user types.StripeUser) error

type TopUpEventHandler func(paymentIntentID, orgID, userID string, amount float64) error

type Stripe struct {
	Cfg               config.Stripe
	eventHandler      EventHandler
	topUpEventHandler TopUpEventHandler
}

func NewStripe(
	cfg config.Stripe,
	eventHandler EventHandler,
	topUpEventHandler TopUpEventHandler,
) *Stripe {
	if cfg.SecretKey != "" {
		stripe.Key = cfg.SecretKey
	}
	return &Stripe{
		Cfg:               cfg,
		eventHandler:      eventHandler,
		topUpEventHandler: topUpEventHandler,
	}
}

func (s *Stripe) Enabled() bool {
	return s.Cfg.SecretKey != "" && s.Cfg.WebhookSigningSecret != ""
}

func (s *Stripe) EnabledError() error {
	if s.Cfg.SecretKey == "" {
		return fmt.Errorf("stripe secret key is required")
	}
	if s.Cfg.WebhookSigningSecret == "" {
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

func (s *Stripe) getSubscriptionURL(id string) string {
	testMode := ""
	if strings.HasPrefix(s.Cfg.SecretKey, "sk_test_") {
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

	successURL := s.Cfg.AppURL + "/account?success=true&session_id={CHECKOUT_SESSION_ID}"
	if params.OrgID != "" {
		successURL = s.Cfg.AppURL + "/orgs/" + params.OrgName + "/billing?success=true&session_id={CHECKOUT_SESSION_ID}"
	}

	cancelURL := s.Cfg.AppURL + "/account?canceled=true"
	if params.OrgID != "" {
		cancelURL = s.Cfg.AppURL + "/orgs/" + params.OrgName + "/billing?canceled=true"
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

func (s *Stripe) GetCheckoutSessionURL(
	orgID string,
	userID string,
	userEmail string,
) (string, error) {
	err := s.EnabledError()
	if err != nil {
		return "", err
	}
	params := &stripe.PriceListParams{
		LookupKeys: stripe.StringSlice([]string{
			s.Cfg.PriceLookupKey,
		}),
	}
	priceResult := price.List(params)
	var price *stripe.Price
	for priceResult.Next() {
		price = priceResult.Price()
	}
	if price == nil {
		return "", fmt.Errorf("price not found")
	}
	checkoutParams := &stripe.CheckoutSessionParams{
		Mode: stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		// this is how we link the subscription to our user
		SubscriptionData: &stripe.CheckoutSessionSubscriptionDataParams{
			Metadata: map[string]string{
				"user_id": userID,
				"org_id":  orgID,
			},
		},
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(price.ID),
				Quantity: stripe.Int64(1),
			},
		},
		CustomerEmail: stripe.String(userEmail),
		SuccessURL:    stripe.String(s.Cfg.AppURL + "/account?success=true&session_id={CHECKOUT_SESSION_ID}"),
		CancelURL:     stripe.String(s.Cfg.AppURL + "/account?canceled=true"),
	}

	newSession, err := session.New(checkoutParams)
	if err != nil {
		return "", err
	}

	return newSession.URL, nil
}

func (s *Stripe) GetPortalSessionURL(
	customerID string,
) (string, error) {
	params := &stripe.BillingPortalSessionParams{
		Customer:  stripe.String(customerID),
		ReturnURL: stripe.String(s.Cfg.AppURL + "/account"),
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
	userID := subscription.Metadata["user_id"]
	if userID == "" {
		return fmt.Errorf("no user_id found in metadata")
	}
	customerData, err := customer.Get(subscription.Customer.ID, nil)
	if err != nil {
		return fmt.Errorf("error loading customer: %s", err.Error())
	}
	user := types.StripeUser{
		HelixID:         userID,
		StripeID:        customerData.ID,
		Email:           customerData.Email,
		SubscriptionID:  subscription.ID,
		SubscriptionURL: s.getSubscriptionURL(subscription.ID),
	}
	if err := s.eventHandler(eventType, user); err != nil {
		return fmt.Errorf("error handling event: %v", err)
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

	if s.topUpEventHandler != nil {
		return s.topUpEventHandler(paymentIntent.ID, orgID, userID, amount)
	}

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
	endpointSecret := s.Cfg.WebhookSigningSecret
	signatureHeader := req.Header.Get("Stripe-Signature")
	event, err := webhook.ConstructEventWithOptions(payload, signatureHeader, endpointSecret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Webhook signature verification failed. %s\n", err.Error())
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
