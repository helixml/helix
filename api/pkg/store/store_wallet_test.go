package store

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/suite"
)

func TestWalletTestSuite(t *testing.T) {
	suite.Run(t, new(WalletTestSuite))
}

type WalletTestSuite struct {
	suite.Suite
	ctx context.Context
	db  *PostgresStore
}

func (suite *WalletTestSuite) SetupTest() {
	suite.ctx = context.Background()

	var storeCfg config.Store
	err := envconfig.Process("", &storeCfg)
	suite.NoError(err)

	store, err := NewPostgresStore(storeCfg)
	suite.Require().NoError(err)
	suite.db = store

}

func (suite *WalletTestSuite) TearDownTestSuite() {
	_ = suite.db.Close()
}

// Wallet CRUD Tests

func (suite *WalletTestSuite) TestCreateWallet_User() {
	userID := system.GenerateID()
	wallet := &types.Wallet{
		UserID:  userID,
		Balance: 100.0,
	}

	createdWallet, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.NoError(err)
	suite.NotNil(createdWallet)
	suite.NotEmpty(createdWallet.ID)
	suite.Equal(userID, createdWallet.UserID)
	suite.Equal("", createdWallet.OrgID)
	suite.Equal(100.0, createdWallet.Balance)
	suite.NotZero(createdWallet.CreatedAt)
	suite.NotZero(createdWallet.UpdatedAt)
}

func (suite *WalletTestSuite) TestCreateWallet_Org() {
	orgID := system.GenerateID()
	wallet := &types.Wallet{
		OrgID:   orgID,
		Balance: 500.0,
	}

	createdWallet, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.NoError(err)
	suite.NotNil(createdWallet)
	suite.NotEmpty(createdWallet.ID)
	suite.Equal("", createdWallet.UserID)
	suite.Equal(orgID, createdWallet.OrgID)
	suite.Equal(500.0, createdWallet.Balance)
}

func (suite *WalletTestSuite) TestCreateWallet_NoUserOrOrg() {
	wallet := &types.Wallet{
		Balance: 100.0,
	}

	_, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.Error(err)
	suite.Contains(err.Error(), "either user_id or org_id must be specified")
}

func (suite *WalletTestSuite) TestCreateWallet_BothUserAndOrg() {
	userID := system.GenerateID()
	orgID := system.GenerateID()
	wallet := &types.Wallet{
		UserID:  userID,
		OrgID:   orgID,
		Balance: 100.0,
	}

	_, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.Error(err)
	suite.Contains(err.Error(), "either user_id or org_id must be specified, not both")
}

func (suite *WalletTestSuite) TestGetWallet() {
	userID := system.GenerateID()
	wallet := &types.Wallet{
		UserID:  userID,
		Balance: 100.0,
	}

	createdWallet, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.NoError(err)

	retrievedWallet, err := suite.db.GetWallet(suite.ctx, createdWallet.ID)
	suite.NoError(err)
	suite.Equal(createdWallet.ID, retrievedWallet.ID)
	suite.Equal(createdWallet.UserID, retrievedWallet.UserID)
	suite.Equal(createdWallet.Balance, retrievedWallet.Balance)
}

func (suite *WalletTestSuite) TestGetWallet_NotFound() {
	_, err := suite.db.GetWallet(suite.ctx, "non-existent-id")
	suite.Error(err)
	suite.Equal(ErrNotFound, err)
}

func (suite *WalletTestSuite) TestGetWallet_EmptyID() {
	_, err := suite.db.GetWallet(suite.ctx, "")
	suite.Error(err)
	suite.Contains(err.Error(), "id not specified")
}

func (suite *WalletTestSuite) TestGetWalletByUser() {
	userID := system.GenerateID()
	wallet := &types.Wallet{
		UserID:  userID,
		Balance: 100.0,
	}

	createdWallet, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.NoError(err)

	retrievedWallet, err := suite.db.GetWalletByUser(suite.ctx, userID)
	suite.NoError(err)
	suite.Equal(createdWallet.ID, retrievedWallet.ID)
	suite.Equal(userID, retrievedWallet.UserID)
}

func (suite *WalletTestSuite) TestGetWalletByUser_NotFound() {
	_, err := suite.db.GetWalletByUser(suite.ctx, "non-existent-user")
	suite.Error(err)
	suite.Equal(ErrNotFound, err)
}

