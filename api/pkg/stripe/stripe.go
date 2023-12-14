package stripe

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/stripe/stripe-go/v76"
	portalsession "github.com/stripe/stripe-go/v76/billingportal/session"
	"github.com/stripe/stripe-go/v76/checkout/session"
	"github.com/stripe/stripe-go/v76/price"
	"github.com/stripe/stripe-go/v76/webhook"
)

type StripeEventWriter func(userID string, stripeCustomerID string, stripeSubscriptionID string, stripeSubscriptionURL string) error

type StripeOptions struct {
	AppURL               string
	SecretKey            string
	WebhookSigningSecret string
	PriceLookupKey       string
}

type Stripe struct {
	Options       StripeOptions
	onSubscribe   StripeEventWriter
	onUnsubscribe StripeEventWriter
}

func NewStripe(
	opts StripeOptions,
	onSubscribe StripeEventWriter,
	onUnsubscribe StripeEventWriter,
) *Stripe {
	if opts.SecretKey != "" {
		stripe.Key = opts.SecretKey
	}
	return &Stripe{
		Options:       opts,
		onSubscribe:   onSubscribe,
		onUnsubscribe: onUnsubscribe,
	}
}

func (s *Stripe) Enabled() bool {
	return s.Options.SecretKey != "" && s.Options.WebhookSigningSecret != ""
}

func (s *Stripe) EnabledError() error {
	if s.Options.SecretKey == "" {
		return fmt.Errorf("stripe secret key is required")
	}
	if s.Options.WebhookSigningSecret == "" {
		return fmt.Errorf("stripe webhook signing secret is required")
	}
	return nil
}

func (s *Stripe) getSubscriptionURL(id string) string {
	testMode := ""
	if strings.HasPrefix(s.Options.SecretKey, "sk_test_") {
		testMode = "/test"
	}
	return fmt.Sprintf("https://dashboard.stripe.com%s/subscriptions/%s", testMode, id)
}

func (s *Stripe) GetCheckoutSessionURL(
	userID string,
	userEmail string,
) (string, error) {
	err := s.EnabledError()
	if err != nil {
		return "", err
	}
	params := &stripe.PriceListParams{
		LookupKeys: stripe.StringSlice([]string{
			s.Options.PriceLookupKey,
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
			},
		},
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(price.ID),
				Quantity: stripe.Int64(1),
			},
		},
		CustomerEmail: stripe.String(userEmail),
		SuccessURL:    stripe.String(s.Options.AppURL + "/account?success=true&session_id={CHECKOUT_SESSION_ID}"),
		CancelURL:     stripe.String(s.Options.AppURL + "/account?canceled=true"),
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
		ReturnURL: stripe.String(s.Options.AppURL + "/account"),
	}

	ps, err := portalsession.New(params)

	if err != nil {
		return "", err
	}

	return ps.URL, nil
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
	endpointSecret := s.Options.WebhookSigningSecret
	signatureHeader := req.Header.Get("Stripe-Signature")
	event, err := webhook.ConstructEvent(payload, signatureHeader, endpointSecret)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Webhook signature verification failed. %s\n", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	switch event.Type {
	case "customer.subscription.deleted":
		var subscription stripe.Subscription
		err := json.Unmarshal(event.Data.Raw, &subscription)
		if err != nil {
			log.Error().Msgf("Error parsing webhook JSON: %s\n", err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		userID := subscription.Metadata["user_id"]
		if userID == "" {
			log.Error().Msgf("No user_id found in metadata")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		err = s.onUnsubscribe(userID, subscription.Customer.ID, subscription.ID, s.getSubscriptionURL(subscription.ID))
		if err != nil {
			log.Error().Msgf("Error writing event: %s\n", err.Error())
		}
		log.Debug().Msgf("🟠 Subscription %s deleted for %s.", subscription.ID, userID)
	case "customer.subscription.updated":
		var subscription stripe.Subscription
		err := json.Unmarshal(event.Data.Raw, &subscription)
		if err != nil {
			log.Error().Msgf("Error parsing webhook JSON: %s\n", err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		userID := subscription.Metadata["user_id"]
		if userID == "" {
			log.Error().Msgf("No user_id found in metadata")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		err = s.onSubscribe(userID, subscription.Customer.ID, subscription.ID, s.getSubscriptionURL(subscription.ID))
		if err != nil {
			log.Error().Msgf("Error writing event: %s\n", err.Error())
		}
		log.Debug().Msgf("🟠 Subscription %s updated for %s.", subscription.ID, userID)
	case "customer.subscription.created":
		var subscription stripe.Subscription
		err := json.Unmarshal(event.Data.Raw, &subscription)
		if err != nil {
			log.Error().Msgf("Error parsing webhook JSON: %s\n", err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		userID := subscription.Metadata["user_id"]
		if userID == "" {
			log.Error().Msgf("No user_id found in metadata")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		err = s.onSubscribe(userID, subscription.Customer.ID, subscription.ID, s.getSubscriptionURL(subscription.ID))
		if err != nil {
			log.Error().Msgf("Error writing event: %s\n", err.Error())
		}
		log.Debug().Msgf("🟠 Subscription %s created for %s.", subscription.ID, userID)
	default:
		fmt.Fprintf(os.Stderr, "Unhandled event type: %s\n", event.Type)
	}
	w.WriteHeader(http.StatusOK)
}
