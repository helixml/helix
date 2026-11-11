package store_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// clearJWTSigningSecret resets the stored secret so each test observes a
// first boot.
func clearJWTSigningSecret(t *testing.T, db *store.PostgresStore) {
	t.Helper()
	_, err := db.GetSystemSettings(context.Background())
	require.NoError(t, err)
	require.NoError(t, db.ClearJWTSigningSecretForTest(context.Background()))
}

func TestEnsureJWTSigningSecret_FirstWriteWins(t *testing.T) {
	ctx := context.Background()
	db := store.GetTestDB()
	clearJWTSigningSecret(t, db)

	first, err := db.EnsureJWTSigningSecret(ctx, "first-candidate")
	require.NoError(t, err)
	require.Equal(t, "first-candidate", first)

	second, err := db.EnsureJWTSigningSecret(ctx, "second-candidate")
	require.NoError(t, err)
	require.Equal(t, "first-candidate", second)
}

func TestEnsureJWTSigningSecret_ConcurrentCallersConverge(t *testing.T) {
	db := store.GetTestDB()
	t.Run("settings row exists", func(t *testing.T) {
		clearJWTSigningSecret(t, db)
		assertReplicasConverge(t, db)
	})
	t.Run("empty database", func(t *testing.T) {
		require.NoError(t, db.DeleteSystemSettingsForTest(context.Background()))
		assertReplicasConverge(t, db)
	})
}

func assertReplicasConverge(t *testing.T, db *store.PostgresStore) {
	ctx := context.Background()
	const replicas = 8
	results := make([]string, replicas)
	var wg sync.WaitGroup
	for i := range replicas {
		wg.Add(1)
		go func() {
			defer wg.Done()
			secret, err := auth.ResolveJWTSecret(ctx, db, "")
			require.NoError(t, err)
			results[i] = secret
		}()
	}
	wg.Wait()
	require.NotEmpty(t, results[0])
	for _, r := range results {
		require.Equal(t, results[0], r)
	}
}

func TestEnsureJWTSigningSecret_SurvivesSettingsUpdate(t *testing.T) {
	ctx := context.Background()
	db := store.GetTestDB()
	clearJWTSigningSecret(t, db)

	// A settings read taken before the secret existed must not erase it.
	stale, err := db.GetSystemSettings(ctx)
	require.NoError(t, err)
	require.Empty(t, stale.JWTSigningSecret)

	secret, err := db.EnsureJWTSigningSecret(ctx, "persisted-secret")
	require.NoError(t, err)

	version := "1.2.3"
	_, err = db.UpdateSystemSettings(ctx, &types.SystemSettingsRequest{OpenCodeVersion: &version})
	require.NoError(t, err)

	again, err := db.EnsureJWTSigningSecret(ctx, "other")
	require.NoError(t, err)
	require.Equal(t, secret, again)
}

// TestGeneratedJWTSecretIsSharedAcrossServerInits models two API processes
// (or one process restarting) against the same database with
// REGULAR_AUTH_JWT_SECRET unset: a session issued by one is accepted by the
// other.
func TestGeneratedJWTSecretIsSharedAcrossServerInits(t *testing.T) {
	ctx := context.Background()
	db := store.GetTestDB()
	clearJWTSigningSecret(t, db)

	cfg := &config.ServerConfig{}

	newAuthenticator := func() *auth.HelixAuthenticator {
		secret, err := auth.ResolveJWTSecret(ctx, db, "")
		require.NoError(t, err)
		a, err := auth.NewHelixAuthenticator(cfg, db, secret, nil)
		require.NoError(t, err)
		return a
	}
	first := newAuthenticator()
	second := newAuthenticator()

	id := system.GenerateUserID()
	user, err := db.CreateUser(ctx, &types.User{
		ID:       id,
		Email:    id + "@example.com",
		Username: id,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.DeleteUser(ctx, user.ID) })

	token, err := first.GenerateUserToken(ctx, user)
	require.NoError(t, err)

	validated, err := second.ValidateUserToken(ctx, token)
	require.NoError(t, err)
	require.Equal(t, user.ID, validated.ID)

	// The env-provided secret still wins over the stored one.
	envSecret, err := auth.ResolveJWTSecret(ctx, db, "operator-provided-secret")
	require.NoError(t, err)
	require.Equal(t, "operator-provided-secret", envSecret)
}
