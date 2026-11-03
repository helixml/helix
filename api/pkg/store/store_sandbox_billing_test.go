package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func newBilledSandbox(t *testing.T, db *PostgresStore, balance float64) (*types.Sandbox, *types.Wallet) {
	t.Helper()
	ctx := context.Background()
	orgID := system.GenerateID()
	wallet, err := db.CreateWallet(ctx, &types.Wallet{OrgID: orgID, Balance: balance})
	require.NoError(t, err)
	sb, err := db.CreateSandbox(ctx, &types.Sandbox{
		ID:             "sbx_" + system.GenerateID(),
		OrganizationID: orgID,
		Owner:          "usr_billing_test",
		Runtime:        types.SandboxRuntimeHeadlessUbuntu,
	})
	require.NoError(t, err)
	require.NoError(t, db.SetSandboxStatus(ctx, sb.ID, types.SandboxStatusRunning, ""))
	sb, err = db.GetSandbox(ctx, sb.ID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.DeleteSandbox(context.Background(), sb.ID)
		_ = db.DeleteWallet(context.Background(), wallet.ID)
	})
	return sb, wallet
}

func usageCharge(sb *types.Sandbox, walletID string, amount float64) SandboxCharge {
	return SandboxCharge{
		SandboxID: sb.ID,
		From:      sb.BillingLastChargedAt,
		To:        sb.BillingLastChargedAt.Add(time.Minute),
		WalletID:  walletID,
		Amount:    amount,
		Meta: types.TransactionMetadata{
			SandboxID:       sb.ID,
			TransactionType: types.TransactionTypeUsage,
		},
	}
}

// Every API replica runs the billing reaper, and a stop settles its final
// charge while the reaper may be mid-tick. Billers that read the same cursor
// must charge the window once: the wallet used to be debited by each of them.
func TestChargeSandboxChargesAWindowOnceUnderConcurrentBillers(t *testing.T) {
	db := GetTestDB()
	ctx := context.Background()
	sb, wallet := newBilledSandbox(t, db, 100)
	charge := usageCharge(sb, wallet.ID, 10)

	const billers = 8
	var wg sync.WaitGroup
	errs := make([]error, billers)
	for i := range billers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = db.ChargeSandbox(ctx, charge)
		}()
	}
	wg.Wait()

	charged := 0
	for _, err := range errs {
		if err == nil {
			charged++
			continue
		}
		require.ErrorIs(t, err, ErrBillingWindowClaimed)
	}
	require.Equal(t, 1, charged)

	after, err := db.GetWallet(ctx, wallet.ID)
	require.NoError(t, err)
	require.InDelta(t, 90.0, after.Balance, 1e-9)
	billed, err := db.GetSandbox(ctx, sb.ID)
	require.NoError(t, err)
	require.WithinDuration(t, charge.To, *billed.BillingLastChargedAt, time.Microsecond)
}

// The debit and the cursor move commit together. A biller holding a cursor
// that has since moved charges nothing, so a crash between the two writes can
// no longer leave a window charged but still due.
func TestChargeSandboxRejectsAStaleCursorWithoutDebiting(t *testing.T) {
	db := GetTestDB()
	ctx := context.Background()
	sb, wallet := newBilledSandbox(t, db, 100)
	first := usageCharge(sb, wallet.ID, 10)
	_, err := db.ChargeSandbox(ctx, first)
	require.NoError(t, err)

	_, err = db.ChargeSandbox(ctx, first)

	require.ErrorIs(t, err, ErrBillingWindowClaimed)
	after, err := db.GetWallet(ctx, wallet.ID)
	require.NoError(t, err)
	require.InDelta(t, 90.0, after.Balance, 1e-9)
}

// An org that cannot cover a window pays what it has, never goes negative,
// and the window is closed so the next charge does not bill it again.
func TestChargeSandboxDrainsAShortWalletAndClosesTheWindow(t *testing.T) {
	db := GetTestDB()
	ctx := context.Background()
	sb, wallet := newBilledSandbox(t, db, 4)
	charge := usageCharge(sb, wallet.ID, 10)

	insufficient, err := db.ChargeSandbox(ctx, charge)

	require.NoError(t, err)
	require.True(t, insufficient)
	after, err := db.GetWallet(ctx, wallet.ID)
	require.NoError(t, err)
	require.InDelta(t, 0.0, after.Balance, 1e-9)
	billed, err := db.GetSandbox(ctx, sb.ID)
	require.NoError(t, err)
	require.WithinDuration(t, charge.To, *billed.BillingLastChargedAt, time.Microsecond)

	// An empty wallet still closes the next window, without a zero-value ledger row.
	next := usageCharge(billed, wallet.ID, 10)
	insufficient, err = db.ChargeSandbox(ctx, next)
	require.NoError(t, err)
	require.True(t, insufficient)
	txns, err := db.ListTransactions(ctx, &ListTransactionsQuery{WalletID: wallet.ID})
	require.NoError(t, err)
	require.Len(t, txns, 1)
	require.InDelta(t, -4.0, txns[0].Amount, 1e-9)
}
