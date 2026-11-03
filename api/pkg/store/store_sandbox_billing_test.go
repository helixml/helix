package store

import (
	"context"
	"math"
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
	t.Cleanup(func() { _ = db.DeleteWallet(context.Background(), wallet.ID) })
	return newRunningSandbox(t, db, orgID), wallet
}

func newRunningSandbox(t *testing.T, db *PostgresStore, orgID string) *types.Sandbox {
	t.Helper()
	ctx := context.Background()
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
	t.Cleanup(func() { _ = db.DeleteSandbox(context.Background(), sb.ID) })
	return sb
}

func usageCharge(sb *types.Sandbox, amount float64) SandboxCharge {
	return SandboxCharge{
		SandboxID: sb.ID,
		OrgID:     sb.OrganizationID,
		From:      sb.BillingLastChargedAt,
		To:        sb.BillingLastChargedAt.Add(time.Minute),
		Amount:    amount,
		Runtime:   sb.Runtime,
	}
}

// Every API replica runs the billing reaper, and a stop settles its final
// charge while the reaper may be mid-tick. Billers that read the same cursor
// must charge the window once: the wallet used to be debited by each of them.
func TestChargeSandboxChargesAWindowOnceUnderConcurrentBillers(t *testing.T) {
	db := GetTestDB()
	ctx := context.Background()
	sb, wallet := newBilledSandbox(t, db, 100)
	charge := usageCharge(sb, 10)

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
	first := usageCharge(sb, 10)
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
	charge := usageCharge(sb, 10)

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
	next := usageCharge(billed, 10)
	insufficient, err = db.ChargeSandbox(ctx, next)
	require.NoError(t, err)
	require.True(t, insufficient)
	txns, err := db.ListTransactions(ctx, &ListTransactionsQuery{WalletID: wallet.ID})
	require.NoError(t, err)
	require.Len(t, txns, 1)
	require.InDelta(t, -4.0, txns[0].Amount, 1e-9)
}

// A legacy row has started_at but no cursor. Its first charge must claim the
// NULL cursor; a `= NULL` regression would make every such charge look
// already-claimed, and the sandbox would run unbilled with no error.
func TestChargeSandboxClaimsAnUnsetCursor(t *testing.T) {
	db := GetTestDB()
	ctx := context.Background()
	sb, wallet := newBilledSandbox(t, db, 100)
	require.NoError(t, db.gdb.Model(&types.Sandbox{}).Where("id = ?", sb.ID).Update("billing_last_charged_at", nil).Error)
	charge := usageCharge(sb, 10)
	charge.From = nil

	_, err := db.ChargeSandbox(ctx, charge)

	require.NoError(t, err)
	after, err := db.GetWallet(ctx, wallet.ID)
	require.NoError(t, err)
	require.InDelta(t, 90.0, after.Balance, 1e-9)
}

// If the debit fails, the cursor must not move: otherwise the window is
// closed without being paid and is never billed.
func TestChargeSandboxLeavesTheCursorWhenTheDebitFails(t *testing.T) {
	db := GetTestDB()
	ctx := context.Background()
	sb := newRunningSandbox(t, db, system.GenerateID()) // org has no wallet
	charge := usageCharge(sb, 10)

	_, err := db.ChargeSandbox(ctx, charge)

	require.ErrorIs(t, err, ErrNotFound)
	unchanged, err := db.GetSandbox(ctx, sb.ID)
	require.NoError(t, err)
	require.WithinDuration(t, *sb.BillingLastChargedAt, *unchanged.BillingLastChargedAt, time.Microsecond)
}

// A free window advances the cursor without needing a wallet, so pricing a
// runtime later never bills the time it was free.
func TestChargeSandboxAdvancesAFreeWindowWithoutAWallet(t *testing.T) {
	db := GetTestDB()
	ctx := context.Background()
	sb := newRunningSandbox(t, db, system.GenerateID())
	charge := usageCharge(sb, 0)

	insufficient, err := db.ChargeSandbox(ctx, charge)

	require.NoError(t, err)
	require.False(t, insufficient)
	billed, err := db.GetSandbox(ctx, sb.ID)
	require.NoError(t, err)
	require.WithinDuration(t, charge.To, *billed.BillingLastChargedAt, time.Microsecond)
}

