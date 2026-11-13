package store

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// Plaintext stored before encryption at rest is encrypted under the current
// key. A value written under another key is an envelope and is left as it is,
// so it stays undecryptable rather than being double-encrypted. Running the
// migration again changes nothing.
func TestEncryptLegacyPlaintextSecrets(t *testing.T) {
	db := GetTestDB()
	ctx := context.Background()

	t.Setenv("HELIX_ENCRYPTION_KEY", strings.Repeat("a", 64))
	keyA, err := crypto.GetEncryptionKey()
	require.NoError(t, err)
	underA, err := crypto.EncryptAES256GCM([]byte("written-under-key-a"), keyA)
	require.NoError(t, err)
	t.Setenv("HELIX_ENCRYPTION_KEY", strings.Repeat("b", 64))
	keyB, err := crypto.GetEncryptionKey()
	require.NoError(t, err)
	underB, err := crypto.EncryptAES256GCM([]byte("written-under-key-b"), keyB)
	require.NoError(t, err)

	owner := system.GenerateUserID()
	create := func(name, value string) string {
		s, err := db.CreateSecret(ctx, &types.Secret{Owner: owner, OwnerType: types.OwnerTypeUser, Name: name, Value: []byte(value)})
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.DeleteSecret(ctx, s.ID) })
		return s.ID
	}
	legacy := create("LEGACY", "sk-legacy-plaintext-value")
	otherKey := create("OTHER_KEY", underA)
	current := create("CURRENT", underB)

	stored := func(id string) string {
		s, err := db.GetSecret(ctx, id)
		require.NoError(t, err)
		return string(s.Value)
	}

	require.NoError(t, db.encryptLegacyPlaintextSecrets(ctx))
	migrated := stored(legacy)
	require.NotEqual(t, "sk-legacy-plaintext-value", migrated)
	plain, err := crypto.DecryptAES256GCM(migrated, keyB)
	require.NoError(t, err)
	require.Equal(t, "sk-legacy-plaintext-value", string(plain))
	require.Equal(t, underA, stored(otherKey))
	require.Equal(t, underB, stored(current))

	require.NoError(t, db.encryptLegacyPlaintextSecrets(ctx))
	require.Equal(t, migrated, stored(legacy))
}
