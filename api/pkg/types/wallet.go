package types

import "time"

// Wallet is a user's wallet for storing credits, it can either
// be associated with a user or an organization.
type Wallet struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	UserID    string    `json:"user_id" gorm:"index"`
	OrgID     string    `json:"org_id" gorm:"index"` // If belongs to an organization
	Balance   float64   `json:"balance"`
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

type TopUp struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	WalletID  string    `json:"wallet_id" gorm:"index"`
	Amount    float64   `json:"amount"`
}
