package store

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// newSecretsMigrationStore returns a store over a fresh schema holding only
// the secrets and migration_scripts tables, so the migration sees no rows
// written by other tests.
func newSecretsMigrationStore(t *testing.T) *PostgresStore {
	t.Helper()
	base := GetTestDB()
	schemaName := "secmig_" + strings.ToLower(system.GenerateUUID()[:8])
	require.NoError(t, base.gdb.Exec("CREATE SCHEMA "+schemaName).Error)
	t.Cleanup(func() { base.gdb.Exec("DROP SCHEMA " + schemaName + " CASCADE") })

	cfg := base.cfg
	cfg.Schema = schemaName
	gdb, err := connect(context.Background(), connectConfig{
		host: cfg.Host, port: cfg.Port, schemaName: schemaName, database: cfg.Database,
		username: cfg.Username, password: cfg.Password, ssl: cfg.SSL,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := gdb.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, gdb.AutoMigrate(&types.Secret{}, &MigrationScript{}))
	return &PostgresStore{cfg: cfg, gdb: gdb}
}

func useKey(t *testing.T, hexKey string) []byte {
	t.Helper()
	t.Setenv("HELIX_ENCRYPTION_KEY", hexKey)
	key, err := crypto.GetEncryptionKey()
	require.NoError(t, err)
	return key
}

func encryptWith(t *testing.T, key []byte, plaintext string) string {
	t.Helper()
	enc, err := crypto.EncryptAES256GCM([]byte(plaintext), key)
	require.NoError(t, err)
	return enc
}

// insertSecret writes a row as an earlier Helix version would have, with the
// given value and last-updated time.
func insertSecret(t *testing.T, db *PostgresStore, id, value string, updated time.Time) {
	t.Helper()
	require.NoError(t, db.gdb.Create(&types.Secret{
		ID: id, Owner: "usr_owner", OwnerType: types.OwnerTypeUser, Name: id,
		Value: []byte(value), Created: updated, Updated: updated,
	}).Error)
}

func storedValue(t *testing.T, db *PostgresStore, id string) string {
	t.Helper()
	s, err := db.GetSecret(context.Background(), id)
	require.NoError(t, err)
	return string(s.Value)
}

func markerSet(t *testing.T, db *PostgresStore) bool {
	t.Helper()
	var m MigrationScript
	require.NoError(t, db.gdb.Where("name = ?", encryptLegacySecretsMarker).Limit(1).Find(&m).Error)
	return m.HasRun
}

var (
	beforeEncryption = time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC)
	afterEncryption  = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	keyRotated       = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	keyAHex          = strings.Repeat("a", 64)
	keyBHex          = strings.Repeat("b", 64)
)

// base64Token looks exactly like an AES-GCM envelope (std base64, 33 bytes
// decoded) but was stored as plaintext.
var base64Token = base64.StdEncoding.EncodeToString([]byte("legacy-api-token-0123456789abcdefg"))

func TestEncryptLegacyPlaintextSecrets(t *testing.T) {
	db := newSecretsMigrationStore(t)
	ctx := context.Background()
	keyA := useKey(t, keyAHex)
	underA := encryptWith(t, keyA, "written-under-key-a")
	keyB := useKey(t, keyBHex)
	underB := encryptWith(t, keyB, "written-under-key-b")
	require.True(t, crypto.IsAES256GCMEnvelope(base64Token))

	insertSecret(t, db, "sec_shaped_plain_old", base64Token, beforeEncryption)
	insertSecret(t, db, "sec_plain_old", "sk-legacy plaintext", beforeEncryption)
	insertSecret(t, db, "sec_plain_unshaped_new", "not base64 at all!", afterEncryption)
	// Another key's ciphertext written after encryption existed but before the
	// current key's earliest row: what a rotated key leaves behind.
	insertSecret(t, db, "sec_old_key", underA, afterEncryption)
	insertSecret(t, db, "sec_current", underB, keyRotated)

	require.NoError(t, db.encryptLegacyPlaintextSecrets(ctx, encryptLegacySecretsBatchSize))

	for id, want := range map[string]string{
		"sec_shaped_plain_old":   base64Token,
		"sec_plain_old":          "sk-legacy plaintext",
		"sec_plain_unshaped_new": "not base64 at all!",
	} {
		plain, err := crypto.DecryptAES256GCM(storedValue(t, db, id), keyB)
		require.NoError(t, err, id)
		require.Equal(t, want, string(plain), id)
	}
	require.Equal(t, underA, storedValue(t, db, "sec_old_key"), "another key's ciphertext must not be wrapped")
	require.Equal(t, underB, storedValue(t, db, "sec_current"))
	require.True(t, markerSet(t, db))

	// Once marked, later starts do nothing, even for a row they would
	// otherwise encrypt.
	insertSecret(t, db, "sec_plain_late", "late plaintext", beforeEncryption)
	migrated := storedValue(t, db, "sec_plain_old")
	require.NoError(t, db.encryptLegacyPlaintextSecrets(ctx, encryptLegacySecretsBatchSize))
	require.Equal(t, "late plaintext", storedValue(t, db, "sec_plain_late"))
	require.Equal(t, migrated, storedValue(t, db, "sec_plain_old"))
}

// When no row decrypts under the configured key, the key may be wrong:
// nothing is changed, no marker is written, and the next start tries again.
func TestEncryptLegacyPlaintextSecrets_NoRowDecryptsAborts(t *testing.T) {
	db := newSecretsMigrationStore(t)
	ctx := context.Background()
	underA := encryptWith(t, useKey(t, keyAHex), "written-under-key-a")
	useKey(t, keyBHex)

	insertSecret(t, db, "sec_old_key", underA, afterEncryption)
	insertSecret(t, db, "sec_plain_old", "sk-legacy plaintext", beforeEncryption)

	require.NoError(t, db.encryptLegacyPlaintextSecrets(ctx, encryptLegacySecretsBatchSize))
	require.Equal(t, underA, storedValue(t, db, "sec_old_key"))
	require.Equal(t, "sk-legacy plaintext", storedValue(t, db, "sec_plain_old"))
	require.False(t, markerSet(t, db))

	// After a secret is written under the current key, the next start runs.
	insertSecret(t, db, "sec_current", encryptWith(t, useKey(t, keyBHex), "v"), keyRotated)
	require.NoError(t, db.encryptLegacyPlaintextSecrets(ctx, encryptLegacySecretsBatchSize))
	require.NotEqual(t, "sk-legacy plaintext", storedValue(t, db, "sec_plain_old"))
	require.Equal(t, underA, storedValue(t, db, "sec_old_key"))
	require.True(t, markerSet(t, db))
}

// Rows are read and encrypted across batch boundaries.
func TestEncryptLegacyPlaintextSecrets_Batches(t *testing.T) {
	db := newSecretsMigrationStore(t)
	key := useKey(t, keyBHex)
	insertSecret(t, db, "sec_0", encryptWith(t, key, "current"), keyRotated)
	plain := []string{"sec_1", "sec_2", "sec_3", "sec_4", "sec_5"}
	for _, id := range plain {
		insertSecret(t, db, id, "plain-"+id, beforeEncryption)
	}

	require.NoError(t, db.encryptLegacyPlaintextSecrets(context.Background(), 2))
	for _, id := range plain {
		got, err := crypto.DecryptAES256GCM(storedValue(t, db, id), key)
		require.NoError(t, err, id)
		require.Equal(t, "plain-"+id, string(got))
	}
}