// A soft-deleted sandbox is gone; a biller still holding its row must not
// charge for a container that no longer exists.
func TestChargeSandboxNeverChargesADeletedSandbox(t *testing.T) {
	db := GetTestDB()
	ctx := context.Background()
	sb, wallet := newBilledSandbox(t, db, 100)
	require.NoError(t, db.DeleteSandbox(ctx, sb.ID))

	_, err := db.ChargeSandbox(ctx, usageCharge(sb, 10))

	require.ErrorIs(t, err, ErrBillingWindowClaimed)
	after, err := db.GetWallet(ctx, wallet.ID)
	require.NoError(t, err)
	require.InDelta(t, 100.0, after.Balance, 1e-9)
}

// A stop settles its own final charge before marking the row stopped. A
// reaper that listed the sandbox while it was running must not charge it
// once it has stopped.
func TestChargeSandboxNeverChargesAStoppedSandbox(t *testing.T) {
	db := GetTestDB()
	ctx := context.Background()
	sb, wallet := newBilledSandbox(t, db, 100)
	require.NoError(t, db.SetSandboxStatus(ctx, sb.ID, types.SandboxStatusStopped, ""))

	_, err := db.ChargeSandbox(ctx, usageCharge(sb, 10))

	require.ErrorIs(t, err, ErrBillingWindowClaimed)
	after, err := db.GetWallet(ctx, wallet.ID)
	require.NoError(t, err)
	require.InDelta(t, 100.0, after.Balance, 1e-9)
}

func TestChargeSandboxRejectsAnInvalidCharge(t *testing.T) {
	db := GetTestDB()
	sb, wallet := newBilledSandbox(t, db, 100)
	for _, amount := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		_, err := db.ChargeSandbox(context.Background(), usageCharge(sb, amount))
		require.Error(t, err, amount)
	}
	after, err := db.GetWallet(context.Background(), wallet.ID)
	require.NoError(t, err)
	require.InDelta(t, 100.0, after.Balance, 1e-9)

	// An unset window end would set the cursor to year 1 and bill ~292 years.
	noEnd := usageCharge(sb, 10)
	noEnd.To = time.Time{}
	_, err = db.ChargeSandbox(context.Background(), noEnd)
	require.Error(t, err)
}

// A PATCH (rename, tags, TTL) reads the row, then writes it back. If a charge
// lands in between, writing the whole row put the old cursor back and the
// next tick billed the same minutes again.
func TestUpdateSandboxDetailsNeverRewindsTheBillingCursor(t *testing.T) {
	db := GetTestDB()
	ctx := context.Background()
	sb, _ := newBilledSandbox(t, db, 100)
	stale := *sb
	charge := usageCharge(sb, 10)
	_, err := db.ChargeSandbox(ctx, charge)
	require.NoError(t, err)

	stale.Name = "renamed"
	_, err = db.UpdateSandboxDetails(ctx, &stale)

	require.NoError(t, err)
	after, err := db.GetSandbox(ctx, sb.ID)
	require.NoError(t, err)
	require.Equal(t, "renamed", after.Name)
	require.WithinDuration(t, charge.To, *after.BillingLastChargedAt, time.Microsecond)
}

// A duplicate start writes running again. Only the transition to running may
// open the billing window; a repeat must not skip unbilled usage.
func TestSetSandboxStatusRunningTwiceKeepsTheBillingCursor(t *testing.T) {
	db := GetTestDB()
	ctx := context.Background()
	sb, _ := newBilledSandbox(t, db, 100)

	require.NoError(t, db.SetSandboxStatus(ctx, sb.ID, types.SandboxStatusRunning, ""))

	after, err := db.GetSandbox(ctx, sb.ID)
	require.NoError(t, err)
	require.WithinDuration(t, *sb.BillingLastChargedAt, *after.BillingLastChargedAt, time.Microsecond)
	require.WithinDuration(t, *sb.StartedAt, *after.StartedAt, time.Microsecond)
}

// The claimed row and the debited wallet must belong to the same org, or a
// caller passing the wrong org id would bill one org for another's sandbox.
func TestChargeSandboxRejectsAnotherOrgsSandbox(t *testing.T) {
	db := GetTestDB()
	ctx := context.Background()
	sb, _ := newBilledSandbox(t, db, 100)
	_, otherWallet := newBilledSandbox(t, db, 100)
	charge := usageCharge(sb, 10)
	charge.OrgID = otherWallet.OrgID

	_, err := db.ChargeSandbox(ctx, charge)

	require.ErrorIs(t, err, ErrBillingWindowClaimed)
	after, err := db.GetWallet(ctx, otherWallet.ID)
	require.NoError(t, err)
	require.InDelta(t, 100.0, after.Balance, 1e-9)
}
