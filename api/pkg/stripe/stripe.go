package stripe

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/davecgh/go-spew/spew"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/customer"
	"github.com/stripe/stripe-go/v76/subscription"
	"github.com/stripe/stripe-go/v76/webhook"
)

type EventHandler func(eventType types.SubscriptionEventType, user types.StripeUser) error

type TopUpEventHandler func(paymentIntentID, orgID, userID string, amount float64) error

type Stripe struct {
	cfg   config.Stripe
	store store.Store
}

func NewStripe(
	cfg config.Stripe,
	store store.Store,
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

var eventMap = map[stripe.EventType]types.SubscriptionEventType{
	"customer.subscription.deleted": types.SubscriptionEventTypeDeleted,
	"customer.subscription.updated": types.SubscriptionEventTypeUpdated,
	"customer.subscription.created": types.SubscriptionEventTypeCreated,
}

// Activates/deactivates subscription for a wallet

// handleInvoicePaymentPaidEvent is received when a user pays an invoice for a subscription
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
			Msg("invoice payment not successful, skipping topup")
		return nil
	}

	spew.Dump(invoice)

	// stripeCustomerID := invoice.Customer.ID
	// if stripeCustomerID == "" {
	// 	log.Error().Any("invoice", invoice).Msgf("no stripe customer id found in invoice")
	// 	return fmt.Errorf("no stripe customer id found in invoice")
	// }

	// ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	// defer cancel()

	// wallet, err := s.store.GetWalletByStripeCustomerID(ctx, stripeCustomerID)
	// if err != nil {
	// 	return fmt.Errorf("failed to get wallet for stripe customer %s: %w", stripeCustomerID, err)
	// }

	// // Calculate the amount in dollars (Stripe amounts are in cents)
	// amount := float64(invoice.Total) / 100.0

	// // Create subscription topup record
	// topUp := &types.TopUp{
	// 	StripePaymentIntentID: invoice.PaymentIntent.ID,
	// 	WalletID:              wallet.ID,
	// 	Amount:                amount,
	// 	Type:                  types.TopUpTypeSubscription,
	// }

	// _, err = s.store.CreateTopUp(ctx, topUp)
	// if err != nil {
	// 	return fmt.Errorf("failed to create subscription topup for wallet %s: %w", wallet.ID, err)
	// }

	// log.Info().
	// 	Str("wallet_id", wallet.ID).
	// 	Str("user_id", wallet.UserID).
	// 	Str("org_id", wallet.OrgID).
	// 	Float64("amount", amount).
	// 	Str("invoice_id", invoice.ID).
	// 	Msg("subscription topup completed successfully")

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