func (suite *WalletTestSuite) TestGetWalletByUser_EmptyUserID() {
	_, err := suite.db.GetWalletByUser(suite.ctx, "")
	suite.Error(err)
	suite.Contains(err.Error(), "user_id not specified")
}

func (suite *WalletTestSuite) TestGetWalletByOrg() {
	orgID := system.GenerateID()
	wallet := &types.Wallet{
		OrgID:   orgID,
		Balance: 500.0,
	}

	createdWallet, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.NoError(err)

	retrievedWallet, err := suite.db.GetWalletByOrg(suite.ctx, orgID)
	suite.NoError(err)
	suite.Equal(createdWallet.ID, retrievedWallet.ID)
	suite.Equal(orgID, retrievedWallet.OrgID)
}

func (suite *WalletTestSuite) TestGetWalletByOrg_NotFound() {
	_, err := suite.db.GetWalletByOrg(suite.ctx, "non-existent-org")
	suite.Error(err)
	suite.Equal(ErrNotFound, err)
}

func (suite *WalletTestSuite) TestGetWalletByOrg_EmptyOrgID() {
	_, err := suite.db.GetWalletByOrg(suite.ctx, "")
	suite.Error(err)
	suite.Contains(err.Error(), "org_id not specified")
}

func (suite *WalletTestSuite) TestUpdateWallet() {
	userID := system.GenerateID()
	wallet := &types.Wallet{
		UserID:  userID,
		Balance: 100.0,
	}

	createdWallet, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.NoError(err)

	// Update the wallet
	createdWallet.Balance = 200.0
	updatedWallet, err := suite.db.UpdateWallet(suite.ctx, createdWallet)
	suite.NoError(err)
	suite.Equal(200.0, updatedWallet.Balance)
	suite.True(updatedWallet.UpdatedAt.After(createdWallet.UpdatedAt))
}

func (suite *WalletTestSuite) TestUpdateWallet_EmptyID() {
	wallet := &types.Wallet{
		Balance: 100.0,
	}

	_, err := suite.db.UpdateWallet(suite.ctx, wallet)
	suite.Error(err)
	suite.Contains(err.Error(), "id not specified")
}

func (suite *WalletTestSuite) TestDeleteWallet() {
	userID := system.GenerateID()
	wallet := &types.Wallet{
		UserID:  userID,
		Balance: 100.0,
	}

	createdWallet, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.NoError(err)

	// Verify wallet exists
	retrievedWallet, err := suite.db.GetWallet(suite.ctx, createdWallet.ID)
	suite.NoError(err)
	suite.NotNil(retrievedWallet)

	// Delete wallet
	err = suite.db.DeleteWallet(suite.ctx, createdWallet.ID)
	suite.NoError(err)

	// Verify wallet is deleted
	_, err = suite.db.GetWallet(suite.ctx, createdWallet.ID)
	suite.Error(err)
	suite.Equal(ErrNotFound, err)
}

func (suite *WalletTestSuite) TestDeleteWallet_EmptyID() {
	err := suite.db.DeleteWallet(suite.ctx, "")
	suite.Error(err)
	suite.Contains(err.Error(), "id not specified")
}

// Wallet Balance Tests

func (suite *WalletTestSuite) TestUpdateWalletBalance_Positive() {
	userID := system.GenerateID()
	wallet := &types.Wallet{
		UserID:  userID,
		Balance: 100.0,
	}

	createdWallet, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.NoError(err)

	// Add 50.0 to balance
	updatedWallet, err := suite.db.UpdateWalletBalance(suite.ctx, createdWallet.ID, 50.0)
	suite.NoError(err)
	suite.Equal(150.0, updatedWallet.Balance)
}

func (suite *WalletTestSuite) TestUpdateWalletBalance_Negative() {
	userID := system.GenerateID()
	wallet := &types.Wallet{
		UserID:  userID,
		Balance: 100.0,
	}

	createdWallet, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.NoError(err)

	// Subtract 30.0 from balance
	updatedWallet, err := suite.db.UpdateWalletBalance(suite.ctx, createdWallet.ID, -30.0)
	suite.NoError(err)
	suite.Equal(70.0, updatedWallet.Balance)
}

