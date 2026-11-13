package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func TestRedeemIdentityAttestation(t *testing.T) {
	db := GetTestDB()
	ctx := context.Background()
	jti := system.GenerateUUID()

	ok, err := db.RedeemIdentityAttestation(ctx, jti, time.Now().Add(5*time.Minute))
	require.NoError(t, err)
	require.True(t, ok, "first redemption succeeds")

	ok, err = db.RedeemIdentityAttestation(ctx, jti, time.Now().Add(5*time.Minute))
	require.NoError(t, err)
	require.False(t, ok, "second redemption is refused")

	_, err = db.RedeemIdentityAttestation(ctx, "", time.Now())
	require.Error(t, err)
}

func TestRedeemIdentityAttestationConcurrent(t *testing.T) {
	db := GetTestDB()
	ctx := context.Background()
	jti := system.GenerateUUID()

	var wg sync.WaitGroup
	results := make(chan bool, 16)
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := db.RedeemIdentityAttestation(ctx, jti, time.Now().Add(5*time.Minute))
			require.NoError(t, err)
			results <- ok
		}()
	}
	wg.Wait()
	close(results)
	wins := 0
	for ok := range results {
		if ok {
			wins++
		}
	}
	require.Equal(t, 1, wins, "exactly one concurrent verification may redeem a token")
}

func TestRedeemIdentityAttestationDropsExpired(t *testing.T) {
	db := GetTestDB()
	ctx := context.Background()
	expired := system.GenerateUUID()
	require.NoError(t, db.gdb.Create(&types.IdentityAttestationRedemption{JTI: expired, ExpiresAt: time.Now().Add(-time.Minute)}).Error)

	ok, err := db.RedeemIdentityAttestation(ctx, system.GenerateUUID(), time.Now().Add(5*time.Minute))
	require.NoError(t, err)
	require.True(t, ok)

	var count int64
	require.NoError(t, db.gdb.Model(&types.IdentityAttestationRedemption{}).Where("jti = ?", expired).Count(&count).Error)
	require.Zero(t, count, "expired redemptions are dropped")
}
