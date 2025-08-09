package types

import (
	"time"

	"github.com/stripe/stripe-go/v76"
)

// Wallet is a user's wallet for storing credits, it can either
// be associated with a user or an organization.
type Wallet struct {
	ID               string    `json:"id" gorm:"primaryKey"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	StripeCustomerID string    `json:"stripe_customer_id" gorm:"index"`

	StripeSubscriptionID           string                    `json:"stripe_subscription_id" gorm:"index"`
	SubscriptionStatus             stripe.SubscriptionStatus `json:"subscription_status"`
	SubscriptionCurrentPeriodStart int64                     `json:"subscription_current_period_start"`
	SubscriptionCurrentPeriodEnd   int64                     `json:"subscription_current_period_end"`
	SubscriptionCreated            int64                     `json:"subscription_created"`

	UserID  string  `json:"user_id" gorm:"index"`
	OrgID   string  `json:"org_id" gorm:"index"` // If belongs to an organization
	Balance float64 `json:"balance"`
}

// Transaction is a record of a transaction on a wallet.
type Transaction struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	WalletID  string    `json:"wallet_id" gorm:"index"`
	Amount    float64   `json:"amount"`

	InteractionID string `json:"interaction_id"`
	LLMCallID     string `json:"llm_call_id"`
}

type TopUpType string

const (
	TopUpTypeRegular      TopUpType = "regular"
	TopUpTypeSubscription TopUpType = "subscription"
)

type TopUp struct {
	ID                    string    `json:"id" gorm:"primaryKey"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	StripePaymentIntentID string    `json:"stripe_payment_intent_id"`
	WalletID              string    `json:"wallet_id" gorm:"index"`
	Amount                float64   `json:"amount"`
	Type                  TopUpType `json:"type"`
}
