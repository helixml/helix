package store

import (
	"context"
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ConnectPortalPersistence keeps Connect's transactional state changes in the
// store layer. The constructor also lets storage tests use an isolated DB.
type ConnectPortalPersistence struct{ db *gorm.DB }

func NewConnectPortalPersistence(db *gorm.DB) *ConnectPortalPersistence {
	return &ConnectPortalPersistence{db: db}
}

func connectPortalStoreError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

func (s *ConnectPortalPersistence) CreatePortalConnectionAttempt(ctx context.Context, item *types.PortalConnectionAttempt) error {
	return s.db.WithContext(ctx).Create(item).Error
}

func (s *ConnectPortalPersistence) GetPortalConnectionAttempt(ctx context.Context, projectID, id string) (*types.PortalConnectionAttempt, error) {
	var item types.PortalConnectionAttempt
	if err := s.db.WithContext(ctx).Where("id = ? AND project_id = ?", id, projectID).First(&item).Error; err != nil {
		return nil, connectPortalStoreError(err)
	}
	return &item, nil
}

func (s *ConnectPortalPersistence) GetPortalConnectionAttemptByFlow(ctx context.Context, hash string, now time.Time) (*types.PortalConnectionAttempt, error) {
	var item types.PortalConnectionAttempt
	if err := s.db.WithContext(ctx).Where("flow_hash = ? AND flow_expires_at > ?", hash, now).First(&item).Error; err != nil {
		return nil, connectPortalStoreError(err)
	}
	return &item, nil
}

func (s *ConnectPortalPersistence) RedeemPortalConnectionInvitation(ctx context.Context, invitationHash, flowHash, csrfHash string, now, expiresAt time.Time) (bool, error) {
	result := s.db.WithContext(ctx).Model(&types.PortalConnectionAttempt{}).
		Where("invitation_hash = ? AND status = ? AND invitation_expires_at > ?", invitationHash, types.PortalConnectionPasswordPending, now).
		Updates(map[string]any{"invitation_hash": "", "flow_hash": flowHash, "csrf_hash": csrfHash, "flow_expires_at": expiresAt})
	return result.RowsAffected == 1, result.Error
}

func (s *ConnectPortalPersistence) RevokePortalConnectionAttempt(ctx context.Context, projectID, id string) error {
	return s.db.WithContext(ctx).Model(&types.PortalConnectionAttempt{}).Where("id = ? AND project_id = ?", id, projectID).Updates(map[string]any{
		"status": types.PortalConnectionRevoked, "session_encrypted": "", "flow_hash": "", "csrf_hash": "", "invitation_hash": "",
	}).Error
}

func (s *ConnectPortalPersistence) AdvancePortalConnectionAttempt(ctx context.Context, step PortalConnectionStep) (types.PortalConnectionStatus, error) {
	var status types.PortalConnectionStatus
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var item types.PortalConnectionAttempt
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", step.ID).First(&item).Error; err != nil {
			return connectPortalStoreError(err)
		}
		if item.Status != step.ExpectedStatus || !item.FlowExpiresAt.After(step.Now) {
			return ErrConflict
		}
		switch step.ExpectedStatus {
		case types.PortalConnectionPasswordPending:
			item.PasswordAttempts++
			if step.Valid {
				item.Status = types.PortalConnectionOTPPending
			} else if item.PasswordAttempts >= 3 {
				item.Status = types.PortalConnectionFailed
			}
		case types.PortalConnectionOTPPending:
			item.OTPAttempts++
			if step.Valid {
				if step.SessionEncrypted == "" {
					return errors.New("missing encrypted session")
				}
				item.SessionEncrypted = step.SessionEncrypted
				item.SessionExpiresAt = step.SessionExpiresAt
				item.Status = types.PortalConnectionConnected
			} else if item.OTPAttempts >= 5 {
				item.Status = types.PortalConnectionFailed
			}
		default:
			return ErrConflict
		}
		status = item.Status
		return tx.Save(&item).Error
	})
	return status, err
}

func (s *ConnectPortalPersistence) CreateSecretIntake(ctx context.Context, item *types.SecretIntake) error {
	return s.db.WithContext(ctx).Create(item).Error
}

func (s *ConnectPortalPersistence) GetSecretIntake(ctx context.Context, projectID, id string) (*types.SecretIntake, error) {
	var item types.SecretIntake
	if err := s.db.WithContext(ctx).Where("id = ? AND project_id = ?", id, projectID).First(&item).Error; err != nil {
		return nil, connectPortalStoreError(err)
	}
	return &item, nil
}

func (s *ConnectPortalPersistence) GetSecretIntakeByFlow(ctx context.Context, hash string, now time.Time) (*types.SecretIntake, error) {
	var item types.SecretIntake
	if err := s.db.WithContext(ctx).Where("flow_hash = ? AND flow_expires_at > ?", hash, now).First(&item).Error; err != nil {
		return nil, connectPortalStoreError(err)
	}
	return &item, nil
}

func (s *ConnectPortalPersistence) RedeemSecretIntakeInvitation(ctx context.Context, invitationHash, flowHash, csrfHash string, now, expiresAt time.Time) (bool, error) {
	result := s.db.WithContext(ctx).Model(&types.SecretIntake{}).
		Where("invitation_hash = ? AND status = ? AND invitation_expires_at > ?", invitationHash, "pending", now).
		Updates(map[string]any{"invitation_hash": "", "flow_hash": flowHash, "csrf_hash": csrfHash, "flow_expires_at": expiresAt})
	return result.RowsAffected == 1, result.Error
}

func (s *ConnectPortalPersistence) SubmitSecretIntake(ctx context.Context, projectID, id, cipher string, now, expiresAt time.Time) (bool, error) {
	result := s.db.WithContext(ctx).Model(&types.SecretIntake{}).
		Where("id = ? AND project_id = ? AND status = ? AND ((flow_hash <> ? AND flow_expires_at > ?) OR (flow_hash = ? AND invitation_expires_at > ?))", id, projectID, "pending", "", now, "", now).
		Updates(map[string]any{"status": "submitted", "values_encrypted": cipher, "values_expires_at": expiresAt, "invitation_hash": ""})
	return result.RowsAffected == 1, result.Error
}

func (s *ConnectPortalPersistence) RevokeSecretIntake(ctx context.Context, projectID, id string) error {
	return s.db.WithContext(ctx).Model(&types.SecretIntake{}).Where("id = ? AND project_id = ?", id, projectID).
		Updates(map[string]any{"status": "revoked", "values_encrypted": "", "invitation_hash": "", "flow_hash": "", "csrf_hash": ""}).Error
}

// TakeSecretIntake atomically clears the stored value before returning it to a
// trusted caller. A failed downstream use cannot replay the same secret.
func (s *ConnectPortalPersistence) TakeSecretIntake(ctx context.Context, projectID, id string, now time.Time) (string, error) {
	var cipher string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var item types.SecretIntake
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND project_id = ?", id, projectID).First(&item).Error; err != nil {
			return connectPortalStoreError(err)
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

func (s *ConnectPortalPersistence) ReapExpiredSecretIntakes(ctx context.Context, now time.Time) error {
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
