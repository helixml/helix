package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func (s *PostgresStore) CreateSecret(ctx context.Context, secret *types.Secret) (*types.Secret, error) {
	if secret.ID == "" {
		secret.ID = system.GenerateSecretID()
	}

	if secret.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	if secret.Scope == "" {
		secret.Scope = types.SecretScopeDev
	}

	secret.Created = time.Now()
	secret.Updated = secret.Created

	err := s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Uniqueness is scoped per (owner, project_id, app_id, name) so the same
		// name can be reused across different projects or apps for the same owner.
		// Within that, the same name may also be reused across non-overlapping
		// environment scopes (e.g. a "dev" and a "prod" secret) — but a "both"
		// secret overlaps every environment, so it collides with any scope.
		var existingSecrets []types.Secret
		if err := tx.Where(
			"owner = ? AND name = ? AND project_id = ? AND app_id = ?",
			secret.Owner, secret.Name, secret.ProjectID, secret.AppID,
		).Find(&existingSecrets).Error; err != nil {
			return err
		}
		for i := range existingSecrets {
			if scopesOverlap(existingSecrets[i].Scope, secret.Scope) {
				scope := "this owner"
				switch {
				case secret.ProjectID != "":
					scope = "this project"
				case secret.AppID != "":
					scope = "this app"
				}
				return fmt.Errorf("%w: a secret with the name '%s' already exists for %s in the %s environment", ErrConflict, secret.Name, scope, secret.Scope)
			}
		}

		// If no overlapping secret found, create the new one
		return tx.Create(secret).Error
	})
	if err != nil {
		return nil, err
	}
	return s.GetSecret(ctx, secret.ID)
}

func (s *PostgresStore) UpdateSecret(ctx context.Context, secret *types.Secret) (*types.Secret, error) {
	if secret.ID == "" {
		return nil, fmt.Errorf("id not specified")
	}

	if secret.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	secret.Updated = time.Now()

	err := s.gdb.WithContext(ctx).Save(secret).Error
	if err != nil {
		return nil, err
	}
	return s.GetSecret(ctx, secret.ID)
}

func (s *PostgresStore) GetSecret(ctx context.Context, id string) (*types.Secret, error) {
	if id == "" {
		return nil, fmt.Errorf("id not specified")
	}

	var secret types.Secret
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&secret).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &secret, nil
}

func (s *PostgresStore) ListSecrets(ctx context.Context, q *ListSecretsQuery) ([]*types.Secret, error) {
	if q.Owner == "" && q.ProjectID == "" {
		return nil, fmt.Errorf("owner or project_id must be specified")
	}

	var secrets []*types.Secret
	query := s.gdb.WithContext(ctx)

	if q.Owner != "" {
		query = query.Where("owner = ?", q.Owner)
	}
	if q.OwnerType != "" {
		query = query.Where("owner_type = ?", q.OwnerType)
	}
	if q.ProjectID != "" {
		query = query.Where("project_id = ?", q.ProjectID)
	}
	if q.UserLevelOnly {
		query = query.Where("project_id = ? AND app_id = ?", "", "")
	}

	err := query.Find(&secrets).Error
	if err != nil {
		return nil, err
	}
	return secrets, nil
}

// ListProjectSecrets returns all secrets for a project (by any owner who has access)
func (s *PostgresStore) ListProjectSecrets(ctx context.Context, projectID string) ([]*types.Secret, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project_id not specified")
	}

	var secrets []*types.Secret
	err := s.gdb.WithContext(ctx).Where("project_id = ?", projectID).Find(&secrets).Error
	if err != nil {
		return nil, err
	}
	return secrets, nil
}

func (s *PostgresStore) DeleteSecret(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}

	err := s.gdb.WithContext(ctx).Delete(&types.Secret{
		ID: id,
	}).Error
	if err != nil {
		return err
	}
	return nil
}

// scopesOverlap reports whether two secret scopes share at least one
// environment, in which case two secrets with the same name would collide.
// An empty scope is treated as the default ("dev").
func scopesOverlap(a, b types.SecretScope) bool {
	if a == "" {
		a = types.SecretScopeDev
	}
	if b == "" {
		b = types.SecretScopeDev
	}
	return a == types.SecretScopeBoth || b == types.SecretScopeBoth || a == b
}

// encryptionAtRestIntroduced is when secret encryption at rest was written
// (59aa0de79e). No Helix build could encrypt a secret before it, so a row last
// written earlier holds plaintext whatever it looks like.
var encryptionAtRestIntroduced = time.Date(2025, 12, 22, 19, 12, 48, 0, time.UTC)

const (
	encryptLegacySecretsMarker    = "encrypt_legacy_plaintext_secrets"
	encryptLegacySecretsBatchSize = 500
)