func (suite *WalletTestSuite) TestUpdateWalletBalance_InsufficientFunds() {
	userID := system.GenerateID()
	wallet := &types.Wallet{
		UserID:  userID,
		Balance: 100.0,
	}

	createdWallet, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.NoError(err)

	// Try to subtract more than available balance
	_, err = suite.db.UpdateWalletBalance(suite.ctx, createdWallet.ID, -150.0)
	suite.Error(err)
	suite.Contains(err.Error(), "insufficient balance")
}

func (suite *WalletTestSuite) TestUpdateWalletBalance_EmptyWalletID() {
	_, err := suite.db.UpdateWalletBalance(suite.ctx, "", 50.0)
	suite.Error(err)
	suite.Contains(err.Error(), "wallet_id not specified")
}

func (suite *WalletTestSuite) TestUpdateWalletBalance_NonExistentWallet() {
	_, err := suite.db.UpdateWalletBalance(suite.ctx, "non-existent-wallet", 50.0)
	suite.Error(err)
	suite.Equal(ErrNotFound, err)
}

// Transaction Tests

func (suite *WalletTestSuite) TestCreateTransaction() {
	userID := system.GenerateID()
	wallet := &types.Wallet{
		UserID:  userID,
		Balance: 100.0,
	}

	createdWallet, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.NoError(err)

	transaction := &types.Transaction{
		WalletID:      createdWallet.ID,
		Amount:        -25.0,
		InteractionID: "interaction-123",
		LLMCallID:     "llm-call-456",
	}

	createdTransaction, err := suite.db.CreateTransaction(suite.ctx, transaction)
	suite.NoError(err)
	suite.NotNil(createdTransaction)
	suite.NotEmpty(createdTransaction.ID)
	suite.Equal(createdWallet.ID, createdTransaction.WalletID)
	suite.Equal(-25.0, createdTransaction.Amount)
	suite.Equal("interaction-123", createdTransaction.InteractionID)
	suite.Equal("llm-call-456", createdTransaction.LLMCallID)
	suite.NotZero(createdTransaction.CreatedAt)
	suite.NotZero(createdTransaction.UpdatedAt)
}

func (suite *WalletTestSuite) TestCreateTransaction_EmptyWalletID() {
	transaction := &types.Transaction{
		Amount:        -25.0,
		InteractionID: "interaction-123",
	}

	_, err := suite.db.CreateTransaction(suite.ctx, transaction)
	suite.Error(err)
	suite.Contains(err.Error(), "wallet_id not specified")
}

func (suite *WalletTestSuite) TestListTransactions() {
	userID := system.GenerateID()
	wallet := &types.Wallet{
		UserID:  userID,
		Balance: 100.0,
	}

	createdWallet, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.NoError(err)

	// Create multiple transactions
	transactions := []*types.Transaction{
		{
			WalletID:      createdWallet.ID,
			Amount:        -10.0,
			InteractionID: "interaction-1",
			LLMCallID:     "llm-call-1",
		},
		{
			WalletID:      createdWallet.ID,
			Amount:        -15.0,
			InteractionID: "interaction-2",
			LLMCallID:     "llm-call-2",
		},
		{
			WalletID:      createdWallet.ID,
			Amount:        -20.0,
			InteractionID: "interaction-3",
			LLMCallID:     "llm-call-3",
		},
	}

	for _, tx := range transactions {
		_, err := suite.db.CreateTransaction(suite.ctx, tx)
		suite.NoError(err)
	}

	// Test listing all transactions for wallet
	query := &ListTransactionsQuery{
		WalletID: createdWallet.ID,
	}

	retrievedTransactions, err := suite.db.ListTransactions(suite.ctx, query)
	suite.NoError(err)
	suite.Len(retrievedTransactions, 3)

	// Verify transactions are ordered by created_at DESC
	suite.True(retrievedTransactions[0].CreatedAt.After(retrievedTransactions[1].CreatedAt) ||
		retrievedTransactions[0].CreatedAt.Equal(retrievedTransactions[1].CreatedAt))
	suite.True(retrievedTransactions[1].CreatedAt.After(retrievedTransactions[2].CreatedAt) ||
		retrievedTransactions[1].CreatedAt.Equal(retrievedTransactions[2].CreatedAt))
}

