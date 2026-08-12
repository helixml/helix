package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func (s *PostgresStore) CreateCodexSubscription(ctx context.Context, sub *types.CodexSubscription) (*types.CodexSubscription, error) {
	if sub.ID == "" {
		sub.ID = system.GenerateCodexSubscriptionID()
	}
	if sub.OwnerID == "" || sub.OwnerType == "" || sub.EncryptedCredentials == "" || sub.CreatedBy == "" {
		return nil, fmt.Errorf("owner, encrypted credentials, and creator are required")
	}
	sub.Created = time.Now()
	sub.Updated = sub.Created
	if err := s.gdb.WithContext(ctx).Create(sub).Error; err != nil {
		return nil, err
	}
	return s.GetCodexSubscription(ctx, sub.ID)
}

func (s *PostgresStore) GetCodexSubscription(ctx context.Context, id string) (*types.CodexSubscription, error) {
	var sub types.CodexSubscription
	if err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&sub).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &sub, nil
}

func (s *PostgresStore) GetCodexSubscriptionForOwner(ctx context.Context, ownerID string, ownerType types.OwnerType) (*types.CodexSubscription, error) {
	var sub types.CodexSubscription
	if err := s.gdb.WithContext(ctx).Where("owner_id = ? AND owner_type = ?", ownerID, ownerType).Order("created DESC").First(&sub).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &sub, nil
}

func (s *PostgresStore) UpdateCodexSubscription(ctx context.Context, sub *types.CodexSubscription) (*types.CodexSubscription, error) {
	if sub.ID == "" {
		return nil, fmt.Errorf("id not specified")
	}
	sub.Updated = time.Now()
	if err := s.gdb.WithContext(ctx).Save(sub).Error; err != nil {
		return nil, err
	}
	return s.GetCodexSubscription(ctx, sub.ID)
}

func (s *PostgresStore) UpdateCodexSubscriptionCredentialsIfNewer(ctx context.Context, id, encryptedCredentials, accountID string, refreshedAt time.Time) (bool, error) {
	result := s.gdb.WithContext(ctx).Model(&types.CodexSubscription{}).
		Where("id = ? AND (last_refreshed_at IS NULL OR last_refreshed_at < ?)", id, refreshedAt).
		Updates(map[string]interface{}{
			"encrypted_credentials": encryptedCredentials,
			"account_id":            accountID,
			"last_refreshed_at":     refreshedAt,
			"status":                "active",
			"updated":               time.Now(),
		})
	return result.RowsAffected == 1, result.Error
}

func (s *PostgresStore) DeleteCodexSubscription(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}
	return s.gdb.WithContext(ctx).Delete(&types.CodexSubscription{ID: id}).Error
}

func (s *PostgresStore) ListCodexSubscriptions(ctx context.Context, ownerID string) ([]*types.CodexSubscription, error) {
	var subs []*types.CodexSubscription
	query := s.gdb.WithContext(ctx)
	if ownerID != "" {
		query = query.Where("owner_id = ?", ownerID)
	}
	if err := query.Order("created DESC").Find(&subs).Error; err != nil {
		return nil, err
	}
	return subs, nil
}

func (s *PostgresStore) GetEffectiveCodexSubscription(ctx context.Context, userID, orgID string) (*types.CodexSubscription, error) {
	sub, err := s.GetCodexSubscriptionForOwner(ctx, userID, types.OwnerTypeUser)
	if err == nil {
		return sub, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if orgID != "" {
		sub, err = s.GetCodexSubscriptionForOwner(ctx, orgID, types.OwnerTypeOrg)
		if err == nil {
			return sub, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}
	return nil, ErrNotFound
}