// encryptLegacyPlaintextSecrets encrypts secret values stored before secrets
// were encrypted at rest. Nothing backfilled those rows. It runs once: a
// migration_scripts row records completion. A row is encrypted only when it
// provably holds plaintext:
//
//   - it does not decrypt under the current key, and
//   - it is not base64 of at least nonce plus tag (no key could have written
//     it), or it was last written before encryptionAtRestIntroduced.
//
// A row that fails to decrypt and was written later may be ciphertext under
// an earlier key (London has such rows), so it is left alone and reported as
// undecryptable where it is read. Encrypting it would wrap the old ciphertext,
// which would then decrypt to that ciphertext and be used as the value.
//
// If rows exist and none decrypts under the current key, the key is probably
// wrong, and encrypting under it would lock the plaintext away from the right
// key. The migration then changes nothing, logs an error, and runs again on
// the next start. A database whose every secret predates encryption is
// indistinguishable from that and is handled the same way: its plaintext
// rows stay unreadable until one secret is written under the current key,
// after which the next start migrates the rest.
func (s *PostgresStore) encryptLegacyPlaintextSecrets(ctx context.Context, batchSize int) error {
	if !s.gdb.Migrator().HasTable(&types.Secret{}) {
		return nil
	}
	var marker MigrationScript
	err := s.gdb.WithContext(ctx).Where("name = ?", encryptLegacySecretsMarker).Limit(1).Find(&marker).Error
	if err != nil {
		return fmt.Errorf("read secrets encryption marker: %w", err)
	}
	if marker.HasRun {
		return nil
	}
	key, err := crypto.GetEncryptionKey()
	if err != nil {
		return fmt.Errorf("get encryption key: %w", err)
	}

	var total, decryptable int
	var plaintextIDs []string
	err = s.eachSecretBatch(ctx, batchSize, func(rows []types.Secret) error {
		for _, row := range rows {
			total++
			switch {
			case secretDecrypts(row, key):
				decryptable++
			case secretIsLegacyPlaintext(row):
				plaintextIDs = append(plaintextIDs, row.ID)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if total > 0 && decryptable == 0 {
		log.Error().Int("secrets", total).
			Msg("No stored secret decrypts with the current HELIX_ENCRYPTION_KEY; not encrypting legacy plaintext secrets, since the key may be wrong. Will retry on next start.")
		return nil
	}

	if len(plaintextIDs) > 0 {
		log.Info().Int("count", len(plaintextIDs)).Msg("Encrypting secrets stored before encryption at rest")
	}
	for start := 0; start < len(plaintextIDs); start += batchSize {
		ids := plaintextIDs[start:min(start+batchSize, len(plaintextIDs))]
		if err := s.encryptLegacySecretBatch(ctx, ids, key); err != nil {
			return err
		}
	}

	if err := s.gdb.WithContext(ctx).Save(&MigrationScript{Name: encryptLegacySecretsMarker, HasRun: true}).Error; err != nil {
		return fmt.Errorf("record secrets encryption marker: %w", err)
	}
	return nil
}

// eachSecretBatch calls fn with the id, value and updated time of every
// secret with a value, batchSize rows at a time in id order.
func (s *PostgresStore) eachSecretBatch(ctx context.Context, batchSize int, fn func([]types.Secret) error) error {
	for after := ""; ; {
		var rows []types.Secret
		err := s.gdb.WithContext(ctx).Select("id", "value", "updated").
			Where("id > ? AND value IS NOT NULL AND length(value) > 0", after).
			Order("id").Limit(batchSize).Find(&rows).Error
		if err != nil {
			return fmt.Errorf("list secrets: %w", err)
		}
		if len(rows) == 0 {
			return nil
		}
		if err := fn(rows); err != nil {
			return err
		}
		after = rows[len(rows)-1].ID
	}
}

// encryptLegacySecretBatch re-reads ids and encrypts those still holding
// legacy plaintext. Matching on the old value skips a row rewritten since.
func (s *PostgresStore) encryptLegacySecretBatch(ctx context.Context, ids []string, key []byte) error {
	var rows []types.Secret
	if err := s.gdb.WithContext(ctx).Select("id", "value", "updated").Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return fmt.Errorf("load secrets to encrypt: %w", err)
	}
	for _, row := range rows {
		if len(row.Value) == 0 || secretDecrypts(row, key) || !secretIsLegacyPlaintext(row) {
			continue
		}
		encrypted, err := crypto.EncryptAES256GCM(row.Value, key)
		if err != nil {
			return fmt.Errorf("encrypt secret %s: %w", row.ID, err)
		}
		if err := s.gdb.WithContext(ctx).Model(&types.Secret{}).
			Where("id = ? AND value = ?", row.ID, row.Value).
			Update("value", []byte(encrypted)).Error; err != nil {
			return fmt.Errorf("encrypt secret %s: %w", row.ID, err)
		}
	}
	return nil
}

func secretDecrypts(row types.Secret, key []byte) bool {
	_, err := crypto.DecryptAES256GCM(string(row.Value), key)
	return err == nil
}

// secretIsLegacyPlaintext reports whether a row that does not decrypt under
// the current key provably holds plaintext rather than another key's
// ciphertext.
func secretIsLegacyPlaintext(row types.Secret) bool {
	return !crypto.IsAES256GCMEnvelope(string(row.Value)) || row.Updated.Before(encryptionAtRestIntroduced)
}
