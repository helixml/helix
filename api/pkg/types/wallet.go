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

type TransactionMetadata struct {
	InteractionID         string          `json:"interaction_id"`
	LLMCallID             string          `json:"llm_call_id"`
	TopUpID               string          `json:"top_up_id"`
	StripePaymentIntentID string          `json:"stripe_payment_intent_id"`
	TransactionType       TransactionType `json:"transaction_type"`
}

type TransactionType string

const (
	TransactionTypeUsage        TransactionType = "usage"
	TransactionTypeTopUp        TransactionType = "top_up"
	TransactionTypeSubscription TransactionType = "subscription"
)

// Transaction is a record of a transaction on a wallet.
type Transaction struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	WalletID  string    `json:"wallet_id" gorm:"index"`
	Amount    float64   `json:"amount"`

	BalanceBefore float64 `json:"balance_before"`
	BalanceAfter  float64 `json:"balance_after"`

	Type TransactionType `json:"type" gorm:"index"`

	InteractionID string `json:"interaction_id"` // For usage
	LLMCallID     string `json:"llm_call_id"`    // For usage

	TopUpID string `json:"top_up_id"` // For top-ups
}

type TopUp struct {
	ID                    string    `json:"id" gorm:"primaryKey"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	StripePaymentIntentID string    `json:"stripe_payment_intent_id"`
	WalletID              string    `json:"wallet_id" gorm:"index"`
	Amount                float64   `json:"amount"`
}

type PaymentIntent struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	WalletID  string    `json:"wallet_id" gorm:"index"`
	Amount    float64   `json:"amount"`
}
