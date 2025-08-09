package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Wallet methods

func (s *PostgresStore) CreateWallet(ctx context.Context, wallet *types.Wallet) (*types.Wallet, error) {
	if wallet.ID == "" {
		wallet.ID = system.GenerateWalletID()
	}

	if wallet.UserID == "" && wallet.OrgID == "" {
		return nil, fmt.Errorf("either user_id or org_id must be specified")
	}

	// If both are specified, it's not good either
	if wallet.UserID != "" && wallet.OrgID != "" {
		return nil, fmt.Errorf("either user_id or org_id must be specified, not both")
	}

	wallet.CreatedAt = time.Now()
	wallet.UpdatedAt = time.Now()

	err := s.gdb.WithContext(ctx).Create(wallet).Error
	if err != nil {
		return nil, err
	}
	return s.GetWallet(ctx, wallet.ID)
}

func (s *PostgresStore) GetWallet(ctx context.Context, id string) (*types.Wallet, error) {
	if id == "" {
		return nil, fmt.Errorf("id not specified")
	}

	var wallet types.Wallet
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&wallet).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &wallet, nil
}

func (s *PostgresStore) GetWalletByUser(ctx context.Context, userID string) (*types.Wallet, error) {
	if userID == "" {
		return nil, fmt.Errorf("user_id not specified")
	}

	var wallet types.Wallet
	err := s.gdb.WithContext(ctx).Where("user_id = ?", userID).First(&wallet).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &wallet, nil
}

func (s *PostgresStore) GetWalletByStripeCustomerID(ctx context.Context, stripeCustomerID string) (*types.Wallet, error) {
	if stripeCustomerID == "" {
		return nil, fmt.Errorf("stripe_customer_id not specified")
	}

	var wallet types.Wallet
	err := s.gdb.WithContext(ctx).Where("stripe_customer_id = ?", stripeCustomerID).First(&wallet).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &wallet, nil
}

func (s *PostgresStore) GetWalletByOrg(ctx context.Context, orgID string) (*types.Wallet, error) {
	if orgID == "" {
		return nil, fmt.Errorf("org_id not specified")
	}

	var wallet types.Wallet
	err := s.gdb.WithContext(ctx).Where("org_id = ?", orgID).First(&wallet).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &wallet, nil
}

// UpdateWallet updates subscription ID, status (does not update balance, for that use dedicated method)
func (s *PostgresStore) UpdateWallet(ctx context.Context, wallet *types.Wallet) (*types.Wallet, error) {
	if wallet.ID == "" {
		return nil, fmt.Errorf("id not specified")
	}

	wallet.UpdatedAt = time.Now()

	err := s.gdb.WithContext(ctx).Model(&types.Wallet{}).Where("id = ?", wallet.ID).Updates(
		map[string]interface{}{
			"updated_at":                        wallet.UpdatedAt,
			"stripe_subscription_id":            wallet.StripeSubscriptionID,
			"subscription_status":               wallet.SubscriptionStatus,
			"subscription_current_period_start": wallet.SubscriptionCurrentPeriodStart,
			"subscription_current_period_end":   wallet.SubscriptionCurrentPeriodEnd,
			"subscription_created":              wallet.SubscriptionCreated,
		},
	).Error
	if err != nil {
		return nil, err
	}
	return s.GetWallet(ctx, wallet.ID)
}

func (s *PostgresStore) DeleteWallet(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}

	err := s.gdb.WithContext(ctx).Delete(&types.Wallet{ID: id}).Error
	if err != nil {
		return err
	}

	return nil
}

