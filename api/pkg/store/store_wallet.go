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

type ListWalletsQuery struct {
	OwnerIDs  []string
	OwnerType types.OwnerType
}

func (s *PostgresStore) ListWallets(ctx context.Context, q *ListWalletsQuery) ([]*types.Wallet, error) {
	if q == nil {
		return nil, fmt.Errorf("query not specified")
	}

	if len(q.OwnerIDs) == 0 {
		return nil, fmt.Errorf("owner_ids not specified")
	}

	query := s.gdb.WithContext(ctx)

	switch q.OwnerType {
	case types.OwnerTypeUser:
		query = query.Where("user_id IN ?", q.OwnerIDs)
	case types.OwnerTypeOrg:
		query = query.Where("org_id IN ?", q.OwnerIDs)
	default:
		return nil, fmt.Errorf("owner_type must be '%s' or '%s'", types.OwnerTypeUser, types.OwnerTypeOrg)
	}

	var wallets []*types.Wallet
	err := query.Order("created_at DESC").Find(&wallets).Error
	if err != nil {
		return nil, err
	}

	return wallets, nil
}

// UpdateWalletSubscription writes only the Stripe-mirrored subscription
// columns. It never touches plan_override or balance, so a sync working from a
// stale wallet read cannot revert an admin plan override.
func (s *PostgresStore) UpdateWalletSubscription(ctx context.Context, walletID string, state types.WalletSubscriptionState) (*types.Wallet, error) {
	return s.updateWalletColumns(ctx, walletID, map[string]interface{}{
		"stripe_subscription_id":            state.StripeSubscriptionID,
		"subscription_status":               state.Status,
		"subscription_current_period_start": state.CurrentPeriodStart,
		"subscription_current_period_end":   state.CurrentPeriodEnd,
		"subscription_cancel_at_period_end": state.CancelAtPeriodEnd,
		"subscription_created":              state.Created,
	})
}

// SetWalletPlanOverride writes only the plan_override column.
func (s *PostgresStore) SetWalletPlanOverride(ctx context.Context, walletID string, plan string) (*types.Wallet, error) {
	return s.updateWalletColumns(ctx, walletID, map[string]interface{}{
		"plan_override": plan,
	})
}

func (s *PostgresStore) updateWalletColumns(ctx context.Context, walletID string, columns map[string]interface{}) (*types.Wallet, error) {
	if walletID == "" {
		return nil, fmt.Errorf("id not specified")
	}
	columns["updated_at"] = time.Now()
	result := s.gdb.WithContext(ctx).Model(&types.Wallet{}).Where("id = ?", walletID).Updates(columns)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	return s.GetWallet(ctx, walletID)
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

// UpdateWalletBalance safely updates the wallet balance using a database transaction.
// It returns nil, nil when a Stripe top-up or subscription invoice was already
// processed.
func (s *PostgresStore) UpdateWalletBalance(ctx context.Context, walletID string, amount float64, meta types.TransactionMetadata) (*types.Wallet, error) {
	if walletID == "" {
		return nil, fmt.Errorf("wallet_id not specified")
	}

	var wallet types.Wallet
	balanceUpdated := false
	err := s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked, err := lockWallet(tx, "id = ?", walletID)
		if err != nil {
			return err
		}
		wallet = *locked

		if meta.TransactionType == types.TransactionTypeTopUp {
			alreadyProcessed, err := topUpAlreadyProcessed(tx, meta)
			if err != nil {
				return err
			}
			if alreadyProcessed {
				return nil
			}
		}

		if meta.StripeInvoiceID != "" {
			claimed, err := claimSubscriptionInvoice(tx, walletID, amount, meta.StripeInvoiceID)
			if err != nil {
				return err
			}
			if !claimed {
				return nil
			}
		}

		currentBalance := wallet.Balance
		if currentBalance+amount < 0 {
			return fmt.Errorf("%s: current balance %.2f, attempted to deduct %.2f", types.ErrorInsufficientBalance,
				currentBalance, -amount)
		}

		if err := applyWalletDelta(tx, walletID, currentBalance, amount, meta); err != nil {
			return err
		}
		balanceUpdated = true

		// If this is a top-up, create a top-up record (for tracking purposes)
		if meta.TransactionType == types.TransactionTypeTopUp {
			topUp := &types.TopUp{
				ID:                      system.GenerateTopUpID(),
				CreatedAt:               time.Now(),
				UpdatedAt:               time.Now(),
				WalletID:                walletID,
				Amount:                  amount,
				StripePaymentIntentID:   meta.StripePaymentIntentID,
				StripeCheckoutSessionID: meta.StripeCheckoutSessionID,
			}

			if err := tx.Create(topUp).Error; err != nil {
				return err
			}
		}

		// Get the updated wallet
		if err := tx.Where("id = ?", walletID).First(&wallet).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	if !balanceUpdated {
		return nil, nil
	}

	return &wallet, nil
}

// lockWallet reads the wallet matching where and holds its row lock until tx ends.
func lockWallet(tx *gorm.DB, where string, arg any) (*types.Wallet, error) {
	var wallet types.Wallet
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where(where, arg).
		First(&wallet).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &wallet, nil
}

// applyWalletDelta adds amount to a wallet locked by lockWallet and records the
// ledger transaction. balanceBefore is the balance read under that lock.
func applyWalletDelta(tx *gorm.DB, walletID string, balanceBefore, amount float64, meta types.TransactionMetadata) error {
	result := tx.Model(&types.Wallet{}).Where("id = ?", walletID).Updates(map[string]interface{}{
		"balance":    gorm.Expr("balance + ?", amount),
		"updated_at": time.Now(),
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return tx.Create(&types.Transaction{
		ID:                 system.GenerateTransactionID(),
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
		WalletID:           walletID,
		Amount:             amount,
		BalanceBefore:      balanceBefore,
		BalanceAfter:       balanceBefore + amount,
		Type:               meta.TransactionType,
		InteractionID:      meta.InteractionID,
		LLMCallID:          meta.LLMCallID,
		SandboxID:          meta.SandboxID,
		SandboxRuntime:     meta.SandboxRuntime,
		SandboxPricingType: meta.SandboxPricingType,
		TopUpID:            meta.TopUpID,
	}).Error
}

// claimSubscriptionInvoice records invoiceID as credited. It reports false when
// the invoice was already recorded; the primary key on the invoice id makes
// that check hold under concurrent deliveries.
func claimSubscriptionInvoice(tx *gorm.DB, walletID string, amount float64, invoiceID string) (bool, error) {
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&types.SubscriptionInvoiceCredit{
		StripeInvoiceID: invoiceID,
		CreatedAt:       time.Now(),
		WalletID:        walletID,
		Amount:          amount,
	})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func topUpAlreadyProcessed(tx *gorm.DB, meta types.TransactionMetadata) (bool, error) {
	var topUp types.TopUp

	if meta.StripeCheckoutSessionID != "" {
		err := tx.Where("stripe_checkout_session_id = ?", meta.StripeCheckoutSessionID).First(&topUp).Error
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return false, err
		}
	}

	if meta.StripePaymentIntentID != "" {
		err := tx.Where("stripe_payment_intent_id = ?", meta.StripePaymentIntentID).First(&topUp).Error
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return false, err
		}
	}

	return false, nil
}

type ListTransactionsQuery struct {
	WalletID        string
	InteractionID   string
	LLMCallID       string
	TransactionType types.TransactionType
	Limit           int
	Offset          int
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

	if q.TransactionType != "" {
		query = query.Where("type = ?", q.TransactionType)
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