func (suite *WalletTestSuite) TestListTransactions_ByInteractionID() {
	userID := system.GenerateID()
	wallet := &types.Wallet{
		UserID:  userID,
		Balance: 100.0,
	}

	createdWallet, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.NoError(err)

	// Create transactions with different interaction IDs
	transactions := []*types.Transaction{
		{
			WalletID:      createdWallet.ID,
			Amount:        -10.0,
			InteractionID: "interaction-1",
		},
		{
			WalletID:      createdWallet.ID,
			Amount:        -15.0,
			InteractionID: "interaction-2",
		},
		{
			WalletID:      createdWallet.ID,
			Amount:        -20.0,
			InteractionID: "interaction-1", // Same interaction ID
		},
	}

	for _, tx := range transactions {
		_, err := suite.db.CreateTransaction(suite.ctx, tx)
		suite.NoError(err)
	}

	// Test filtering by interaction ID
	query := &ListTransactionsQuery{
		WalletID:      createdWallet.ID,
		InteractionID: "interaction-1",
	}

	retrievedTransactions, err := suite.db.ListTransactions(suite.ctx, query)
	suite.NoError(err)
	suite.Len(retrievedTransactions, 2)

	for _, tx := range retrievedTransactions {
		suite.Equal("interaction-1", tx.InteractionID)
	}
}

func (suite *WalletTestSuite) TestListTransactions_ByLLMCallID() {
	userID := system.GenerateID()
	wallet := &types.Wallet{
		UserID:  userID,
		Balance: 100.0,
	}

	createdWallet, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.NoError(err)

	// Create transactions with different LLM call IDs
	transactions := []*types.Transaction{
		{
			WalletID:  createdWallet.ID,
			Amount:    -10.0,
			LLMCallID: "llm-call-1",
		},
		{
			WalletID:  createdWallet.ID,
			Amount:    -15.0,
			LLMCallID: "llm-call-2",
		},
		{
			WalletID:  createdWallet.ID,
			Amount:    -20.0,
			LLMCallID: "llm-call-1", // Same LLM call ID
		},
	}

	for _, tx := range transactions {
		_, err := suite.db.CreateTransaction(suite.ctx, tx)
		suite.NoError(err)
	}

	// Test filtering by LLM call ID
	query := &ListTransactionsQuery{
		WalletID:  createdWallet.ID,
		LLMCallID: "llm-call-1",
	}

	retrievedTransactions, err := suite.db.ListTransactions(suite.ctx, query)
	suite.NoError(err)
	suite.Len(retrievedTransactions, 2)

	for _, tx := range retrievedTransactions {
		suite.Equal("llm-call-1", tx.LLMCallID)
	}
}

func (suite *WalletTestSuite) TestListTransactions_WithLimit() {
	userID := system.GenerateID()
	wallet := &types.Wallet{
		UserID:  userID,
		Balance: 100.0,
	}

	createdWallet, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.NoError(err)

	// Create 5 transactions
	for i := 0; i < 5; i++ {
		transaction := &types.Transaction{
			WalletID: createdWallet.ID,
			Amount:   -10.0,
		}
		_, err := suite.db.CreateTransaction(suite.ctx, transaction)
		suite.NoError(err)
	}

	// Test with limit
	query := &ListTransactionsQuery{
		WalletID: createdWallet.ID,
		Limit:    3,
	}

	retrievedTransactions, err := suite.db.ListTransactions(suite.ctx, query)
	suite.NoError(err)
	suite.Len(retrievedTransactions, 3)
}

func (suite *WalletTestSuite) TestListTransactions_WithOffset() {
	userID := system.GenerateID()
	wallet := &types.Wallet{
		UserID:  userID,
		Balance: 100.0,
	}

	createdWallet, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.NoError(err)

	// Create 5 transactions
	for i := 0; i < 5; i++ {
		transaction := &types.Transaction{
			WalletID: createdWallet.ID,
			Amount:   -10.0,
		}
		_, err := suite.db.CreateTransaction(suite.ctx, transaction)
		suite.NoError(err)
	}

	// Test with offset
	query := &ListTransactionsQuery{
		WalletID: createdWallet.ID,
		Limit:    2,
		Offset:   2,
	}

	retrievedTransactions, err := suite.db.ListTransactions(suite.ctx, query)
	suite.NoError(err)
	suite.Len(retrievedTransactions, 2)
}

// TopUp Tests

