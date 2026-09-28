package store

import (
	"context"
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SecretIntakePersistence keeps transactional secret intake changes in the store layer.
// The constructor also lets storage tests use an isolated DB.
type SecretIntakePersistence struct{ db *gorm.DB }

func NewSecretIntakePersistence(db *gorm.DB) *SecretIntakePersistence {
	return &SecretIntakePersistence{db: db}
}

func secretIntakeStoreError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

func (s *SecretIntakePersistence) CreateSecretIntake(ctx context.Context, item *types.SecretIntake) error {
	return s.db.WithContext(ctx).Create(item).Error
}

func (s *SecretIntakePersistence) GetSecretIntake(ctx context.Context, projectID, id string) (*types.SecretIntake, error) {
	var item types.SecretIntake
	if err := s.db.WithContext(ctx).Where("id = ? AND project_id = ?", id, projectID).First(&item).Error; err != nil {
		return nil, secretIntakeStoreError(err)
	}
	return &item, nil
}

func (s *SecretIntakePersistence) GetSecretIntakeByFlow(ctx context.Context, hash string, now time.Time) (*types.SecretIntake, error) {
	var item types.SecretIntake
	if err := s.db.WithContext(ctx).Where("flow_hash = ? AND flow_expires_at > ?", hash, now).First(&item).Error; err != nil {
		return nil, secretIntakeStoreError(err)
	}
	return &item, nil
}

func (s *SecretIntakePersistence) RedeemSecretIntakeInvitation(ctx context.Context, intakeID, invitationHash, flowHash, csrfHash string, now, expiresAt time.Time) (bool, error) {
	result := s.db.WithContext(ctx).Model(&types.SecretIntake{}).
		Where("id = ? AND invitation_hash = ? AND status = ? AND invitation_expires_at > ?", intakeID, invitationHash, "pending", now).
		Updates(map[string]any{"invitation_hash": "", "flow_hash": flowHash, "csrf_hash": csrfHash, "flow_expires_at": expiresAt})
	return result.RowsAffected == 1, result.Error
}

func (s *SecretIntakePersistence) SubmitSecretIntake(ctx context.Context, projectID, id, cipher string, now, expiresAt time.Time) (bool, error) {
	result := s.db.WithContext(ctx).Model(&types.SecretIntake{}).
		Where("id = ? AND project_id = ? AND status = ? AND ((flow_hash <> ? AND flow_expires_at > ?) OR (flow_hash = ? AND invitation_expires_at > ?))", id, projectID, "pending", "", now, "", now).
		Updates(map[string]any{"status": "submitted", "values_encrypted": cipher, "values_expires_at": expiresAt, "invitation_hash": ""})
	return result.RowsAffected == 1, result.Error
}

func (s *SecretIntakePersistence) RevokeSecretIntake(ctx context.Context, projectID, id string) error {
	return s.db.WithContext(ctx).Model(&types.SecretIntake{}).Where("id = ? AND project_id = ?", id, projectID).
		Updates(map[string]any{"status": "revoked", "values_encrypted": "", "invitation_hash": "", "flow_hash": "", "csrf_hash": ""}).Error
}

// TakeSecretIntake atomically clears the stored value before returning it to a
// trusted caller. A failed downstream use cannot replay the same secret.
func (s *SecretIntakePersistence) TakeSecretIntake(ctx context.Context, projectID, id string, now time.Time) (string, error) {
	var cipher string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var item types.SecretIntake
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND project_id = ?", id, projectID).First(&item).Error; err != nil {
			return secretIntakeStoreError(err)
		}
		if item.Status != "submitted" || !item.ValuesExpiresAt.After(now) || item.ValuesEncrypted == "" {
			return ErrConflict
		}
		cipher = item.ValuesEncrypted
		result := tx.Model(&types.SecretIntake{}).Where("id = ? AND status = ?", id, "submitted").
			Updates(map[string]any{"status": "consumed", "values_encrypted": "", "flow_hash": "", "csrf_hash": ""})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrConflict
		}
		return nil
	})
	return cipher, err
}

func (s *SecretIntakePersistence) ReapExpiredSecretIntakes(ctx context.Context, now time.Time) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&types.SecretIntake{}).Where("status = ? AND values_expires_at <= ?", "submitted", now).
			Updates(map[string]any{"status": "expired", "values_encrypted": "", "flow_hash": "", "csrf_hash": ""}).Error; err != nil {
			return err
		}
		if err := tx.Model(&types.SecretIntake{}).Where("status = ? AND flow_expires_at <= ? AND flow_hash <> ?", "pending", now, "").
			Updates(map[string]any{"status": "expired", "invitation_hash": "", "flow_hash": "", "csrf_hash": ""}).Error; err != nil {
			return err
		}
		return tx.Model(&types.SecretIntake{}).Where("status = ? AND invitation_expires_at <= ? AND flow_hash = ?", "pending", now, "").
			Updates(map[string]any{"status": "expired", "invitation_hash": ""}).Error
	})
}