// UpdateWalletBalance safely updates the wallet balance using a database transaction
func (s *PostgresStore) UpdateWalletBalance(ctx context.Context, walletID string, amount float64) (*types.Wallet, error) {
	if walletID == "" {
		return nil, fmt.Errorf("wallet_id not specified")
	}

	var wallet *types.Wallet
	err := s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Get the current wallet with a row lock
		var currentWallet types.Wallet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", walletID).First(&currentWallet).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}

		// Calculate new balance
		newBalance := currentWallet.Balance + amount
		if newBalance < 0 {
			return fmt.Errorf("insufficient balance: current balance %.2f, attempted to deduct %.2f", currentWallet.Balance, -amount)
		}

		// Update the balance
		currentWallet.Balance = newBalance
		currentWallet.UpdatedAt = time.Now()

		if err := tx.Save(&currentWallet).Error; err != nil {
			return err
		}

		wallet = &currentWallet
		return nil
	})

	if err != nil {
		return nil, err
	}

	return wallet, nil
}

// Transaction methods

func (s *PostgresStore) CreateTransaction(ctx context.Context, transaction *types.Transaction) (*types.Transaction, error) {
	if transaction.ID == "" {
		transaction.ID = system.GenerateTransactionID()
	}

	if transaction.WalletID == "" {
		return nil, fmt.Errorf("wallet_id not specified")
	}

	transaction.CreatedAt = time.Now()
	transaction.UpdatedAt = time.Now()

	err := s.gdb.WithContext(ctx).Create(transaction).Error
	if err != nil {
		return nil, err
	}
	return transaction, nil
}

type ListTransactionsQuery struct {
	WalletID      string
	InteractionID string
	LLMCallID     string
	Limit         int
	Offset        int
}

func (s *PostgresStore) ListTransactions(ctx context.Context, q *ListTransactionsQuery) ([]*types.Transaction, error) {
	var transactions []*types.Transaction

	query := s.gdb.WithContext(ctx)

	if q.WalletID != "" {
		query = query.Where("wallet_id = ?", q.WalletID)
	}

	if q.InteractionID != "" {
		query = query.Where("interaction_id = ?", q.InteractionID)
	}

	if q.LLMCallID != "" {
		query = query.Where("llm_call_id = ?", q.LLMCallID)
	}

	if q.Limit > 0 {
		query = query.Limit(q.Limit)
	}

	if q.Offset > 0 {
		query = query.Offset(q.Offset)
	}

	err := query.Order("created_at DESC").Find(&transactions).Error
	if err != nil {
		return nil, err
	}

	return transactions, nil
}

// TopUp methods

func (s *PostgresStore) CreateTopUp(ctx context.Context, topUp *types.TopUp) (*types.TopUp, error) {
	if topUp.ID == "" {
		topUp.ID = system.GenerateTopUpID()
	}

	if topUp.WalletID == "" {
		return nil, fmt.Errorf("wallet_id not specified")
	}

	if topUp.Amount <= 0 {
		return nil, fmt.Errorf("amount must be greater than 0")
	}

	topUp.CreatedAt = time.Now()
	topUp.UpdatedAt = time.Now()

	err := s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Create the top-up record
		if err := tx.Create(topUp).Error; err != nil {
			return err
		}

		// Update the wallet balance
		if err := tx.Model(&types.Wallet{}).Where("id = ?", topUp.WalletID).
			UpdateColumn("balance", gorm.Expr("balance + ?", topUp.Amount)).Error; err != nil {
			return err
		}

		// Update the wallet's updated_at timestamp
		if err := tx.Model(&types.Wallet{}).Where("id = ?", topUp.WalletID).
			UpdateColumn("updated_at", time.Now()).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return topUp, nil
}

type ListTopUpsQuery struct {
	WalletID string
	Limit    int
	Offset   int
}

func (s *PostgresStore) ListTopUps(ctx context.Context, q *ListTopUpsQuery) ([]*types.TopUp, error) {
	var topUps []*types.TopUp

	query := s.gdb.WithContext(ctx)

	if q.WalletID != "" {
		query = query.Where("wallet_id = ?", q.WalletID)
	}

	if q.Limit > 0 {
		query = query.Limit(q.Limit)
	}

	if q.Offset > 0 {
		query = query.Offset(q.Offset)
	}

	err := query.Order("created_at DESC").Find(&topUps).Error
	if err != nil {
		return nil, err
	}

	return topUps, nil
}