func (suite *WalletTestSuite) TestCreateTopUp() {
	userID := system.GenerateID()
	wallet := &types.Wallet{
		UserID:  userID,
		Balance: 100.0,
	}

	createdWallet, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.NoError(err)

	topUp := &types.TopUp{
		WalletID: createdWallet.ID,
		Amount:   50.0,
	}

	createdTopUp, err := suite.db.CreateTopUp(suite.ctx, topUp)
	suite.NoError(err)
	suite.NotNil(createdTopUp)
	suite.NotEmpty(createdTopUp.ID)
	suite.Equal(createdWallet.ID, createdTopUp.WalletID)
	suite.Equal(50.0, createdTopUp.Amount)
	suite.NotZero(createdTopUp.CreatedAt)
	suite.NotZero(createdTopUp.UpdatedAt)

	// Verify wallet balance was updated
	updatedWallet, err := suite.db.GetWallet(suite.ctx, createdWallet.ID)
	suite.NoError(err)
	suite.Equal(150.0, updatedWallet.Balance)
}

func (suite *WalletTestSuite) TestCreateTopUp_EmptyWalletID() {
	topUp := &types.TopUp{
		Amount: 50.0,
	}

	_, err := suite.db.CreateTopUp(suite.ctx, topUp)
	suite.Error(err)
	suite.Contains(err.Error(), "wallet_id not specified")
}

func (suite *WalletTestSuite) TestCreateTopUp_ZeroAmount() {
	userID := system.GenerateID()
	wallet := &types.Wallet{
		UserID:  userID,
		Balance: 100.0,
	}

	createdWallet, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.NoError(err)

	topUp := &types.TopUp{
		WalletID: createdWallet.ID,
		Amount:   0.0,
	}

	_, err = suite.db.CreateTopUp(suite.ctx, topUp)
	suite.Error(err)
	suite.Contains(err.Error(), "amount must be greater than 0")
}

func (suite *WalletTestSuite) TestCreateTopUp_NegativeAmount() {
	userID := system.GenerateID()
	wallet := &types.Wallet{
		UserID:  userID,
		Balance: 100.0,
	}

	createdWallet, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.NoError(err)

	topUp := &types.TopUp{
		WalletID: createdWallet.ID,
		Amount:   -50.0,
	}

	_, err = suite.db.CreateTopUp(suite.ctx, topUp)
	suite.Error(err)
	suite.Contains(err.Error(), "amount must be greater than 0")
}

func (suite *WalletTestSuite) TestListTopUps() {
	userID := system.GenerateID()
	wallet := &types.Wallet{
		UserID:  userID,
		Balance: 100.0,
	}

	createdWallet, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.NoError(err)

	// Create multiple top-ups
	topUps := []*types.TopUp{
		{
			WalletID: createdWallet.ID,
			Amount:   25.0,
		},
		{
			WalletID: createdWallet.ID,
			Amount:   50.0,
		},
		{
			WalletID: createdWallet.ID,
			Amount:   75.0,
		},
	}

	for _, topUp := range topUps {
		_, err := suite.db.CreateTopUp(suite.ctx, topUp)
		suite.NoError(err)
	}

	// Test listing all top-ups for wallet
	query := &ListTopUpsQuery{
		WalletID: createdWallet.ID,
	}

	retrievedTopUps, err := suite.db.ListTopUps(suite.ctx, query)
	suite.NoError(err)
	suite.Len(retrievedTopUps, 3)

	// Verify top-ups are ordered by created_at DESC
	suite.True(retrievedTopUps[0].CreatedAt.After(retrievedTopUps[1].CreatedAt) ||
		retrievedTopUps[0].CreatedAt.Equal(retrievedTopUps[1].CreatedAt))
	suite.True(retrievedTopUps[1].CreatedAt.After(retrievedTopUps[2].CreatedAt) ||
		retrievedTopUps[1].CreatedAt.Equal(retrievedTopUps[2].CreatedAt))
}

func (suite *WalletTestSuite) TestListTopUps_WithLimit() {
	userID := system.GenerateID()
	wallet := &types.Wallet{
		UserID:  userID,
		Balance: 100.0,
	}

	createdWallet, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.NoError(err)

	// Create 5 top-ups
	for i := 0; i < 5; i++ {
		topUp := &types.TopUp{
			WalletID: createdWallet.ID,
			Amount:   10.0,
		}
		_, err := suite.db.CreateTopUp(suite.ctx, topUp)
		suite.NoError(err)
	}

	// Test with limit
	query := &ListTopUpsQuery{
		WalletID: createdWallet.ID,
		Limit:    3,
	}

	retrievedTopUps, err := suite.db.ListTopUps(suite.ctx, query)
	suite.NoError(err)
	suite.Len(retrievedTopUps, 3)
}

func (suite *WalletTestSuite) TestListTopUps_WithOffset() {
	userID := system.GenerateID()
	wallet := &types.Wallet{
		UserID:  userID,
		Balance: 100.0,
	}

	createdWallet, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.NoError(err)

	// Create 5 top-ups
	for i := 0; i < 5; i++ {
		topUp := &types.TopUp{
			WalletID: createdWallet.ID,
			Amount:   10.0,
		}
		_, err := suite.db.CreateTopUp(suite.ctx, topUp)
		suite.NoError(err)
	}

	// Test with offset
	query := &ListTopUpsQuery{
		WalletID: createdWallet.ID,
		Limit:    2,
		Offset:   2,
	}

	retrievedTopUps, err := suite.db.ListTopUps(suite.ctx, query)
	suite.NoError(err)
	suite.Len(retrievedTopUps, 2)
}

// Integration Tests

func (suite *WalletTestSuite) TestWalletTransactionFlow() {
	// Create a wallet
	userID := system.GenerateID()
	wallet := &types.Wallet{
		UserID:  userID,
		Balance: 100.0,
	}

	createdWallet, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.NoError(err)
	suite.Equal(100.0, createdWallet.Balance)

	// Add funds via top-up
	topUp := &types.TopUp{
		WalletID: createdWallet.ID,
		Amount:   50.0,
	}

	_, err = suite.db.CreateTopUp(suite.ctx, topUp)
	suite.NoError(err)

	// Verify balance increased
	updatedWallet, err := suite.db.GetWallet(suite.ctx, createdWallet.ID)
	suite.NoError(err)
	suite.Equal(150.0, updatedWallet.Balance)

	// Create a transaction
	transaction := &types.Transaction{
		WalletID:      createdWallet.ID,
		Amount:        -25.0,
		InteractionID: "test-interaction",
		LLMCallID:     "test-llm-call",
	}

	_, err = suite.db.CreateTransaction(suite.ctx, transaction)
	suite.NoError(err)

	// Deduct funds from wallet
	_, err = suite.db.UpdateWalletBalance(suite.ctx, createdWallet.ID, -25.0)
	suite.NoError(err)

	// Verify final balance
	finalWallet, err := suite.db.GetWallet(suite.ctx, createdWallet.ID)
	suite.NoError(err)
	suite.Equal(100.0, finalWallet.Balance)

	// List transactions
	query := &ListTransactionsQuery{
		WalletID: createdWallet.ID,
	}

	transactions, err := suite.db.ListTransactions(suite.ctx, query)
	suite.NoError(err)
	suite.Len(transactions, 1)
	suite.Equal(-25.0, transactions[0].Amount)
	suite.Equal("test-interaction", transactions[0].InteractionID)

	// List top-ups
	topUpQuery := &ListTopUpsQuery{
		WalletID: createdWallet.ID,
	}

	topUps, err := suite.db.ListTopUps(suite.ctx, topUpQuery)
	suite.NoError(err)
	suite.Len(topUps, 1)
	suite.Equal(50.0, topUps[0].Amount)
}

func (suite *WalletTestSuite) TestConcurrentWalletBalanceUpdates() {
	userID := system.GenerateID()
	wallet := &types.Wallet{
		UserID:  userID,
		Balance: 100.0,
	}

	createdWallet, err := suite.db.CreateWallet(suite.ctx, wallet)
	suite.NoError(err)

	// Simulate concurrent balance updates
	results := make(chan error, 3)

	go func() {
		_, err := suite.db.UpdateWalletBalance(suite.ctx, createdWallet.ID, 10.0)
		results <- err
	}()

	go func() {
		_, err := suite.db.UpdateWalletBalance(suite.ctx, createdWallet.ID, 20.0)
		results <- err
	}()

	go func() {
		_, err := suite.db.UpdateWalletBalance(suite.ctx, createdWallet.ID, -5.0)
		results <- err
	}()

	// Wait for all goroutines to complete
	for i := 0; i < 3; i++ {
		err := <-results
		suite.NoError(err)
	}

	// Verify final balance (100 + 10 + 20 - 5 = 125)
	finalWallet, err := suite.db.GetWallet(suite.ctx, createdWallet.ID)
	suite.NoError(err)
	suite.Equal(125.0, finalWallet.Balance)
}
